# TAU951M-P200 limitations

How device-independent configuration is realized on the Allystar
TAU951M-P200 (HD9510), relative to perfect realization of the full
model (`SEMANTICS.md`). Measured on firmware 3.018.2acec91c
(2026-07-03, UART at 115200). Perfectly-realized behavior is not
listed. This is the RTK member of the trio; its signal plan equals the
TAU1201's.

## Signals

Supported signal set (L1/L5 dual band, identical to the TAU1201):
GPS L1 C/A + L5, GAL E1 + E5a, GLO L1, BDS B1I + B2a, QZSS L1 C/A +
L5. satpulse intersects requests with this identity-deduced plan
before writing; the silicon's own clamp (which also accepts a zero
mask) backs the verify readback.

## Properties

- Antenna cable delay does not exist; the CFG-PPS Offset field is
  factory calibration (530 ns on this unit) and is preserved, never
  exposed.
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

The factory PPS polarity is falling, which the satpulsetool vocabulary
cannot express (`--pps` realizes the full pulse bundle with rising
polarity; see `BUGS.md`, unresolved observations). Every probing run
therefore reports one honest not-left-as-found failure on
`timePulse.polarityRising`; the polarity is restored out-of-band after
runs (after a --disruptive run the NVM copy needs the same restore,
since the run's saves persist the rising polarity). The
characterization itself is run-to-run identical, and the disruptive
NVM-comparison failures on this unit all reduce to the same polarity
field.

This unit silently drops requests beyond ~12 outstanding (same-id
bursts, stage-0 finding); the configurator's per-id serialization
keeps well under that. Its NVM held a stray NAV-TIME rate 18 that soft
resets kept resurrecting; a factory clear removed it once, yet it
reappeared after a later reload - some receiver-persistent store
neither the save mask nor factory clear controls. Running state is
kept at rate 0.
