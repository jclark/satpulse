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

## u-blox M8T, /dev/ttyS0 (UART)

Not yet probed. Baud rate unknown; discover by scanning (9600 is the M8 UART default).
