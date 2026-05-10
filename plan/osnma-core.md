# OSNMA core support (#105)

Galileo Open Service Navigation Message Authentication. This plan covers the
core SatPulse-side work needed for initial workable OSNMA support on receivers
that implement it, initially u-blox. Quectel LC29H and Septentrio have OSNMA
support and can be added later.

This is deliberately not the full trusted-time manager for the daemon. It gets
SatPulse to the point where OSNMA can be provisioned, enabled, observed at both
the fix-authenticated and selected native u-blox status levels, and supplied
with trusted time by explicit tool/socket operations. Autonomous daemon
maintenance of trusted time is a follow-on plan.

The core pieces are:

1. Report whether a navigation fix is NMA-verified.
2. Refactor trusted-time setting around a receiver-specific
   `TrustedTimePacketBuilder`, and make `satpulsetool gps` able to set trusted
   time directly, including from SNTP.
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
separate, harder problem.

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

### Estimate Type

Use `TimeEstimate` for the trusted-time setting path. It carries whichever
timebase the source can provide:

```go
type TimeEstimate struct {
    UTC time.Time
    TAI ptime.Time
    TimeOfEstimate time.Time
    Accuracy time.Duration
}
```

`UTC` and `TAI` are optional; zero means absent. At least one must be present.
There is no `Trusted bool`: trust is expressed by the caller choosing to use
the trusted-time operation. `TimeOfEstimate` carries the monotonic anchor used
to extrapolate the estimate to the actual packet-build time.

The initial tool path will often be UTC-only, from the system clock or SNTP. An
ongoing daemon path may later have TAI/GNSS time, for example when SatPulse is
running with a PHC. A no-PHC mode may remain UTC-only. The builder chooses the
best receiver message form from the timebases present.

### Builder Interface

Receivers that can set trusted time expose a builder in the result returned by
`gpscfg.Configure`:

```go
type TrustedTimePacketBuilder interface {
    TrustedTimePacket(est *TimeEstimate, now time.Time) ([]byte, error)
}
```

This interface only constructs receiver bytes. The caller owns port locking and
writing.

Trusted time is no longer passed through `ConfigTarget` or
`ConfigOptions.TimeAssist`. `gpscfg.Configure` returns the builder even when no
persistent receiver configuration is being changed, using `ForceProbe` when the
caller needs identification only.

For u-blox, the configurator builds a packet builder that stores the GNSS time
system to use when a TAI estimate is converted to `UBX-MGA-INI-TIME_GNSS`. If a
TAI/GNSS estimate is present, the builder emits `UBX-MGA-INI-TIME_GNSS` using
that stored GNSS; otherwise it emits `UBX-MGA-INI-TIME_UTC`. SatPulse's
`gps.timeGNSS` configuration already writes the related u-blox timing keys
(`CFG-TP-TIMEGRID_TP1`, `CFG-NAVSPG-UTCSTANDARD`, and `CFG-RATE-TIMEREF` where
supported), so the builder should use the same GNSS choice. The expectation
that `MGA-INI-TIME_UTC` plus `CFG-NAVSPG-UTCSTANDARD` determines the
trusted-time reference system should be verified empirically with
`UBX-NAV-TIMETRUSTED`.

Septentrio and Quectel can later return builders for their one-shot
trusted-time commands. They do not need the u-blox-style ongoing refresh logic
in this core phase.

### `satpulsetool gps`

The existing hidden `--sys-time-trusted` path should be cleaned up to use the
builder and should no longer be hidden once this work ships. The tool flow is:

1. Run the normal receiver probe/configure path enough to obtain a
   `TrustedTimePacketBuilder`.
2. Build a `TimeEstimate`.
3. Ask the builder for bytes at `time.Now()`.
4. Write the bytes to the receiver or daemon socket.

The tool should also add:

- `--trusted-time-ntp <server>`: query the server with the existing
  `time/lib/sntp` client and use the returned UTC time, monotonic anchor, and
  accuracy bound.
- `--trusted-time-uncertainty <duration>`: supply an explicit uncertainty when
  the source does not provide one, such as a manually trusted system clock.

For u-blox, `satpulsetool gps` should try to fill both `UTC` and `TAI` in the
`TimeEstimate` so the builder can use `MGA-INI-TIME_GNSS` with the configured
GNSS time system. The leap-second offset is opportunistic:

- System-clock path: if kernel NTP state reports a usable `TAIOffset`, derive
  `TAI` from the UTC estimate plus that offset. Kernel NTP state cannot be
  relied on to report this; chrony configuration determines whether it is set.
  If it is absent, keep the estimate UTC-only.
- SNTP path: use SNTP for the UTC estimate, monotonic anchor, and accuracy. If
  the local kernel NTP state also reports a usable `TAIOffset`, use that offset
  only for UTC-to-TAI conversion; do not treat local kernel NTP as the time
  accuracy source for the SNTP estimate. If the offset is absent, keep the
  estimate UTC-only.

The cleaner long-term path is for `satpulsetool gps` to learn the leap-second
offset from the receiver, since the receiver usually knows it. That implies
polling/observing receiver time or leap-second messages during the probe, and
is probably more complexity than the first core implementation needs. An
explicit leap-second/UTC-offset option can be an escape hatch, but should not be
the primary user workflow.

With `satpulsetool gps --socket`, this gives an operational interim path for
refreshing u-blox trusted time from cron or a systemd timer without building the
full daemon manager first.

## Provisioning Merkle and Public Keys

The Merkle tree root and public key(s) are persistent receiver state. They
change rarely: the Merkle root is intended to last around ten years, while
public keys rotate through the key-rollover mechanism in the broadcast. Users
should provision them explicitly with `satpulsetool gps --msg-file`.

Today, the hidden `--osnma` flag provisions a compiled-in Merkle root through
`ConfigOptions.OSNMA.MerkleTreeRoot`. Move provisioning out of code:

- Merkle root and public key provisioning live in
  `configs/gpsmsg/osnma.toml`, including the `osnma-pubkey-2` tag.
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
delta fields. For `SEC-OSNMA`, include OSNMA enabled/header status, time-sync
requirement/status, DSM authentication status, TESLA key authentication status,
timing authentication, authenticated satellite count, MAC ADKD type, and
Merkle/public-key validity/source fields.

Wire this into the daemon in `time/app/daemon/daemon.go`:

- Construct `ubxObs := logobs.NewUBXLogObserver(lg)` near the existing
  `GPSLogObserver`.
- Add `ubxObs` to `combineObservers`.
- Keep this always enabled with the daemon's normal logging; receiver output is
  still controlled by the message configuration in `configs/gpsmsg/osnma.toml`.

Add focused tests for `UBXLogObserver.NativeMsg`:

- It returns `true` for `*ubxbin.NavTimeTrusted`.
- It returns `true` for `*ubxbin.SecOsnma`.
- It returns `false` for other tags or unrelated UBX messages.

## Implementation Order

1. Reporting:
   - Add the NMA-verified field to `NavEpochMsg`.
   - Fill it from u-blox `UBX-NAV-PVT.nmaFixStatus`.
   - Expose it in the web UI and Prometheus.

2. UBX native observability:
   - Add `time/internal/logobs/ubxlog.go`.
   - Log `UBX-NAV-TIMETRUSTED` and `UBX-SEC-OSNMA` through the native-message
     observer hook.
   - Wire the observer into `time/app/daemon/daemon.go`.
   - Add focused observer tests.

3. Trusted-time builder and tool path:
   - Replace the current `TimeEstimate` fields with optional UTC, optional TAI
     (`ptime.Time`), monotonic anchor, and accuracy.
   - Remove the old trusted-time-as-assistance path
     (`ConfigOptions.TimeAssist` / `ConfigTarget` plumbing).
   - Add `TrustedTimePacketBuilder` to the configure/probe result.
   - Implement the u-blox builder for `MGA-INI-TIME_UTC` and
     `MGA-INI-TIME_GNSS`.
   - Update `satpulsetool gps --sys-time-trusted` to use the builder and make
     it a supported option rather than a hidden one.
   - Add `--trusted-time-ntp` using `time/lib/sntp`.
   - Add an uncertainty option for sources that cannot provide max error.
   - Document the cron/systemd timer plus daemon-socket refresh pattern.

4. Provisioning cleanup:
   - Move Merkle/public-key provisioning to `configs/gpsmsg/osnma.toml`.
   - Remove the compiled-in Merkle root and related configurator plumbing.

5. Enable cleanup:
   - Rename/replace the old hidden `--osnma` behavior with
     `--nma=osnma|off`.
   - Add `gps.nma` TOML.
   - Wire the NMA property to the configurator, gated by `gps.config`.

## Follow-On: Daemon Trusted-Time Manager

The full daemon manager is substantially harder than this core OSNMA plan and
is not yet a settled design. The issues, configuration shape, and candidate
approaches are tracked separately in `plan/daemon-trusted-time.md`.
