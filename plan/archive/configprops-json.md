# ConfigProps JSON serialization

## Goal
Add `UnmarshalJSON` to `ConfigProps` so it can be deserialized from JSON. Combined with the existing `MarshalJSON`, this makes `ConfigTarget` fully JSON round-trippable and eliminates the need for a DTO in the Wails adapter.

## Current state
`ConfigProps` has private fields with a `valid` bitmask and setter/getter methods. `MarshalJSON` exists (via `serializableMap`), producing a map where keys are property names and values are the property values. Only properties with their `valid` bit set are included.

`ConfigTarget` has three fields:
- `Props ConfigProps` -- properties to set
- `Get PropIDs` -- bitmask of properties to read back
- `Opts ConfigOptions` -- options (messages, save, reset, etc.)

`ConfigOptions` is already JSON-serializable after the `opt.Val` migration. `PropIDs` is a uint32.

## Changes

### 0. Refactor SignalSet.GNSSStringGroups to return map[string][]string

Rename `GNSSStringGroups` (e.g. to `GNSSSignalMap`) and change its return type from `[][]string` to `map[string][]string`, where keys are GNSS constellation names and values are signal name lists.

Update all callers:
- `SignalSet.String()` in `signal.go`
- `serializableMap` in `configtarget.go`
- `printSignals` in `internal/gpscmd/gpscmd.go`
- `testlog.go` entry building and `TestLogConfigEntry.SignalsEnabled` field type
- `replay_test.go` signal comparison

Add an inverse function to parse `map[string][]string` back into a `SignalSet`.

### 1. Add UnmarshalJSON to ConfigProps
`UnmarshalJSON` deserializes a JSON map into `ConfigProps`. Each key present in the map calls the corresponding setter. Keys absent from the map leave properties unchanged.

The JSON shape matches what `serializableMap` produces:

```json
{
  "signalsEnabled": {"GPS": ["L1", "L5"], "GAL": ["E1", "E5b"]},
  "timeGNSS": "GPS",
  "timePulse": {
    "width": 0.0001,
    "period": 1.0,
    "alignToGNSS": true,
    "onlyWhenLocked": true,
    "polarityRising": true
  },
  "mode": {
    "static": true,
    "fixedPosECEF": [4000000.0, 500000.0, 4800000.0],
    "fixedPosAcc": 0.1
  },
  "antennaCableDelay": 0.00000005,
  "navMsgAuth": "OSNMA",
  "rtcmBaseID": 1,
  "minElevation": 10.0
}
```

Implementation: unmarshal into `map[string]json.RawMessage`, then for each key present, parse the value and call the setter. Unknown keys are an error.

Unit conversions on unmarshal (inverse of marshal):
- `timePulse.width`, `timePulse.period`: seconds (float64) -> `time.Duration`
- `antennaCableDelay`: seconds (float64) -> `time.Duration`
- `minElevation`: degrees (float64) -> `Angle`
- `mode.fixedPosECEF`: meters (float64 array) -> `Length` array (`Point3D`)
- `mode.fixedPosLLH`: degrees (float64 array) -> `Angle` array
- `mode.height`: meters (float64) -> `Length`
- `mode.fixedPosAcc`: meters (float64) -> `Length`

For `timePulse`, individual sub-fields set individual PropIDs (matching the existing per-field setters). If the frontend sends only `{"timePulse": {"period": 1.0}}`, only `PropIDTimePulsePeriod` is set.

For `mode`, `PosType` is inferred from which keys are present: `fixedPosECEF` -> `PosTypeECEF`, `fixedPosLLH` -> `PosTypeLLH`, neither -> `PosTypeNone`.

For `navMsgAuth`, `serializableMap` manually maps to `"none"` / `"OSNMA"` strings. `NavMsgAuth` has no `UnmarshalText` or parse method, so `UnmarshalJSON` needs the reverse string-to-value mapping inline.

### 2. Add JSON support to PropIDs for Get
`Get` is a `PropIDs` bitmask. For JSON, represent it as an array of property name strings:

```json
["timePulse", "mode", "signalsEnabled"]
```

Add `UnmarshalJSON` to `PropIDs`: parse a JSON array of strings, look up each name in a camelCase name-to-bits table, OR together the bits. Add `MarshalJSON` for symmetry.

Property names match the keys used in `serializableMap`: `signalsEnabled`, `timeGNSS`, `timePulse`, `mode`, `antennaCableDelay`, `navMsgAuth`, `rtcmBaseID`, `minElevation`.

Note: `timePulse` as a Get name maps to `PropIDTimePulse` (the combined bitmask for all time pulse sub-properties). The existing `propNames` slice uses PascalCase and per-bit granularity, so a separate lookup table is needed.

## Result
`ConfigTarget` is directly JSON-deserializable. The Wails adapter in `app.go` can accept a `ConfigTarget` (or a thin wrapper) and pass it straight through to `gpscfg.Configure`. No DTO needed for properties or options.

## Files changed
- `gps/gpsprot/signal.go` (refactor `GNSSStringGroups` to return `map[string][]string`, add inverse parse function)
- `gps/gpsprot/configtarget.go` (`UnmarshalJSON` on `ConfigProps`, JSON support on `PropIDs`)
- `internal/gpscmd/gpscmd.go` (update `printSignals` for new map type)
- `internal/gpscmd/testlog.go` (update `SignalsEnabled` field type and builder)
- `internal/gpscmd/replay_test.go` (update signal comparison)
- `internal/gpscmd/testdata/*.jsonl` (update `signalsEnabled` from `[["GPS","L1",...]]` to `{"GPS":["L1",...]}` -- use a `jq` script to transform in place)
