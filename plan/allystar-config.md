# Allystar configurator ([#344](https://github.com/jclark/satpulse/issues/344))

Implement `gpsprot.ConfigProtocol` and `gpsprot.Configurator` for Allystar
receivers, enabling `satpulsetool gps` and `satpulsed` to configure them
(probe, enable messages, time pulse, time mode, signal selection, speed, NVM).

Built with the implement-configprotocol skill, which owns the process and the
gpshwtest/SEMANTICS.md contract. The CASIC configurator (`casic-config`
branch, `gps/internal/casic/cascfg*.go`) is the design model: the Allystar
binary protocol has the same shape (class/id-addressed messages, ACK/NAK
naming class+id, empty-payload polls), and hardware probing confirms the same
correlation rules apply. This plan records the Allystar-specific inputs.

## Test hardware

Three receivers spanning the capability range (see CLAUDE.local.md for
devices and speeds; all firmware 3.018):

| Model | Hardware | Notes |
|-------|----------|-------|
| TAU1201 | HD8040D | dual-band timing; no RTCM output (NAKs 0xF8 targets) |
| TAU1302 | HD9310 | between the other two in age and capability; RTCM output |
| TAU951M-P200 | HD9510 | RTK; RTCM MSM output (MSM4+MSM7 verified); 12-deep request queue |

Capability flags (`ConfigSupport`) must come from per-receiver observation,
never from model-name extrapolation.

## Protocol answers (hardware, 2026-07-03)

Resolved on all three receivers with back-to-back single-write bursts:

- **Correlation**: ACK/NAK names class+id only. A poll is answered by the
  message itself (echoing its id); polls get NO ack. Responses can arrive
  OUT OF REQUEST ORDER across distinct ids (observed on both TAU1201 and
  TAU951M with CFG-PRT/MON-VER/CFG-ELEV mixes). Therefore: correlate
  strictly per class+id, at most one live request per class+id (the CASIC
  `promote()` rule), and never assume cross-id response ordering. For
  CFG-MSG, the data response echoes the target group+id, but the ACK/NAK
  names only 0x06 0x01, so CFG-MSG requests serialize like any same-id
  requests.
- **Pipelining**: distinct-id and same-id bursts all answered on all three
  units, typically within 10 ms at 115200. The TAU951M silently drops
  requests beyond ~12 outstanding (16-burst: exactly 12 answered); the
  others handled 16. Keep the outstanding set small; per-id serialization
  plus phase gating achieves this naturally.
- **NAK information**: an unsupported CFG id is SILENT to polls (CFG-GEOFENCE
  on all three: no NAK, no data). Id-level discovery therefore uses optional
  polls where timeout-after-retries means absence. CFG-MSG with an unknown
  or unsupported target NAKs both poll and set forms (clean per-message
  discovery; TAU1201 NAKs RTCM 0xF8 targets this way).
- **ACK guarantee**: an ACK is not validation - CFG-NMEAVER accepted and
  APPLIED value 0, which the documentation calls "not support". No
  ack-without-apply seen yet; stage 0 spot-checks each property class, and
  the on-ACK readback rule gets its narrow exception only where hardware
  earns it.
- **Poll semantics**: one poll, one answer. CFG-PRT polls are per-port
  addressed (portID in the poll payload) and each answers for its port.
- **Resets**: CFG-SIMPLERST modes 0-3 (reset/cold/warm/hot) are documented
  and observed to produce no ACK (no-response requests, succeed on send);
  modes 0x10/0x11/0x80 (stop/start/clear-TRK) do ACK.
- **Line budget**: default NMEA load (GGA/GSA/GSV/RMC/ZDA/TXT at 1 Hz) is
  comfortable at 115200. Out-of-box default baud not yet established.
- **CFG-MSG discovery map** (poll sweep, all three units, 2026-07-03; full
  tables in CONTEXT.md): discovery is fully NAK-driven (no silent
  targets). F0 ids beyond the doc: GST 0x08 (all units), DTM 0x0A
  (TAU1201/TAU951M), JAM 0x21 (TAU951M); TXT 0x20 rate 0 genuinely
  silences $GNTXT (verified). F8 sub-id = RTCM type - 1000 (proprietary
  type - 4000): TAU951M/TAU1302 expose 1005, eph 1019-1046, MSM4 and
  MSM7 for GPS/GLO/GAL/SBAS/QZS/BDS, and 4065-family ids; TAU1201 NAKs
  all F8. NAV-PVT (0xC1) NAKs on all three units; NAV-SVSTATE (0x32)
  exists on TAU1201/TAU1302 only. NAV-TIME/TIMEUTC/CLOCK/CLOCK2/SVINFO/
  SVIN/AUTO all emit at 1 Hz when enabled (verified on TAU1201).
  NAV-SVINFO is one row per SATELLITE (dual-band sats appear once), so
  per-signal satellite info has no carrier. TAU1302 as-found emits MSM7
  + ephemerides (rate 1) - that is its baseline, not leftover state.

## Already implemented (master)

- `gps/lib/asbin`: framing/checksum, ACK-ACK/ACK-NAK, MON-VER, CFG structs
  (CfgCfg, CfgNavSat, CfgSurvey, CfgFixedECEF, CfgPrt, CfgMsg, CfgPps,
  CfgSimpleRst, CfgElev, CfgNmeaVer), NAV structs (POSECEF, POSLLH, DOP,
  TIME, VELECEF, VELNED, CLOCK, SVIN, TIMEUTC, SVINFO, AUTO)
- `gps/internal/as`: packet processor converting NAV-POSECEF/POSLLH/
  VELECEF/VELNED/TIME/TIMEUTC/SVINFO/SVIN/AUTO/DOP to gpsprot messages,
  with NavEpoch tracking
- `configs/gpsmsg/allystar/allystar.toml`: hardware-verified tags for every
  CFG message above plus RTCM enables, speed changes, resets - the stage-0
  instrument and recovery tool
- Registration plumbing: `as.PacketFormat` and `as.NewPacketProcessor` are
  registered in `gps/gpsreg/reg.go`; `VendorAllystar` exists but
  `CreateConfigProtocols` does not yet return an Allystar ConfigProtocol

NAV-PVT (0x01 0xC1) and NAV-SVSTATE (0x01 0x32) have registered ids but no
decode. Enabling them is in scope where useful; implementing their decode is
NOT (separate work on master, per the configuration-only ruling).

## Design

Derived from the protocol answers; same skeleton as CASIC, simplified where
Allystar is simpler (no firmware-family divergence observed so far - all
three units speak the same CFG dialect, differing only in which messages
exist).

- **Probe**: poll MON-VER (0x0A 0x04). State-neutral, repeatable; all units
  answer with firmware/hardware strings (also feeds `ReceiverInfo`). MON
  polls get no ACK, so probe consumption tracks pending MON-VER answers so a
  late probe response is never misattributed to the configurator's own
  MON-VER request (correlation key is shared).
- **One request type** (`asReq`) with behavior flags (nakOK, onAck, onNak,
  onData, noAck, optional, speedAfter), constructors funneling through one
  entry point. `optional` covers both NAK-means-absent (CFG-MSG targets)
  and silence-means-absent (id discovery polls).
- **Phased generation** gated on all-earlier-final: query -> set -> message
  enables -> speed -> NVM. Queries first (read-modify-write for structs
  where we set only some fields, e.g. CFG-PPS preserving GPIO); speed after
  everything whose ACKs it could garble; NVM last so a save persists
  fallback outcomes and the new baud rate.
- **Achieved values from ACKs**: `onAck` records the accepted struct into
  the readback cache; `ConfigProps()` reports from those caches. No verify
  re-polls unless stage 0 finds ack-without-apply for a specific message.
- **Speed change**: CFG-PRT set carrying `speedAfter`, followed by a
  solicited poll whose answer at the new rate confirms the change directly;
  `MaybeSpeedChangeSucceeded` as the heuristic secondary path. Never
  retried. Open stage-0 questions below (which port, ACK rate, timing).

## Stage 0: remaining design-shaping unknowns

Resolve on hardware with message-file tags and targeted experiments before
committing to the property mappings; record findings in CONTEXT.md and fold
conclusions back into this plan. Audit the protocol document for each
"absent" conclusion - "could not find it" is weaker than a documented audit.

**Documented-absence audit: DONE 2026-07-03** (full results in CONTEXT.md).
Absent from the protocol: time-of-pulse message (no TIM class; POSTIME
removed 2017), end-of-epoch marker, leap-second announcements (current
leapSec lives in NAV-TIME 0x01 0x05, the GNSS/TAI time carrier; NAV-TIMEUTC
has no leap field), RTCM base station id, active-port identification,
antenna cable delay (only candidate: CFG-PPS Offset S4 ns, semantics
undocumented), nav-rate configuration (no CFG-RATE), GST rate control (no
0xF0 sub-id), INF/proprietary-NMEA channels. NAV-SVINFO is per-satellite
only (flags/quality bits "Refer to manual"); NAV-SVSTATE is per-sat eph/alm
state, NOT per-signal - so `--sats-out sig` has no documented carrier.
Raw output is RXM-DUMPRAW (0x02 0x01), a SET with U1 enable - NOT a CFG-MSG
target - and its output format is undocumented. Doc 0xF8 ids: 1005=0x05,
MSM7 GPS/GLO/GAL/QZS/BDS = 0x4D/0x57/0x61/0x75/0x7F, eph 1019/1020/1042/
1044/1046 = 0x13/0x14/0x2A/0x2C/0x2D, 4065=0x41; MSM4/MSM5 ids "TBD" in doc
(hardware supplies MSM4 ids). CFG-CFG: action U4 0/1/2 = save/load/clear,
mask bit0 baud, bit1 NMEA rates, bit2 nav settings, 0xFFFFFFFF factory;
load semantics undocumented. SIMPLERST modes 0-3 documented no-ACK; what
each preserves undocumented. Default baud undocumented.

Remaining hardware unknowns:

- **CFG-PPS offset field**: reads 530 ns on TAU1201/TAU951M, 0 on TAU1302.
  Doc says only "defined by user", ns, default 0. Determine what it does
  (pulse offset? cable-delay analog?) and whether it maps to
  `AntennaCableDelay`.
- **Raw output**: RXM-DUMPRAW set (0x02 0x01 payload U1 0/1) per receiver -
  enable, observe whether anything is emitted and in what framing
  (acknowledge-but-never-emit is common; format undocumented).
- ~~SVINFO per-signal probe~~: DONE - per-satellite only (dual-band sats
  appear once); `sig` is absence.
- **RTCM output detail**: MSM7 + eph emission OBSERVED on TAU1302
  (as-found baseline). Still to observe: MSM4 and 1005 emission on both
  RTCM units, MSM7 on TAU951M. TAU1201 shows absence (all F8 NAK).
- **NAV emission on the other two units**: TAU1201 verified for
  TIME/TIMEUTC/CLOCK/CLOCK2/SVINFO/AUTO/SVSTATE; spot-check the same on
  TAU951M (no SVSTATE there) and TAU1302.
- **CFG-NAVSAT semantics**: set masks including signals a unit lacks - ACK
  and clamp, or refuse? Does readback echo the written mask or the
  effective one? Any coupled signals? Determines whether the signal set
  needs a post-set verify poll.
- **Survey/time-mode semantics**: how CFG-SURVEY, CFG-FIXEDLLA/ECEF interact
  (what does "mobile" look like - all zeros?); how to stop or restart an
  in-progress survey (writing zeros does NOT stop one on TAU1201);
  whether a fixed position while surveying is refused or queued; NAV-SVIN
  as progress signal. Determines `SurveyAgain` and the SetStatic default
  path.
- **Baud-change handshake**: at which rate the ACK arrives, when the port
  actually switches, whether setting the OTHER UART's rate disturbs the
  live link, whether we can identify which UART we are on (if not, --speed
  sets both, as the message file does today), safe resume timing.
- **NVM model**: CFG-CFG mask bit semantics per unit (baud/NMEA-rate/nav
  bits - what does each cover? is mask 0 vacuous?), reload behavior (ACK in
  place? how long? message file hints 3 s), what factory reset restores
  (default baud!), and a discriminating persistence oracle (USB
  unbind/rebind power cycle if soft resets preserve too much).
- **ACK-without-apply spot checks**: for each property class, set a value
  and independently poll once during stage 0 (not in the shipped
  configurator) to confirm ACK means applied.
- ~~NMEA coverage~~: DONE - full F0 map per unit in CONTEXT.md. GST has
  the undocumented id 0x08 on all units (the doc-absence was a doc gap);
  DTM 0x0A and JAM 0x21 exist on newer units; TXT rate 0 silences
  $GNTXT (verified). NAK-driven per-unit discovery works.
- **Default baud** out of the box (factory-reset consequences for recovery).

## Stage 1: missing asbin CFG structs

Add structs the configurator needs that asbin lacks (from stage 0 findings;
candidates: CFG-FIXEDLLA, CFG-DOP if needed for qual, MON-INFO if it carries
useful identity). Round-trip tests per struct, audited against the
registration list. Add corresponding get-*/set tags to the message file
with verification comments (message-file additions that are independently
useful belong on master, cherry-picked or committed there).

## Stage 2: ConfigProtocol, probe, NMEA output control

Files: `gps/internal/as/ascfgprot.go` (ConfigProtocol),
`gps/internal/as/ascfg.go` (Configurator core, asReq, phases),
`gps/internal/as/ascfgmsg.go` (message enables). Register in
`gps/gpsreg/reg.go` under `VendorAllystar` and the `VendorUnknown` list.

- Probe/ProbeOK per Design; ReceiverInfo from MON-VER (split SW/HW strings).
- `--nmea-out`, `--nmea`/`--binary` via CFG-MSG F0 targets: named sentences
  enabled at rate 1, unnamed group members disabled; TXT limitation shown
  as characterization, not error.

Functionality: `satpulsetool gps --nmea-out RMC,GGA` works on all three.

## Stage 2a: PVT and satellite message enabling

`--pvt-out` mapping to already-processed messages (confirm in stage 0):

```
pos          -> NAV-POSLLH   (ecef modifier -> NAV-POSECEF)
vel          -> NAV-VELNED   (ecef modifier -> NAV-VELECEF)
time         -> NAV-TIMEUTC  (tai modifier -> NAV-TIME, GNSS time)
qual         -> NAV-DOP + NAV-AUTO (fix state, sat counts)
survey       -> NAV-SVIN
leap         -> NAV-TIME (carries current leapSec + validity bit;
                announcements have no carrier - audited)
tp, epoch    -> absent (audited: no time-of-pulse message, no EOE)
```

`--sats-out sat` -> NAV-SVINFO; `sig` is absence (audited AND verified on
hardware: SVINFO is one row per satellite even for dual-band sats,
SVSTATE is per-sat eph/alm state).

Functionality: `satpulsetool gps --pvt-out pos,time --sats-out sat` works.

## Stage 2b: RTCM and raw output

`--rtcm-out MSM4|MSM7|ARP|auto` via CFG-MSG 0xF8 targets with NAK-driven
absence (TAU1201 reports nothing achieved, no error). Sub-ids follow
type-1000: MSM4 = 0x4A/0x54/0x5E/0x68/0x72/0x7C, MSM7 = 0x4D/0x57/0x61/
0x6B/0x75/0x7F (GPS/GLO/GAL/SBAS/QZS/BDS), ARP 1005 = 0x05. `--raw-out`
per stage-0 RXM-DUMPRAW findings (a set message 0x02 0x01, not a CFG-MSG
target). `--rtcm-base-id` absent (audited: no DF003 carrier).

Functionality: `satpulsetool gps --rtcm-out auto` works on TAU1302/TAU951M
and shows absence on TAU1201.

## Stage 3: NVM operations

CFG-CFG save (minimal via touched-section mask if stage 0 shows the mask is
honored, else all), `--save-all`, reload, `--reset` (reload + SIMPLERST cold
per the message file's reset recipe), `--factory-reset` (clear +
SIMPLERST), no-ACK handling for SIMPLERST modes 0-3.

Functionality: `--save --save-all --reset --reload --factory-reset` work.

## Stage 4: time pulse

CFG-PPS (15-byte Cynosure II/III form on all three units): Width ->
duty*period, Period -> period, PolarityRising -> polarity, OnlyWhenLocked ->
sync, GPIO preserved from the query-phase readback. AlignToGNSS/TimeGNSS
have no CFG-PPS carrier (audited: the message has no time-base or
GNSS-select fields) - both absent. AntennaCableDelay iff the offset field
proves to be that (stage 0).

Functionality: `satpulsetool gps --pps 0.1` works.

## Stage 5: time mode (survey / fixed position)

CFG-SURVEY + CFG-FIXEDECEF/LLA per stage-0 semantics: `--survey`
(+time/acc), `--fixed-pos-ecef/llh`, `--mobile`, SetStatic default path
preserving an existing fixed position and a running survey unless
SurveyAgain. Report achieved mode truthfully (survey params are not
readable back as a mode; the resulting fixed position is).

Functionality: `--survey --fixed-pos-ecef --mobile` work; `satpulsed`
default static mode triggers survey-in.

## Stage 6: signal selection

CFG-NAVSAT U4 per-signal mask <-> `gpsprot.SignalSet`: deduce the supported
set per receiver (stage 0), intersect the request, set, record achieved
from the ACK (plus post-set verify poll only if stage 0 showed clamping).

Functionality: `--gnss --band --signal` work.

## Stage 7: speed change and show-port

CFG-PRT set with `GetSpeedChangeAfter`, confirmation via solicited poll at
the new rate, `MaybeSpeedChangeSucceeded` heuristic secondary; combined
`--speed --save` persists the NEW rate (speed phase precedes NVM phase).
`--show-port` reports what stage 0 established is knowable.

Functionality: `satpulsetool gps --speed 460800` works reliably.

## Verification

- Offline fake-receiver tests (`gps/internal/as/ascfg_test.go`) through the
  real PacketProcessor -> ConfigProtocol -> ConfigDirector path. The fake
  mimics DISCOVERED behavior: cross-id response reordering, the TAU951M
  12-deep queue drop, silence for unknown ids, NAK for unknown CFG-MSG
  targets, ACK-and-apply of doc-invalid values, no-ACK resets, baud
  switches. Every hardware quirk found later gets encoded in the fake with
  a failing-first regression test.
- Round-trip tests for every new asbin struct.
- Replay tests: per-receiver stubs and one command file per option area in
  `internal/gpscmd/testdata/`, committed traces for all three receivers,
  `replay_as_test.go` asserting byte-identical outgoing packets.
- gpshwtest characterization to green, run-to-run identical baselines for
  all three receivers, committed on this branch with per-receiver notes in
  `gpshwtest/HW/`. Disruptive coverage (speed, NVM persistence) included,
  with recovery.
- NEWS.md entry covering the full configuration scope.

## Reference materials

- Protocol spec: `~/gps-protocol-docs/allystar/Allystar-2.3.6.md` (2019
  vintage; our 3.018 firmware has messages beyond it - NAV-TIMEUTC id 0x21,
  NAV-SVIN 0x31, NAV-VELECEF 0x11, NAV-VELNED 0x12, NAV-AUTO 0xC0, MSM4
  0xF8 ids - already captured in the message file; trust the message file's
  verification comments over the spec)
- CASIC configurator (design model): `casic-config` branch,
  `gps/internal/casic/cascfg*.go`, `plan/archive/casic-config.md`
- Message file: `configs/gpsmsg/allystar/allystar.toml`
- ConfigDirector contract: `gps/gpsprot/configprotocol.go`; semantics:
  `gpshwtest/SEMANTICS.md`
