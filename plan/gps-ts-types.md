# TypeScript types for GPS JSON serialisation

TypeScript interface declarations describing the JSON wire format of `gps/gpsprot` and `gps/ptime` types, with a Go-driven validation flow that catches drift without requiring TypeScript tooling in normal builds.

## Problem

The desktop GUI and web dashboard both consume JSON-serialised gpsprot messages. TypeScript interfaces for these types are currently hand-written and scattered across frontend source files (`app.tsx`, `status-panel.tsx`, `pvt-panel.tsx`, `web/svg.tsx`). When a Go struct field or `MarshalJSON` method changes, the TypeScript copies silently go stale. With two frontends sharing these types, the problem doubles.

## Package

Create `gps/ts/` as a source-only npm package (`@satpulse/gps`). No build step. Consumers import it via `file:` dependency.

```
gps/ts/
  package.json            # { "name": "@satpulse/gps" }
  gpsprot.ts              # interfaces for gpsprot message and value types
  ptime.ts                # Time, UTCTime type aliases
  gpsio.ts                # PacketLogEntry
  tsconfig.json           # used only by go:generate for tsc --noEmit
  validate.gen.ts         # checked-in generated file: typed consts with JSON literals
  gen.go                  # go:generate program
  gen_test.go             # drift detection test
```

## `ptime.ts`

Type aliases for `ptime` JSON representations:

| Go type | JSON representation | TypeScript |
|---|---|---|
| `ptime.Time` | `MarshalText`: decimal string `"seconds.nanoseconds"` (e.g. `"1741320000.000000000"`) | `type Time = string` |
| `ptime.UTCTime` | `MarshalJSON`: ISO 8601 string (e.g. `"2025-03-07T04:00:00Z"`) | `type UTCTime = string` |

## `gpsprot.ts`

Imports `Time` and `UTCTime` from `./ptime`.

### Scalar value types

These Go types have custom `MarshalJSON` methods. In TypeScript they are type aliases documenting what the JSON value actually is:

| Go type | JSON representation | TypeScript |
|---|---|---|
| `Length` | number (meters, e.g. `6378137.0`) | `type Length = number` |
| `Angle` | number (degrees, e.g. `51.477928`) | `type Angle = number` |
| `Speed` | number (m/s, e.g. `0.015`) | `type Speed = number` |
| `Duration` | number (seconds, e.g. `3600.5`) | `type Duration = number` |
| `GNSS` | string (e.g. `"GPS"`, `"GAL"`) | `type GNSS = string` |
| `SVID` | string (e.g. `"G01"`, `"E05"`) | `type SVID = string` |
| `SignalID` | string (e.g. `"L1 C/A"`, `"E5a"`) | `type SignalID = string` |
| `Tag` | string (e.g. `"UBX"`, `"NMEA"`) | `type Tag = string` |
| `FixLevel` | string (`"none"`, `"code"`, `"codeCorrected"`, `"carrierFloat"`, `"carrierFixed"`, `"notMeasured"`) | `type FixLevel = string` |
| `FixDim` | string (`"2D"`, `"3D"`, `"timeOnly"`, `"velocityOnly"`) | `type FixDim = string` |
| `TimeRef` | number (Go stringer enum, serialised as integer) | `type TimeRef = number` |
| `SatelliteUsedValidity` | number (integer enum) | `type SatelliteUsedValidity = number` |
| `CorrKind` | string array of leaf correction names (e.g. `["SBAS"]`, `["PPP-RTK"]`) | `type CorrKind = string[]` |
| `AuxSrc` | string array (e.g. `["DR"]`, `["INS"]`) | `type AuxSrc = string[]` |
| `GNSSSet` | string array (e.g. `["GPS","GAL","BDS","GLO"]`) | `type GNSSSet = string[]` |
| `SignalSet` | object mapping GNSS name to signal name arrays (e.g. `{"GPS":["L1","L5"],"GAL":["E1"]}`) | `type SignalSet = Record<string, string[]>` |
| `time.Duration` | number (nanoseconds, Go default integer serialisation) | `type StdDuration = number` |
| `time.Time` | string (RFC 3339, Go default) | `type StdTime = string` |

### Message interfaces

```ts
interface LookAngles {
    azimuth: number;     // int16, degrees 0-360
    elevation: number;   // int8, degrees
}

interface SignalInfo {
    id?: SignalID;
    cn0: number;         // uint8
    used?: boolean;
}

interface SVInfo {
    id: SVID;
    lookAngles?: LookAngles;
    signals: SignalInfo[];
    used?: boolean;
}

interface SatellitesMsg {
    tag?: Tag;
    nativeMsgID?: string;
    info: SVInfo[];                     // json:"info" (field name SVs)
    usedValidity?: SatelliteUsedValidity;
}

interface TimeMsg {
    taiTime?: Time;                     // ptime.Time
    utcTime?: UTCTime;                  // ptime.UTCTime
    accuracy?: StdDuration;             // time.Duration as integer nanoseconds
    utcOffset?: number;                 // uint8
    pulseOffset?: number;               // *float64
    gnss?: GNSS;
    ref?: TimeRef;
    tag?: Tag;
    nativeMsgID?: string;
}

interface SurveyMsg {
    position?: [Length, Length, Length];  // Point3D, omitzero
    accuracy: Length;
    obsCount: number;                   // uint32
    obsTime: Duration;
    valid: boolean;
    inProgress: boolean;
}

interface PosGeoMsg {
    latLon: [Angle, Angle];             // [lat, lon]
    height?: Length;
    heightMSL?: Length;
    tag: Tag;
    nativeMsgID: string;
}

interface PosECEFMsg {
    pos: [Length, Length, Length];        // Point3D
    tag: Tag;
    nativeMsgID: string;
}

interface VelGeoMsg {
    velNED?: [Speed, Speed, Speed];
    groundSpeed?: Speed;
    speed3D?: Speed;
    course?: Angle;
    tag: Tag;
    nativeMsgID: string;
}

interface VelECEFMsg {
    vel: [Speed, Speed, Speed];
    tag: Tag;
    nativeMsgID: string;
}

interface PVMsgBundle {
    posGeo?: PosGeoMsg;
    posECEF?: PosECEFMsg;
    velGeo?: VelGeoMsg;
    velECEF?: VelECEFMsg;
}

interface Accuracy {
    pos?: Length;
    hor?: Length;
    vert?: Length;
    speed?: Speed;
    groundSpeed?: Speed;
    course?: Angle;
}

interface DOP {
    geom?: number;       // float64
    pos?: number;
    hor?: number;
    vert?: number;
    time?: number;
}

interface NavEpochMsg {
    fixLevel?: FixLevel;
    fixDim?: FixDim;
    correction?: CorrKind;
    auxSrc?: AuxSrc;
    acc?: Accuracy;
    dop?: DOP;
    diffAge?: Duration;
    rtcmRefBaseID?: number;             // uint16
    numSVUsed?: number;                 // uint16
    numSVTracked?: number;              // uint16
    numSVInView?: number;               // uint16
    signalsUsed?: SignalSet;
    tag?: Tag;
    startTime: StdTime;                 // time.Time
}

interface LeapSecondMsg {
    // embedded ptime.LeapSecond fields (no json tags -- uses field names)
    OffChangeTime: Time;                // ptime.Time
    UTCOffBefore: number;               // int16
    UTCOffAfter: number;                // int16
    gnss?: GNSS;
}

interface ReceiverInfo {
    vendor: string;
    firmware: string;
    hardware: string;
    supportedGNSS: GNSSSet;
}
```

Note: `Priority` fields (on PosGeoMsg etc.) have `json:"-"` and are excluded.

## `gpsio.ts`

Interface for `gps/app/gpsio.PacketLogEntry`:

```ts
import type { Tag } from './gpsprot';

interface PacketLogEntry {
    t: string;           // TimeMicro (custom string format)
    tag?: Tag;
    msg?: string;
    bin?: string;        // HexString
    ascii?: string;
    speed?: number;
    out: boolean;
}
```

## Validation flow

### `gen.go` -- go:generate program

A Go program (`go generate ./gps/ts`) that:

1. Constructs sample instances of each message type with all fields populated.
2. Marshals each to JSON.
3. Writes `validate.gen.ts` with JSON literals as typed const initialisers:

```ts
// Code generated by go generate; DO NOT EDIT.
import type { SatellitesMsg, TimeMsg, NavEpochMsg, ... } from './gpsprot';

const _satellites: SatellitesMsg = {"tag":"UBX","info":[{"id":"G01","lookAngles":{"azimuth":45,"elevation":30},"signals":[{"id":"L1 C/A","cn0":42,"used":true}],"used":true}],"usedValidity":2};
const _time: TimeMsg = {"taiTime":"1741320000.000000000","utcTime":"2025-03-07T04:00:00Z","accuracy":25000000,"gnss":"GPS","tag":"UBX","nativeMsgID":"NAV-TIMEGPS"};
const _navEpoch: NavEpochMsg = {"fixLevel":"carrierFixed","fixDim":"3D","correction":["RTCM"],"acc":{"hor":0.015,"vert":0.025},"dop":{"pos":1.2,"hor":0.8},"numSVUsed":12,"signalsUsed":{"GPS":["L1","L5"],"GAL":["E1"]},"tag":"UBX","startTime":"2025-03-07T04:00:00Z"};
// ... one const per message type
```

4. Runs `npx tsc --noEmit` to validate the generated file against the hand-written interfaces. If `tsc` fails, the interfaces need updating.

The generated `validate.gen.ts` is checked in.

### `gen_test.go` -- drift detection

A Go test that re-generates the JSON samples in memory and compares against the checked-in `validate.gen.ts`. If the JSON has changed (because a Go type was modified), the test fails:

```
gpsprot JSON serialisation has changed; run: go generate ./gps/ts
```

This test runs as part of `make test` and requires no TypeScript tooling.

### Normal workflow

- `make` -- no TypeScript tooling needed, no `go generate` needed.
- `make test` -- runs `gen_test.go`; fails if Go JSON output has drifted from checked-in `validate.gen.ts`.
- `go generate ./gps/ts` -- re-generates `validate.gen.ts` and runs `tsc` to validate. Requires `npx tsc` (Node installed). Run only when the test tells you to.
- Developer updates hand-written interfaces if `tsc` reports errors, then re-runs `go generate`.

## Consumers

Desktop frontend `desktop/frontend/package.json`:
```json
"@satpulse/gps": "file:../../gps/ts"
```

Web dashboard (after [web-toolchain.md](web-toolchain.md) reorganisation):
```json
"@satpulse/gps": "file:../../gps/ts"
```

Both import types only:
```ts
import type { SatellitesMsg, SVInfo } from '@satpulse/gps/gpsprot';
import type { Time } from '@satpulse/gps/ptime';
```

## Migration

The interfaces are derived from existing hand-written TypeScript in the desktop frontend. These are the current locations and their mapping:

### `gpsprot.ts` sources

| Current location | Types | Go source |
|---|---|---|
| `desktop/frontend/src/app.tsx:62-100` | `TimeMsg`, `SurveyMsg`, `SatellitesMsg`, `SVInfo`, `SignalInfo` | `gps/gpsprot/msg.go` |
| `desktop/frontend/src/app.tsx:78-80` | `LeapSecondState` (becomes `LeapSecondMsg`) | `gps/gpsprot/msg.go`, `gps/ptime/ptime.go` |
| `desktop/frontend/src/status-panel.tsx:4-31` | `NavEpochMsg` (with inline `Accuracy`, `DOP`) | `gps/gpsprot/msg.go` |
| `web/svg.tsx:1-17` | `SVInfo`, `LookAngles`, `SignalInfo` (duplicate) | `gps/gpsprot/msg.go` |

### `gpsio.ts` sources

| Current location | Types | Go source |
|---|---|---|
| `desktop/frontend/src/app.tsx:52-60` | `PacketLogEntry` | `gps/app/gpsio/log.go` |

### After migration

Remove the scattered declarations from frontend source files and replace with imports from `@satpulse/gps`. The web dashboard migration happens as part of [web-toolchain.md](web-toolchain.md).

## Verify

- `npm install` in each consumer resolves `@satpulse/gps` via `file:` link
- `go generate ./gps/ts` produces `validate.gen.ts` and `tsc --noEmit` passes
- `go test ./gps/ts` passes with checked-in `validate.gen.ts`
- Modifying a json tag in `msg.go` causes `go test ./gps/ts` to fail
- Desktop GUI compiles and works with imported types
