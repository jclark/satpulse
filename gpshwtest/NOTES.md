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

## u-blox M8T, /dev/ttyS0 (UART)

Not yet probed. Baud rate unknown; discover by scanning (9600 is the M8 UART default).
