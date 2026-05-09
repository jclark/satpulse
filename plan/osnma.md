# OSNMA support (#105)

Galileo Open Service Navigation Message Authentication.  This
plan covers the satpulse-side work needed to support OSNMA on
GNSS receivers that implement it (initially u-blox; Quectel
LC29H and Septentrio have OSNMA support and may be added later).

OSNMA support consists of seven pieces, addressed in their
respective sections below in the order they will be
implemented:

1. **Reporting** OSNMA status through satpulse's existing
   message-flow plumbing.
2. **Supplying initial trusted time** to the receiver, which
   OSNMA requires for the TESLA-key timing check.
3. **Supplying ongoing trusted time** to refresh the
   receiver's propagated trusted-time accuracy as it decays.
4. **Provisioning** the receiver with the Merkle tree root
   and public key(s) (cleanup of existing flow).
5. **Enabling** OSNMA on the receiver (cleanup of existing
   flow).
6. **Trusted time from NTP** -- a stronger trusted-time source
   that augments pieces 2 and 3.
7. **HTTPS cross-check** -- an authenticated integrity check
   layered on top of whatever source pieces 2 and 3 use.

Reporting goes first because it's the new-functionality
starting point: provisioning and enabling already work via the
existing `--osnma` flag, so pieces 4 and 5 are cleanup that can
wait.  Piece 2 follows reporting -- it doesn't depend on piece
1, but it's the next new-functionality step.  Piece 3 depends
on pieces 1 and 2.  Pieces 4 and 5 are independent of
everything else and can land at any point.  Pieces 6 and 7 are
extensions that strengthen the trusted-time path; pieces 1-5
cover a working OSNMA system on their own.  Pieces 6 and 7 are
independent of each other: piece 7 layers on top of whatever
source piece 2 uses, including the kernel-NTP path that exists
without piece 6.  The SNTP primitive for piece 6a is already
implemented in `time/lib/sntp`; what remains for piece 6 is
its integration into pieces 2 and 3.

Pieces 1 and 2 alone give a working OSNMA system on u-blox --
the receiver propagates trusted time on its TCXO with decaying
accuracy, so initial supply works "for a while."  Ongoing
supply (piece 3) is the production-quality follow-up.

## Reporting OSNMA

`NavEpochMsg` gains an `opt.Val[bool]` field carrying the
NMA-verified status of the fix, populated by the u-blox
decoder from the `nmaFixStatus` bit in `UBX-NAV-PVT`.  This is
the prerequisite for the ongoing-supply path (piece 3).

The new field is surfaced in two existing sinks:

- Web UI: expose the boolean alongside the other fields from
  `NavEpochMsg` already shown.
- Prometheus: expose as a metric alongside the other
  `NavEpochMsg`-derived metrics.

Both are trivial pass-through additions.

Future reporting extensions (out of scope of this plan, but
the data flow accommodates them):

- `UBX-SEC-OSNMA` is already decoded at the binary level in
  `gps/lib/ubxbin/sec.go`; surfacing its detail through the
  `gpsprot.MsgHandler` / Observer plumbing remains.
- Log changes in authentication status (with `WARN` level on
  loss of authentication).

## Supplying initial trusted time

OSNMA requires the receiver to know the current time accurately
to apply the protocol.  Per the OSNMA Receiver Guidelines
(referenced in the u-blox F9 HPG 1.51 docs), accuracy must be
better than 15 s to use MAC ADKD type 0, better than 165 s to
use MAC ADKD type 12, and beyond 165 s OSNMA cannot be applied
at all.

satpulse sets this trusted time once at startup; the
receiver maintains it on its TCXO from there.  Initial supply
works "for a while" because TCXO drift takes time to push the
accuracy past 15 s and then 165 s; once it does, OSNMA falls
back from ADKD type 0 to type 12 and eventually stops.
Occasional refresh (piece 3) is sufficient because we just need
to keep the receiver-side accuracy under those thresholds.

This section establishes the trusted-time setting path; piece 3
builds on it for u-blox ongoing refresh.

### Architecture

Trusted time is not configuration in the user-facing
`gps.config` sense: it is not a persistent receiver setting, and
a user with persistent receiver config (`gps.config = false`)
still needs trusted time set on every cold boot for OSNMA to
work.  SatPulse models this as setting the receiver's internal
trusted-time clock, not as assistance.  u-blox transports the
operation through
`UBX-MGA-INI-TIME_UTC` / `UBX-MGA-INI-TIME_GNSS` with the
`trustedSource` bit set, but that is a u-blox implementation
detail.  Quectel and Septentrio use dedicated trusted-time
commands.

The initial set is part of the existing `gpscfg.Configure`
operation as an implementation detail.  This reuses receiver
identification and the existing request machinery, while keeping
normal receiver configuration changes gated by `gps.config` and
trusted-time setting gated by `gps.setTrustedTime`.  Future TTFF
assistance (time/position/ephemeris/almanac hints, or a stream of
vendor assistance data) remains a separate feature and is not
modeled here.

### Probing

`gpscfg.Configure` already has `ConfigOptions.ForceProbe`,
which forces the probe to run when the configurator has nothing
to do.  Trusted-time setting reuses that mechanism: when
`gps.setTrustedTime = true` but `gps.config = false`,
`ForceProbe` is set so the probe still identifies the receiver
and selects the correct trusted-time command.

### Time estimate types

The startup/configuration path uses a UTC-only estimate:

```go
type UTCTimeEstimate struct {
    // UTC time, monotonic anchor, accuracy; exact field names TBD.
}
```

The estimate is anchored in monotonic time (`TimeOfEstimate`) so
the receiver backend can extrapolate to "now" at the moment it
builds the packet.  A zero `UTCTimeEstimate` means no initial
trusted-time estimate is present, using the same `IsZero`
convention as the existing time-assist path.  The existing
`Trusted bool` field is removed -- trust is not a property of
the estimate itself; it is expressed by placing the estimate in
`ConfigOptions.TrustedTime`.

The ongoing refresh path uses a richer estimate:

```go
type TimeEstimate struct {
    UTCTimeEstimate
    TAI ptime.Time // optional; zero means unavailable
}
```

The embedded UTC estimate is always available when a
`TimeEstimate` is usable.  The TAI field is populated when the
source naturally has GNSS/TAI time, such as receiver-fed-back
authenticated time while SatPulse is running with a PHC.  In
other modes, including a no-PHC serial-time setup, ongoing
refresh may be UTC-only.  The packet builder chooses the
receiver message form from the data available: for u-blox,
`UBX-MGA-INI-TIME_GNSS` when TAI is present and
`UBX-MGA-INI-TIME_UTC` otherwise.

### Trusted-time packet builder

Receivers that need ongoing trusted-time refresh expose a packet
builder in `gpscfg.Result`:

```go
type TrustedTimePacketBuilder interface {
    TrustedTimePacket(est *TimeEstimate, now time.Time) ([]byte, error)
}
```

This interface only constructs receiver bytes.  The caller owns
port locking and writing.  It is not required for receivers that
only need a one-shot trusted-time set: Quectel and Septentrio can
generate their startup command directly inside their configurator
and return nil for the ongoing builder.  u-blox uses the same
builder internally from its configurator for the initial packet
and returns it in `gpscfg.Result` because u-blox also needs
ongoing refresh.

### Building the initial `UTCTimeEstimate`

The initial `UTCTimeEstimate` is constructed from the system
clock.  Both the daemon (at startup) and `satpulsetool gps`
use the same construction:

1. Read kernel NTP state via `gps/lib/ntptime.Get`.
2. If `Synchronized`: use kernel time, `MaxError` for accuracy,
   and the monotonic timestamp of the read.
3. If not `Synchronized`: invoke the configured external
   program (TBD configuration knob).  The program's contract
   is to assert that the current system clock can be trusted
   up to a max error in seconds, which it prints as a single
   decimal number on stdout.  Non-zero exit + stderr message
   on failure.  On failure, skip trusted-time supply with a
   warning.

The two branches differ only in how accuracy was obtained.  The
trustedness of the receiver write is expressed by putting the
estimate in the trusted-time setting field of `ConfigTarget`, not
by a flag on the estimate itself.

### Daemon-side initial supply

The daemon does the initial supply at startup, synchronously
(no goroutine yet).  `gps.setTrustedTime` controls whether this
operation is requested:

- unset: defaults to `gps.config && gps.nma == "osnma"`;
- `true`: set trusted time regardless of `gps.config`;
- `false`: do not set trusted time.

When trusted-time setting is enabled, the daemon:

1. Builds the initial `UTCTimeEstimate` (above).
2. Stores it in `ConfigTarget.Opts.TrustedTime`.
3. Sets `ForceProbe` if normal receiver configuration is not
   otherwise needed.
4. Calls `gpscfg.Configure`, whose selected configurator builds
   and writes the receiver-specific trusted-time packet as part
   of the normal request flow.

This happens during startup before stream/proxy goroutines
begin writing to the port, so no lock contention.  If the
selected receiver requires ongoing refresh, `gpscfg.Result`
also carries a `TrustedTimePacketBuilder` for piece 3.

### `satpulsetool gps` one-shot

`satpulsetool gps` uses the same `UTCTimeEstimate` construction and
the same configurator path.  If the user requests trusted-time
setting via the new flag(s) (naming TBD; replaces
`--sys-time-trusted`), the tool stores the estimate in
`ConfigTarget.Opts.TrustedTime` and lets the selected
configurator emit the receiver-specific one-shot packet.

### TOML configuration

`[gps]` is broadened from "receiver configuration only" to
"receiver interaction/control".  Normal configuration changes
remain gated by `gps.config`, but trusted-time setting is a
separate receiver operation:

```toml
[gps]
config = true
nma = "osnma"
setTrustedTime = true

[trustedTime]
# how trusted time is obtained and validated
```

`gps.nma` does not default -- OSNMA requires explicit setup
(Merkle provisioning), so it stays opt-in.

Piece 2 introduces the `gps.nma` TOML field only far enough to
parse it and compute the `gps.setTrustedTime` default.  Piece 5
wires the same field to receiver OSNMA enable configuration.

`gps.setTrustedTime` is optional and defaults as follows:

- unset: `true` when `gps.config = true` and `gps.nma =
  "osnma"`, otherwise `false`;
- `true`: set the receiver's trusted-time clock regardless of
  `gps.config`;
- `false`: never set trusted time, even when OSNMA is enabled.

The `[trustedTime]` table says how SatPulse obtains and validates
the time value.  It is distinct from future TTFF assistance,
which may later use an `[assist]` / `[assist.pull]` shape for
time/position hints or streamed ephemeris/almanac/vendor data.

The remaining sub-configuration is to be designed:

- the path to the external uncertainty program

## Supplying ongoing trusted time

u-blox propagates trusted time on its TCXO with decaying
accuracy.  Once OSNMA is locked, the receiver's own
authenticated time becomes the trusted source feeding itself
back, keeping the propagated accuracy fresh.  Some receivers
(Septentrio, apparently Quectel) hold accuracy without ongoing
input; for them, this piece is a no-op.

### Prerequisite

The reporting work (piece 1, NMA-verified flag on
`NavEpochMsg`) must land first.  The `TrustedTimePacketBuilder`,
`UTCTimeEstimate`, and `TimeEstimate` infrastructure from piece 2
are the substrate.

### Daemon goroutine

A new package (location TBD; sibling of `gps/app/stream`)
provides the trusted-time refresh goroutine.  It is constructed
with:

- the `TrustedTimePacketBuilder` from `gpscfg.Result`
- the `PacketFormat` for the identified protocol
- the connection / `OutPortLock`

```go
func Run(ctx context.Context, b TrustedTimePacketBuilder,
    pf PacketFormat, port OutPortLock,
    ch <-chan TimeEstimate)
```

For each `TimeEstimate` received on the channel:

1. Acquire the port write-lock (shared with `stream.pull` and
   the proxy feature).
2. Compute `now := time.Now()`.
3. Call `TrustedTimePacket(est, now)` on the
   `TrustedTimePacketBuilder`.
4. Write the resulting bytes to the port.
5. Release the lock.

Startup follows the phase-1 / phase-2 pattern from
`plan/stream-pull-daemon.md`: the goroutine is prepared before
fallible startup steps and started immediately before the
dispatcher `Run`, so the dispatcher is available to drain any
events the goroutine may produce and shutdown does not deadlock.

### Trigger logic

This is an operational policy, not a cryptographic guarantee:
once NMA-verified is observed on `NavEpochMsg` and
`phcsync.Controller` is in tracking mode, the daemon treats
the receiver's GNSS time as suitable for trusted-time refresh.
It builds a `TimeEstimate` from that time, with TAI populated
when the current SatPulse mode has a TAI/GNSS time source, and
sends it to the refresh goroutine's channel.

Refresh is rare, not per epoch.  The 165 s OSNMA accuracy
ceiling means the receiver's propagated trusted time has plenty
of headroom; refresh just needs to happen often enough to keep
the receiver-side accuracy under that ceiling (and ideally
under 15 s for ADKD type 0).  The exact cadence is to be worked
out -- specifically *not* "every NMA-verified epoch."

For u-blox, the ongoing path uses the same
`TrustedTimePacketBuilder` implementation as the initial
configurator step.  It emits `UBX-MGA-INI-TIME_GNSS` when TAI is
available, avoiding the leap-second ambiguity that UTC carries,
and falls back to `UBX-MGA-INI-TIME_UTC` for UTC-only estimates.

## Provisioning Merkle and public keys

The Merkle tree root and public key(s) change rarely (the root
is intended to last around ten years; pubkeys rotate via the
key-rollover mechanism in the broadcast).  They are persistent
receiver state.  The user provisions them once via
`satpulsetool gps --msg-file`; the daemon does not touch them.

Today, the hidden `--osnma` flag in `satpulsetool gps`
provisions a compiled-in Merkle root via
`ConfigOptions.OSNMA.MerkleTreeRoot`.  Provisioning moves out
of code into the message file:

- Merkle root and public key provisioning live in
  `configs/gpsmsg/osnma.toml` (already covers them, including
  the `osnma-pubkey-2` tag).  The user runs `satpulsetool gps
  --msg-file osnma.toml --tag <tag>` to apply.
- The compiled-in `OSNMAMerkleTreeRoot` constant in
  `internal/gpscmd/gpsflags.go`, the
  `ConfigOptions.OSNMA.MerkleTreeRoot` field in `gpsprot`, and
  the configurator's `mgaOSNMAMerkle` call are retired.

The OSNMA *enable* part of the old `--osnma` flag does not move
out of code -- it stays as the renamed `--nma=osnma|off` flag
and the new `gps.nma` TOML knob (see Enabling).

## Enabling OSNMA

A new TOML knob (proposed: `gps.nma`, peer to `gps.config` and
`gps.setTrustedTime`) turns OSNMA processing on or off.  Initial
values:

- unset / `"none"` (default): no OSNMA.
- `"osnma"`: OSNMA processing enabled.  u-blox: sets
  `CFG-GAL-USE_OSNMA = 1`.

When `gps.config = true`, the daemon applies the corresponding
config-key setting via the configurator.  When `gps.config =
false` the daemon assumes the user provisioned the setting
persistently and only verifies/probes.

### gpsprot changes

The enable bit is carried by a `ConfigProp` in
`gps/gpsprot/configtarget.go` (extending the existing
`ConfigProps` / `PropIDs` framework that already covers
signalsEnabled, mode, antennaCableDelay, etc.).  The current
`NavMsgAuth` enum and `PropIDNavMsgAuth` are repurposed /
renamed to fit the new vocabulary -- exact naming TBD, but the
property holds an enum value matching the `gps.nma` TOML values
(`none` / `osnma` initially, extensible later).

Both call sites feed the same ConfigProp:

- The satpulsetool `--nma=osnma|off` flag (see below).
- The daemon's `gps.nma` TOML knob, applied during configurator
  setup the same way the existing properties (e.g.
  `PropIDMode`, `PropIDTimeGNSS`) are.

### satpulsetool flag

The hidden `--osnma` flag becomes `--nma=osnma|off`.  Unlike
the old flag, it only affects the `CFG-GAL-USE_OSNMA` enable
bit; it no longer provisions the Merkle root (that work has
moved to `osnma.toml` -- see Provisioning).  The flag is no
longer hidden once the broader OSNMA story is shipped.

### Other OSNMA-related flags

u-blox exposes two further independent flags:

- `CFG-NAVSPG-ONLY_AUTHDATA`: restrict the navigation solution
  to authenticated data only.
- `CFG-GAL-OSNMA_TIMESYNC`: require trusted time before
  applying OSNMA (default on).

These are handled by the `osnma.toml` message file (tags
`osnma-only-auth` / `osnma-only-auth-off` and `osnma-timesync`
/ `osnma-timesync-off`).  Promoting either to a daemon TOML
knob is a future step; the `ConfigProp` shape may need
extension at that point.

## Trusted time from NTP

A stronger trusted-time source than the system clock or the
receiver's own authenticated time: directly query a remote
NTP server (preferably NTS-authenticated) located in a
geographically distant part of the world.  The argument is
that a localized GNSS spoofer or jammer could plausibly affect
both the satpulse receiver and any nearby NTP servers (which
are often GNSS-disciplined themselves), but a distant NTS
server is a much harder target.

This piece augments pieces 2 and 3:

- **Piece 2 (initial supply):** if a remote NTP server is
  configured and reachable, use it as the source for the
  initial `UTCTimeEstimate` instead of the kernel NTP state.
- **Piece 3 (ongoing supply):** if a remote NTP server is
  configured and reachable, prefer it over the
  receiver-fed-back authenticated time as the ongoing-supply
  source.  Receiver-fed-back remains the fallback when remote
  NTP isn't configured or is unreachable.

When there's no internet connectivity (or no remote NTP
configured), the daemon falls back to the other sources:
kernel NTP for initial supply, receiver-fed-back for ongoing.

This piece can itself be staged:

a. **Single SNTP query to a single configured server.**
   Simplest viable form -- one TOML setting (server
   hostname), one round-trip query, plain SNTP.  Already a
   meaningful improvement over the kernel-NTP / receiver-fed
   sources for users who can configure a distant server.
b. **Multiple servers with an agreement policy** (e.g.
   median offset across responses; reject outliers).  Defends
   against a single compromised or unreachable server.
   Candidates may be supplied as explicit server hostnames, as
   one or more NTP *pools*, or any mix of the two -- the exact
   configuration shape is TBD.  A pool name (e.g.
   `pool.ntp.org`) resolves to a rotating set of A/AAAA records
   that the client re-resolves periodically, picking a fresh
   subset of IPs per resolution; an explicit hostname pins
   identity (you talk to the same host on every query).  Both
   feed the same agreement policy, but pool entries need
   re-resolution logic and a way to bound the picked subset
   size.  In Go this is mechanically straightforward: one
   goroutine per candidate, results into a buffered channel,
   exit when a configured quorum of successes arrives or the
   timeout expires.  Pool members fan into the same channel
   after their initial DNS resolution.
c. **NTS** (authenticated NTP) for the queries.  Defends
   against on-path tampering with the NTP responses
   themselves.  Note: an NTS implementation likely does not
   make sense in this codebase.  The HTTPS cross-check
   (section "HTTPS cross-check") covers the same threat
   model with stdlib-only dependencies and far less protocol
   complexity, while pieces 6a/6b already provide the
   precision NTS would.  The `beevik/nts` package is also
   not cleanly separable from `beevik/ntp` (its session
   methods are unexported), so adopting it would mean either
   accepting the full pair or maintaining a fork.  Stage 6c
   is retained here as a possible future option but should
   not be assumed in scope.

Configuration TOML shape is TBD.

### Overlap with startup

NTP queries should be fired as early as possible during daemon
startup -- ideally in parallel with the receiver-identification
probe and configurator setup -- so that DNS resolution and
round-trip latency overlap with configuration work that has to
happen anyway.  The result is consumed at the moment
trusted-time supply is needed, by which time it is normally
already available.  This applies equally to single-server (6a)
and multi-server (6b) configurations.

The SNTP primitive (`Query` returning a `Result` with UTC
time, monotonic anchor, and accuracy bound) is already
implemented in `time/lib/sntp`; the remaining work is wiring
it into pieces 2 and 3 as a preferred source.  Pieces 1-5
remain sufficient on their own for a working OSNMA system
that uses the local system clock as the initial trust anchor.

## HTTPS cross-check

A separate integrity check on whatever trusted-time estimate
piece 2 or 3 is about to supply: query a configured HTTPS
endpoint with HEAD, parse the response `Date:` header, and
refuse to supply trusted time if the source's UTC time and
the HTTPS server's UTC time disagree by more than a
configured threshold.

The cross-check is source-agnostic: it validates the estimate
produced by piece 2's system-clock construction, by piece 6a's
single SNTP query, or by piece 6b's multi-server agreement.
The same primitive applies in all configurations.

### Why HTTPS

The threat model is the same as for piece 6: a localized GNSS
spoofer that may also affect nearby GNSS-disciplined NTP
servers.  TLS authenticates the HTTPS server (cert chain), and
a configured endpoint can be a global CDN edge or major site
well outside the spoofer's blast radius.  An attacker would
need to compromise the server itself or the CA system to lie
through this channel -- bars far higher than spoofing local
GNSS plus nearby NTP.

`Date:` is mandatory in HTTP/1.1 responses (RFC 7231 sec.
7.1.1.2), so any well-behaved HTTPS endpoint supplies it.
Go's `http.ParseTime` accepts the three legal HTTP-date
formats.

### Precision and threshold

`Date:` is IMF-fixdate (second resolution), so the cross-check
gives roughly `1s + RTT/2` of uncertainty -- two to three
orders of magnitude worse than SNTP.  That is fine for a
cross-check: comparing `|sourceTime - httpsTime|` against a
threshold around 5 seconds catches meaningful disagreement
without flapping on benign skew between the HTTPS edge clock
and the trusted-time source.  Five seconds is well under
OSNMA's 15-second ADKD-type-0 ceiling, so even a check that
happens to be near the threshold edge does not push the
receiver out of OSNMA's operating range.

### Policy when HTTPS is unreachable

If the cross-check is configured but the HTTPS endpoint is
unreachable at the moment trusted time is needed, the daemon
refuses to supply trusted time and logs a warning.  The
alternative -- silently reverting to no cross-check on network
failure -- means an attacker who can disrupt the configured
HTTPS endpoint also defeats the spoof check, which is strictly
worse than refusing.

### Configuration

TBD; minimally a URL (e.g. `https://www.cloudflare.com/`) and
optionally a disagreement threshold.  Multi-endpoint agreement
is a natural extension if needed, mirroring piece 6b's policy
for SNTP, but is not part of the initial implementation.

## Implementation order

The pieces are listed in their implementation order; each
sub-list captures the steps within the piece.

1. Reporting: `NavEpochMsg` NMA-verified field; u-blox decoder
   fills it from `nmaFixStatus`; expose in web UI and as a
   Prometheus metric.
2. Initial trusted time:
   a. Split the time-estimate types: `UTCTimeEstimate` carries
      UTC time, monotonic anchor, and accuracy for the
      startup/configuration case; `TimeEstimate` embeds
      `UTCTimeEstimate` and adds optional TAI (`ptime.Time`) for
      ongoing refresh sources that naturally have GNSS/TAI time.
      Remove `Trusted bool`; trust is expressed by the operation.
   b. `UTCTimeEstimate` construction from the system clock:
      kernel-NTP path, then external-program path for the
      unsynchronized case.
   c. Replace `ConfigOptions.TimeAssist` with
      `ConfigOptions.TrustedTime` (`UTCTimeEstimate`); a zero
      estimate means absent.  Configurators emit the
      receiver-specific one-shot trusted-time packet from this
      field.
   d. Add `gpsprot.TrustedTimePacketBuilder` for receivers that
      require ongoing refresh:
      `TrustedTimePacket(*TimeEstimate, time.Time) ([]byte,
      error)`.
   e. u-blox implements `gpsprot.TrustedTimePacketBuilder`,
      producing both `UBX-MGA-INI-TIME_UTC` and
      `UBX-MGA-INI-TIME_GNSS`, and calls it internally from the
      configurator for the initial trusted-time packet.
      Quectel/Septentrio trusted-time support can be implemented
      directly in their configurator paths and need not expose a
      builder if they do not require ongoing refresh.
   f. `satpulsetool gps`: new flag(s) replacing
      `--sys-time-trusted`; stores the estimate in
      `ConfigOptions.TrustedTime` and lets the configurator emit
      the packet.
   g. Daemon-side initial supply at startup (synchronous):
      introduce/parse `gps.setTrustedTime` TOML and parse
      `gps.nma` for defaulting only (`gps.config && gps.nma ==
      "osnma"`); add `ForceProbe` plumbing, build
      `UTCTimeEstimate`, store it in `ConfigOptions.TrustedTime`,
      and run `gpscfg.Configure` before stream/proxy goroutines
      start.
3. Ongoing trusted time:
   a. `gpscfg.Result` exposes `TrustedTimePacketBuilder` when
      the selected receiver requires ongoing trusted-time
      refresh; for u-blox OSNMA this is required, while
      Septentrio/Quectel can return nil if their one-shot
      trusted-time command is sufficient.
   b. Daemon goroutine package and its `Run` entry point.
   c. Daemon wiring: phase-1 prepare / phase-2 start of the
      goroutine.
   d. `NavEpochMsg` NMA-verified trigger gated on
      `phcsync.Controller` tracking mode; sends `TimeEstimate`
      on the goroutine's channel.
4. Provisioning cleanup: retire the compiled-in Merkle
   constant and `ConfigOptions.OSNMA.MerkleTreeRoot`; remove
   `mgaOSNMAMerkle` from the configurator path.
5. Enabling cleanup: rename `--osnma` to `--nma=osnma|off`;
   wire the already-parsed `gps.nma` TOML knob to the
   configurator.
6. Final cleanup: remove `--sys-time-trusted` and the old
   `ConfigOptions.TimeAssist` name.  Trusted-time setting is no
   longer modeled as generic assistance.
7. Trusted time from NTP, staged:
   a. SNTP primitive (DONE: `time/lib/sntp`).  Remaining
      work: integrate as preferred source for pieces 2 and 3
      when reachable.
   b. Multiple servers with agreement policy (explicit
      hostnames, pools, or a mix).
   c. NTS for authenticated queries -- likely not
      implemented; superseded by step 8.
8. HTTPS cross-check: HEAD request to a configured HTTPS URL,
   parse `Date:` header, refuse trusted time supply if the
   source's estimate disagrees by more than a configured
   threshold.  Source-agnostic: layers on top of kernel NTP,
   single SNTP, or multi-server agreement.
