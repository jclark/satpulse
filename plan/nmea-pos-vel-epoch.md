# NMEA position, velocity, and epoch detection

Prerequisite: [position-velocity-messages.md](position-velocity-messages.md) (defines `PosGeoMsg`, `VelGeoMsg`, `Speed` type, and `MsgHandler` methods).

Prerequisite: [nav-epoch.md](nav-epoch.md) (defines `NavEpochMsg` and `MsgHandler.NavEpoch`).

Prerequisite: [nmea-ext-handler-epoch.md](nmea-ext-handler-epoch.md) (defines `NavEpoch` struct, `CheckEpoch` helper, revised `ExtSentenceHandler` interface, and `handleEpoch`/`flushEpoch` on `PacketProcessor`).

Enables: [solution-metadata.md](solution-metadata.md) (populates `NavEpochMsg` with fix quality, DOPs, corrections from GGA/GSA/RMC metadata).

Enables: [multi-prot-nav-epoch.md](multi-prot-nav-epoch.md) (cross-protocol epoch coordination via `NavEpochManager`).

## Motivation

The NMEA packet processor currently extracts only time (from RMC and ZDA) and satellites (from GSV/GSA). Position and velocity data in RMC, GGA, and VTG sentences is ignored. Additionally, NMEA does not emit `NavEpochMsg`.

A navigation epoch is a single navigation solution. Within one epoch, a receiver may output multiple NMEA sentences that describe the same solution in different ways: RMC gives lat/lon and speed, GGA gives lat/lon with height, VTG gives speed in different units. These are not independent observations — they are different views of the same fix. Without epoch boundaries, an application has no way to distinguish "two messages describing the same fix" from "two messages from two successive fixes". This distinction is essential for any application that consumes position/velocity data. For example, an application plotting positions needs one dot per epoch, not one dot per position message. An application selecting the best position source within an epoch (e.g. preferring GGA over RMC because GGA includes height) needs to know which messages belong together.

This plan adds:
1. Position/velocity extraction from RMC, GGA, and VTG (Step 5 of [position-velocity-messages.md](position-velocity-messages.md)).
2. Epoch boundary detection for standard NMEA sentences, using the `NavEpoch`/`CheckEpoch`/`handleEpoch` infrastructure from [nmea-ext-handler-epoch.md](nmea-ext-handler-epoch.md).

Position and velocity messages are emitted immediately (low latency). `NavEpochMsg` is emitted at the end of each epoch, after all position/velocity messages for that epoch have been dispatched. The parsers receive a `*NavEpoch` parameter so that solution-metadata.md can later populate the embedded `NavEpochMsg` with fix quality, DOPs, and corrections — but this plan does not write any metadata fields.

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

Each sentence gets a single parser that calls `CheckEpoch` (when it has time-of-day) and returns all `gpsprot` messages the sentence can produce. The parser receives and returns `*NavEpoch`. This works well because NMEA sentences mix concerns: RMC carries time, position, and velocity in one sentence.

The parser that creates a `*Msg` is responsible for setting `Tag` and `NativeMsgID` on it.

All code goes in `nmea.go`. Add a comment above the sentence parsers documenting the convention that each parser sets `Tag` and `NativeMsgID` on the messages it creates.

#### parseRMC

Replaces the existing time-only `parseRMC`:

```go
func parseRMC(sen *ApprovedSentence, epoch *NavEpoch) (
    *gpsprot.MsgBundle, *NavEpoch, error,
)
```

Always calls `CheckEpoch(epoch, sen.Fields[0])` to participate in epoch detection. Always returns a non-nil bundle containing at least a `TimeMsg`. When status (field 1) is not "A", the bundle contains only a `TimeMsg` with nil `UTCTime`.

- **TimeMsg**: always present. `UTCTime` populated from fields 0 (time) and 8 (date) when status is "A" and both fields are non-empty; nil otherwise.
- **PosGeoMsg**: `LatLon` from fields 2-5. `Height` and `HeightMSL` unset. Nil when status is not "A" or lat/lon fields are empty.
- **VelGeoMsg**: `GroundSpeed` from field 6 (knots). `Course` from field 7 (omitted when empty). Nil when status is not "A" or speed field is empty.

Sets `Tag` and `NativeMsgID` (e.g. "GPRMC") on each message. The `epoch` parameter is unused for now (hook for solution-metadata.md).

#### parseGGA

```go
func parseGGA(sen *ApprovedSentence, epoch *NavEpoch) (
    pos *gpsprot.PosGeoMsg, newEpoch *NavEpoch, err error,
)
```

Calls `CheckEpoch(epoch, sen.Fields[0])`. Nil when quality (field 5) is "0" or "" or lat/lon fields are empty.

- **PosGeoMsg**: `LatLon` from fields 1-4. `HeightMSL` from field 8. `Height` (ellipsoidal) = field 8 + field 10 when both present.

The existing `parseGGA` in `nmeasats.go` (which extracts only `numSV`) is unused and can be removed.

#### parseVTG

```go
func parseVTG(sen *ApprovedSentence, epoch *NavEpoch) (
    vel *gpsprot.VelGeoMsg, newEpoch *NavEpoch, err error,
)
```

Calls `CheckEpoch(epoch, "")` — VTG has no time-of-day but participates in the epoch.

- **VelGeoMsg**: `GroundSpeed` from field 6 (km/h). `Course` from field 0 (omitted when empty). Nil when mode (field 8, when present) is "N" or speed field is empty.

### Parsing helpers

```go
func parseLatLon(latField, nsField, lonField, ewField string) ([2]gpsprot.Angle, bool)
func parseFloatField(s string) (float64, bool)
```

`parseLatLon` finds the decimal point and splits 2 characters before it to separate degrees from minutes (minutes are always `MM.xxx`), converts minutes to degrees, applies N/S E/W sign.

### Dispatch integration

The `Dispatch` method is extended. Each case calls the sentence parser, then calls `handleEpoch` (from [nmea-ext-handler-epoch.md](nmea-ext-handler-epoch.md)) with the returned `*NavEpoch`, then dispatches messages. This is the same `handleEpoch` used by the ext handler path — the post-call epoch logic is uniform.

```go
case "RMC":
    bundle, newEpoch, err := parseRMC(sen, p.curNavEpoch)
    if err != nil {
        return false, err
    }
    p.handleEpoch(newEpoch, tRead)
    bundle.Dispatch(h, tRead)
    return true, nil
case "GGA":
    pos, newEpoch, err := parseGGA(sen, p.curNavEpoch)
    if err != nil {
        return false, err
    }
    p.handleEpoch(newEpoch, tRead)
    if h != nil && pos != nil { h.PosGeo(pos, tRead) }
    return true, nil
case "VTG":
    vel, newEpoch, err := parseVTG(sen, p.curNavEpoch)
    if err != nil {
        return false, err
    }
    p.handleEpoch(newEpoch, tRead)
    if h != nil && vel != nil { h.VelGeo(vel, tRead) }
    return true, nil
```

## Implementation steps

### Step 1: Parsing helpers and parseRMC

Add `parseLatLon`, `parseFloatField` to `nmea.go`. Replace the existing `parseRMC` with the new version that calls `CheckEpoch` and returns `*NavEpoch`. Update the RMC case in `Dispatch` to use `handleEpoch`. Remove `dispatchTime` (no longer needed once RMC is converted; ZDA can be handled directly).

Tests:
- Existing RMC time tests must still pass
- `parseLatLon`: equator, prime meridian, N/S/E/W, empty fields
- RMC position and velocity from known sentences
- Bundle with nil UTCTime and no position/velocity when status is "V"

Run `make test`.

### Step 2: parseGGA

Add `parseGGA` to `nmea.go`. Add the GGA case to `Dispatch`. Remove the unused `parseGGA` and `ggaSentence` from `nmeasats.go`. Remove `TestGGAParse` from `nmeasats_test.go`.

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

### Step 4: Epoch integration tests

Tests:
- Epoch boundary: two epochs with different time of day, verify `NavEpochMsg` emitted between them
- No emission before first boundary
- `Tag` is "NMEA", `StartTime` is correct
- Ext handler epoch boundary: PQTM messages with changing time-of-day trigger flush
- Ext handler EOE: returns `(bundle, nil, nil)`, epoch is flushed
