# NMEA GSV handling: bug fixes and F9T workaround

Background on the existing GSV/GSA flushing design lives in
`plan/archive/nmea-flush.md`; the relevant parts are summarised below.

This plan addresses two existing bugs, one missing comment, and a workaround
for an F9T firmware bug.

## Background: existing flush design

`gps/internal/nmea/nmeasats.go` combines GSV (satellites in view) and GSA
(satellites active in solution) sentences into a single `SatellitesMsg`.
GSV sentences arrive as multi-part "M of N" sequences; on NMEA 4.10+
receivers each GNSS constellation emits a separate GSV sequence per signal
ID, so we cannot predict up front how many sequences belong to one
navigation solution.

The buffer makes one assumption: the receiver outputs all sentences for a
single navigation solution in one burst with no idle gap (>= 0.1s of no
data) inside the burst. It does *not* assume that idle gaps separate every
pair of bursts (bandwidth may be saturated), and it makes no assumption
about the order of different sentence types within a burst.

Two flush triggers are used:

1. **`idle()`** -- the primary trigger. When the receiver pauses, emit
   whatever GSV/GSA data has accumulated.
2. **Repeated `(gnss, sigID)`** -- the backup trigger when no idle arrives.
   Seeing the same `(gnss, sigID)` key twice means a new cycle has started.

To keep the backup trigger from picking an arbitrary mid-cycle boundary
when we first start listening, the buffer uses a `haveBoundary` flag.
`possibleBoundary()` is called on `idle()` and on every format change
(different sentence type than the previous one); the first call sets
`haveBoundary` and clears the buffer, so initial partial data is
discarded. Once `haveBoundary` is true, subsequent calls are no-ops.

Duplicate `(gnss, sigID)` detection then drives the actual
`SatellitesMsg` emission between cycles when `idle()` doesn't arrive.

## Phase 1: existing bugs

### Bug 1: duplicate detection happens too late in the repeated sequence

In `gsvProcess`, the duplicate `(gnss, sigID)` check fires on the
*closing* sentence of a sequence (`gsv.numMsg == gsv.msgNum`). By the
time that closer arrives, every earlier sentence of the repeated
sequence has already been appended to `sb.gsvs` across previous
`process()` calls. The subsequent `flush()` therefore emits cycle 1
plus cycle 2's entire first completed sequence as a single
`SatellitesMsg`, and `clear()` discards that sequence's data along with
the rest.

The rollover has to be recognized at the *start* of the repeated
sequence so that cycle 1 can be flushed before any sentence of cycle 2
enters the buffer. The natural trigger is `msgNum == 1`, but a noisy
stream may drop the leading sentence of cycle 2 entirely; gating only
on `msgNum == 1` lets cycle 2 leak in when its first sentence arrives
mid-sequence. The fix is to treat every *sequence start* (as classified
by `checkMsgNum`) as a duplicate-check trigger and record the key in
`gsvKeys` there, so any same-key restart -- whether `msgNum == 1` or a
resumed mid-sequence -- triggers the flush.

The invariant we maintain: `sb.gsvs` may hold at most one sequence per
effective `(gnss, sigID)` key.

The flush boundary should fall between cycles, not anywhere inside
cycle 2's first sequence.

### Bug 2: `checkMsgNum` errors on legitimate `msgNum == 1`

The earlier `checkMsgNum` would report an error whenever a same-key
sequence restarted at `msgNum == 1` after a previously buffered same-key
sequence, even though that restart is exactly what we expect at the
start of a new sequence.

In the refactored design `checkMsgNum` returns
`(seqStart bool, err error)` and `seqStart == true` is the regime where
the duplicate-key check runs. `msgNum == 1` is unconditionally a
sequence start: `checkMsgNum` returns `(true, nil)` early, never
reaches the bogus error branch, and the duplicate-flush in `gsvProcess`
clears the buffer before any further classification. Phase 2's F9T
mitigation goes through the same `msgNum == 1` path, so it cannot
re-expose the bug either.

The error branches in `checkMsgNum` now only fire for genuine
malformed numbering (e.g. `msgNum >= 2` arriving with no compatible
predecessor in the buffer), which is the correct lost-data
diagnostic.

### Missing comment: pre-`haveBoundary` duplicate-flush is a startup compromise

`nmea-flush.md` documents the `haveBoundary` mechanism but doesn't
describe what the duplicate-`(gnss, sigID)` flush trigger does when
`haveBoundary` is still false. If we joined the stream mid-cycle,
`gsvKeys` starts recording from an arbitrary point, so the wrap from
"first key recorded" back to the same key brackets the tail of one cycle
plus the head of the next -- a partial, mixed-cycle dataset. The first
duplicate-flush in this state can emit such data.

This is accepted as a startup compromise: once `haveBoundary` is set
(via `idle()` or a format change), subsequent flushes are properly
aligned. A comment in `nmeasats.go` near the duplicate detection should
describe this honestly rather than imply the wrap always brackets exactly
one cycle. Note that `nmea-flush.md` explicitly disclaims any assumption
about sentence ordering within a burst, so we can't lean on order
consistency to justify the pre-`haveBoundary` flush.

## Phase 2: F9T firmware bug

### Symptom

A capture from a u-blox ZED-F9T contains bursts where two consecutive
GAGSV sequences carry the same signal ID 7, instead of the expected
sigID=7 then sigID=3 pattern. Comparing with the bursts that come out
correctly, the second sequence's payload corresponds to what should have
been sigID=3 (Galileo E5b) but is mislabeled as sigID=7 (E1) in the
firmware.

The bursts containing the bug appear roughly every other second in the
capture; the receiver alternates between correct and buggy output.

A correct burst (timestamp 02:54:54), GAGSV portion only -- sigID=7
followed by sigID=3:

```
$GAGSV,3,1,11,04,09,181,19,06,11,205,40,09,09,229,20,10,61,234,47,7*72
$GAGSV,3,2,11,11,32,223,42,12,81,299,39,16,28,332,26,23,30,110,29,7*76
$GAGSV,3,3,11,28,05,159,,31,61,044,31,33,33,029,13,7*4F
$GAGSV,3,1,11,04,09,181,21,06,11,205,42,09,09,229,27,10,61,234,53,3*7D
$GAGSV,3,2,11,11,32,223,42,12,81,299,48,16,28,332,30,23,30,110,32,3*79
$GAGSV,3,3,11,28,05,159,,31,61,044,36,33,33,029,22,3*4E
```

A buggy burst (timestamp 02:54:55), GAGSV portion only -- sigID=7 twice:

```
$GAGSV,3,1,11,04,09,181,19,06,11,205,40,09,09,229,20,10,61,234,47,7*72
$GAGSV,3,2,11,11,32,223,42,12,81,299,39,16,28,332,26,23,30,110,28,7*77
$GAGSV,3,3,11,28,05,159,,31,61,044,32,33,33,029,15,7*4A
$GAGSV,3,1,11,04,09,181,21,06,11,205,42,09,09,229,27,10,61,234,53,7*79
$GAGSV,3,2,11,11,32,223,42,12,81,299,48,16,28,332,30,23,30,110,32,7*7D
$GAGSV,3,3,11,28,05,159,,31,61,044,36,33,33,029,21,7*49
```

Both sequences in the buggy burst contain the same Galileo SVIDs (4, 6,
9, 10, 11, 12, 16, 23, 28, 31, 33) with identical azimuth and elevation
values; only CN0 differs. That's characteristic of two distinct signals
(E1 and E5b, which view the same satellites at the same geometry) rather
than literal duplicates.

Across the 56 GAL epochs in the capture, the receiver emitted the
correct (sigID=7 then sigID=3) pattern in 28 epochs and the buggy
(sigID=7 then sigID=7) pattern in 22 epochs -- the firmware bug is
intermittent, not consistent. Useful framing for the mitigation choice
below: dropping one of the two sequences loses CN0 data for a single
signal, not satellite-presence data.

Surrounding context for the buggy burst: the GAGSV sentences arrive
back-to-back with no idle gap and no other sentence type interleaved,
between the preceding GLGSV sequences and the following GBGSV sequences.

### What goes wrong today

Within a single burst we see:
1. GAGSV(sigID=7) sequence starts -> key `(GA, 7)` enters `gsvKeys`.
2. GAGSV(sigID=7) starts again, still inside the same burst.
3. The duplicate check in `gsvProcess` triggers a `flush()` mid-burst.
   The resulting `SatellitesMsg` is missing the GBGSV data that comes
   later in the burst, and the next `idle()` produces a second, partial
   `SatellitesMsg` for the same epoch.

This violates the "one sequence per `(gnss, sigID)` per burst" assumption
that the duplicate-flush trigger relies on.

### Relationship between Phase 1 Bug 1 and the F9T workaround

Both fire on the same observable trigger: a sequence start whose
effective key is already in `gsvKeys`. Phase 1's rule (flush) is the
correct response under the original "one sequence per key per cycle"
assumption. Phase 2's rule (drop the preceding sequence, don't flush)
is the correct response when the F9T violates that assumption. The two
rules apply different actions to overlapping triggers, so we must
disambiguate.

### Detection criterion

The disambiguator combines three conditions; F9T applies iff all hold:

- `gsv.msgNum == 1` (the new sequence is restarting at its head).
- `sb.lastFormat == "GSV"` (the immediately preceding sentence was
  also GSV, so no other sentence type sits between the two same-key
  GAGSV sequences).
- The trailing entries of `sb.gsvs` are a completed same-key sequence
  (the immediately preceding completed sequence's effective key
  matches gsv's). When this is true we drop those trailing entries.

Otherwise the trigger is a real cycle boundary and we flush.

"Effective key" means `(gnss, sigID)` normally and `(gnss, 0)` when
`mixedSigIDs` is in effect, matching the existing duplicate-flush
trigger. The F9T bug only manifests with explicit signal IDs and so
shouldn't interact with `mixedSigIDs` mode in practice, but the
detection criterion uses the same key the rest of the buffer does for
consistency.

We don't need to check for "no idle between" because an idle would
already have flushed and cleared the buffer, so `lastGSV` would not
exist. The `lastFormat == "GSV"` check covers the analogous case where
some non-GSV sentence (e.g. GSA) sat between the two same-key
sequences -- that's a real cycle boundary, not F9T.

### Failure mode: single-key cycles

The disambiguator fails when a receiver emits exactly one
`(gnss, sigID)` sequence per cycle (e.g., a GPS-only receiver with one
signal). For such a receiver, every cycle boundary looks like a
back-to-back same-key collision, so the Phase 2 rule would drop every
cycle's data instead of emitting it.

This requires both (a) absence of idle gaps between cycles (so the
backup trigger has to fire at all) and (b) a single-key cycle. Most
receivers cycle through multiple GNSS constellations, so the
immediately-preceding-key collision specifically indicates an
F9T-style violation rather than a cycle boundary. We accept this gap
in the disambiguator rather than introduce a more complex rule, but
should revisit if a receiver in this configuration shows up.

### Mitigation

When the F9T pattern is detected, drop the preceding sequence's GSV
entries from `sb.gsvs` and let the new sequence accumulate normally.
The key stays in `gsvKeys`, so a later cycle 2 (with a key collision
that *isn't* the immediate predecessor) still triggers the Phase 1
duplicate-flush.

Trade-off: we keep the newer of the two sequences and discard the
older. Since the F9T bug means at least one of the two is mislabeled
and we can't tell which payload is "really" sigID=7 vs sigID=3, keeping
the newer is a simple, predictable choice. As noted in the symptom
section, the cost is one signal's CN0 data per buggy burst, not
satellite-presence data. (Relabeling based on prior bursts is
explicitly out of scope.)

### Test

Add a test in `nmeasats_test.go` that feeds a back-to-back GAGSV-7 /
GAGSV-7 burst (modeled on the buggy burst above) and asserts:
- a single `SatellitesMsg` is emitted at the burst boundary,
- the message contains the GBGSV data that follows the duplicated GAGSV
  sequences,
- the preceding GAGSV-7 sequence's SVs are absent (only the newer one's
  data is retained).

## Sequencing

Phase 1 should land first and stand on its own. Phase 2 doesn't strictly
*build on* Phase 1 -- it adds a disambiguator for the trigger Phase 1
already handles. Landing them in order keeps each change small.

## Appendix: packet log excerpts

Raw packet log lines from a u-blox ZED-F9T capture, NMEA sentences only.
Two consecutive 1Hz bursts are shown: the first is correct (GAGSV sigID=7
then sigID=3), the second exhibits the firmware bug (GAGSV sigID=7 twice).

### Correct burst (02:54:54)

```
{"t":"2026-04-29T02:54:54.092852Z","tag":"NMEA","msg":"GNRMC","ascii":"$GNRMC,025454.00,A,1343.91014,N,10038.68424,E,0.009,,290426,,,A,V*1A\r\n"}
{"t":"2026-04-29T02:54:54.094291Z","tag":"NMEA","msg":"GNVTG","ascii":"$GNVTG,,T,,M,0.009,N,0.017,K,A*32\r\n"}
{"t":"2026-04-29T02:54:54.094291Z","tag":"NMEA","msg":"GNGGA","ascii":"$GNGGA,025454.00,1343.91014,N,10038.68424,E,1,12,0.54,-0.1,M,-27.4,M,,*4C\r\n"}
{"t":"2026-04-29T02:54:54.094291Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,11,30,19,07,06,17,14,22,,,,,0.98,0.54,0.82,1*02\r\n"}
{"t":"2026-04-29T02:54:54.094308Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,80,79,83,78,82,81,,,,,,,0.98,0.54,0.82,2*09\r\n"}
{"t":"2026-04-29T02:54:54.094308Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,23,11,16,12,31,06,09,10,,,,,0.98,0.54,0.82,3*00\r\n"}
{"t":"2026-04-29T02:54:54.094563Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,06,02,11,03,07,10,20,09,29,,,,0.98,0.54,0.82,4*0F\r\n"}
{"t":"2026-04-29T02:54:54.097346Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,1,14,01,02,037,,03,22,059,19,06,65,268,48,07,12,164,34,1*61\r\n"}
{"t":"2026-04-29T02:54:54.097346Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,2,14,09,03,139,,11,28,233,43,13,02,246,,14,69,058,37,1*6A\r\n"}
{"t":"2026-04-29T02:54:54.097369Z","tag":"UBX","msg":"MON-VER","bin":"b5620a0400000e34","out":true}
{"t":"2026-04-29T02:54:54.097398Z","ascii":"VERSIONB\r\n","out":true}
{"t":"2026-04-29T02:54:54.097355Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,3,14,17,37,008,22,19,35,328,36,21,01,203,,22,62,015,35,1*6B\r\n"}
{"t":"2026-04-29T02:54:54.097357Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,4,14,24,02,312,,30,38,192,46,1*65\r\n"}
{"t":"2026-04-29T02:54:54.097357Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,1,14,01,02,037,,03,22,059,22,06,65,268,48,07,12,164,25,6*6E\r\n"}
{"t":"2026-04-29T02:54:54.098491Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,2,14,09,03,139,,11,28,233,44,13,02,246,,14,69,058,40,6*6A\r\n"}
{"t":"2026-04-29T02:54:54.098575Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,3,14,17,37,008,25,19,35,328,,21,01,203,,22,62,015,,6*68\r\n"}
{"t":"2026-04-29T02:54:54.098651Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,4,14,24,02,312,,30,38,192,40,6*64\r\n"}
{"t":"2026-04-29T02:54:54.098818Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,1,07,67,05,133,,78,33,147,43,79,83,064,49,80,32,338,38,1*73\r\n"}
{"t":"2026-04-29T02:54:54.099315Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,2,07,81,23,014,33,82,48,321,48,83,28,253,48,1*41\r\n"}
{"t":"2026-04-29T02:54:54.100731Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,1,07,67,05,133,,78,33,147,46,79,83,064,49,80,32,338,27,3*7A\r\n"}
{"t":"2026-04-29T02:54:54.100813Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,2,07,81,23,014,15,82,48,321,47,83,28,253,47,3*47\r\n"}
{"t":"2026-04-29T02:54:54.101455Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,1,11,04,09,181,19,06,11,205,40,09,09,229,20,10,61,234,47,7*72\r\n"}
{"t":"2026-04-29T02:54:54.10154Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,2,11,11,32,223,42,12,81,299,39,16,28,332,26,23,30,110,29,7*76\r\n"}
{"t":"2026-04-29T02:54:54.101625Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,3,11,28,05,159,,31,61,044,31,33,33,029,13,7*4F\r\n"}
{"t":"2026-04-29T02:54:54.101787Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,1,11,04,09,181,21,06,11,205,42,09,09,229,27,10,61,234,53,3*7D\r\n"}
{"t":"2026-04-29T02:54:54.102071Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,2,11,11,32,223,42,12,81,299,48,16,28,332,30,23,30,110,32,3*79\r\n"}
{"t":"2026-04-29T02:54:54.102149Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,3,11,28,05,159,,31,61,044,36,33,33,029,22,3*4E\r\n"}
{"t":"2026-04-29T02:54:54.102324Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,1,17,01,42,105,30,02,60,233,46,03,68,147,46,04,21,100,,1*7B\r\n"}
{"t":"2026-04-29T02:54:54.102328Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,2,17,05,38,251,,06,44,140,46,07,36,175,47,08,40,016,22,1*77\r\n"}
{"t":"2026-04-29T02:54:54.102409Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,3,17,09,32,166,37,10,47,180,48,11,48,309,48,12,10,182,,1*7F\r\n"}
{"t":"2026-04-29T02:54:54.102496Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,4,17,16,50,342,,20,15,164,41,27,10,043,,29,36,283,45,1*73\r\n"}
{"t":"2026-04-29T02:54:54.102574Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,5,17,30,46,354,,1*43\r\n"}
{"t":"2026-04-29T02:54:54.102657Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,1,17,01,42,105,,02,60,233,,03,68,147,,04,21,100,,*49\r\n"}
{"t":"2026-04-29T02:54:54.102737Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,2,17,05,38,251,,06,44,140,,07,36,175,,08,40,016,,*47\r\n"}
{"t":"2026-04-29T02:54:54.102817Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,3,17,09,32,166,46,10,47,180,49,11,48,309,,12,10,182,,3*76\r\n"}
{"t":"2026-04-29T02:54:54.103078Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,4,17,16,50,342,,20,15,164,,27,10,043,,29,36,283,,3*75\r\n"}
{"t":"2026-04-29T02:54:54.103162Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,5,17,30,46,354,,3*41\r\n"}
{"t":"2026-04-29T02:54:54.103162Z","tag":"NMEA","msg":"GNGLL","ascii":"$GNGLL,1343.91014,N,10038.68424,E,025454.00,A,A*7B\r\n"}
```

### Buggy burst (02:54:55)

```
{"t":"2026-04-29T02:54:55.086931Z","tag":"NMEA","msg":"GNRMC","ascii":"$GNRMC,025455.00,A,1343.91015,N,10038.68425,E,0.010,,290426,,,A,V*13\r\n"}
{"t":"2026-04-29T02:54:55.087286Z","tag":"NMEA","msg":"GNVTG","ascii":"$GNVTG,,T,,M,0.010,N,0.018,K,A*35\r\n"}
{"t":"2026-04-29T02:54:55.087369Z","tag":"NMEA","msg":"GNGGA","ascii":"$GNGGA,025455.00,1343.91015,N,10038.68425,E,1,12,0.54,-0.1,M,-27.4,M,,*4D\r\n"}
{"t":"2026-04-29T02:54:55.087431Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,11,30,19,07,06,17,14,22,,,,,0.98,0.54,0.82,1*02\r\n"}
{"t":"2026-04-29T02:54:55.08752Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,80,79,83,78,82,81,,,,,,,0.98,0.54,0.82,2*09\r\n"}
{"t":"2026-04-29T02:54:55.087614Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,23,11,16,12,31,06,09,10,,,,,0.98,0.54,0.82,3*00\r\n"}
{"t":"2026-04-29T02:54:55.087614Z","tag":"NMEA","msg":"GNGSA","ascii":"$GNGSA,A,3,06,02,11,03,07,10,20,09,29,,,,0.98,0.54,0.82,4*0F\r\n"}
{"t":"2026-04-29T02:54:55.087816Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,1,14,01,02,037,,03,22,059,19,06,65,268,48,07,12,164,34,1*61\r\n"}
{"t":"2026-04-29T02:54:55.088309Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,2,14,09,03,139,,11,28,233,43,13,02,246,,14,69,058,37,1*6A\r\n"}
{"t":"2026-04-29T02:54:55.08855Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,3,14,17,37,008,22,19,35,328,36,21,01,203,,22,62,015,34,1*6A\r\n"}
{"t":"2026-04-29T02:54:55.088627Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,4,14,24,02,312,,30,38,192,46,1*65\r\n"}
{"t":"2026-04-29T02:54:55.088723Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,1,14,01,02,037,,03,22,059,22,06,65,268,48,07,12,164,24,6*6F\r\n"}
{"t":"2026-04-29T02:54:55.088806Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,2,14,09,03,139,,11,28,233,44,13,02,246,,14,69,058,39,6*64\r\n"}
{"t":"2026-04-29T02:54:55.089556Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,3,14,17,37,008,25,19,35,328,,21,01,203,,22,62,015,,6*68\r\n"}
{"t":"2026-04-29T02:54:55.089556Z","tag":"NMEA","msg":"GPGSV","ascii":"$GPGSV,4,4,14,24,02,312,,30,38,192,40,6*64\r\n"}
{"t":"2026-04-29T02:54:55.105725Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,1,07,67,05,133,,78,33,147,43,79,83,064,49,80,32,338,38,1*73\r\n"}
{"t":"2026-04-29T02:54:55.105737Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,2,07,81,23,014,33,82,48,321,48,83,28,253,48,1*41\r\n"}
{"t":"2026-04-29T02:54:55.10635Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,1,07,67,05,133,,78,33,147,46,79,83,064,49,80,32,338,27,3*7A\r\n"}
{"t":"2026-04-29T02:54:55.106359Z","tag":"NMEA","msg":"GLGSV","ascii":"$GLGSV,2,2,07,81,23,014,15,82,48,321,47,83,28,253,47,3*47\r\n"}
{"t":"2026-04-29T02:54:55.10659Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,1,11,04,09,181,19,06,11,205,40,09,09,229,20,10,61,234,47,7*72\r\n"}
{"t":"2026-04-29T02:54:55.106682Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,2,11,11,32,223,42,12,81,299,39,16,28,332,26,23,30,110,28,7*77\r\n"}
{"t":"2026-04-29T02:54:55.106687Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,3,11,28,05,159,,31,61,044,32,33,33,029,15,7*4A\r\n"}
{"t":"2026-04-29T02:54:55.107201Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,1,11,04,09,181,21,06,11,205,42,09,09,229,27,10,61,234,53,7*79\r\n"}
{"t":"2026-04-29T02:54:55.107288Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,2,11,11,32,223,42,12,81,299,48,16,28,332,30,23,30,110,32,7*7D\r\n"}
{"t":"2026-04-29T02:54:55.107359Z","tag":"NMEA","msg":"GAGSV","ascii":"$GAGSV,3,3,11,28,05,159,,31,61,044,36,33,33,029,21,7*49\r\n"}
{"t":"2026-04-29T02:54:55.107464Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,1,17,01,42,105,30,02,60,233,46,03,68,147,46,04,21,100,,1*7B\r\n"}
{"t":"2026-04-29T02:54:55.107546Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,2,17,05,38,251,,06,44,140,46,07,36,175,47,08,40,016,22,1*77\r\n"}
{"t":"2026-04-29T02:54:55.107632Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,3,17,09,32,166,37,10,47,180,48,11,48,309,48,12,10,182,,1*7F\r\n"}
{"t":"2026-04-29T02:54:55.107711Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,4,17,16,50,342,,20,15,164,41,27,10,043,,29,36,283,45,1*73\r\n"}
{"t":"2026-04-29T02:54:55.107819Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,5,17,30,46,354,,1*43\r\n"}
{"t":"2026-04-29T02:54:55.108279Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,1,17,01,42,105,,02,60,233,,03,68,147,,04,21,100,,*49\r\n"}
{"t":"2026-04-29T02:54:55.108362Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,2,17,05,38,251,,06,44,140,,07,36,175,,08,40,016,,*47\r\n"}
{"t":"2026-04-29T02:54:55.10848Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,3,17,09,32,166,46,10,47,180,49,11,48,309,,12,10,182,,3*76\r\n"}
{"t":"2026-04-29T02:54:55.10848Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,4,17,16,50,342,,20,15,164,,27,10,043,,29,36,283,,3*75\r\n"}
{"t":"2026-04-29T02:54:55.108533Z","tag":"NMEA","msg":"GBGSV","ascii":"$GBGSV,5,5,17,30,46,354,,3*41\r\n"}
{"t":"2026-04-29T02:54:55.108533Z","tag":"NMEA","msg":"GNGLL","ascii":"$GNGLL,1343.91015,N,10038.68425,E,025455.00,A,A*7A\r\n"}
```
