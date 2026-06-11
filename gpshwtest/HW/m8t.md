# LEA-M8T limitations

How device-independent configuration is realized on the u-blox LEA-M8T-0, relative to perfect realization of the full model (`SEMANTICS.md`). Measured on firmware TIM 1.10 PROTVER 22.0 (2026-06-10/11, UART at 9600). Perfectly-realized behavior is not listed. A satpulsetool gen 8 backend bug found on this receiver (antenna cable delay sets silently ignored, `BUGS.md` history) was fixed on 2026-06-11; with the fix, cable delay realizes requests exactly.

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

- Fixed position is stored in ECEF whatever form it is set in; readback is ECEF-only. The conversion happens on the receiver, not in satpulsetool: a position set as LLH goes out as a TMODE2 write with the LLA flag and the geodetic values, and the receiver's own poll response 4 seconds later returns the LLA flag cleared with converted ECEF coordinates (decoded from run `runs/20260610-180323`, probes 046/047; the converted point agrees with the accepted one within 5 mm per axis). The set response truthfully reports the accepted LLH, readback truthfully reports the stored ECEF; the characterization records this as `mode.fixedPos: storedAs ECEF`. ECEF coordinates quantized to 0.01 m (TMODE2 cm fields), LLH to 1e-7 deg with height to 0.01 m, fixed-position accuracy to 0.001 m.
- The RTCM base ID property does not exist: setting reports nothing achieved and readback omits it, consistent with the absent `rtcmBaseID` capability flag.

## Message output

- Per-signal satellite information (`sig`) delivers nothing: this firmware has no per-signal message. Per-satellite (`sat`) delivers.
- RTCM output requests fail with an error, consistent with the absent RTCM capability flags (the error rather than silent non-realization is the historical behavior noted in the semantics doc). These refusals are excluded from the characterization: the absent capability flags in `supports` already declare them.

## Testing notes

As-found running configuration: NMEA GGA, GLL, GSA, GSV, RMC, VTG, ZDA; mobile mode; GPS, GLO, and QZSS (L1 C/A only) enabled. satpulsetool finds the UART speed by scanning; gpshwtest locks in the speed reported by `--show-port`. A full run takes about 8.5 minutes.

The unit was found resting at the factory 9600 baud, but its intended persistent speed is 38400 (`/etc/satpulse.d/ttyS0.toml`) - most plausibly lost to the gen 8 reload defect below before gpshwtest first measured, then faithfully preserved as "as found" by every run since. The 38400 resting speed was restored and saved on 2026-06-11 (verified to survive a reload), so runs now find the receiver at 38400; sessions still raise the link to 115200.

Raw output saturates the 9600 baud line: the receiver's transmit side overruns, and packets - including poll replies and ACKs - are lost or corrupted. Configuration then becomes unreliable in a way satpulse cannot help: invocations fail with "no response" while the write may actually have applied with only its ACK lost, so on a saturated link a reported error does not imply an unchanged configuration; detection can fail outright. Recovery needed low-level CFG-MSG writes (`-m`) to disable the raw messages. Raising the serial speed for the session avoids all of this. gpshwtest now does so by default (raises the link to 115200 at session start and restores the as-found speed at the end); with that in place a full sweep completes with no saturation failures.

## NVM and saving

The first disruptive run found a gen 8 reload defect (`--reload` sent CFG-CFG with deviceMask 0x00 and loaded factory defaults, not NVM), since fixed; on this unit the as-found NVM configuration happens to equal the factory defaults, which had masked it in the default-path reload probe. With the fix, save granularity is discovered and classified per property: minimum elevation and positioning mode persist together (the navConf section); time GNSS is anomalous - one --time-gnss change writes CFG-TP5, CFG-RATE, and CFG-NAV5 to keep the time system consistent, spanning the navConf and rxmConf save sections, so its save persists members of both groups (a satpulse realization fact, not a receiver defect); antenna cable delay and time pulse width persist together (both are CFG-TP5 fields, so they share a save section). The receiver's own granularity is a clean partition of its NVM sections. Message enablement saved with --save persists (the gen 8 --nmea-out realization saves the ioPort, msgConf, and navConf sections, so a navConf property canary rides along - messageOutput therefore classifies into the navConf group).

As on the F9P, NVM held a QZSS signal set without L1S, which the constellation-level vocabulary cannot reproduce; NVM now holds the full QZSS set (loudly reported once).

satpulsetool does not scan baud rates: with no `-s` it opens the port at its current termios state. A receiver left at a non-resting speed (for example 115200 in NVM after a save-all at the raised session speed) is unreachable until the right `-s` is given; gpshwtest's speed rediscovery therefore tries candidate speeds explicitly.

The full disruptive sweep (save granularity, save-all recovery, reset, the 57600 speed probe, factory reset, NVM recovery) completes end to end with the receiver verified left as found at 9600. The 2026-06-11 sweep with the cable-delay fix reported no failures at all; the vetted characterization regenerated from it is checked in at `baselines/LEA-M8T-0-TIM-1.10-PROTVER-22.0.json` (the cable-delay entries disappeared, the fixed position characterizes as `mode.fixedPos: storedAs ECEF` with proper LLH quanta, and cable delay classified into the time pulse save group).
