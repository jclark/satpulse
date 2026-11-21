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
  `$PQTMLSTMSG,OK,End` - a one-shot read of as-found output state
  (ultimately unused; see "Message-output readback").
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
| SignalsEnabled | PQTMCFGSIGNAL (per-signal masks) + PQTMCFGCNST (constellation gates); RESTART-ONLY - see "Restart-only settings" below |
| TimeGNSS | none - absent (no PPS time-reference knob in the protocol; doc audit done) |
| TimePulseWidth | PQTMCFGPPS2 Duration (fallback PQTMCFGPPS on ERROR,3) |
| TimePulsePeriod | PQTMCFGPPS2 Period; with PPS fallback, constant 1000 ms |
| TimePulseAlignToGNSS | no knob - fixed behavior, report as constant (value from stage-0 doc audit/observation) |
| TimePulseOnlyWhenLocked | PQTMCFGPPS/PPS2 Mode (1 = always, 2 = fix only) |
| TimePulsePolarityRising | PQTMCFGPPS/PPS2 Polarity |
| Mode (static/survey/fixed) | PQTMCFGSVIN (0 disable / 1 survey / 2 fixed ECEF), effective ONLY under base mode (PQTMCFGRCVRMODE=2) after save+restart - fixed mode saved+restarted in rover mode verifiably changes nothing. NMEA re-enables live in the base-mode table, so a mode change can keep the daemon's feed. RESTART-ONLY - see "Restart-only settings" below |
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
- RTCMMsg: PQTMCFGMSGRATE over RTCM3-1005/MSM groups + PQTMCFGRTCM
  (MSM type 4/7, ephemeris mode).
- RawMsg: no raw observation/navigation output exists in the protocol
  (RAW-PPPB2B/QZSSL6/HASE6 are PPP correction streams, RTCM MSM is
  the only observation carrier and belongs to RTCMMsg) - stage-0 doc
  audit to confirm, then ConfigSupportRaw absent.

Message-output readback (revised): the query phase originally read the
as-found message table with PQTMLSTMSG (R01A06S+) to skip redundant
rate writes and to turn off enabled-but-unmodeled standard NMEA/RTCM
sentences on a complete (non-Other) request. That was dropped: the
only real payoff was pruning obscure sentences the model does not
name, no other backend reads a message list, and the dump cost a
guaranteed 1 s idle-timeout stall per run. Message sets now go out
unconditionally for every modeled member the target names. RTCM is its
own output protocol: a request that leaves RTCM3 with no output
switches the RTCM3 OutputProt bit off (and a later request that wants
output switches it back on) with one live PQTMCFGPROT write, silencing
the RTCM3 messages the model does not name. NMEA is more delicate,
because this receiver's native PVT/satellite messages (PQTM*, and the
GSV sentence) are themselves NMEA and share the NMEA OutputProt bit -
see "Native-NMEA message model" below.

## Native-NMEA message model

Because PQTM* and GSV ride the shared NMEA OutputProt bit, NMEAMsg and
PVTMsg/SatsMsg are no longer independent (see plan/native-nmea.md for
the full model and the ConfigSupportNativeNMEA flag). The configurator
realizes the two-level model.

Step 1 - determine the level from the target:

- Message level: NMEAMsg names explicit sentences (NMEAMsgAny bits set).
- Semantic level: PVTMsg or SatsMsg is set (NMEAMsg none or unset).
- Mixed: explicit NMEA sentences together with PVTMsg/SatsMsg. This is
  contradictory; the configurator does nothing for the NMEA/PVT/Sats
  output (no error - the frontend raises that, gated on the flag) and
  leaves the NMEA OutputProt bit untouched. RTCM, its own protocol,
  still applies.

Step 2 - drive the NMEA OutputProt bit:

- Message level: on if any standard sentence is wanted; else clear
  (nmea-out none alone clears the wire).
- Semantic level: never clear - only turn on for a wanted native
  carrier (PQTM or GSV) - EXCEPT when NMEAMsg, PVTMsg and SatsMsg are
  all specified and all want nothing, which clears. So: any carrier
  wanted -> on; else all-three-off -> clear; else leave as found. This
  keeps a bare pvt-out off from silently killing standard NMEA it was
  never told about, and lets the daemon's NMEAMsg=None + PVTMsg/SatsMsg
  keep the wire up for its PVT messages (the bug this fixes).
- Neither level engaged (e.g. only rtcm-out): leave the NMEA bit as
  found.

"On if wanted" is idempotent (a no-op write when already on); the bit
is set before the per-message rate writes so they take effect, so the
CFGPROT write carries phaseMsg. With no CFGPROT readback the bit cannot
be toggled, so standard sentences are disabled individually instead
(existing fallback). When the bit is cleared, the per-message disables
for NMEA-wire messages are redundant and skipped.

This removes the former GSV union (SatsMsg overriding NMEAMsg for the
shared GSV sentence): that request is now the mixed case. The union
tests are replaced by mix-does-nothing tests; new tests cover the
daemon combo (carriers keep the wire up), the all-three-off clear, and
nmea-out none alone (message-level clear, unchanged).

Quectel declares ConfigSupportNativeNMEA once that flag lands
(plan/native-nmea.md); the backend behavior above needs no flag and
lands first.

## Restart-only settings (owner ruling, 2026-07-07)

CFGSIGNAL/CFGCNST (SignalsEnabled) and CFGSVIN/CFGRCVRMODE (Mode)
are ACKed and stored but take effect only after PQTMSAVEPAR plus a
restart. Daemon configuration is volatile by documented contract
(satpulse.toml(5): power-cycle undoes all changes; the daemon never
changes enabled GNSS per the timeGNSS note), so the configurator
must never write NVM or restart behind the user's back. Ruling:

- The configurator performs these sets ONLY when the target also
  carries Save plus a restart operation (satpulsetool --save with
  --reload, which maps to SRR here, or --reset; NOT --factory-reset,
  which restores NVM defaults and cannot combine with save). The
  stored sets ride the existing pipeline (sets -> SAVEPAR ->
  restart) and take effect at boot: persistence and outage are both
  explicitly user-requested.
- Without save+restart the configurator silently does nothing for
  these settings: no error, and no stored-but-ineffective writes
  left on the receiver. Readback of these properties reports the
  effective (as-found) state.
- Warning the user is the tools' job, not the configurator's: the
  ConfigSupport qualifier flags (signal/mode/rtcmMSM OnlyWithReset,
  landed on this branch by owner ruling) declare the behavior, and
  the tools derive warnings themselves from (a) what they asked for,
  (b) the declared support, and (c) the post-config state - the
  effective readback for signals and mode, the observed RTCM message
  types (ReceiverInfo.MsgTypes) for the MSM type. satpulsetool warns
  with the missing flags; satpulsed warns only for the static-mode
  case (its config is volatile by contract, so signals are not its
  business and MSM is only ever a preference there).
- Mode additionally requires RCVRMODE=2 (base mode), which swaps to
  the per-mode message table (NMEA off, RTCM on) and forces 1 Hz.
  Message-rate sets are live even in base mode, so satpulsed's own
  volatile message configuration restores its feed on the next run.
- Verify item: --reset maps to PQTMCOLD; SRR is verified to load
  NVM at boot, COLD is not yet - confirm saved sets apply after
  COLD before relying on it.

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
4. ANSWERED (escalation resolved - see "Restart-only settings").
   CFGSIGNAL and CFGCNST have NO live effect: with GLO masked off
   by either command, GLONASS stayed tracked and used in the fix
   >15 s; the setting took effect only after PQTMSAVEPAR + PQTMSRR
   (verified both directions). Effect-timing table so far: LIVE =
   CFGMSGRATE, CFGELETHD, CFGPROT (all observed immediate);
   RESTART-ONLY = CFGSIGNAL, CFGCNST (verified applied after
   SAVEPAR+SRR), CFGFIXRATE (stored, output unchanged after 9 s;
   restart-apply presumed). Still untested: PPS (no pin
   instrumentation this bench), RCVRMODE, RTCM, RSID.
5. MOSTLY ANSWERED. No usable ACK at the old rate: the port
   switches ~2 ms after the request (any OK goes out at the new
   rate). Speed change is a single awaiting write confirmed by its
   own OK when the host catches it, by incidental traffic at the new
   rate, or by the OK elicited by a repeat of the same write (the
   family-standard MaybeSpeedChangeSucceeded + repeat pattern;
   hardware-verified across 460800/115200/230400, both the caught-OK
   and lost-OK/repeat paths). Saved-rate-after-SRR and power-cycle
   persistence remain bounded (no root for USB unbind/rebind).
6. PART-ANSWERED. SAVEPAR answers bare OK sent alone; pipelining
   SAVEPAR+SRR loses both responses (SRR reboots first) though the
   save completes - always wait for SAVEPAR's OK before SRR. SRR
   answers nothing; module back within ~15 s; boot loads NVM
   (saved state was in effect after reboot). Still open: SAVEPAR
   coverage, RESTOREPAR, COLD/WARM/HOT effects, boot banner
   details, restart-discards-unsaved-changes confirmation.
7. ANSWERED (output side). Clearing OutputProt's NMEA bit on the
   active port silences all periodic output immediately but PQTM
   command responses still arrive (set OK and get responses
   verified) - no saw-off risk on output. InputProt on the active
   port deliberately untested: lockout risk with no second UART
   wired; treat as unknown and never clear it on the active port.
8. ANSWERED. RCVRMODE and CFGSVIN are both RESTART-ONLY. Effective
   base mode (after save+SRR): NMEA off, RTCM3 on at 1 Hz (MSM4
   1074/1084/1094/1124 + 1005 + 1033), PQTM responses unaffected,
   and LSTMSG shows a PER-MODE table (RTCM entries only).
   PQTMSVINSTATUS and its disable are mode-gated (ERROR,1 outside
   effective base mode) - the message-file "broken on R02" note
   explained. Survey: stored mode 1 is inert until restart
   (SVINSTATUS means are continuous averaging telemetry, Valid=0,
   CfgDur reads 0); after restart the survey runs (CfgDur=60 as
   configured, Obs 1/s, Valid 1 -> 2 at Obs==CfgDur, mean+MeanAcc
   frozen thereafter). The result does NOT transfer into CFGSVIN
   readback - it lives only in SVINSTATUS (and the emitted 1005),
   so a saved mode 1 presumably re-surveys every boot (not
   separately verified).
9. MOSTLY ANSWERED. LSTMSG (bare, current port) dumps one
   `,OK,1,1,<MsgName>,<Rate>[,<MsgVer>]` line per ENABLED message
   only - disabled messages vanish (no rate-0 entries), so the
   as-found read cannot distinguish "disabled" from "nonexistent";
   terminator `,OK,End`. One dump line was observed aborted
   mid-sentence with the dump restarting (rare, ~1 in 12); a reader
   keys on End and re-issues if an entry may have been lost.
   CLRMSG untested.
10. ANSWERED. Same underlying state: Duration set via PPS2 shows
    in legacy PPS readback and vice versa. A legacy PPS,W does NOT
    clobber PPS2-only fields (Userdelay 250 and Period survived a
    subsequent PPS,W) - PPS2-with-PPS-fallback is safe. Register
    semantics only (no pin instrumentation on this bench).
11. PART-ANSWERED. Unknown sentence names answer `<name>,ERROR,3`
    echoing the unknown name - never silence - so ERROR,3 capability
    discovery is definitive and any PQTM-speaking firmware answers
    the probe with something. PQTMVERNO answered reliably under
    default load at 460800 in every run so far; state-neutrality
    assumed (read-only query), reliability under heavier load not
    yet stressed.
12. ANSWERED (documented audit over lg290p_1.3.md). No
    raw-measurement PQTM message - the spec itself names RTCM as
    the raw-measurement carrier - so ConfigSupportRaw is absent and
    MSM stays under RTCMMsg. Leap seconds appear only as fields in
    PQTMPVT/NAV/PPPNAV (no UTC-model query). No antenna-cable-delay
    field beyond PPS2 Userdelay. No PPS time-reference knob:
    TimeGNSS absent, align-to-GNSS is fixed behavior.

Exit criteria: every design assumption above marked verified or
corrected; the message file gains verified tags for every command the
configurator will send.

## Stage 1: CFG wire structs in gps/lib/qtmmsg

One struct per CFG command tuple (CfgUART, CfgPPS, CfgPPS2, CfgProt,
CfgMsgRate, CfgSVIN, CfgRcvrMode, CfgFixRate, CfgCnst, CfgSignal,
CfgRSID, CfgRTCM, CfgEleThd, ...), each serializing to the W form and
parsing from the OKData form, plus VERNO/UNIQID responses. Round-trip
tests audited against the registration list. (Message names and fields use the spec's own
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

3. Message output: NMEAMsg/PVTMsg/SatsMsg via CFGMSGRATE (one set per
   modeled member, no readback), plus whole-protocol on/off via
   CFGPROT for a complete NMEA request.
4. RTCM output: CFGMSGRATE RTCM groups + CFGRTCM + RSID, plus
   whole-protocol on/off via CFGPROT for a complete RTCM request.
5. Time pulse: PPS2 with PPS fallback.
6. Mode/survey/fixed position: CFGSVIN + RCVRMODE, gated on
   save+restart per "Restart-only settings".
7. Signals: CFGSIGNAL/CFGCNST, gated on save+restart per
   "Restart-only settings".
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
