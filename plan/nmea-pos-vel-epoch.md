# NMEA position, velocity, and epoch detection

Prerequisite: [position-velocity-messages.md](position-velocity-messages.md) (defines `PosGeoMsg`, `VelGeoMsg`, `Speed` type, and `MsgHandler` methods).

Prerequisite: [nav-epoch.md](nav-epoch.md) (defines `NavEpochMsg` and `MsgHandler.NavEpoch`).

Enables: [solution-metadata.md](solution-metadata.md) (populates `NavEpochMsg` with fix quality, DOPs, corrections from GGA/GSA/RMC metadata).

Enables: [multi-prot-nav-epoch.md](multi-prot-nav-epoch.md) (cross-protocol epoch coordination via `NavEpochManager`).

## Motivation

The NMEA packet processor currently extracts only time (from RMC and ZDA) and satellites (from GSV/GSA). Position and velocity data in RMC, GGA, and VTG sentences is ignored. Additionally, NMEA does not emit `NavEpochMsg`.

A navigation epoch is a single navigation solution. Within one epoch, a receiver may output multiple NMEA sentences that describe the same solution in different ways: RMC gives lat/lon and speed, GGA gives lat/lon with height, VTG gives speed in different units. These are not independent observations — they are different views of the same fix. Without epoch boundaries, an application has no way to distinguish "two messages describing the same fix" from "two messages from two successive fixes". This distinction is essential for any application that consumes position/velocity data. For example, an application plotting positions needs one dot per epoch, not one dot per position message. An application selecting the best position source within an epoch (e.g. preferring GGA over RMC because GGA includes height) needs to know which messages belong together.

This plan adds:
1. Position/velocity extraction from RMC, GGA, and VTG (Step 5 of [position-velocity-messages.md](position-velocity-messages.md)).
2. A `navEpochBuffer` that detects epoch boundaries and emits `NavEpochMsg`.

Position and velocity messages are emitted immediately (low latency). `NavEpochMsg` is emitted at the end of each epoch, after all position/velocity messages for that epoch have been dispatched. The parsers receive a `*NavEpochMsg` parameter so that solution-metadata.md can later populate it with fix quality, DOPs, and corrections — but this plan does not write any metadata fields.

## NMEA field conventions

Field numbers below refer to `ApprovedSentence.Fields` indices (0-indexed from the first data field after the address).

### Latitude/longitude

NMEA represents latitude as `DDMM.MMMM` (degrees and decimal minutes) and longitude as `DDDMM.MMMM`. A separate N/S or E/W field indicates the sign.

```
latitude  = DD + MM.MMMM / 60.0    (positive north, negative south)
longitude = DDD + MM.MMMM / 60.0   (positive east, negative west)
```

Convert to `gpsprot.Angle` via `DegreesFromFloat`.

### Speed

RMC field 6: speed over ground in knots. VTG field 6: speed in km/h. Convert to `gpsprot.Speed` via `MetersPerSecondFromFloat` (knots * 1852/3600; km/h / 3.6).

### Course

RMC field 7 and VTG field 0: course over ground in degrees true north. Convert to `gpsprot.Angle` via `DegreesFromFloat`.

### Height

GGA field 8: altitude above mean sea level in meters. GGA field 10: geoid separation in meters (geoid height above WGS-84 ellipsoid). Ellipsoidal height = altMSL + geoidSep. Convert to `gpsprot.Length` via `Meters`.

## Design

### One parser per sentence

Each sentence gets a single parser that returns all `gpsprot` messages the sentence can produce and receives a `*NavEpochMsg` for future metadata accumulation. This works well because NMEA sentences mix concerns: RMC carries time, position, and velocity in one sentence.

All code goes in `nmea.go`.

#### parseRMC

Replaces the existing time-only `parseRMC`:

```go
func parseRMC(sen *ApprovedSentence, epoch *gpsprot.NavEpochMsg) (
    tm *gpsprot.TimeMsg, pos *gpsprot.PosGeoMsg, vel *gpsprot.VelGeoMsg, err error,
)
```

Returns nil for all messages when status (field 1) is not "A".

- **TimeMsg**: from fields 0 (time) and 8 (date). Nil when time or date is empty.
- **PosGeoMsg**: `LatLon` from fields 2-5. `Height` and `HeightMSL` unset. Nil when lat/lon fields are empty.
- **VelGeoMsg**: `GroundSpeed` from field 6 (knots). `Course` from field 7 (omitted when empty). Nil when speed field is empty.

Sets `Tag` and `NativeMsgID` (e.g. "GPRMC") on each non-nil message.

#### parseGGA

```go
func parseGGA(sen *ApprovedSentence, epoch *gpsprot.NavEpochMsg) (
    pos *gpsprot.PosGeoMsg, err error,
)
```

- **PosGeoMsg**: `LatLon` from fields 1-4. `HeightMSL` from field 8. `Height` (ellipsoidal) = field 8 + field 10 when both present. Nil when quality (field 5) is "0" or "" or lat/lon fields are empty.

The existing `parseGGA` in `nmeasats.go` (which extracts only `numSV`) is unused and can be removed.

#### parseVTG

```go
func parseVTG(sen *ApprovedSentence, epoch *gpsprot.NavEpochMsg) (
    vel *gpsprot.VelGeoMsg, err error,
)
```

- **VelGeoMsg**: `GroundSpeed` from field 6 (km/h). `Course` from field 0 (omitted when empty). Nil when mode (field 8, when present) is "N" or speed field is empty.

### Parsing helpers

```go
func parseLatLon(latField, nsField, lonField, ewField string) ([2]gpsprot.Angle, bool)
func parseFloatField(s string) (float64, bool)
```

`parseLatLon` splits at position `len(s)-7` to separate degrees from minutes, converts minutes to degrees, applies N/S E/W sign.

### Dispatch integration

The `Dispatch` method is extended. Each case calls the sentence parser, notifies the `navEpochBuffer` (for RMC and GGA which carry time of day), and dispatches all non-nil messages. The `navEpochBuffer.timeOfDay` call happens *before* the parser runs, so that any epoch flush occurs before the new epoch's data is parsed.

```go
case "RMC":
    p.nb.timeOfDay(sen.Fields[0], tRead, h)
    tm, pos, vel, err := parseRMC(sen, p.nb.epoch())
    if err != nil {
        return false, err
    }
    if h != nil {
        if tm != nil { h.Time(tm, tRead) }
        if pos != nil { h.PosGeo(pos, tRead) }
        if vel != nil { h.VelGeo(vel, tRead) }
    }
    return true, nil
case "GGA":
    p.nb.timeOfDay(sen.Fields[0], tRead, h)
    pos, err := parseGGA(sen, p.nb.epoch())
    if err != nil {
        return false, err
    }
    if h != nil && pos != nil { h.PosGeo(pos, tRead) }
    return true, nil
case "VTG":
    vel, err := parseVTG(sen, p.nb.epoch())
    if err != nil {
        return false, err
    }
    if h != nil && vel != nil { h.VelGeo(vel, tRead) }
    return true, nil
```

### navEpochBuffer

Tracks epoch boundaries for NMEA, separate from `satellitesBuffer`.

#### Epoch boundary detection

RMC and GGA carry UTC time of day in field 0. The buffer tracks the current time-of-day string. When a different time of day arrives, the previous epoch is flushed. VTG and GSA have no time of day and simply belong to whatever epoch is current.

```go
type navEpochBuffer struct {
    curTimeOfDay string
    curMsg       *gpsprot.NavEpochMsg
    tRead        time.Time
}

func (nb *navEpochBuffer) epoch() *gpsprot.NavEpochMsg {
    return nb.curMsg
}

func (nb *navEpochBuffer) timeOfDay(tod string, tRead time.Time, h gpsprot.MsgHandler) {
    if nb.curMsg != nil && tod != nb.curTimeOfDay {
        nb.flush(h)
    }
    if nb.curMsg == nil {
        nb.curMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
        nb.tRead = tRead
    }
    nb.curTimeOfDay = tod
}

func (nb *navEpochBuffer) flush(h gpsprot.MsgHandler) {
    if nb.curMsg != nil && h != nil {
        nb.curMsg.Tag = Tag
        h.NavEpoch(nb.curMsg, nb.tRead)
    }
    nb.curMsg = nil
}
```

The `Idle` method calls `nb.flush(...)` in addition to the existing `sb.idle(...)`.

#### Hook for solution-metadata.md

The `epoch()` method returns the current `*NavEpochMsg`. When solution-metadata.md extends the parsers to write quality/DOP/correction metadata from GGA, GSA, and RMC, those values go into this pointer.

#### Hook for multi-prot-nav-epoch.md

When `NavEpochManager` is implemented, the flush path is refactored: the NMEA processor calls `manager.EpochStarted(p, tRead)` on time-of-day change and implements `FlushNavEpoch() *NavEpochMsg`.

## Implementation steps

### Step 1: Parsing helpers and parseRMC

Add `parseLatLon`, `parseFloatField` to `nmea.go`. Replace the existing `parseRMC` with the new version. Update the RMC case in `Dispatch`.

Tests:
- Existing RMC time tests must still pass
- `parseLatLon`: equator, prime meridian, N/S/E/W, empty fields
- RMC position and velocity from known sentences
- Nil returns when status is "V"

Run `make test`.

### Step 2: parseGGA

Add `parseGGA` to `nmea.go`. Add the GGA case to `Dispatch`. Remove the unused `parseGGA` from `nmeasats.go` if it does not break `satellitesBuffer`.

Tests:
- Position with height (MSL + geoid sep -> ellipsoidal)
- Nil when quality is "0"
- Empty geoid separation (Height unset, HeightMSL set)

Run `make test`.

### Step 3: parseVTG

Add `parseVTG` to `nmea.go`. Add the VTG case to `Dispatch`.

Tests:
- Speed from km/h field, course
- Nil when mode is "N"
- Empty course field

Run `make test`.

### Step 4: navEpochBuffer

Add `navEpochBuffer` struct and methods. Add `nb` field to `PacketProcessor`. Wire up `timeOfDay` in RMC and GGA cases. Wire up `flush` in `Idle`.

Tests:
- Epoch boundary: two epochs with different time of day, verify `NavEpoch` emitted between them
- Idle flush: one epoch then `Idle`, verify `NavEpoch` emitted
- No emission before first boundary
- `Tag` is "NMEA", `StartTime` is correct
