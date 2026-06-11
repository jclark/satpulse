# LEA-M8T limitations

How device-independent configuration is realized on the u-blox LEA-M8T-0, relative to perfect realization of the full model (`SEMANTICS.md`). Measured on firmware TIM 1.10 PROTVER 22.0 (2026-06-10, UART at 9600). Perfectly-realized behavior is not listed. Two satpulsetool gen 8 backend bugs found on this receiver are in `BUGS.md` (antenna cable delay ignored; fixed position echoed as LLH but stored as ECEF), so those properties are not yet fully characterized.

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
- RTCM output requests fail with an error, consistent with the absent RTCM capability flags (the error rather than silent non-realization is the historical behavior noted in the semantics doc). These refusals are excluded from the characterization: the absent capability flags in `supports` already declare them.

## Testing notes

As-found running configuration: NMEA GGA, GLL, GSA, GSV, RMC, VTG, ZDA; mobile mode; GPS, GLO, and QZSS (L1 C/A only) enabled. satpulsetool finds the UART speed by scanning; gpshwtest locks in the speed reported by `--show-port`. A full run takes about 8.5 minutes.

Raw output saturates the 9600 baud line: the receiver's transmit side overruns, and packets - including poll replies and ACKs - are lost or corrupted. Configuration then becomes unreliable in a way satpulse cannot help: invocations fail with "no response" while the write may actually have applied with only its ACK lost, so on a saturated link a reported error does not imply an unchanged configuration; detection can fail outright. Recovery needed low-level CFG-MSG writes (`-m`) to disable the raw messages. Raising the serial speed for the session avoids all of this. gpshwtest now does so by default (raises the link to 115200 at session start and restores the as-found speed at the end); with that in place a full sweep completes with no saturation failures, leaving only the gen 8 backend bugs in `BUGS.md`.

## NVM and saving

The first disruptive run found a gen 8 reload defect (`--reload` sent CFG-CFG with deviceMask 0x00 and loaded factory defaults, not NVM), since fixed; on this unit the as-found NVM configuration happens to equal the factory defaults, which had masked it in the default-path reload probe. With the fix, save granularity is discovered: minimum elevation and positioning mode persist via the navConf section, antenna cable delay and pulse width via rxmConf, and the time GNSS realization spans both sections, so saving it persists the union. The persists-together relation is therefore not a partition (the NVM sections partition, but the property-to-section mapping is many-to-many), and the characterization carries the per-property observations verbatim - a mismatch with the partition model in SEMANTICS.md worth a semantics decision.

As on the F9P, NVM held a QZSS signal set without L1S, which the constellation-level vocabulary cannot reproduce; NVM now holds the full QZSS set (loudly reported once).

satpulsetool does not scan baud rates: with no `-s` it opens the port at its current termios state. A receiver left at a non-resting speed (for example 115200 in NVM after a save-all at the raised session speed) is unreachable until the right `-s` is given; gpshwtest's speed rediscovery therefore tries candidate speeds explicitly.

The full disruptive sweep (save granularity, save-all recovery, reset, the 57600 speed probe, factory reset, NVM recovery) completes end to end with the receiver verified left as found at 9600; every reported failure traces to the gen 8 satpulsetool defects in `BUGS.md`. The vetted characterization is checked in at `baselines/LEA-M8T-0-TIM-1.10-PROTVER-22.0.json`; consecutive disruptive runs produce it byte-identically. Note it deliberately carries the satpulsetool-bug-shaped entries verbatim (antenna cable delay frozen, fixed LLH unreadable): when the gen 8 bugs are fixed those entries will change, and the baseline diff is the signal to re-vet and update it.
