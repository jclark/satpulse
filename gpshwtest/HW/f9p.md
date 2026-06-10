# ZED-F9P limitations

How device-independent configuration is realized on the u-blox ZED-F9P, relative to perfect realization of the full model (`docs/gps-config-semantics.md`). Measured on firmware HPG 1.51 PROTVER 27.50 (2026-06-10, USB at 38400). Perfectly-realized behavior is not listed: everything not mentioned here realized requests exactly.

## Signals

Supported signal set:

| GNSS | Signals |
|------|---------|
| GPS  | L1 C/A, L2C |
| GAL  | E1, E5b |
| GLO  | L1, L2 |
| BDS  | B1I, B2I |
| QZSS | L1 C/A, L1S, L2C |
| SBAS | L1 C/A |

Receiver validity rules (requested sets the receiver refuses; refusals are transactional - configuration confirmed unchanged):

- A major constellation cannot be enabled on a single signal, with one exception: every set giving GPS, GAL, GLO, or QZSS only one of its signals is refused, including single-signal disables; BDS on B1I alone is accepted (B2I alone is refused).
- QZSS-only and SBAS-only sets are refused; either is accepted together with GPS.
- No coupling: GPS alone, with QZSS and SBAS disabled, is accepted.

The refusal appears on the wire as an ACK-NAK plus `GPTXT inv sig cfg` / `bad cfg RAM`. The valid-combination table is in the ZED-F9P integration manual (not in `../gps-protocol-docs`; the interface description defers to it). Replay testdata from HPG 1.12 (`internal/gpscmd/testdata/f9p-signal.jsonl`) agrees where it overlaps, so this is not firmware drift.

A signal-set change triggers an internal GNSS-subsystem restart; allow ~2 s after the ACK before the next command (interface description documents ACK + 0.5 s).

## Property resolution

- Time pulse width: quantized to 1 us.
- Fixed position: latitude/longitude to 1e-9 deg; ECEF coordinates, LLH height, and fixed-position accuracy to 0.1 mm (CFG-TMODE cm fields plus 0.01 mm high-precision fields).
- Readback preserves the coordinate system the fixed position was set in (ECEF stays ECEF, LLH stays LLH).
- Antenna cable delay (1 ns requests), minimum elevation (integer degrees), RTCM base ID, and time GNSS realize exactly at probed values.

## Message output

- ARP (RTCM 1005) is not emitted in mobile positioning mode - there is no station position to report. The only message-output limitation found; all other requested information was delivered.

## Testing notes

As-found running configuration: NMEA GGA, GLL, GSA, GSV, RMC, VTG; mobile mode; all supported constellations at full band. A full gpshwtest run takes about 4 minutes (80 observations) and consecutive runs produce byte-identical characterizations.
