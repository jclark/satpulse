# Quectel LG290P high-level configuration (#350)

High-level (device-independent) configuration - gpsprot
ConfigProtocol/Configurator - for the Quectel LG290P, following ubx
(#45), unc (#139), CASIC (#229), Septentrio (#341), and Allystar
(#344). Governing semantics: gpshwtest/SEMANTICS.md.

Target hardware: LG290P at /dev/ttyUSB0, 460800 baud, firmware
LG290P03AANR02A01S (the newest documented). The protocol reference is
~/gps-protocol-docs/quectel/lg290p_1.3.md (spec v1.3, covering the
LG29xP/LGx80P family); this plan targets the LG290P and keeps the
family divergences (LG580P/LG680P extras) out of scope until such a
unit exists.

## Already implemented

- gps/lib/qtmmsg - PQTM periodic message parsing (13 types including
  PVT, NAV, VEL, EPE, DOP, SVINStatus, EOE) and request/response
  classification (ClassifyRequest/ClassifyResponse with error-code
  mapping).
- gps/internal/quectel - NMEA ext-handler converting
  PVT/NAV/PPPNAV/VEL/EPE/SVIN/DOP/EOE to gpsprot messages.
- configs/gpsmsg/quectel/lg290p.toml - broad, partially
  hardware-verified message file; the stage-0 instrument.
- PQTM rides the shared NMEA packet format; no new framing is needed
  (unlike Unicore, which has its own ASCII format).

## Protocol properties that shape the design

Everything here is from the v1.3 spec; stage 0 verifies each on
hardware before the design is committed.

- Text sentences with symmetric set/get: `$PQTMCFGX,W,<full tuple>` ->
  `$PQTMCFGX,OK` (no value echo); `$PQTMCFGX,R[,<instance>]` ->
  `$PQTMCFGX,OK,<full tuple>`. Sets are whole-tuple writes: partial
  update requires read-modify-write.
- Failures carry reason codes: `$PQTMCFGX,ERROR,<code>` with 1 =
  invalid parameters, 2 = failed execution, 3 = unsupported command.
  Code 3 gives NAK-driven capability discovery without poll-first
  (unlike CASIC, whose NAKs are reasonless).
- The correlation key is the sentence name alone. A set ACK carries no
  instance fields, so for multi-instance commands (CFGUART by port,
  CFGMSGRATE by message name, CFGSAT by system/signal) only one
  request per sentence name may be outstanding. Distinct names may
  pipeline; the tolerated depth is a stage-0 measurement.
- Configuration is volatile: PQTMSAVEPAR persists to NVM, and boot
  loads NVM (spec note at lg290p_1.3.md:3183). Most sets are believed
  to apply live (message-file verification history); which do not is
  a stage-0 question per command.
- Resets answer nothing: PQTMSRR (system reset), PQTMCOLD/WARM/HOT
  (engine starts). The boot banner $PQTMVER announces restart.
  PQTMRESTOREPAR (factory defaults) ACKs and takes effect after
  restart.
- Three UARTs; CFGUART and CFGMSGRATE have current-port forms, and
  CFGUART,R with no index answers with the active port's Index, so
  the active port is identifiable (PropIDPort).
- PQTMLSTMSG (R01A06S+) dumps the current port's whole message-rate
  table, one OK line per enabled message, terminated by
  `$PQTMLSTMSG,OK,End` - a one-shot read of as-found output state.
- Feature availability is firmware-gated
  (lg290p-firmware-versions.md); the configurator discovers gaps via
  ERROR,3 and shows them as absence.

## Firmware-version gating (owner directive)

The configurator must work on firmware versions other than the
latest. The probe's PQTMVERNO answer carries the version string,
whose LG290P pattern is `LG290P03AANR<major>A<minor>S` (e.g.
LG290P03AANR01A03S = R01A03S); parse the R<major>A<minor>S suffix and
gate command availability on it per lg290p-firmware-versions.md
(PQTMCFGELETHD/SIGGRP/GEOSEP from R01A05S, PQTMLSTMSG/CLRMSG/
CFGNAVMODE from R01A06S, PQTMCFGPPS2/CFGPPP/CFGEVENT from R02A01S,
and so on). Gating avoids sending commands known to be absent; it
never overrides what the receiver actually answers. If the version
string is not an LG290P pattern (other LG29xP/LGx80P family modules,
or an unrecognized string), be optimistic: assume features are
present and rely on NAK recovery when they are not.

Config code must recover cleanly from NAKs everywhere (owner
directive; also the standing elegance bar): an ERROR response leaves
the assumed configuration unchanged, downgrades gated features to
absence where the code is 3 (unsupported command), and surfaces
genuine value refusals (codes 1/2) as errors only where the request
carried a user-requested value.

## Receiver configuration state representation

The architectural choice made before coding (owner directive from the
Septentrio bring-up). Precedent: the Septentrio configurator
(septentrio-config branch, gps/internal/septentrio/scfgvals.go) is
the existing text-protocol member - typed per-item reply state in the
receiver's own vocabulary, one parser per item, nil = never read back
= property absent. Its plan records why flat key/value state (no
uniform key space) and unc-style command-string diffing were rejected
there. Options considered for PQTM:

1. ubx-new style (config keys/values): no analogue - PQTM has no
   key/value space, only per-command tuples. Rejected.
2. unc style (store command strings, diff to regenerate): workable,
   but PQTM get responses and set commands differ in shape
   (`OK,<tuple>` vs `W,<tuple>`), so string diffing needs reshaping
   anyway; typed values fall out of parsing either one. Rejected.
3. Typed per-command state (the Septentrio shape): one struct per CFG
   command holding its tuple, keyed by sentence name (plus instance
   fields where the command is multi-instance). Populated by the
   query phase's OKData responses; a set is generated as a full tuple
   from (as-found state with target properties applied).

Choice: option 3. The set/get tuples are identical field-for-field,
so one struct per command serves both parsing the get response and
serializing the set, mirroring how qtmmsg already decodes periodic
messages with fieldenc. Read-modify-write is explicit and minimal:
query once in a query phase, mutate typed fields, write full tuples.

One deliberate divergence from Septentrio: there a set ACK echoes the
achieved state line and is parsed as readback; the PQTM set ACK
carries no values. Per the ACK-is-readback ruling the request's own
tuple is recorded as the assumed configuration on ACK - no
query-after-set verify pass. A post-set readback enters only if stage
0 shows the ACK's own semantics to be "applied the intersection"
(ACK-and-clamp), where reading back is reading what the ACK means.

## Request scheduling

Modeled on the shared skeleton of the branch configurators: one
request type with behavior flags (nakOK, optional, noReply, onData),
phases query -> set -> msg -> speed -> NVM, each gated on all earlier
requests being final; NVM last so a save persists fallback outcomes
and the new baud. promote() readies a request only when no earlier
live request shares its sentence name (single-flight per name;
distinct names pipeline up to the stage-0-measured depth -
Septentrio is fully single-flight, CASIC/Allystar pipeline per
class+id; PQTM's name-only correlation supports the latter if the
firmware tolerates it). Group message-enables are NAK-tolerant where
stage 0 shows per-unit target gaps (Allystar pattern); errors carry
reason codes, so ERROR,3 (unsupported) can be distinguished from
ERROR,1/2 when deciding nakOK vs genuine failure. The master
ConfigDirector contract suffices as-is (the Septentrio and Allystar
branches needed zero contract changes).

## Property mapping

Properties (readback-capable):

| gpsprot property | carrier |
|---|---|
| SignalsEnabled | PQTMCFGSIGNAL (per-signal masks; PQTMCFGCNST constellation gates - relationship is a stage-0 question) |
| TimeGNSS | none - absent (no PPS time-reference knob in the protocol; doc audit done) |
| TimePulseWidth | PQTMCFGPPS2 Duration (fallback PQTMCFGPPS on ERROR,3) |
| TimePulsePeriod | PQTMCFGPPS2 Period; with PPS fallback, constant 1000 ms |
| TimePulseAlignToGNSS | no knob - fixed behavior, report as constant (value from stage-0 doc audit/observation) |
| TimePulseOnlyWhenLocked | PQTMCFGPPS/PPS2 Mode (1 = always, 2 = fix only) |
| TimePulsePolarityRising | PQTMCFGPPS/PPS2 Polarity |
| Mode (static/survey/fixed) | PQTMCFGSVIN (0 disable / 1 survey / 2 fixed ECEF) + PQTMCFGRCVRMODE (rover/base) interplay - stage 0 |
| AntennaCableDelay | candidate: PQTMCFGPPS2 Userdelay (ns) - semantic check in stage 0; otherwise absent |
| NavMsgAuth | none - absent |
| RTCMBaseID | PQTMCFGRSID |
| MinElevation | PQTMCFGELETHD |
| BaudRate | PQTMCFGUART (current-port form) |
| Port (read-only) | PQTMCFGUART,R Index echo |

Message output (ConfigOptions, no readback):

- PVTMsg: PQTMCFGMSGRATE over PQTM messages, preferring processed
  ones - PQTMPVT/PQTMNAV (pos/vel/time; choose in stage 0 by content
  and firmware reach), PQTMEPE+PQTMDOP (quality), PQTMEOE (epoch),
  PQTMSVINSTATUS (survey). No time-pulse or TAI message exists -
  absence.
- NMEAMsg: PQTMCFGMSGRATE over RMC/GGA/GSV/GSA/VTG/GLL/GBS/GNS/GST/
  ZDA/HDT/THS.
- SatsMsg: GSV.
- RTCMMsg: PQTMCFGMSGRATE over RTCM3-1005/1006/MSM groups +
  PQTMCFGRTCM (MSM type 4/7, ephemeris mode). Receiver output beyond
  the modeled group (1033, ephemeris, 1230) gets the explicit Other
  member semantics from the Allystar ruling.
- RawMsg: no raw observation/navigation output exists in the protocol
  (RAW-PPPB2B/QZSSL6/HASE6 are PPP correction streams, RTCM MSM is
  the only observation carrier and belongs to RTCMMsg) - stage-0 doc
  audit to confirm, then ConfigSupportRaw absent.

Operations:

- Save: PQTMSAVEPAR (single scope - Minimal and All coincide).
- Reset: Reload -> PQTMSRR (boot reloads NVM); Cold -> PQTMCOLD;
  Factory -> PQTMRESTOREPAR + PQTMSRR.
- Survey: PQTMCFGSVIN mode 1 with CFG_CNT (MinDur) and 3D_AccLimit
  (AccLimit) - so ConfigSupportSurveyAcc present.
- Speed: PQTMCFGUART; ordered before the NVM phase (semantics ruling:
  a save persists the new rate).
- SetStatic: no stationary dynamics model exists (PQTMCFGNAVMODE
  offers normal/flight/mower/agriculture only); static semantics come
  from Mode via CFGSVIN. NAVMODE is left as found.

## Stage 0: hardware resolution of design-shaping unknowns

Instrument: `satpulsetool gps -m configs/gpsmsg/quectel/lg290p.toml
-t <tag>` (add tags as needed, with verification comments). Findings
go to CONTEXT.md and this plan. Questions, per
protocol-questions.md:

1. ANSWERED. Identity: PQTMVERNO ->
   `LG290P03AANR02A01S,2025/12/12,11:21:01` (data-only reply, no OK
   field); PQTMUNIQID -> `OK,8,0000183B31B3C252`.
2. ANSWERED. Pipelining: 8 distinct-name back-to-back requests
   (and 16 mixed sets+gets) all answered, strictly in request
   order; typical latency 1-3 ms, occasional ~100 ms stall behind
   the periodic output burst. Set ACKs are bare `NAME,OK`;
   multi-instance get responses echo their instance
   (`PQTMCFGMSGRATE,OK,GGA,1`), so gets are self-identifying.
   Same-name concurrent depth is moot: the shared NMEA send path
   serializes per name (single-flight), which is the planned
   configurator policy anyway.
3. ANSWERED. ACK guarantee: out-of-range sets (ELETHD 95.0,
   FIXRATE 0) answer ERROR,1 and leave config unchanged
   (readback-verified). No clamping observed. CFGSIGNAL ACKs and
   stores masks the spec calls impossible (GPS L1 off stored and
   read back as 06 while L1 tracking continued) - readback reflects
   stored intent, not effective state. Error codes seen: 1, 3.
4. PART-ANSWERED, ESCALATED. CFGSIGNAL and CFGCNST have NO live
   effect: with GLO masked off by either command, GLONASS stayed
   tracked and used in the fix >15 s; the setting took effect only
   after PQTMSAVEPAR + PQTMSRR (verified both directions). Owner
   ruling needed on how SignalsEnabled maps onto apply-at-restart
   semantics (see PROGRESS.md). Live-vs-restart still untested for:
   message rates, PPS, FIXRATE, ELETHD, RCVRMODE (message-file
   history says rates/PPS/fixed-pos apply live).
5. Baud-change handshake: at which rate does the CFGUART OK arrive;
   timing of the switch; confirm at the new rate with a solicited
   distinct-name query; does a saved rate re-apply after SRR? No
   root access is available, so USB unbind/rebind power cycling is
   NOT possible; where only a power cycle would discriminate
   persistence, record the finding as bounded ("not verified across
   power cycle") rather than claiming it.
6. PART-ANSWERED. SAVEPAR answers bare OK sent alone; pipelining
   SAVEPAR+SRR loses both responses (SRR reboots first) though the
   save completes - always wait for SAVEPAR's OK before SRR. SRR
   answers nothing; module back within ~15 s; boot loads NVM
   (saved state was in effect after reboot). Still open: SAVEPAR
   coverage, RESTOREPAR, COLD/WARM/HOT effects, boot banner
   details, restart-discards-unsaved-changes confirmation.
7. CFGPROT trap: if OutputProt's NMEA bit is cleared on the active
   port, do PQTM command responses still arrive, or does the
   configurator saw off the branch it sits on? Same question for
   InputProt (can commands still be sent?).
8. Survey/base interplay: does CFGSVIN work in rover mode
   (ERROR,2?); what does RCVRMODE=2 actually change (LSTMSG dump
   before/after; NMEA off? MSM4+1005 on? fix rate forced 1 Hz);
   PQTMSVINSTATUS on R02A01S (message file says broken - retest);
   how survey completion manifests (Valid flag transition; does the
   result transfer into CFGSVIN readback?).
9. LSTMSG as as-found read: exact dump format, End terminator,
   whether disabled messages appear; CLRMSG semantics.
10. PPS vs PPS2: do both address the same underlying state on
    R02A01S; Userdelay readback semantics (register only - no pin
    instrumentation planned; word findings accordingly).
11. PART-ANSWERED. Unknown sentence names answer `<name>,ERROR,3`
    echoing the unknown name - never silence - so ERROR,3 capability
    discovery is definitive and any PQTM-speaking firmware answers
    the probe with something. PQTMVERNO answered reliably under
    default load at 460800 in every run so far; state-neutrality
    assumed (read-only query), reliability under heavier load not
    yet stressed.
12. Doc-audit confirmations: no raw obs output, no TimeGNSS knob, no
    antenna cable delay other than PPS2 Userdelay, no leap-second/
    UTC-parameter query (PQTMPVT carries LeapS).

Exit criteria: every design assumption above marked verified or
corrected; the message file gains verified tags for every command the
configurator will send.

## Stage 1: CFG wire structs in gps/lib/qtmmsg

One struct per CFG command tuple (CfgUART, CfgPPS, CfgPPS2, CfgProt,
CfgMsgRate, CfgSVIN, CfgRcvrMode, CfgFixRate, CfgCnst, CfgSignal,
CfgRSID, CfgRTCM, CfgEleThd, ...), each serializing to the W form and
parsing from the OKData form, plus PQTMLSTMSG dump parsing and
VERNO/UNIQID responses. Round-trip tests audited against the
registration list. (Message names and fields use the spec's own
terminology.)

## Stage 2: ConfigProtocol, probe, and registration

- ProbePacket: $PQTMVERNO. ProbeOK: version data received. NativeMsg
  routing of PQTM responses into the configurator (the probe's
  response is a plain data message the configurator itself may also
  request - late-probe bookkeeping only if stage 0 shows collisions).
- Configurator skeleton with the typed-state model, query phase, and
  ReceiverInfo from VERNO (+UNIQID).
- gpsreg wiring: CreateConfigProtocols for VendorQuectel and the
  VendorUnknown probe list.

## Stages 3+ (feature by feature, each verified on hardware)

3. Message output: NMEAMsg/PVTMsg/SatsMsg via CFGMSGRATE with
   as-found state from LSTMSG.
4. RTCM output: CFGMSGRATE RTCM groups + CFGRTCM + RSID, with the
   Other-member group semantics.
5. Time pulse: PPS2 with PPS fallback.
6. Mode/survey/fixed position: CFGSVIN + RCVRMODE.
7. Signals: CFGSIGNAL/CFGCNST.
8. MinElevation, remaining properties.
9. Speed change, ordered before NVM.
10. NVM save/reset (SAVEPAR, SRR, COLD, RESTOREPAR).

Stage order after 2 may be resequenced by stage-0 findings; record
deviations here with rationale.

## Verification

Per the skill's ladder:

1. Offline fake-receiver tests through the real PacketProcessor ->
   ConfigProtocol -> ConfigDirector path (fake mimics discovered
   hardware behavior; failing test first for every hardware bug).
   Layout modeled on septentrio-config's scfg_test.go: a harness
   driving director.Actions() against a command->response map of
   verbatim captured sentences, with a reduced-capability variant to
   exercise ERROR,3 drop paths.
2. Replay tests: internal/gpscmd/testdata/quectel/lg290p-<area>.jsonl
   with a receiver stub + per-area command files; scenario list
   enumerated against the property model as a scope audit.
3. gpshwtest characterization to a clean committed baseline +
   gpshwtest/HW/lg290p.md, including a --disruptive run with restore
   verification (NVM content included).
4. Message delivery claims by observation only; watch windows sized
   to documented emission schedules.

## Open decisions

- PQTMPVT vs PQTMNAV as the primary PVT carrier (stage 0: content,
  firmware reach - PQTMNAV only exists from R01A05S - and what the
  processor already consumes best).
- Whether PPS2 Userdelay maps to a gpsprot delay property or stays
  unmapped (semantic check; related: #161).
