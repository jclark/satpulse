# Findings from hardware sessions

Empirical knowledge gathered while working toward `GOAL.md`, recorded here so later sessions (possibly on other machines) can use it - for designing probes and for vetting characterizations. Append new findings with receiver, firmware, and date.

## ZED-F9P, HPG 1.51 PROTVER 27.50 (2026-06-10, /dev/ttyACM0)

Signal combinations, measured via `satpulsetool gps --gnss ... [--band L1] --json` (all 20 L1-only constellation combinations tried):

- L1-only is accepted only when the enabled set is exactly BDS (readback `BDS: [B1I]`), optionally plus SBAS. Every L1-only combination including GPS, GAL, GLO, or QZSS is refused.
- A refusal is an ACK-NAK to the CFG-VALSET, with `$GPTXT,01,01,01,inv sig cfg` and `$GPTXT,01,01,01,bad cfg RAM` emitted; it is transactional (readback confirms the configuration is unchanged). Even a minimal single-key write of `CFG-SIGNAL-GPS_L2C_ENA=0` is refused this way.
- So this receiver will not run GPS, GAL, GLO, or QZSS without their second signal (L2C, E5b, L2, L2C respectively); BDS is the one constellation it runs single-band.
- Full-band subsets of the four majors all realize exactly as requested. GPS-only with QZSS and SBAS disabled is accepted, so there is no enforced GPS/QZSS coupling on this firmware at full band.
- The recorded replay testdata (`internal/gpscmd/testdata/f9p-signal.jsonl`, captured on HPG 1.12) agrees where it overlaps: `--gnss GPS,GAL --band L1` recorded as a CFG-VALSET rejection, `--gnss BDS --band L1` recorded OK with `BDS: [B1I]`. The behavior is not firmware drift.
- The valid-combination table is in the ZED-F9P integration manual, which is NOT in `../gps-protocol-docs` (only the interface description `u-blox/F9-HPG-1.51.md` is, and it defers to the integration manual for valid signal configurations).

Other observations from the same session:

- Time pulse, antenna cable delay, min elevation, time GNSS (GPS/GAL/BDS/GLO), survey, fixed ECEF, fixed LLH, and RTCM base ID all set and read back consistently through `satpulsetool gps` (probed with round values only; resolution/quantization not yet measured).
- Changing the GNSS signal set triggers an internal GNSS-subsystem restart (documented in the interface description: wait for the ACK plus 0.5 s before the next command); a ~2 s settle worked reliably.

Scalar property probes via the gpshwtest program (2026-06-10, same firmware):

- Time pulse width is quantized to 1 us (requested 123.456 us reads back 123 us).
- Antenna cable delay (ns granularity, up to 32767 ns), min elevation (integer degrees), RTCM base ID (0-4095), and time GNSS (GPS/GAL/BDS/GLO) all realize exactly as requested.
- Units: the `--json` config object reports antennaCableDelay and timePulse.width in seconds; the CLI flags take nanoseconds (`--ant-cable-delay`) and seconds (`--pps`). Probe values must be compared in model (JSON) units.
- A full scalar-probe run (16 observations, ~45 invocations) takes about 45 s; two consecutive runs produced byte-identical characterizations.

Positioning mode probes (same session):

- Fixed position is quantized to 0.1 mm in every representation: ECEF X/Y/Z, LLH height, and fixed-position accuracy all 1e-4 m; latitude/longitude 1e-9 deg. Matches u-blox CFG-TMODE cm + 0.01 mm high-precision fields.
- The mode readback preserves the coordinate system the position was set in (fixedPosECEF vs fixedPosLLH+height); there is no conversion on readback.
- Survey minimum duration and accuracy limit are write-only: they appear neither in the set response nor in --show-config. This is model-wide (the mode JSON has no survey fields), not receiver-specific.
- Positioning mode is set as a whole: --fixed-pos-llh without --fixed-pos-acc resets the accuracy to its default (20 m). Probes must always pass the accuracy explicitly.
- The port/baudRate fields appear in the JSON config object only with --show-port, not with plain --show-config.

Signal combination probes via the program (same session; extends the manual findings above):

- QZSS-only and SBAS-only are refused; GPS+QZSS and GPS+SBAS are accepted. Augmentation systems need a major constellation enabled (only probed paired with GPS).
- The only accepted band-limited combination in the whole probe set (singles x L1/L2/E5b, all x L1/L2/L5/E5/L6/L1+L2) is BDS+L1 -> B1I. Even BDS+E5b (B2I alone) is refused: no constellation runs on only its second signal either.
- Full signal sets at full band: GPS [L1,L2C], GAL [E1,E5b], GLO [L1,L2], BDS [B1I,B2I], QZSS [L1,L1S,L2C], SBAS [L1]. Enabling GPS alone does not pull in QZSS or SBAS (no coupling).
- A signals run (63 observations total) takes just under 2 min including 2 s settles after each accepted signal change; still byte-identical across consecutive runs.

NMEA output probes (same session):

- --nmea-out realizes exactly at the sentence-type level: RMC alone, GGA+ZDA, and none all emit precisely what was requested (observed over 4 s captures). Default set on this receiver: GGA, GLL, GSA, GSV, RMC, VTG.
- --nmea-out none is realized as disabling the NMEA protocol on the port (CFG-USBOUTPROT-NMEA=0), not as per-message disables. With binary output not enabled, the receiver then emits nothing at all.
- satpulsetool detection of a fully-silenced receiver is intermittent: in one session the MON-VER poll went unanswered twice 1.5 s apart (-> "GPS detection failed: no output detected from GPS") while identical polls 5 s earlier and 4 s later were answered instantly. Cause not yet diagnosed (possibly a port reopen/DTR timing interaction). The program retries a detection failure once after 2 s; worth investigating in satpulsetool itself.
- Recovery from the silent state: satpulsetool gps --nmea --nmea-out GGA,GLL,GSA,GSV,RMC,VTG (UBX polls still work, sometimes needing the retry).

RTCM and raw output probes (same session):

- The packet log carries the RTCM message type number in the msg field (e.g. "1074"), so RTCM checks need no payload parsing.
- --rtcm-out MSM4 emits the x4 MSM for each enabled constellation plus 1230 (GLONASS code-phase biases), which was not requested; MSM7 likewise adds 1230. ARP (1005) is not emitted in mobile positioning mode - no station position to report - so a mobile-mode probe records it as missing.
- --raw-out obs is realized as UBX RXM-RAWX (~1 Hz), nav as UBX RXM-SFRBX (bursts, >20/s); the kinds are absolute (setting nav alone disables obs), realizations are disjoint, and none silences both.
- One transient: a combined "--rtcm-out none --raw-out obs" invocation once produced no RAWX afterwards, not reproducible. The program now settles 1 s after each message-output change before observing.

PVT and satellite output probes via replay (same session):

- --nmea and --binary are mode switches with canonical baselines, not just protocol toggles: --nmea enables the NMEA protocol, disables the UBX periodic messages, and resets the sentence set to RMC only; --binary disables NMEA and enables NAV-PVT plus a leap-second message. Neither can be combined with the message-output flags in one invocation, so restoring "NMEA with sentence set X" takes two invocations (--nmea, then --nmea-out X).
- Message output semantics are best-effort at the message level (PVTMsgOff in configtarget.go turns off *unneeded* messages): delivering more than requested is normal, e.g. ptp/ntp also deliver position and velocity because quality information on u-blox rides in NAV-PVT, and MSM output adds 1230. The characterization records only missing requested information.
- ptp is realized as NAV-PVT + NAV-TIMEGPS + TIM-TP: pulse time arrives as a time event with ref=PrePulse and taiTime; tai and ecef content selectors are honored; sat/sig produce satellites events, sig with per-signal entries.
- Survey progress messages cannot be verified in mobile mode (NAV-SVIN only flows during a survey-in); the survey flag is excluded from PVT expectations.

## u-blox LEA-M8T-0, TIM 1.10 PROTVER 22.0 (2026-06-10, /dev/ttyS0 UART 9600)

First full program runs (the unfamiliar-receiver test: no new code needed; two bugs in generic comparisons fixed). Run takes ~8.5 min at 9600 baud. Findings:

- satpulsetool detects it without -s (scan finds 9600); the program locks in the speed reported by --show-port for the rest of the run.
- Fixed position is stored in ECEF however it is set: a position set as LLH reads back converted to ECEF (TMODE2). ECEF quantized to 0.01 m (cm fields), fixed-position accuracy to 0.001 m. The set response echoes the requested LLH form (quantized to 1e-7 deg), which an independent readback cannot confirm - the program compares only the representation-shared mode fields.
- Antenna cable delay is not settable: requests are accepted but the value never changes (stayed at the receiver's 50 ns). Characterized as notSettable.
- Signal sets are single-band as expected. Requesting all six constellations enables five: GLO is clipped when BDS is also requested (M8 concurrent-GNSS limit). QZSS enable turns on L1+L1S; a receiver found with QZSS L1-only (the factory default) cannot be put back, because per-signal selection is not in the configuration vocabulary - the as-found state is unreachable and the first run reports an honest restore failure. satpulsetool design question: should enabling QZSS include L1S?
- sats-out sig delivers nothing (PROTVER 22 has no NAV-SIG); sat works. RTCM output and band selection are refused, matching the absent supports flags.
- Reproducible satpulsetool issue: setting --raw-out nav right after raw observations were enabled times out with "no response to request" (twice, including the retry), while the same invocation on a quiet line succeeds. Likely the ~1.2 s response wait expiring behind RXM-RAWX transmissions (~0.6 s each) saturating the 9600 baud line.
- Transient "no response"/"detection failed" errors are now retried once and reported as failures when they persist, never as receiver limitations (they would otherwise pollute the characterization).
