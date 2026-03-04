# CASIC V6 NAV2 empirical verification notes

Tested on ATGM332D-6N74 (V6 firmware) at /dev/ttyUSB0 115200 baud, 2026-03-03.

## NAV2-SAT not supported

NAV2-SAT (0x11 0x04) enable via CFG-MSG is ACK'd but the receiver never outputs any NAV2-SAT messages. All other NAV2 messages work (SOL, PVH, DOP, TIMEUTC, SIG). NAV2-SIG is a superset of NAV2-SAT (per-signal rather than per-satellite, with additional correction and solution flag fields), so NAV2-SIG should be used instead.

## GNSSID mapping (0a)

Verified by enabling all constellations (`gnss-all`) and cross-referencing NAV2-SIG `gnssid` field with NMEA GSV talker IDs and SigID (which equals CFG-NAVBAND bit position, see below).

Confirmed with single-constellation test: Galileo-only produced only GNSSID=3 entries.

| gnssid | System  | Evidence |
|--------|---------|----------|
| 0      | GPS     | SigID=0 (GPS L1C/A, bit 0); SVIDs match GPGSV |
| 1      | BDS     | SigID=10/11/14 (B1I GEO/MEO, B1C); SVIDs match BDGSV |
| 2      | GLONASS | SigID=5 (GLO L1, bit 5); FreqID matches known GLONASS frequency slots |
| 3      | Galileo | SigID=7 (GAL E1, bit 7); SVIDs match GAGSV; Galileo-only test confirmed |
| 4      | QZSS    | SigID=19 (QZSS L1C/A, bit 19); SVIDs match GQGSV |
| 5      | SBAS    | Not observed (no SBAS SVs in view) |
| 6      | IRNSS   | Not observed (no IRNSS SVs in view) |

This is the same numbering as V5 `casbin.GNSSID` (GPS=0, BDS=1, GLN=2) and `Nav2TimeSrc` (GPS=0, BDS=1, GLN=2, GAL=3, IRN=4), extended with QZSS=4, SBAS=5, IRNSS=6.

The plan's predicted ordering (0=GPS, 1=GLN, 2=GAL, 3=BDS) was wrong. The actual ordering follows the ZKW convention used throughout V5 and V6.

## Signal ID mapping (0b)

SigID in NAV2-SIG equals the CFG-NAVBAND bit position. This is NOT sequential numbering.

| SigID | Signal     | CFG-NAVBAND bit | Observed |
|-------|------------|-----------------|----------|
| 0     | GPS L1C/A  | 0               | Yes      |
| 2     | GPS L5     | 2               | No (not tracked during test) |
| 3     | SBAS L1    | 3               | No (no SBAS SVs) |
| 4     | SBAS L5    | 4               | No |
| 5     | GLO L1     | 5               | Yes      |
| 7     | GAL E1     | 7               | Yes      |
| 8     | GAL E5a    | 8               | No (not tracked during test) |
| 10    | BDS B1I GEO | 10             | Yes (SVIDs 2, 3, 60) |
| 11    | BDS B1I MEO | 11             | Yes (SVIDs 6-45) |
| 14    | BDS B1C    | 14              | Yes (dual-band BDS MEO SVs) |
| 15    | BDS B2a    | 15              | No (not tracked during test) |
| 19    | QZSS L1C/A | 19             | Yes      |
| 21    | QZSS L5    | 21              | No (not tracked during test) |
| 23    | IRNSS L5   | 23              | No (no IRNSS SVs) |

## SVID offsets (0c)

| System   | SVID in NAV2-SIG | gpsprot.SVID.Num expects | Conversion |
|----------|------------------|--------------------------|------------|
| GPS      | Raw PRN (1-32)   | Raw PRN                  | Direct     |
| BDS      | Raw PRN          | Raw PRN                  | Direct     |
| GLONASS  | Slot number (1-24) | Slot number            | Direct     |
| Galileo  | Raw PRN          | Raw PRN                  | Direct     |
| QZSS     | Satellite number (1-10) | PRN-192 (same thing) | Direct |
| SBAS     | Not observed     | PRN-100                  | Subtract 100 |
| IRNSS    | Not observed     | Raw PRN                  | Direct (assumed) |

QZSS verified: GQGSV shows SVIDs 2 and 7, matching NAV2-SIG GNSSID=4 SVIDs 2 and 7. These are QZSS satellite numbers (J-02, J-07), which happen to equal PRN-192.

SBAS not observed, but the receiver sends raw numbers for all other constellations, so it almost certainly sends raw SBAS PRNs (120-158). Subtract 100 for `gpsprot.SVID.Num`.

## FreqID field

For GLONASS, FreqID = frequency_channel + 8, mapping [-7,+6] to [1,14]. Verified:

| SVID | FreqID | Freq channel | Known |
|------|--------|-------------|-------|
| 3    | 13     | +5          | R03=+5 |
| 4    | 14     | +6          | R04=+6 |
| 15   | 8      | 0           | R15=0  |
| 16   | 7      | -1          | R16=-1 |
| 18   | 5      | -3          | R18=-3 |
| 19   | 11     | +3          | R19=+3 |

For non-GLONASS constellations, FreqID appears to equal SVID (unused/undefined).

## Impact on plan

1. **GNSSID mapping**: Both `nav2GNSSIDToGNSS` and `nav2TimeSrcToGNSS` use the same numbering (GPS=0, BDS=1, GLN=2, GAL=3). They can share a single mapping function extended with QZSS=4, SBAS=5, IRNSS=6. The existing V5 `gnssIDToGNSS` could also be extended.

2. **SigID mapping**: Use a lookup table indexed by CFG-NAVBAND bit position (not sequential). The mapping is `sigID -> gpsprot.Signal` keyed by GNSSID (since the same SigID value means different signals for different constellations -- though in practice each SigID is unique because different constellations use different bit ranges).

3. **NAV2-SAT**: Remove NAV2-SAT from the implementation plan. Use NAV2-SIG for both satellite info extraction and correction disambiguation.

4. **SVID handling**: No offsets needed for GPS, BDS, GLONASS, Galileo, QZSS. SBAS may need PRN-100 offset but cannot verify without SBAS SVs in view.

## NAV2-SIG trailing bytes

NAV2-SIG packets from the ATGM332D-6N74 consistently contain 4 extra bytes beyond the documented `8+16*N` payload structure (where N=NumTrkTot). The spec (ZKW F8 section 3.9.7) documents `N=0 to numTrkTot-1` with no mention of trailing data.

Observations from 60 seconds of capture (2026-03-04):
- All 60 packets had exactly 4 trailing bytes
- The trailing bytes change when NumTrkTot changes:
  - NumTrkTot=46: `c0c70410` (2 packets)
  - NumTrkTot=47: `14001105` (47 packets)
  - NumTrkTot=48: `70000510` (11 packets)
- Within a given NumTrkTot value, the trailing bytes remain constant
- The meaning is unknown; 32 bits is insufficient for a per-track bitmask (48 tracks observed)

Design decision: `Nav2Sig` implements `AllowTrailing` so ParseMsg accepts arbitrary trailing bytes. The array size is determined by `NumTrkTot` from the fixed header, not computed from payload length. This is robust against future firmware changes that might alter the trailing data.
