# Convert SinoGNSS RANGE raw observations to RINEX (#476)

## Problem and scope

`satpulsetool convobs` converts raw observations to RINEX from UBX
RXM-RAWX (`gps/lib/rnxubx`), RTCM MSM7 (`gps/lib/rnxrtcm`), Unicore OBSVM
(`gps/lib/rnxunc`) and, on the `sbf-rinex` branch (#342), Septentrio
MeasEpoch (`gps/lib/rnxsbf`). NovAtel-format receivers have no path:
`gps/lib/novmsg` names RANGE (43) but does not decode it.

This plan adds RANGE from SinoGNSS receivers, end to end: decode in
`gps/lib/novmsg`, a mapping of satellite and signal numbers to RINEX
codes, a converter package, and a `convobs` input.

Only OEM7 and SinoGNSS receivers output RANGE (see the RangeID comment
in `gps/lib/novmsg/other.go`): ByNav documents only RANGECMP, and an M10
accepts RANGE but outputs nothing; Unicore documents neither and uses
its own OBSVM. The mapping is the documented OEM7 one, with the SinoGNSS
differences layered on top, and `convobs` selects the SinoGNSS layer
with `--vendor`, as `annotate` and `decode` do. There is no NovAtel
receiver to test with, so only SinoGNSS is tested; OEM7 RANGE should
convert correctly with the OEM7 mapping, but is untested, and the
documentation says so.

First, start a long K901 capture of RANGE and MSM7, which stage 5 needs
and which runs in the background while stages 1 to 4 are implemented.
The work then goes in stages, each usable on its own:

1. RANGE decoder in `novmsg`.
2. RINEX mapping in `novmsg`: the OEM7 base, and the SinoGNSS layer.
3. Converter package `gps/lib/rnxnov`.
4. `convobs` input, docs and NEWS.
5. Validation against RTCM MSM7 from the same epochs.

The main target is the K901, which is on the antenna with an accurately
known position; the K803 is a second receiver with a partly different
signal set.

## Sources

- OEM7 Commands and Logs Reference Manual: RANGE (3.151), Table 12 (PRN
  numbers), Table 164 (channel tracking status), Table 167 (RINEX
  mappings).
- SinoGNSS K8/K9 manual: RANGE (3.2.6.1), Table 2-2 (PRN numbers). The
  translation lacks the tracking-status tables it refers to (3-11 and
  3-13), so the SinoGNSS signal types below come from the captures.
- Test data: `gps/testdata/packets/sinognss/K901/raw-obs.jsonl` (TIME
  and RANGE) and `raw-cross.jsonl` (RANGE and MSM7 for GPS, GLONASS,
  Galileo and BDS in the same epochs, 30 s), and the same two files for
  the K803. All RANGE in the corpus is binary.

## First: start the K901 capture

`raw-cross.jsonl` has RANGE and MSM7 from the same epochs, but only 30 s,
which is too short to see arcs, slips and satellites rising and setting,
or to go through PPP. Stage 5 needs a long capture from the K901, which
is on the antenna with the known position; start it at the beginning so
that it is ready by then.

Configure the K901 with this message file, and check the output for
`Error!`:

```
[default.line]
eol = "\r\n"
[[line]]
text = "UNLOGALL"
[[line]]
text = "log timeb ontime 1"
[[line]]
text = "log rangeb ontime 1"
[[line]]
text = "log rtcm1077b ontime 1"
[[line]]
text = "log rtcm1087b ontime 1"
[[line]]
text = "log rtcm1097b ontime 1"
[[line]]
text = "log rtcm1117b ontime 1"
[[line]]
text = "log rtcm1127b ontime 1"
```

```
satpulsetool gps -d <device> -s 115200 -m k901-range-msm7.toml
```

Then capture passively for two hours, in the background:

```
satpulsetool serial -d <device> -s 115200 --packet-log k901-range-msm7-<date>.jsonl -t 7200
```

and afterwards restore the saved configuration with the message file's
`reload` tag.

- MSM7 is requested for every system the K901 outputs it for: GPS,
  GLONASS, Galileo, QZSS and BDS.
- In the K901's `raw-cross.jsonl` a RANGE averages 3.6 kB and the MSM7
  messages together 1.1 kB per epoch, so this is about 5 kB/s, inside
  the 11.5 kB/s of 115200 baud; check the capture has every RANGE and
  MSM7 epoch.
- The capture is too large for the repository (the packet log holds
  each byte as two hex digits, about 35 MB per hour); keep it outside
  the repository. The golden fixtures are 15-minute slices of it, as
  for the other `convobs` fixtures.

## Stage 1: RANGE decoder

`RANGE` has the same shape as Unicore OBSVM (`uncmsg.ObsVM`): a count,
then fixed records. Model it the same way, as a `novmsg.Chunked` type
registered for binary and ASCII:

```go
// RangeFixed contains the fixed-length fields of the RANGE log.
type RangeFixed struct {
	NumObs uint32
}

// Range represents the RANGE log: one record per tracked signal.
type Range struct {
	RangeFixed
	Obs []RangeObs
}

// RangeObs is one signal observation in a RANGE log.
type RangeObs struct {
	PRN      uint16
	GloFreq  uint16 // GLONASS frequency channel + 7
	PSR      float64
	PSRStd   float32
	ADR      float64
	ADRStd   float32
	Dopp     float32
	CN0      float32
	LockTime float32
	Status   ChTrStatus
}
```

`ChTrStatus` is a `uint32` with `MarshalText`/`UnmarshalText` for the
8-hex-digit ASCII form (as `uncmsg.TrackingStatus`), and accessors for
the Table 164 fields the converter needs:

| Bits | Field |
|---|---|
| 0-4 | tracking state |
| 10 | phase lock |
| 11 | parity known |
| 12 | code locked |
| 16-18 | satellite system (0 GPS, 1 GLONASS, 2 SBAS, 3 Galileo, 4 BDS, 5 QZSS, 6 NavIC) |
| 21-25 | signal type |
| 28 | half cycle added |

SinoGNSS status words decode consistently with this layout: `08009c24`
is tracking state 4 (phase lock loop), phase lock, parity known, code
locked, GPS, signal type 0 (L1 C/A), primary L1 channel.

RANGE has the same layout for OEM7 and SinoGNSS, so it goes in the
shared registries, not in a variant map in `gps/internal/nov`. Once
registered, `annotate` shows decoded RANGE logs.

Tests, per `gps/lib/novmsg` convention: binary round trips of K901 and
K803 RANGE logs from the corpus. There is no ASCII RANGE in the corpus;
capture a `log rangea once` from the K901 for an ASCII round trip, and
the binary RANGE of the same epoch for a binary/ASCII pair as the
existing SinoGNSS pairs do. An ASCII RANGE with about 100 observations is
about 12 kB, so request it with `once` rather than at 1 Hz at 115200.

## Stage 2: mapping to RINEX

In `gps/lib/novmsg`, following `uncmsg`, `ubxbin`, `rtcmbin` and
`sbfbin`: functions from RANGE's satellite system, PRN and signal type
to a RINEX system letter, satellite number and two-character signal
code, for OEM7 and for SinoGNSS.

They are built in two layers: the OEM7 tables as documented, and the
SinoGNSS tables, which list only the differences and fall back to
OEM7. This is the same layering as the NovAtel variants in
`gps/internal/nov/processor.go`, where the SinoGNSS variant is the OEM7
registry with its own entries overriding. The converter lives in
`gps/lib`, so it cannot use `gps/internal/nov`'s `Variant`; it takes a
small `novmsg` type naming the mapping (OEM7 or SinoGNSS), which
`convobs` sets from the vendor.

### Satellite numbering

| System | OEM7 RANGE PRN | RINEX number | SinoGNSS RANGE PRN | RINEX number |
|---|---|---|---|---|
| GPS | 1-32 | PRN | same | |
| SBAS | 120-158 | PRN-100 | same | |
| QZSS L1S (SBAS) | 183-191 | skip | not seen | |
| GLONASS | 38-61 (slot+37) | PRN-37 | same | |
| Galileo | 1-36 | PRN | same | |
| QZSS | 193-202 | PRN-192 | 131-140 | PRN-130 |
| BDS | 1-63 | PRN | 141-203 | PRN-140 |
| NavIC | 1-14 | PRN | 62-70 | PRN-61 |

The SinoGNSS ranges are Table 2-2 of its manual, and match the captures:
QZSS 132, 133 and 137 and BDS 141 upwards in RANGE, with QZSS 3 and 7 in
GSV. NavIC 63 and 70 are seen in K803 RANGE. The GLONASS frequency
channel is `GloFreq - 7`; when it is not 0-13 (unknown), leave
`SignalValues.Frq` unset.

The SinoGNSS satellite logs use different numbers for Galileo (71-106)
and SBAS (PRN+100): these are not RANGE numbers and must not be mixed in.

### Signal codes

OEM7 (Tables 164 and 167):

| System | Signal type -> RINEX |
|---|---|
| GPS | 0 1C, 5 2P, 9 2W, 14 5Q, 16 1L, 17 2S |
| GLONASS | 0 1C, 1 2C, 5 2P, 6 3Q |
| Galileo | 2 1C, 6 6B, 7 6C, 12 5Q, 17 7Q, 20 8Q |
| BDS | 0 and 4 2I, 1 and 5 7I, 2 and 6 6I, 7 1P, 9 5P, 11 7D |
| QZSS | 0 1C, 14 5Q, 16 1L, 17 2S, 27 6L |
| SBAS | 0 1C, 6 5I |
| NavIC | 0 5A |

SinoGNSS differences, from the K901 and K803 captures. Each signal type
was identified by its carrier frequency, computed from `psr/|adr|`, and
by comparison with NMEA 4.11 GSV signal IDs:

| System | SinoGNSS type | Signal | Carrier (MHz) | OEM7 type | RINEX (proposed) |
|---|---|---|---|---|---|
| GPS | 2 | L5 | 1176.45 | 14 | 5Q |
| BDS | 8 | B1C | 1575.42 | 7 | 1P |
| BDS | 12 | B2a | 1176.45 | 9 | 5P |
| BDS | 17 | B2I (BDS-2 only) | 1207.14 | 1 | 7I |
| BDS | 19 | B2b (BDS-3) | 1207.14 | 11 | 7D |
| GPS, QZSS | 17 | L2C | 1227.60 | 17 | 2X |

The other types seen match OEM7: GPS 0, 9, 16; GLONASS 0, 5;
Galileo 2, 7, 12, 17, 20; BDS 0, 2; QZSS 0, 14, 16; SBAS 0, 6;
NavIC 0. SinoGNSS reports BDS GEO satellites with the D1 types (0, 2),
not OEM7's D2 types (4-6); both give the same RINEX code.

Type 17 is L2C as in OEM7, but OEM7 tracks L2C(M) (2S), and SinoGNSS
tracks L2C(M+L) (2X): see the stage 5 results. The RINEX attribute
letters for the five SinoGNSS-only types are taken from the OEM7 signal
of the same name. They are not documented; stage 5
checks them against MSM7, which carries RINEX signal identities.

A combination not in either table is skipped, as `rnxunc` skips unknown
OBSVM signals.

Tests: table-driven over both tables, plus every system and signal type
combination found in the K901 and K803 RANGE captures, so that a
mapping gap in the corpus fails the test.

## Stage 3: converter

New package `gps/lib/rnxnov`, modelled on `gps/lib/rnxunc`, whose OBSVM
record and status bits are the same as RANGE's:

```go
// New creates a Converter that writes records to sink, using the given
// mapping of satellites and signals.
func New(sink rinex.Sink, mapping novmsg.RangeMapping) *Converter

// ConvertRange converts one RANGE log.
func (c *Converter) ConvertRange(h *novmsg.MsgHdr, m *novmsg.Range) (bool, error)
```

Per record:

- Time: `rinex.TimeFromGPSWeekSeconds(Week, MillisecondsOfWeek/1000)`
  from the header.
- Satellite and signal from stage 2; skip the record if either is
  unmapped.
- `PR` = `PSR` when code locked (bit 12) and finite.
- `CP` = `-ADR` when phase locked (bit 10) and finite. NovAtel ADR is
  accumulated Doppler range, opposite in sign to RINEX carrier phase;
  this is what `rnxunc` does for OBSVM.
- `Do` = `Dopp`, positive for approaching satellites, when finite.
- `CN0` = `CN0` when non-zero.
- `Frq` for GLONASS from `GloFreq`.
- `Arc` from lock time, with the same rules and tolerance as `rnxunc`: a
  lock time of zero, or one that grows by less than the elapsed time,
  starts a new arc; the increment waits for an epoch with carrier phase.
- `HC` when phase locked and parity not known (bit 11 clear), the
  half-cycle ambiguity that OEM7 describes for bits 11 and 28.

Records with no observation values are dropped. The converter is not
safe for concurrent use and expects one receiver's logs in time order,
as the other converters.

Tests in `package rnxnov`, with a test sink and `novmsg.Range` literals:
the validity bits, the ADR sign, GLONASS channel, lock-time arcs, the
half-cycle flag, and one conversion of a K901 RANGE log from the corpus
against expected RINEX values.

## Stage 4: convobs

In `internal/convobscmd`, following the SBF change on `sbf-rinex`
(commit "convobs: add the sbf input format"):

- New input formats `novb` and `nova` for NovAtel-format binary and
  ASCII RANGE, tags `gpsreg.TagNovAtelBin` and `TagNovAtelAscii`,
  parsed with `novmsg.ParseBinMsg` and `novmsg.ParseAsciiMessage`.
- Auto-selection under `--from raw`: `isRawObs` recognizes RANGE by
  message ID, `rawObservationNames` gets "NOVB RANGE" and "NOVA RANGE",
  the packet-log prefilter gets the `RANGE` marker, and the
  no-observations messages list them.
- Mapping: a `--vendor` option as `annotate` and `decode` have,
  resolved with `SATPULSE_VENDORS` through the same `cmd.ResolveVendors`:
  SinoGNSS selects the SinoGNSS mapping, anything else OEM7. The tag
  cannot select it, since NOVB and NOVA are shared by every
  NovAtel-format vendor. A SinoGNSS log converted without the vendor
  loses QZSS, BDS and NavIC (their PRNs fall outside the OEM7 ranges) and
  GPS L5, so the command warns once when it skips RANGE records it
  cannot map.
- `docs/man/satpulsetool-convobs.1.md` lists the new formats and
  `--vendor`, and says RANGE conversion has been tested with SinoGNSS
  receivers only: OEM7 RANGE should convert, but has not been tested.
- A NEWS entry for the new input format.

Tests in `convobs_test.go`: conversion of a K901 packet log, `--from
raw` auto-selection, the vendor selecting the SinoGNSS mapping, and the
warning without it; the golden cases are in stage 5.

## Stage 5: validation

Two independent references:

- RTKLIB Explorer's `convbin -r nov`, the reference for the existing
  `convobs` golden tests (`internal/convobscmd/testdata`). Its RANGE
  decoder (`decode_rangeb` in `src/rcv/novatel.c`) is OEM7 only.
- RTCM MSM7 from the same epochs, converted by `convobs --from rtcm`,
  itself checked against RTKLIB by the existing RTCM golden tests.

### Capture

The K901 capture started before stage 1 (see "First: start the K901
capture").

### RTKLIB

What `decode_rangeb` does with a SinoGNSS RANGE log:

- Satellites: it uses the OEM7 PRN ranges (`satno` with QZSS 193-202,
  BDS 1-50, NavIC 1-14), so SinoGNSS QZSS, BDS and NavIC are dropped.
  GPS, GLONASS (PRN-37), Galileo and SBAS are converted.
- Signals: `sig2code` is the OEM7 table plus Galileo type 1 as 1C ("E1BC
  (Bynav M2)"); SinoGNSS GPS type 2 (L5) is dropped.
- Values: carrier phase is `-adr`; pseudorange is zeroed without code
  lock and carrier phase and Doppler without phase lock; GLONASS records
  are dropped while parity is unknown; LLI gets a slip when the lock time
  grows by less than the elapsed time minus 0.05 s, and the half-cycle
  bit when parity is unknown.

So a SinoGNSS log converted by `convobs` without `--vendor` (the OEM7
mapping) should match `convbin` exactly, up to the compatibility
differences above: this tests the OEM7 mapping, which is otherwise
untested. With `--vendor sinognss` the two should agree on the GPS,
GLONASS, Galileo and SBAS records, which the SinoGNSS layer leaves to
the OEM7 base. The compatibility
differences that SatPulse should not copy by default get options, as
`--unc-omit-do-without-cp` does for OBSVM, or ignored signals in the
golden test, with the reason in the test.

Also to settle when generating the goldens: which `-ro` options
`convbin` needs (for signals sharing a frequency slot, such as GPS L1
C/A and L1C, or L2 P(Y) and L2C), and whether this build drops any
signal for its frequency limit, as it drops BDS B3I in the Unicore case.

Fixtures, from one 15-minute slice of the K901 capture:

- `k901-novb-<date>.novb`: `satpulsetool pack -t novb -m RANGE`.
- `k901-rtcm-<date>.rtcm`: `satpulsetool pack -t rtcm`.
- A `convbin` golden for each, with the flags in `testdata/Makefile`.

Golden test cases:

- RANGE without `--vendor` against its golden.
- RANGE with `--vendor sinognss` against the same golden, ignoring the
  satellites and signals only the SinoGNSS layer adds or recodes (QZSS,
  BDS, NavIC, GPS 5Q, and GPS L2C, which `convbin` writes as 2S).
- RTCM against its golden, as for the existing RTCM cases.

### MSM7

Convert the capture twice, `--from novb --vendor sinognss` and `--from
rtcm`, and compare the two RINEX outputs with `gps/lib/rinex/diffobs`.
Then do the same with the K803's `raw-cross.jsonl`. This is the check
for the SinoGNSS layer, which RTKLIB does not cover. Expected:

- The same satellites and, where both logs carry a signal, the same
  signal codes. This settles the attribute letters proposed in stage 2.
  MSM7 is output for GPS, GLONASS, Galileo, QZSS and BDS; SBAS and NavIC
  can only be checked against RTKLIB (SBAS) or for plausibility (NavIC).
- Pseudorange, Doppler and C/N0 agreeing within MSM7's resolution.
- Carrier phase agreeing up to a constant per signal arc. A difference
  of a quarter or half cycle on particular signals is a phase-alignment
  difference; OEM7 Table 167 gives a RINEX phase shift per signal (for
  example -0.25 for GPS L2C and L5, 0.25 for BDS B1C and B2a). Neither
  `gps/lib/rinex` nor the existing converters apply phase shifts, and
  RTCM MSM phase is aligned. If RANGE phase is not, decide between
  aligning it in the converter and writing the shifts to the RINEX
  header; record the result in the plan. RTKLIB Explorer has a phase
  shift option in its RINEX writer, which may show what it expects.

### Results from the 30 s captures

Before the long capture, the K901 and K803 `raw-cross.jsonl` were
converted with `--from novb --vendor sinognss` and `--from rtcm` and
compared with `diffobs`, and their NOVB RANGE (`pack -t NOVB -m RANGE`)
was converted by `convobs --from novb` and by `convbin -r nov -v 3.04 -od
-os`.

Against MSM7, on both receivers:

- Pseudorange agrees within 5 mm, and C/N0 within MSM7's resolution.
- Carrier phase agrees up to a whole number of cycles per arc (fraction
  at most 0.001 cycle) on every signal both logs carry, including GPS
  L2W and L5, BDS B1C and B2a, and Galileo E5a and E5b. So RANGE phase is
  aligned as MSM phase is on these signals. MSM7 carries no L1C, and on
  the same satellite the K901's L1C phase is a quarter cycle behind its
  L1C/A phase (median 0.248 cycle over two hours, GPS and QZSS), where
  RINEX Table A45 requires L1L to be aligned to L1C; its L2C is aligned
  with L2 P(Y). So the SinoGNSS mapping adds 0.25 cycle to the phase of
  type 16; the OEM7 mapping applies no shifts, since there is no OEM7
  receiver to measure.
- Doppler differs by the same fraction of the carrier frequency on
  every signal: 16.5 to 20.5 ppb on the K901 and 556 to 561 ppb on the
  K803. The RANGE Doppler is the one consistent with the
  carrier phase rate (within 0.2 ppb); the SinoGNSS MSM7 Doppler has
  the receiver clock drift removed. The long capture comparison has to
  allow for this.
- The SinoGNSS-only types GPS 2 (5Q), BDS 8 (1P), 12 (5P) and 17 (7I)
  match the MSM7 signals of those codes. BDS 19 (7D) and GPS 16 (1L)
  are not in the MSM7, so they are not checked.
- GPS type 17, OEM7 L2C(M), which the OEM7 mapping makes 2S, is the
  signal the receiver's own MSM7 labels 2X (signal ID 17, L2C(M+L)), as
  it does for QZSS in 1117: pseudorange and phase agree as for the other
  signals. A C/N0 comparison with the mosaic-G5 on the same antenna
  confirms M+L tracking. Each receiver's C/N0 on a signal, less its C/N0
  on L1 C/A (B1I for BDS), matches the mosaic's within 0.8 dB for GPS
  and QZSS L5Q, Galileo E5a Q and E5b Q, and BDS B3I, B1C and B2a; for
  L2C the K901's is 3.0 dB above the mosaic's L2C(L) on GPS and 2.9 dB
  on QZSS, the doubling of power of tracking M and L together. So the
  SinoGNSS layer maps type 17 to 2X for GPS and QZSS.
- The same comparison, against the mosaic-G5 with GPS L1C, QZSS L1C and
  B2b added to its tracking, and against the UM980 on SIGNALGROUP 2,
  over 15 minutes, gives (K901 gap less reference gap, median over
  satellites): L2C +3.1 dB (mosaic L2C-L) and +3.3 dB (UM980 2L); B2b
  -0.9 and -0.5 dB against B2b_I (7D); B1C, which the K901 declares
  pilot (MSM7 signal ID 31, 1P), +0.5 and +0.7 dB against B1C pilot;
  GPS and QZSS L1C +1.0 and +1.1 dB against L1C-P. The controls (GPS L5Q,
  Galileo E5a and E5b, BDS B2I, B3I and B2a) are within -0.8 to +0.3 dB.
  So B2b stays 7D and B1C 1P. L1C is not settled: it is 0.4 to 0.6 dB
  above B1C, which is pilot, and 1.5 dB above the other controls. None of
  the lab's receivers reports L1C data separately (the mosaic and UM980
  report the pilot, the X20P has no L1C in any signal plan, and the
  K901's MSM7 carries no L1C), so deciding between 1L and 1X needs the
  L1C data/pilot power split, which is not in the local documentation.
- MSM7 in these captures has no QZSS, so QZSS is not checked.

Over the two-hour K901 capture, `--from novb --vendor sinognss` against
`--from rtcm` confirms these: pseudorange within 5 mm and phase within
0.001 cycle of a whole number on every shared signal, and a Doppler
offset common to all signals at each epoch (median spread 0.001 ppb),
the clock drift, which varies from -21 to +96 ppb. MSM7 has phase on
about 80,000 records where RANGE has the phase lock flag clear; RANGE's
ADR on those differs from the MSM7 phase by arbitrary fractions of a
cycle, so the converter is right to drop it.

Against `convbin`, the OEM7 mapping (no `--vendor`) agrees on every
record, except:

- `convbin` drops Galileo E6C and E5AltBOC (its frequency limit).
- `convbin` reads glofreq as the frequency channel + 8 (`decode_rangeb`
  comments it "GLONASS FCN+8"), where the OEM7 and SinoGNSS manuals
  say + 7, so its GLONASS channels are one lower. `convobs` gives the
  channels MSM7 gives, so the golden test has to ignore the GLONASS
  channel.
- For a record without phase lock, `convbin` drops the Doppler and sets
  the half-cycle flag (2 records in the K803 capture).

### PPP

Run the RINEX from the long K901 capture through PPP (CSRS-PPP) as an
end-to-end check of carrier phase, and compare the result with the
antenna's known position.

## Open decisions

- The name and form of the `novmsg` type selecting the mapping
  (`RangeMapping` above).
- Whether the input format names should be `novb`/`nova` or name the
  log (`range`).
- Whether GPS and QZSS type 16 is 1L (pilot) or 1X (data and pilot).
- Whether `convobs` gets an option to drop Doppler without phase lock, as
  `--unc-omit-do-without-cp` does for OBSVM, or the golden test ignores
  those records.
