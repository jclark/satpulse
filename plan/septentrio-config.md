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
`gpshwtest/SEMANTICS.md` contract, its rulings, and the
`ConfigProtocol`/`Configurator`/`ConfigDirector` machinery. This plan
records the Septentrio-specific inputs and the staged build order.

Stage 0 ran against a real mosaic-G5 (v1.1.0, 2026-07-05); every
protocol fact below marked "verified" was observed on that unit.
The remaining stage-0 item is the disruptive save/reset session
(stage 7 prerequisite).

## Prerequisites

Core landed and hardware-verified (`plan/septentrio-core.md`):
`gps/lib/sbfbin` decode, the `gps/internal/septentrio` conversion layer,
and the Tier 2 message-file layer with the `"septentrio"` response
analyzer (`plan/archive/septentrio-msgfile.md`). The command knowledge
(mnemonics, argument shapes, the four `$R*` reply shapes) already exists
there and is reused, not re-derived.

Reply framing is designed in `plan/archive/septentrio-msgfile.md` and
implemented (`gps/internal/septentrio/rpacket.go`): a whole reply
(echo + state lines + prompt) frames as ONE `TagReply` packet; `lst*`
replies frame unit by unit (`$-- BLOCK n / m` sections ending at the
prompt). What is still missing is delivery: `TagReply` has a
`PacketFormat` but no `PacketProcessor`, so framed replies never reach
a `NativeMsgHandler` (stage 1).

## Protocol answers (hardware, 2026-07-05)

- **Correlation**: the CLI has no queueing; one command, one framed
  reply, completed by the prompt (`USB1>` on our connection). The
  `Configurator` is single-flight: one outstanding request at a time
  (model `ubx`'s `configRequest`/`requestOps` scaled to single-flight,
  not `unc`'s phase enum). Correlation is by exact command-echo:
  `$R:`/`$R;` reproduce the command verbatim; `$R?` carries real error
  text (verified shape: `setSignalTracking: Argument 'Signal' is
  invalid!`) that becomes `ConfigRequest.GetError()` directly.
  Single-flight is itself the correlation key for anchor-less state
  lines. Replies arrive in 1-3 ms on USB; no line saturation at the
  default load.
- **Probe**: `getReceiverCapabilities` (`grc`), no arguments. Verified
  on a fresh USB connection with no escape prefix: repeatable,
  byte-identical, state-neutral. One state line carrying the
  supported-signal list (29 on the G5), the port list, the capability
  list (`GalOSNMA` and `PPPGalileoHAS-SIS` present on this unit), and
  the max measurement/PVT rates (50, 50). The ack is a family-wide
  probe (CLI is byte-identical X5/G5) and the single source for all
  capability gating. Late-probe bookkeeping: the probe's echo is
  `getReceiverCapabilities` exactly; the configurator's own grc
  request shares that correlation key, so pending-probe accounting is
  needed (director-contract.md) unless the configurator reuses the
  probe's parsed answer instead of re-asking - prefer reuse.
- **Command escape**: a wedged connection (data-input mode) accepts
  commands again after ten `S` characters + Enter, answered by a BARE
  prompt (not a framed `$R` reply). The probe does NOT use it (probes
  are state-neutral; grc works on fresh connections). `Configure()`
  sends it once, first, as a no-reply request (succeed on send,
  MaybeComplete-style absorption of the bare prompt), matching
  verified message-file practice.
- **Omitted arguments keep their current value** (verified throughout,
  e.g. `setPPSParameters, , , , Galileo` changes only TimeScale). This
  is the read-modify-write primitive: most sets need no prior query.
- **`+`/`-` element ops**: `+X` adds one element (verified). `-X` is
  NOT universal: `setSignalTracking, -GALE5` is REFUSED even though
  `+GALE5` was accepted; removal from the tracking list requires
  writing the explicit full list. NMEA sentence lists do support `-`
  (verified in the msgfile work).
- **Refusal leaves configuration unchanged** (verified:
  out-of-range `setCalibCommonDelay, 10000.5` refused, prior value
  intact). The receiver does not clamp; range clamping is the
  configurator's job.
- **grc does not bound set enums**: `setSignalTracking, +GALE5` (E5
  AltBOC, absent from grc's supported list) is accepted and applied.
  Do not blindly intersect requests with grc's list; intersect only
  what the property model requires (unsupported gpsprot signals are
  shown as absence via the ReceiverInfo signal set).
- **exeSBFOnce never emits to the issuing connection** (verified
  three blocks, raw capture; instant to another connection). The
  receiver acks and stores the request but nothing arrives, so the
  one-shot fetch of `ReceiverSetup` (5902) is unavailable on the
  connection we configure over. The block's own OnChange schedule
  delivers it instead: per the SBF reference it is "generated every
  60 seconds and each time a user-command is entered to change one
  or more values in the block", so enabled on a stream it arrives
  within a minute (there is no emit-on-enable; a 4 s stage-0 watch
  missed the 60 s period). The one-shot DOES deliver same-connection
  when the block is enabled on a stream bound to that connection -
  but enabling it is a configuration change, so the active identity
  fetch uses the Identification file instead (see above).
- **Reads**: one `get` -> one reply; multi-value replies use one
  state line per unit: `getElevationMask, all` -> 2 lines (Tracking,
  PVT); no-arg `getSBFOutput` -> 14 lines (Stream1-10 + Res1-4);
  no-arg `getNMEAOutput` -> 10 lines. `getSatelliteTracking` returns
  ONE line with a per-satellite list (~230 entries), so constellation
  state is derived, not read directly. `getSignalUsage` returns one
  line with TWO lists (PVT, NavData).
- **Defaults on this unit** (Boot config "Equal to RxDefault!"):
  RoverMode `StandAlone+DGNSS+RTKFixed`; tracking list is a strict
  20-of-29 subset of capability; PVTLevel=loose/MeasLevel=off OSNMA;
  elevation masks 5/5; PPS sec1/Low2High/0.00/GPS/60/5.0 per doc
  (this unit as-found runs MaxHoldover 1 / width 100 in RAM only).

## Septentrio-specific design

### Probe and identification

- `ProbePacket()` sends `grc`; `ProbeOK()` parses the reply
  (signals, ports, capabilities) and caches it.
- `ReceiverInfo()`: supported GNSS/signals from grc's signal list via
  the coarse signal table below; `Vendor = "Septentrio"`; `Hardware`
  ("mosaic-G5 P3") and `Firmware` ("1.1.0") from a ReceiverSetup SBF
  block when one arrives (a user configuration that emits it gets
  identity for free), else fetched with `lstInternalFile,
  Identification` - the one ASCII carrier of the firmware version -
  parsed as XML from the lst block units. Owner ruling (revised
  2026-07-06 after the one-shot's limits surfaced): identity must
  not change the receiver configuration, even in RAM, and must not
  delay `--show-receiver`; the lst fetch satisfies both (millisecond
  reply, no side effects). The ReceiverSetup one-shot alternative
  was rejected: `exeSBFOnce` delivers to its own connection ONLY
  when the block is enabled on a stream bound to it, and enabling it
  is a configuration change with observable periodic emissions.

### Coarse signal table (gpsprot.Signal <-> Septentrio name)

```
SigGPSL1CA  GPSL1CA     SigGALE1    GALE1BC     SigQZSSL1CA QZSL1CA
SigGPSL1C   GPSL1C      SigGALE5a   GALE5a      SigQZSSL1C  QZSL1C
SigGPSL2P   GPSL2PY     SigGALE5b   GALE5b      SigQZSSL1S  QZSL1S
SigGPSL2C   GPSL2C      SigGALE6    GALE6BC     SigQZSSL2C  QZSL2C
SigGPSL5    GPSL5       SigBDSB1I   BDSB1I      SigQZSSL5   QZSL5
SigGLOL1    GLOL1CA     SigBDSB1C   BDSB1C      SigQZSSL6   QZSL6
SigGLOL2    GLOL2CA     SigBDSB2I   BDSB2I      SigNAVICL5  NAVICL5
SigGLOL3    GLOL3       SigBDSB2b   BDSB2b      SigSBASL1CA GEOL1
                        SigBDSB2a   BDSB2a      SigSBASL5   GEOL5
                        SigBDSB3I   BDSB3I
```

No Septentrio carrier (absence): SigGLOL1OC, SigGLOL2OC, SigNAVICL1,
SigQZSSL5S. No gpsprot analogue (preserved, never touched): GALE5
(AltBOC), GLOL2P, QZSL1CB. This is config's own coarse table, distinct
from the conversion layer's signal-number -> `SignalID` table (two
tables, not one).

### Configuration-state representation

The key design question (owner directive): how to represent the
receiver's internal configuration state. Chosen: **the reply state
lines, one typed entry per configuration item, values kept in the
receiver's own vocabulary** (`nativeProps` in `scfgvals.go`: interval
enums, polarity names, signal-name lists verbatim; numerics parsed).
Get replies and set acks update it through the SAME parser, because
the CLI is symmetric: a get reply's state line carries exactly the
set command's argument vector (`PPSParameters, sec1, Low2High, ...`
answers both `getPPSParameters` and `setPPSParameters, ...`), so
"the messages the receiver returns" and "the commands that produce
the state" coincide. Device-independent properties are converted at
this boundary, in both directions, on demand.

Options considered:

- **ubx-new key/value analogue**: a flat map item-name -> argument
  vector, with generic get/set name derivation. Rejected: Septentrio
  has no uniform key space - each item's fields need bespoke typed
  conversion anyway (enums, floats, lists, slot references), so the
  generic map only removes field names and type safety without
  removing any conversion code.
- **unc-style command strings with diff generation**: unc stores each
  item's state as the command string that produces it and generates
  updates by diffing target against current native state - necessary
  there because Unicore read-modify-write requires regenerating the
  whole command. Rejected for Septentrio: omitted arguments keep
  their current value (verified), so the RECEIVER does the merge and
  sets are built sparsely from the target properties alone; full
  command reconstruction from stored state is never needed. The one
  genuine read-modify-write (snt's explicit signal list - no `-`
  removal) operates on a list value, not on command text.
- **lstConfigFile Current as the state** (the receiver's own config
  dump, which is literally a list of set commands): one query would
  fetch everything. Rejected: the dump is a DIFF from RxDefault, so
  default values are invisible without a fragile per-firmware
  default table; it arrives in block units framed for humans; and
  the per-item get commands are verified and trivially parseable.

The chosen shape is the ubx-old philosophy (query-response messages
as the state) adapted to an ASCII CLI, and matches unc in WHAT is
stored (per-item native state, receiver vocabulary) while dropping
unc's command-diff machinery, which Septentrio's omitted-argument
semantics make unnecessary.

### Property mapping

| `PropID` | Septentrio command(s) | Notes |
|---|---|---|
| `SignalsEnabled` | `setSatelliteTracking` (`sst`) + `setSignalTracking` (`snt`) + `setSignalUsage` (`snu`) | See below. |
| `TimeGNSS` | `setPPSParameters` `TimeScale` arg | GPS/Galileo/BeiDou/GLONASS all verified; `UTC`/`RxClock` have no device-independent analogue. |
| `TimePulseWidth`/`Period`/`AlignToGNSS`/`OnlyWhenLocked`/`PolarityRising` | `setPPSParameters` (`Interval`, `PulseWidth`, `Polarity`, `MaxHoldover`) | PPS1 only. OnlyWhenLocked ~ MaxHoldover 1; !OnlyWhenLocked = 0 (never time out). Interval is an enum (off/msec10..sec10/...); non-representable periods are clamped to the nearest supported value and reported truthfully. |
| `Mode` (static/rover, fixed pos) | `setPVTMode` (`spm`) + `setStaticPosGeodetic`/`setStaticPosCartesian` | `Static==false` -> `setPVTMode, Rover` (RoverMode arg OMITTED - keeps the receiver's current rover-mode list; verified default includes DGNSS). `Static==true, PosTypeNone` -> `setPVTMode, Static, , auto`. `Static==true` + LLH/ECEF -> write Geodetic1/Cartesian1, then `setPVTMode, Static, , Geodetic1`/`Cartesian1`. |
| `AntennaCableDelay` | `setCalibCommonDelay` (`scco`) | ns; receiver REFUSES out-of-range (verified), so clamp to -10000..10000 client-side before the wire. |
| `NavMsgAuth` | `setGalOSNMAUsage` (`sou`) | See "OSNMA". |
| `RTCMBaseID` | `setRTCMv3Formatting` `ReferenceID` | 0-4095, clamp client-side (first reply field, verified readback). |
| `MinElevation` | `setElevationMask` (`sem`), `Engine = PVT` | Solution mask only; the tracking mask is out-of-group, untouched (ubx precedent; readback shape verified). |
| `BaudRate` | none on USB | Owner ruling: ubx USB model - reads back 0 ("not applicable"), sets are no-ops on USB connections. `setCOMSettings` affects only physical COM ports we cannot reach. |
| `Port` (read-only) | none | From the connection descriptor in the reply prompt (`USB1>` verified). |

`Survey` and its `ConfigSupport*` flags are unset: Septentrio has no
parameterized/terminating/observable survey operation (`setPVTMode,
Static, , auto` is an auto-computed reference, surfaced as
`Mode.Static`+`PosTypeNone`, not a survey).

**SignalsEnabled realization.** Three commands realize one property:
`sst` gates by constellation, `snt` by signal (tracking), `snu` by
signal (usage: PVT + NavData). Because `snt` has no `-` removal
(verified), a set is read-modify-write: query `gnt`/`gnu` in the query
phase, then write explicit full lists = (requested signals mapped
through the table) UNION (unmapped signals currently present - GALE5,
GLOL2P, QZSL1CB stay as found). `sst` gets the constellation list
derived from the target signal set (`all` when every constellation has
signals). Achieved value: the set's own readback state line names the
achieved list (verified immediate and exact), reported through the
table in reverse. Requested signals with no Septentrio carrier are
absence; requested signals outside grc's supported list are still sent
if they map (grc does not bound the enum) and the reply tells the
truth.

### Capability-gated features

- **Galileo HAS / PPP** - no dedicated property. When `SignalsEnabled`
  includes `SigGALE6` (GALE6BC) **and** grc reports
  `PPPGalileoHAS-SIS`, append `setPVTMode, , +PPP` after the signal
  requests (verified: composes into RoverMode without disturbing the
  rest). Otherwise a no-op (E6 still tracked if requested).
- **OSNMA** - `sou` takes (PVTLevel off/loose/strict, MTRoot,
  MeasLevel off/loose); factory default is PVTLevel=loose,
  MeasLevel=off (verified readback `GalOSNMAUsage, loose, "", off`).
  Mapping: report `OSNMA` iff PVTLevel != off OR MeasLevel != off
  (truthful: loose PVT-level authentication is active by default on
  this receiver); `None` -> `sou, off, , off`; `OSNMA` ->
  `sou, loose` (PVTLevel only; MTRoot and MeasLevel left as found -
  MTRoot is simulation-only and must stay blank in live operation).
  Loose is the owner-ruled interim; strict (requires `exeSetTime`
  trusted time) slots in once the osnma branch's
  TrustedTimePacketBuilder machinery lands - keep the sou request
  shape ready for a `strict` level but do not build a parallel
  TimeAssist path. Gated on the `GalOSNMA` capability (present on
  this unit).
- **Dual PPS** - `gpsprot.TimePulse` models one output; map all
  `TimePulse*` onto PPS1 (`setPPSParameters`) on both models. G5's
  PPS2 (`setPPS2Parameters`) is not exposed (cross-backend gpsprot
  API change, out of scope).

### Message output control and stream ownership

The configurator OWNS exactly two streams: **SBF Stream1** and **NMEA
Stream1** (the same streams the verified message files and the
shipped-daemon configuration use). All other streams (SBF Stream2-10 +
Res1-4, NMEA Stream2-10) are out-of-group: never read-modified, never
disabled (the Allystar lesson). A complete output request replaces the
owned stream's message list, port (our own connection, from the
prompt) and interval in one `setSBFOutput`/`setNMEAOutput` command;
"off" sets the owned stream to `none, none, off`.

- `--nmea-out`: NMEA Stream1 sentence list (GGA/GLL/GSA/GSV/RMC/ZDA
  etc. per the message file's verified tags).
- `--pvt-out`/`--sats-out`/`--raw-out`: SBF Stream1 block list, chosen
  to match what the septentrio-msg branch decodes (PVTGeodetic,
  EndOfPVT, xPPSOffset, ChannelStatus, MeasEpoch, ReceiverSetup, ...;
  exact per-option lists fixed at stage 5 against the conversion
  layer's registration list). FlexRate-exempt blocks (xPPSOffset,
  ReceiverSetup, ...) ride the same list and emit OnChange (verified).
- `--rtcm-out`/`--rtcm-base-id`: `setRTCMv3Output` message selection
  (MSM4/MSM7/Nav expansion verified in the msgfile work) +
  `setRTCMv3Formatting`. Actual emission additionally needs base mode
  + reference position - observation, not enablement, is the
  evidence.

### Save / reset

(All hardware-verified in the disruptive session, 2026-07-05.)

- **Save** - one granularity: `exeCopyConfigFile, Current, Boot`
  (`eccf`); both `SaveMinimal` and `SaveAll` map to it. Verified:
  lcf Boot shows the saved config; a later reload restores the
  SAVED config, not defaults. `eccf, RxDefault, Boot` restores a
  factory Boot (RxDefault is a valid source).
- `ResetReload` -> `exeCopyConfigFile, Boot, Current` (verified
  in-place, no restart).
- `ResetCold` -> `exeResetReceiver, Hard, PVTData+SatData`;
  `ResetFactory` -> `exeResetReceiver, Hard, all` (the Config erase
  resets Current AND Boot to defaults, permanent settings kept).
  Both reboot, but the STOP>-terminated reply frames and ACKS
  BEFORE the connection drops, so reset requests complete normally;
  USB re-enumerates in ~1.5 s with stable ttyACM numbering, and the
  CLI answers again after ~7 s (Soft) / ~25 s (Hard). The
  standalone `factoryReset` command is NOT used (its reset-at-next-
  power-cycle mark could fire long after the session).

### `ConfigSupportFlags`

`ConfigSupportFull &^ (Survey | SurveyAcc | SurveyMsg)` (the `unc`
pattern, so future flags are picked up automatically). Every G5-vs-X5
difference reduces to a capability/port/signal check against the grc
reply - **never** a `ReceiverInfo.Hardware` model-string test.

## Stages

Stage 0 (design unknowns) is DONE except the disruptive session; its
findings are in this revision. Remaining stages, each ending with
green `make test` and committed work:

### Stage 1: reply delivery

`gps/internal/septentrio/scfgproc.go`: a PacketProcessor for
`TagReply` packets that parses the framed reply (ack kind, echo,
state lines, prompt; `lst` block units) into a typed reply value and
forwards it via `NativeMsg`. Registered in
`gpsreg.CreatePacketProcessors`. Offline tests from the captured
verbatim replies (grc, gets, `$R?` refusals, lif/lcf block units).

### Stage 2: ConfigProtocol + probe + ReceiverInfo

`scfgprot.go`: `septentrio.NewConfigProtocol()` - grc probe,
ProbeOK parsing (signals/ports/capabilities), the coarse signal
table, Hardware/Firmware capture from ReceiverSetup native
messages. Registration in `gpsreg.CreateConfigProtocols`
(VendorSeptentrio + the VendorUnknown probe list) lands with stage
3, when Configure() is real. Probe verified against the real G5.

### Stage 3: Configurator core

`scfg.go`: single-flight request engine (one request type with
behavior flags; constructors funneling through one entry point),
exact-echo correlation, prompt-is-completion, `$R?` text ->
`GetError()`, the leading escape request, phase gating
(escape -> queries -> sets -> output streams -> NVM), pending-probe
reuse. Offline fake-receiver tests through the real
PacketProcessor -> ConfigProtocol -> ConfigDirector path, the fake
mimicking VERIFIED behavior (exact echo, 1-3 ms replies, refusal
leaves state, no `-` removal on snt, omitted-args-keep-current).

### Stage 4: properties

Property mappings per the table: SignalsEnabled (read-modify-write,
three commands), TimePulse* + TimeGNSS, MinElevation (PVT engine),
AntennaCableDelay (client clamp), Mode + static position, NavMsgAuth,
RTCMBaseID, BaudRate/Port per ruling. PPP composition. Fake tests per
property including refusal and absence paths.

### Stage 5: message output control

Owned-stream realization of --nmea-out/--pvt-out/--sats-out/--raw-out/
--rtcm-out/--rtcm-base-id; per-option block lists fixed against the
conversion layer's actual decode set. Hardware observation of each
enabled output (emission, not acks).

### Stage 6: save / resets

The disruptive stage-0 session first (STOP> timing, re-enumeration,
recovery procedure, Boot restore, factory-wipe survival of
setCalibCommonDelay); then --save/--reload/--reset/--factory-reset
per the verified mappings, with no-response handling.

### Stage 7: verification ladder

Replay traces (internal/gpscmd/testdata/septentrio/, one scenario per
option area, replay_septentrio_test.go, byte-exact outgoing packets);
gpshwtest characterization on an integration-branch build to a clean
run-to-run identical baseline, committed here with HW/mosaic-g5.md;
NEWS entry.

## Open decisions

- **PPS2 has no gpsprot property** (cross-backend API question, not
  resolved here).
- **Vendor-specific G5 knobs with no gpsprot analogue**
  (`setSignalAuthentication`, `setHoldoverTrigger`, per-signal
  `setCalibSignalDelay`) are out of scope - each would need a new
  device-independent property.
- **NavMsgAuth on a default receiver reads back OSNMA** (PVTLevel
  loose is the factory default). This is truthful readback of real
  behavior; flagged for owner visibility, not blocking.

## Reference materials

- Protocol docs: `~/gps-protocol-docs/septentrio/mosaic-G5-v1.1.0.md`
  (authority for the test unit), `mosaic-X5-v4.15.1.md`.
- Verified message files: `configs/gpsmsg/septentrio/mosaic.toml`,
  `mosaic-g5.toml` (per-entry "Verified" notes are authoritative).
- Reply framing: `plan/archive/septentrio-msgfile.md`,
  `gps/internal/septentrio/rpacket.go`.
- ConfigDirector contract: `gps/gpsprot/configprotocol.go`; semantics:
  `gpshwtest/SEMANTICS.md`; reference configurators: `ubx`
  (request/ops model), `unc` (sequencing), casic/allystar branches
  (case law, via `git show`).
