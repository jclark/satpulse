# OSNMA core support (#105)

Galileo Open Service Navigation Message Authentication. This plan covers the
core SatPulse-side work needed for initial workable OSNMA support on receivers
that implement it, initially u-blox. Quectel LC29H and Septentrio have OSNMA
support and can be added later.

This is deliberately not the full trusted-time manager for the daemon. It gets
SatPulse to the point where OSNMA can be provisioned, enabled, observed at both
the fix-authenticated and selected native u-blox status levels, and supplied
with trusted time by explicit tool/socket operations. Autonomous daemon
maintenance of trusted time is covered separately in
`plan/daemon-trusted-time.md`.

The core pieces are:

1. Report whether a navigation fix is NMA-verified.
2. Refactor trusted-time setting around a receiver-specific
   `TrustedTimePacketBuilder`. Phase 1 cleans up the existing
   `satpulsetool gps --sys-time-trusted` path; later phases add explicit
   uncertainty for system-clock use, SNTP, and richer u-blox timebase handling.
3. Provision OSNMA Merkle/public-key data from `osnma.toml` instead of compiled
   code.
4. Enable OSNMA with explicit `gps.nma` / `--nma` controls.
5. Log selected native u-blox OSNMA and trusted-time status messages from the
   daemon.

The result is useful but imperfect. In particular, u-blox trusted time decays
unless refreshed. The pragmatic interim refresh path is to run
`satpulsetool gps --socket ...` from cron or a systemd timer; the daemon socket
is intentionally writable so normal Unix permissions can control who is allowed
to perform these receiver writes. A daemon-owned trusted-time state machine is a
separate, harder problem covered by `plan/daemon-trusted-time.md`.

## Reporting OSNMA

`NavEpochMsg` gains an `opt.Val[bool]` field carrying the NMA-verified status
of the fix, populated by the u-blox decoder from the `nmaFixStatus` bit in
`UBX-NAV-PVT`.

The new field is surfaced in existing sinks:

- Web UI: expose the boolean alongside the other `NavEpochMsg` fields already
  shown.
- Prometheus: expose it as a metric alongside the other `NavEpochMsg`-derived
  metrics.

This protocol-independent field remains the right surface for sinks such as the
web UI and Prometheus. Richer native status stays out of `gpsprot` and is logged
through protocol-specific observer code.

## Per-signal authentication

`gpsprot.SignalInfo` gains an `Auth bool` field reporting whether the
navigation data carried by that signal was authenticated in the current
navigation epoch. The u-blox decoder populates it from the `authStatus` bit in
`UBX-NAV-SIG` `sigFlags` (add a `NavSigAuth` constant in `ubxbin` alongside
the existing flag bits). `bool` rather than `opt.Val[bool]`: the value is
asymmetric. `true` means the signal's navigation data is known to be
authenticated; `false` means that is not known. There is no "known to be
unauthenticated" value -- failed authentication causes the receiver to
discard the data rather than report a failure.

Currently the only per-signal data authentication is Galileo OSNMA on E1
I/NAV; other protocols can populate `Auth` as their authentication support is
added.

## Trusted-Time Packet Builder

OSNMA requires the receiver to know the current time accurately enough to apply
the TESLA-key timing check. Per the OSNMA Receiver Guidelines referenced in the
u-blox F9 HPG 1.51 docs, accuracy must be better than 15 s to use MAC ADKD type
0, better than 165 s to use MAC ADKD type 12, and beyond 165 s OSNMA cannot be
applied.

SatPulse should model this as setting the receiver's internal trusted-time
clock, not as assistance. u-blox transports the operation through
`UBX-MGA-INI-TIME_UTC` / `UBX-MGA-INI-TIME_GNSS` with the `trustedSource` bit
set, but that is a u-blox implementation detail. Septentrio and Quectel model
trusted time with specific commands, and many receivers have assistance
features without OSNMA support.

Trusted time is also not receiver configuration in the user-facing `gps.config`
sense: it is not persistent, and a user with persistent receiver configuration
still needs to set trusted time after a cold boot. Internally, however, the
builder is returned by the existing configure/probe path because that path
already identifies the receiver and selects the protocol-specific backend.

### Phases

Trusted-time support should land in small phases. The first phase is only the
clean system-clock path needed to make the existing hidden flag supportable; it
does not take on explicit uncertainty, SNTP, daemon refresh, or u-blox
GNSS-time packets.

#### Phase 1: System Clock To Trusted UTC

Phase 1 keeps `TimeEstimate` close to the existing UTC estimate shape:

```go
type TimeEstimate struct {
    EstimatedTime time.Time
    TimeOfEstimate time.Time
    Accuracy time.Duration
    LeapSecond ptime.LeapSecondState
}
```

`EstimatedTime` is UTC. `TimeOfEstimate` carries the monotonic anchor used to
extrapolate the estimate to packet-build time. `LeapSecond` stays for now:
`satpulsetool gps --sys-time-trusted` can fill it from the kernel NTP state,
and the u-blox `MGA-INI-TIME_UTC` packet can use it for the `leapSecs` field.
Do not remove it as part of phase 1; any later replacement with UTC/TAI fields
needs a separate design decision. There is no `Trusted bool`: trust is
expressed by the caller choosing to use the trusted-time operation.

Receivers that can set trusted time expose a packet builder through the normal
configure/probe result path:

```go
type TrustedTimePacketBuilder interface {
    TrustedTimePacket(est *TimeEstimate, now time.Time) ([]byte, error)
}
```

This interface only constructs receiver bytes. The caller owns port locking and
writing.

Trusted-time estimates are no longer passed through `ConfigTarget` or
`ConfigOptions.TimeAssist`. Phase 1 replaces the existing
`TimeAssist TimeEstimate` field with a simple request flag:

```go
type ConfigOptions struct {
    // ...
    TrustedTime bool // try to return a TrustedTimePacketBuilder
}
```

`TrustedTime` does not provide time to the receiver. It tells the configurator
that the caller wants trusted-time packet construction support. Later phases
may allow protocol-specific extra polling needed to initialize richer builders.

Make the builder a first-class part of the config API rather than an optional
capability or generic extra:

```go
type Configurator interface {
    ConfigProps() *ConfigProps
    ReceiverInfo() *ReceiverInfo
    TrustedTimePacketBuilder() TrustedTimePacketBuilder

    GenerateRequests() error
    GetRequestCount() (count int, complete bool)
    Request(index int) ConfigRequest
}
```

and carry it through `gpscfg.Configure`:

```go
type Result struct {
    ReceiverInfo              *gpsprot.ReceiverInfo
    ConfigProps               *gpsprot.ConfigProps
    TrustedTimePacketBuilder  gpsprot.TrustedTimePacketBuilder
    PacketFormatsDetected     []gpsprot.Tag
}
```

The result is best effort. If `ConfigOptions.TrustedTime` is false, backends
should not do trusted-time-specific work and should normally return nil. If it
is true, backends that support trusted time should try to return a builder;
unsupported receivers or unresolved dependencies leave the builder nil, and the
tool path decides how to report that to the user.

For u-blox, phase 1 returns a builder for `UBX-MGA-INI-TIME_UTC` only. The
builder sets the `trustedSource` bit unconditionally, uses `LeapSecond` to set
`leapSecs` when the TAI-UTC offset is known, and otherwise sends unknown leap
seconds. If monotonic extrapolation crosses an announced leap-second boundary,
the builder gives up on the leap-second value and increases the time accuracy
by one second. Direct caller misuse such as a nil estimate or zero
`EstimatedTime` is a programming error and should panic at the builder
boundary; normal command-level failures such as an unsupported receiver should
remain ordinary errors.

The existing hidden `--sys-time-trusted` path should be cleaned up to use this
builder and should no longer be hidden once phase 1 ships. The tool flow is:

1. Set `ConfigOptions.TrustedTime` and run the normal receiver probe/configure
   path enough to try to obtain a `TrustedTimePacketBuilder`.
2. Build a `TimeEstimate`.
3. Ask the builder for bytes at `time.Now()`.
4. Write the bytes to the receiver or daemon socket.

For phase 1, `--sys-time-trusted` is system-clock only. `gpscmd` obtains the
estimate from the kernel NTP state, requires the clock to be synchronized, and
fills `LeapSecond` from the kernel-reported TAI offset and leap-second status.
It should also update the usage text and man page so the option is documented
as a supported command option.

#### Phase 2: System Clock With Explicit Uncertainty

After the synchronized system-clock path is clean, add:

- `--trusted-time-uncertainty <duration>`: supply an explicit uncertainty if
  the user wants to trust the host system clock even though the kernel NTP
  state does not report synchronization or cannot provide a useful max-error
  bound.

This option is not a separate time source. It is a system-clock policy knob:
without it, `--sys-time-trusted` should continue to require synchronized kernel
NTP state; with it, the command can use the host clock with the user-supplied
accuracy bound. Leap-second information remains best effort. If the kernel
state cannot provide a usable TAI offset, the `TimeEstimate` should leave
`LeapSecond` unknown.

#### Phase 3: SNTP Source

After the system-clock paths are clean, add the only other trusted-time source
in this core plan:

- `--trusted-time-ntp <server>`: query the server with the existing
  `time/lib/sntp` client and use the returned UTC time, monotonic anchor, and
  accuracy bound.

This phase can also revisit where the estimate is constructed in the command
flow. The cleanest long-term flow is for flag parsing to record the requested
trusted-time source, and for the command to estimate time immediately before
packet construction and write. That is not required for phase 1.

With `satpulsetool gps --socket`, this gives an operational interim path for
refreshing u-blox trusted time from cron or a systemd timer without building the
full daemon manager first. Document that pattern with the phase 3 tool work.

#### Phase 4: TAI/GNSS-Aware Builders

For u-blox, a later phase should let `satpulsetool gps` fill both UTC and TAI
information in the `TimeEstimate` so the builder can use
`MGA-INI-TIME_GNSS` with the configured GNSS time system when that is better
than `MGA-INI-TIME_UTC`. The leap-second offset is opportunistic:

- System-clock path: if kernel NTP state reports a usable `TAIOffset`, derive
  `TAI` from the UTC estimate plus that offset. Kernel NTP state cannot be
  relied on to report this; chrony configuration determines whether it is set.
  If it is absent, keep the estimate UTC-only.
- SNTP path: use SNTP for the UTC estimate, monotonic anchor, and accuracy. If
  the local kernel NTP state also reports a usable `TAIOffset`, use that offset
  only for UTC-to-TAI conversion; do not treat local kernel NTP as the time
  accuracy source for the SNTP estimate. If the offset is absent, keep the
  estimate UTC-only.

The existing plan to replace the current `EstimatedTime`/`LeapSecond` estimate
with optional UTC and optional TAI fields should be re-evaluated in this phase.
Keeping explicit leap-second state may still be useful, especially when the
receiver or kernel knows the offset but the source estimate is UTC-only.

The u-blox GNSS choice is resolved from the explicit target `timeGNSS` value
first. If no `timeGNSS` is being set and `ConfigOptions.TrustedTime` is true,
the configurator may query the receiver's current timing configuration to learn
the existing GNSS time system. This query is not done on unrelated configure
paths. If the GNSS time system is still unknown, a builder may still use
`MGA-INI-TIME_UTC` for UTC estimates; it cannot build `MGA-INI-TIME_GNSS` from a
TAI-only estimate without a known GNSS time system.

Do not assume all OSNMA-capable-looking u-blox receivers accept trusted-time
MGA packets in the same way. Before treating the u-blox builder as production
quality, verify the u-blox protocol-version support for `UBX-MGA-INI-TIME_UTC`
and `UBX-MGA-INI-TIME_GNSS`, including the `trustedSource` bit. The issue #105
history only establishes useful empirical points: `--sys-time-trusted` was part
of a working ZED-F9P HPG 1.51 / PROTVER 27.50 OSNMA setup, while NEO-F10T TIM
3.01 / PROTVER 42.01 did not support OSNMA. It does not establish a minimum
protocol version for trusted-time MGA, nor whether setting `trustedSource` can
cause rejection or different behavior on receivers where plain assistance time
would otherwise be accepted. Test this with ACK/NAK behavior and
`UBX-NAV-TIMETRUSTED`.

Septentrio and Quectel can later return builders for their one-shot
trusted-time commands. They do not need the u-blox-style ongoing refresh logic
in this core plan.

## Provisioning Merkle and Public Keys

The Merkle tree root and public key(s) are persistent receiver state. They
change rarely: the Merkle root is intended to last around ten years, while
public keys rotate through the key-rollover mechanism in the broadcast. Users
should provision them explicitly with `satpulsetool gps --msg-file`.

Today, the hidden `--osnma` flag provisions a compiled-in Merkle root through
`ConfigOptions.OSNMA.MerkleTreeRoot`. Move provisioning out of code:

- Merkle root and public key provisioning live in
  `configs/gpsmsg/u-blox/osnma.toml`, including the `osnma-pubkey-2` tag.
- The user runs `satpulsetool gps --msg-file osnma.toml --tag <tag>` to apply
  the needed records.
- Retire the compiled-in `OSNMAMerkleTreeRoot` constant in
  `internal/gpscmd/gpsflags.go`, the `ConfigOptions.OSNMA.MerkleTreeRoot`
  field in `gpsprot`, and the configurator's `mgaOSNMAMerkle` call.

The OSNMA enable bit stays in code as `--nma=osnma|off` and `gps.nma`.

## Enabling OSNMA

Add an explicit NMA selection:

- `gps.nma` in TOML.
- `satpulsetool gps --nma=osnma|off`.

Initial values:

- unset / `"none"`: no OSNMA change.
- `"osnma"`: enable OSNMA processing. For u-blox, set
  `CFG-GAL-USE_OSNMA = 1`.
- `"off"`: disable OSNMA processing where supported.

For u-blox, do not treat the existence of `CFG-GAL-USE_OSNMA` in SatPulse's
schema as proof that a receiver can enable OSNMA. The issue #105 history reports
a u-blox failure mode where `CFG-GAL-USE_OSNMA` appears to be recognized in a
wildcard `CFG-GAL-*` `VALGET`, but the receiver rejects the `VALSET`. The
configurator should first check whether `CFG-GAL-USE_OSNMA` is recognized by
querying `CFG-GAL-*`, then apply a specific skip override for the known
false-positive case: NEO-F10T with TIM 3.01 / PROTVER 42.01. That receiver
should not attempt the `CFG-GAL-USE_OSNMA` `VALSET` even if the key appears in
the wildcard query. The configuration result remains best effort: report the
observed `NavMsgAuth` / NMA state when available, leave it unset when unknown,
and let the command/UI show that OSNMA was not enabled. Add empirical records
for other firmware/protocol combinations as they are tested, especially F10T
firmware where OSNMA is expected to start working.

Receiver enable configuration remains gated by `gps.config`. When
`gps.config = true`, the daemon applies the corresponding config-key setting
via the configurator. When `gps.config = false`, the daemon assumes the user
has already configured the receiver persistently.

`gps.nma` should be defined early enough for the trusted-time tooling to use it
for defaults or UI consistency, but wiring it to receiver configuration belongs
to this enable step.

The u-blox-only settings below remain in the message file for now:

- `CFG-NAVSPG-ONLY_AUTHDATA`: tags `osnma-only-auth` /
  `osnma-only-auth-off`.
- `CFG-GAL-OSNMA_TIMESYNC`: tags `osnma-timesync` /
  `osnma-timesync-off`.

Promoting either to daemon TOML is a later choice.

## Protocol-Specific UBX Observability

u-blox exposes useful native OSNMA and trusted-time state that does not fit
cleanly into the current protocol-independent `gpsprot` model:

- `UBX-NAV-TIMETRUSTED`: reports the receiver's internal trusted-time state,
  including reference system, validity, propagated accuracy, and delta fields.
- `UBX-SEC-OSNMA`: reports u-blox OSNMA state in more detail than a single
  authenticated-fix boolean.

Use the native-message observer hook for this rather than forcing these fields
through `gpsprot`. The u-blox packet processor already calls the native-message
path only for parsed messages that were not converted into protocol-independent
messages, so handled messages such as `NAV-PVT`, `NAV-TIME*`, `NAV-SAT`, and
`NAV-SIG` are not duplicated.

Add `time/internal/logobs/ubxlog.go` with a `UBXLogObserver` that embeds
`obs.DefaultObserver`, depends on `gps/lib/ubxbin`, and implements:

```go
func (o *UBXLogObserver) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) bool
```

The observer should:

- Return `false` for non-UBX messages and unhandled UBX message types.
- Type-switch on `*ubxbin.NavTimeTrusted` and `*ubxbin.SecOsnma`.
- Log concise structured entries through `slog` for the useful status fields.
- Return `true` after logging one of the recognized messages, so the dispatcher
  does not also emit the generic unused-native-message debug log.

Initial log fields should be enough for operational diagnosis without turning
the log into a dump of the entire UBX struct. For `NAV-TIMETRUSTED`, include
the reference system, validity bits, initial and propagated time accuracy, and
delta fields. For `SEC-OSNMA`, log a derived operational state plus only the
fields that explain that state. Avoid per-SV data and low-level details such as
TESLA chain ids unless later field experience shows they are needed.

Wire this into the daemon in `time/app/daemon/daemon.go`:

- Construct `ubxObs := logobs.NewUBXLogObserver(lg)` near the existing
  `GPSLogObserver`.
- Add `ubxObs` to `combineObservers`.
- Keep this always enabled with the daemon's normal logging; receiver output is
  still controlled by the message configuration in `configs/gpsmsg/u-blox/osnma.toml`.

Add focused tests for `UBXLogObserver.NativeMsg`:

- It returns `true` for `*ubxbin.NavTimeTrusted`.
- It returns `true` for `*ubxbin.SecOsnma`.
- It returns `false` for other tags or unrelated UBX messages.

### SEC-OSNMA Operational States

`UBX-SEC-OSNMA` should be logged in the same spirit as trusted time: recognize
every message, but emit a normal daemon log line only when the derived
operational state changes significantly. The log entry should state the
receiver's OSNMA state and include the minimal fields needed to explain it.

Operational states, in evaluation order:

- `disabled`: the receiver is not executing OSNMA.
- `timeSyncFailed`: OSNMA cannot proceed because the trusted-time requirement
  is unmet.
- `serviceFailed`: the authenticated Galileo NMA service status says OSNMA is
  invalid and must not be used.
- `dsmFailed`: DSM-KROOT or DSM-PKR authentication has failed or cannot be
  applied.
- `missingTrustMaterial`: OSNMA cannot proceed because the current public key or
  Merkle root is not valid.
- `teslaFailed`: TESLA key authentication has failed or is impossible for the
  current key.
- `badData`: OSNMA data is present but receiver monitoring reports malformed or
  inconsistent OSNMA/MAC data.
- `noData`: OSNMA is enabled, but the receiver is not currently collecting
  usable OSNMA data.
- `pending`: OSNMA appears enabled and not blocked, but no satellite navigation
  data has authenticated yet.
- `authenticating`: at least one satellite's navigation data has authenticated.

State distinction is top-down: the first matching state wins. This prevents
secondary symptoms from hiding the root cause. For example, an OSNMA Alert
Message can invalidate public-key and Merkle-root material; `dsmFailed` should
therefore win over `missingTrustMaterial`.

State distinction rules:

- `disabled`: `osnmaEnabled` is false.
- `timeSyncFailed`: `timSyncEnabled` is true and `timSyncStatus` is
  `noTrustedTime`, `notAccurate`, or `failed`.
- `serviceFailed`: `nmaStatus` is `invalid`.
- `dsmFailed`: `dsmAuthenticationStatus` is an alert or failure state:
  `alert`, `KROOTFail`, `PKRFail`, `unknownPK`, `PKDecompressFail`,
  `configNotSupported`, or `NMTMissingMerkle`.
- `missingTrustMaterial`: current `pubKeyVal` or `merkleRootVal` is false.
- `teslaFailed`: `teslaKeyAuthStatus` is `fail`, `past`, or `oldRoot`.
- `badData`: any of `wrongData`, `wrongFlxMac`, or `wrongMaclt` is true.
- `noData`: `osnmaEnabled` is true and either `numberSVs` is zero or `noData`
  is true.
- `pending`: none of the above states applies, and `authenticatedSVs` is zero.
- `authenticating`: `authenticatedSVs` is greater than zero.

Fields shown by state:

- `disabled`: `state`, `osnmaEnabled`.
- `timeSyncFailed`: `state`, `timeSyncStatus`, and `timeSyncDiff` only when
  the receiver reports a meaningful pass/fail difference.
- `serviceFailed`: `state`, `nmaStatus`, `cpks`.
- `dsmFailed`: `state`, `dsmAuth`, `cpks`.
- `missingTrustMaterial`: `state`, `pubKeyValid`, `merkleRootValid`,
  `dsmAuth`, `cpks`.
- `teslaFailed`: `state`, `teslaKeyAuth`.
- `badData`: `state`, `osnmaSVs`, `wrongData`, `wrongFlxMac`, `wrongMaclt`.
- `noData`: `state`, `osnmaSVs`, `noData`.
- `pending`: `state`, `osnmaSVs`, `nmaStatus`, `dsmAuth`, `teslaKeyAuth`,
  `timeSyncStatus`.
- `authenticating`: `state`, `nmaStatus`, `osnmaSVs`, `authenticatedSVs`,
  `timeSyncEnabled`, `macAdkdType`, `timingAuth`, and
  `authenticatedTiming`.

Logging triggers:

- Always log the first recognized `UBX-SEC-OSNMA` message.
- Always log when the derived operational `state` changes.
- Within a state, log when one of that state's displayed non-count fields
  changes. This catches meaningful transitions such as service test vs
  operational mode, time-sync status changes, DSM/TESLA failures, public-key or
  Merkle-root validity changes, or MAC ADKD mode changes.
- Treat `osnmaSVs`, `authenticatedSVs`, and `authenticatedTiming` as noisy
  counts. Track a high-water mark for each count. Log a count when it reaches a
  new high-water mark. If it later falls to less than half of that high-water
  mark, log the drop and reset the high-water mark to the lower value. This
  logs meaningful degradation and then shows recovery without logging ordinary
  satellite visibility churn.

## Implementation Order

1. Trusted-time builder, phase 1:
   - Keep `TimeEstimate` as a UTC estimate with `TimeOfEstimate`, `Accuracy`,
     and `LeapSecond`; remove only the old `Trusted` field.
   - Remove the old trusted-time-as-assistance path
     (`ConfigOptions.TimeAssist` / `ConfigTarget` plumbing).
   - Add `ConfigOptions.TrustedTime` as the explicit request for builder
     creation.
   - Add `TrustedTimePacketBuilder()` to `gpsprot.Configurator` and
     `TrustedTimePacketBuilder` to the `gpscfg.Configure` result.
   - Implement the u-blox builder for trusted `MGA-INI-TIME_UTC`, using
     `LeapSecond` when available.
   - Update `satpulsetool gps --sys-time-trusted` to use the builder and make
     it a supported option rather than a hidden one.
   - Update the usage text and man page.

2. UBX native observability:
   - Add `time/internal/logobs/ubxlog.go`.
   - Log `UBX-NAV-TIMETRUSTED` and `UBX-SEC-OSNMA` through the native-message
     observer hook.
   - Wire the observer into `time/app/daemon/daemon.go`.
   - Add focused observer tests.

3. Reporting:
   - Add the NMA-verified field to `NavEpochMsg`.
   - Fill it from u-blox `UBX-NAV-PVT.nmaFixStatus`.
   - Expose it in the web UI and Prometheus.

4. Per-signal authentication:
   - Add a `NavSigAuth` flag constant to `ubxbin`.
   - Add the `Auth` field to `gpsprot.SignalInfo`.
   - Fill it from u-blox `UBX-NAV-SIG` `sigFlags.authStatus`.

5. Trusted-time phase 2:
   - Add `--trusted-time-uncertainty` for system-clock use when the kernel NTP
     state is not synchronized or cannot provide a useful max-error bound.

6. Trusted-time phase 3:
   - Add `--trusted-time-ntp` using `time/lib/sntp`.
   - Document the cron/systemd timer plus daemon-socket refresh pattern.

7. Trusted-time phase 4:
   - Revisit the estimate type for optional TAI/GNSS support without assuming
     `LeapSecond` should disappear.
   - Implement the u-blox builder for `MGA-INI-TIME_GNSS`.
   - Verify trusted-time MGA ACK/NAK behavior and `UBX-NAV-TIMETRUSTED` on
     target receiver/protocol versions.

8. Provisioning cleanup:
   - Move Merkle/public-key provisioning to `configs/gpsmsg/u-blox/osnma.toml`.
   - Remove the compiled-in Merkle root and related configurator plumbing.

9. Enable cleanup:
   - Rename/replace the old hidden `--osnma` behavior with
     `--nma=osnma|off`.
   - Add `gps.nma` TOML.
   - Wire the NMA property to the configurator, gated by `gps.config`.

## Follow-On: Daemon Trusted-Time Manager

Tracked separately in `plan/daemon-trusted-time.md`.
