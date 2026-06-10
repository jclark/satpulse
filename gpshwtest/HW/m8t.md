# LEA-M8T limitations

How device-independent configuration is realized on the u-blox LEA-M8T-0, relative to perfect realization of the full model (`docs/gps-config-semantics.md`). Measured on firmware TIM 1.10 PROTVER 22.0 (2026-06-10, UART at 9600). Perfectly-realized behavior is not listed. Two satpulsetool gen 8 backend bugs found on this receiver are in `BUGS.md` (antenna cable delay ignored; fixed position echoed as LLH but stored as ECEF), so those properties are not yet fully characterized.

## Signals

Supported signal set (single-band):

| GNSS | Signals |
|------|---------|
| GPS  | L1 C/A |
| GAL  | E1 |
| GLO  | L1 |
| BDS  | B1I |
| QZSS | L1 C/A, L1S |
| SBAS | L1 C/A |

- The protocol reports a limit on simultaneously enabled major constellations: a request for all six constellations realizes five, with GLONASS dropped (satpulse's documented fixup preference). No error; the achieved set shows it.
- Band-restricted requests realize the same sets (everything is L1-band); requests restricted to other bands have an empty intersection with the supported set.
- The unit was found with QZSS L1 C/A enabled but L1S disabled. That signal set is not denotable in the constellation-by-band request syntax, so after signal probing the as-found state cannot be restored (QZSS comes back as L1 C/A + L1S); gpshwtest reports this as an honest restore failure.

## Properties

- Fixed position is stored in ECEF whatever form it is set in; readback is ECEF-only. ECEF coordinates quantized to 0.01 m (TMODE2 cm fields), fixed-position accuracy to 0.001 m.
- The RTCM base ID property does not exist: setting reports nothing achieved and readback omits it, consistent with the absent `rtcmBaseID` capability flag.

## Message output

- Per-signal satellite information (`sig`) delivers nothing: this firmware has no per-signal message. Per-satellite (`sat`) delivers.
- RTCM output requests fail with an error, consistent with the absent RTCM capability flags (the error rather than silent non-realization is the historical behavior noted in the semantics doc).

## Testing notes

As-found running configuration: NMEA GGA, GLL, GSA, GSV, RMC, VTG, ZDA; mobile mode; GPS, GLO, and QZSS (L1 C/A only) enabled. satpulsetool finds the UART speed by scanning; gpshwtest locks in the speed reported by `--show-port`. Raw output saturates the 9600 baud line and makes configuration unreliable (see `BUGS.md`); raising the speed for the session would avoid this. A full run takes about 8.5 minutes.
