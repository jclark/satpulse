# TAU951M-P200 limitations

How device-independent configuration is realized on the Allystar
TAU951M-P200 (HD9510), relative to perfect realization of the full
model (`SEMANTICS.md`). Measured on firmware 3.018.2acec91c
(2026-07-03, UART at 115200). Perfectly-realized behavior is not
listed. This is the RTK member of the trio; its signal plan equals the
TAU1201's.

## Starting state

The standard NMEA set (GGA, GSA, GSV, RMC, ZDA) at 1 Hz, saved to
NVM, everything else as shipped; `setup/tau951m.sh` establishes it.
The unit is 5 Hz-native and its CFG-MSG rates are divisors of the
native cycle, so 1 Hz output means stored rate 5 on every sentence.
The unit's FACTORY rate table is inconsistent: GGA is 1 (5 Hz output)
while GSA/GSV/RMC/ZDA are 5 (1 Hz) - verified 2026-07-13 by factory
resets reproducibly restoring exactly that split. Any factory reset
therefore leaves GGA at 5 Hz until the setup script is re-run (the
rate estimator computes divisor 5 and the save persists it).

## Signals

Supported signal set (L1/L5 dual band, identical to the TAU1201):
GPS L1 C/A + L5, GAL E1 + E5a, GLO L1, BDS B1I + B2a, QZSS L1 C/A +
L5. satpulse intersects requests with this identity-deduced plan
before writing; the silicon's own clamp (which also accepts a zero
mask) backs the verify readback.

## Properties

- Antenna cable delay does not exist; the CFG-PPS Offset field is
  factory-set (530 ns on this unit, surviving factory reset -
  evidently calibration) and is preserved, never exposed. Time-pulse
  findings on this unit are CFG-PPS register readbacks; the PPS pin
  itself was not observed (no timing instrumentation).
- Fixed position is ECEF-only in storage (0.01 m quantum); no stored
  position accuracy (reads zero). LLH sets are converted.
- Minimum elevation carries float32-radian rounding.
- Time pulse alignment and timing constellation have no carrier.
- The active UART is not identifiable; a CFG-PRT baud set switches the
  arriving port.

## Message output

- `tp`, `leap` (announcement date), and per-signal `sig` deliver
  nothing (no carriers; NAV-SVINFO is per-satellite).
- RTCM MSM4 and MSM7 emit per enabled constellation (verified GPS,
  GLO, GAL, QZSS, BDS); ARP 1005 emits only while a fixed position is
  set - with a zero CFG-FIXEDECEF it is accepted and silent, unlike
  the TAU1302, which emits a 1005 even at zero.
- Raw output is the single RXM-DUMPRAW switch; RXM-RAW frames here run
  ~5.4 KB each (about half the 115200 line budget). The DISABLE is
  answered with a NAK yet applied - unique to this unit among the
  three - so a NAK on that request is not a failure.

## Testing notes

As-found running configuration: NMEA GGA, GSA, GSV, RMC, ZDA at 1 Hz
(TXT off, unlike the TAU1201); mobile mode; full supported signal set;
PPS 1 s period, 1% duty, FALLING polarity, GPIO 13, offset 530 ns;
NMEA version 4.11.

The factory CFG-PPS polarity field is falling, which the satpulsetool
vocabulary cannot express (`--pps` realizes the full pulse bundle with
rising polarity; see `BUGS.md`, unresolved observations). Every probing run
therefore reports one honest not-left-as-found failure on
`timePulse.polarityRising`; the polarity is restored out-of-band after
runs (after a --disruptive run the NVM copy needs the same restore,
since the run's saves persist the rising polarity). The
characterization itself is run-to-run identical, and the disruptive
NVM-comparison failures on this unit all reduce to the same polarity
field.

This unit silently drops requests beyond ~12 outstanding (same-id
bursts, stage-0 finding); the configurator's per-id serialization
keeps well under that. Its NVM held a stray NAV-TIME rate 18: binary
CFG-MSG rates fall outside the save mask's granularity, so a running-0
save does not overwrite it and a soft reset reloads 18. It also seemed
to survive a factory clear and reappear after a later reload, read at
the time as a receiver-persistent store beyond the save mask and
factory clear. That reading is unreliable: reset and factory-clear are
unacknowledged commands (SIMPLERST, CFG-CFG clear), and satpulsetool
had a bug (fixed on master, #359) where a no-response command sent as
the last write before the tool exits could be clipped by the baud
restore on close and never reach the receiver - so a clear or reload
that seemed not to take effect may not have executed. Re-verify the
NVM-clear behavior on this unit with the fixed tool before treating it
as a receiver limitation. Running state is kept at rate 0.
