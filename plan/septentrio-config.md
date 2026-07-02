# Septentrio high-level (Tier 1) configuration (#341)

A follow-up add-on to **`plan/septentrio-core.md`** (#340): a
`gpsprot.ConfigProtocol`/`Configurator` that promotes Septentrio from
Tier 2 (message-file configuration) to Tier 1 (device-independent
`--gnss`/`--pps`/mode/fixed-position/etc. through `satpulsetool gps` and
the daemon's auto-configuration). **Depends on `sbfbin.md`** (probe /
`ReceiverSetup` decode) and, procedurally, on the rest of core. Target
hardware is the mosaic-G5; X5 differences are noted inline.

**Build this with the `implement-configprotocol` skill.** That skill
owns the *process* and the parts common to every backend: the
`gpshwtest/SEMANTICS.md` contract (truthful achieved values,
readback-is-the-ack not a re-poll, refusal leaves config unchanged,
nonexistence shown not announced, best-effort realization), its
rulings, and the `ConfigProtocol`/`Configurator`/`ConfigDirector`
machinery. This plan stays deliberately light: it records only the
Septentrio-specific inputs that process needs, and defers the "how" to
the skill. It will be fleshed out when hardware arrives; until then it
is derived from the mosaic-G5 v1.1.0 and mosaic-X5 v4.15.1 guides only,
and every fact here is subject to the skill's re-verify-against-hardware
step.

## Prerequisites

Core landed and hardware-verified (`plan/septentrio-core.md`):
`gps/lib/sbfbin` decode, the `gps/internal/septentrio` conversion layer,
and the Tier 2 message-file layer with the `"septentrio"` response
analyzer (`plan/septentrio-msgfile.md`). The command knowledge
(mnemonics, argument shapes, the four `$R*` reply shapes) already exists
there and is reused, not re-derived.

**Reply framing is designed in `plan/septentrio-msgfile.md`, not here.**
The ASCII reply channel needs real `PacketFormat`s (the `EmptyTag`
fallback is unusable -- `gps/scan` gives no packet boundaries for
unrecognized bytes): a settled `$R` reply format (Format 1) for the
`$R:`/`$R;`/`$R!`/`$R?` acks, and an unresolved Format 2 for the
headerless state lines and the unterminated `COMx>` prompt (needs a
captured session). This Tier 1 plan reuses that framing; how the framed
replies reach the config machinery as typed values is an implementation
detail for the `implement-configprotocol` skill against real hardware,
not specified in duplicate here.

## Septentrio-specific inputs

### Probe and identification

- `ProbePacket()` sends `getReceiverCapabilities` (`grc`), no arguments.
  One round trip that (a) proves the receiver is present (the CLI is
  byte-identical X5/G5, so an ack is a family-wide probe), (b) reports
  the enabled *capabilities* (`SBAS`, `RTKRover`, `GalOSNMA`,
  `PPPGalileoHAS-SIS`, `xPPSOutput`, ...), supported signals, ports, and
  default intervals, and (c) is the single source for all capability
  gating below. State-neutral; may be repeated per the skill's probing
  ruling.
- `ReceiverInfo()`: supported GNSS/signals from `grc`'s signal list (via
  the same signal-name <-> `gpsprot` table the conversion layer built --
  one table, not two); `Vendor = "Septentrio"`; `Hardware`/`Firmware`
  from the SBF `ReceiverSetup` (5902) block, requested once via
  `exeSBFOnce` and read back through `NativeMsg` (`grc` doesn't carry
  them).

### Sequential, exact-echo correlation

The CLI has no queueing (wait for the prompt between commands), so the
`Configurator` is single-flight: one outstanding request at a time
(model `ubx`'s `configRequest`/`requestOps`, scaled to single-flight,
rather than `unc`'s phase enum). Correlation is by exact command-echo --
`$R:`/`$R;` reproduce the command verbatim; `$R?` carries real error
text that becomes `ConfigRequest.GetError()` directly. Single-flight
concurrency is itself the correlation key for anchor-less state lines.
A command completes on its prompt -- the `CD>` "command done" signal
(guide sec 3.1; framed by Format 2, see `plan/septentrio-msgfile.md`).
Restart commands (`exeResetReceiver`) end in `STOP>` instead, after
which the line goes quiet.

### Property mapping

| `PropID` | Septentrio command(s) | Notes |
|---|---|---|
| `SignalsEnabled` | `setSatelliteTracking` (`sst`) + `setSignalTracking` (`snt`) + `setSignalUsage` (`snu`) | Three commands realize one property: `sst` gates by constellation, `snt`/`snu` by concrete signal (the guide pairs them: both needed to track). Derive the constellation set from the target signals (`all` when all present); intersect with `grc`'s supported-signal list (best-effort). |
| `TimeGNSS` | `setPPSParameters` `TimeScale` arg | Restricted to `gpsprot.GNSS`'s choices (GPS/Galileo/BeiDou/GLONASS); the protocol's `UTC`/`RxClock` have no device-independent analogue. |
| `TimePulseWidth`/`Period`/`AlignToGNSS`/`OnlyWhenLocked`/`PolarityRising` | `setPPSParameters` (`Interval`, `PulseWidth`, `Polarity`) | Primary xPPS (PPS1) only -- see "Dual PPS". |
| `Mode` (static/rover, fixed pos) | `setPVTMode` (`spm`) + `setStaticPosGeodetic`/`setStaticPosCartesian` | `Static==false` -> `setPVTMode, Rover, <RoverMode>, auto`. `Static==true, PosTypeNone` -> `setPVTMode, Static, , auto`. `Static==true` + LLH/ECEF -> write the static position, then `setPVTMode, Static, , GeodeticN`/`CartesianN`. |
| `AntennaCableDelay` | `setCalibCommonDelay` (`scco`) | Pseudorange-level delay (ns, clamp -10000..10000) -- **not** `setPPSParameters`' `Delay` (which moves only the pulse). |
| `NavMsgAuth` | `setGalOSNMAUsage` (`sou`) [+ `exeSetTime`] | See "OSNMA". |
| `RTCMBaseID` | `setRTCMv3Formatting` `ReferenceID` | 0-4095 (clamp). |
| `MinElevation` | `setElevationMask` (`sem`), `Engine = PVT` | Solution mask (the receiver's separate tracking mask is not exposed). |
| `BaudRate` | `setCOMSettings` (`scs`) | `GetSpeedChangeAfter` + a repeat-confirmation of the identical command (the `unc` pattern); the change request itself is never retried. |
| `Port` (read-only) | none | From the connection descriptor in the reply prompt. |

`Survey` and its `ConfigSupport*` flags are unset: Septentrio has no
parameterized/terminating/observable survey operation (`setPVTMode,
Static, , auto` is an auto-computed reference, surfaced as
`Mode.Static`+`PosTypeNone`, not a survey).

### Capability-gated features

- **Galileo HAS / PPP** -- no dedicated property. When `SignalsEnabled`
  includes `SigGALE6` (Septentrio `GALE6BC`) **and** `grc` reports
  `PPPGalileoHAS-SIS`, append `setPVTMode, , +PPP` after the signal
  requests (composing into the `RoverMode` bitmask). Otherwise a no-op
  (E6 still tracked if requested; nonexistence shown, not announced).
  Convergence/HAS status is a *decode* concern (`NavEpochMsg.Correction`
  = `CorrPPPHAS` once `Mode==PPP`), not configuration.
- **OSNMA** -- `sou` has `off`/`loose`/`strict` vs `NavMsgAuth`'s
  `None`/`OSNMA`. `None`->`off`; `OSNMA`->`loose` by default, or
  `strict` if `ConfigOptions.TimeAssist` supplies a trusted time (send
  `exeSetTime` first). The X5-`setNTPClient` vs G5-`exeSetTime`
  difference collapses into "send `exeSetTime` if we have an estimate."
  Gated on the `GalOSNMA` capability.
- **Dual PPS** -- `gpsprot.TimePulse` models one output; map all
  `TimePulse*` onto PPS1 (`setPPSParameters`) on both models. G5's PPS2
  (`setPPS2Parameters`) is not exposed -- a second-pulse property is a
  cross-backend gpsprot API change, out of scope (see Open decisions).

### Save / reset

- **Save** -- one granularity: `exeCopyConfigFile, Current, Boot`
  (`eccf`); both `SaveMinimal` and `SaveAll` map to it (single-group
  granularity is an allowed `SEMANTICS.md` point, not a limitation).
- `ResetReload` -> `exeCopyConfigFile, Boot, Current`; `ResetCold` ->
  `exeResetReceiver, Hard, +PVTData+SatData`; `ResetFactory` ->
  `exeResetReceiver, Hard, all`.

### `ConfigSupportFlags` and capability gating

`ConfigSupportFull &^ (Survey | SurveyAcc | SurveyMsg)` (the `unc`
pattern, so future flags are picked up automatically); everything else
has a mapping above. Every G5-vs-X5 difference reduces to a
capability/port/signal check against the `grc` reply -- **never** a
`ReceiverInfo.Hardware` model-string test.

## Open decisions

- **PPS2 has no gpsprot property** (cross-backend API question, not
  resolved here).
- **Vendor-specific G5 knobs with no gpsprot analogue**
  (`setSignalAuthentication`, `setHoldoverTrigger`, per-signal
  `setCalibSignalDelay`) are out of scope -- each would need a new
  device-independent property.
- **Default OSNMA level without `TimeAssist`** is `loose`; confirm
  before landing (the guide frames `strict` as the more complete
  behavior).

## Testing

Follow the `implement-configprotocol` skill's verification ladder:
offline tests through the real `PacketProcessor` -> `ConfigProtocol` ->
`ConfigDirector` path (encoding real quirks, not assumed behaviour);
committed replay traces once hardware exists; `gpshwtest`
characterization against the mosaic-G5 per `SEMANTICS.md` (receiver
limitations recorded as data, not failures); and observe-don't-enable
for anything message-related (enabling PPP is not evidence of a
converged HAS solution). Hardware has not arrived; stage 0 must
re-verify the guide-derived facts above -- notably the `setCOMSettings`
baud-switch handshake and the state-line counts of multi-value `get*`
commands.
