# Dashboard position and quality cards

Add position/velocity and solution quality information to the web dashboard. The backend already sends `posvel` and `quality` SSE events on every navigation epoch; the frontend just doesn't subscribe to them yet.

This is a timing application -- the most important quality information is what affects timing reliability. Position and velocity are secondary (the antenna rarely moves). The web redesign will replace the quality display with a proper status bar, so keep things simple here.

## Backend changes

### Flatten array fields in `PosVelSSE`

The `EventFormat` / `addFields` system maps top-level keys to rows 1:1. Flatten the array fields in `PosVelSSE` into separate scalar fields, matching the pattern `SurveySSE` already uses for X/Y/Z:

- `PosECEF opt.Val[[3]gpsprot.Length]` -> `PosECEFX`, `PosECEFY`, `PosECEFZ opt.Val[gpsprot.Length]`
- `VelNED opt.Val[[3]gpsprot.Speed]` -> `VelN`, `VelE`, `VelD opt.Val[gpsprot.Speed]`
- `VelECEF opt.Val[[3]gpsprot.Speed]` -> `VelECEFX`, `VelECEFY`, `VelECEFZ opt.Val[gpsprot.Speed]`

Update `buildPosVelSSE` in `sseobs` to populate the new fields.

### Split fix fields in `QualitySSE`

Replace `Fix []string` with separate typed fields so the frontend can display them on separate lines:

- `FixLevel gpsprot.FixLevel` (e.g. `"carrierFixed"`, `"code"`)
- `FixDim gpsprot.FixDim` (e.g. `"3D"`, `"timeOnly"`)

These types already implement `MarshalJSON` as quoted strings. Remove `buildFixKeywords`; set the fields directly from `NavEpochMsg` in `buildQualitySSE`.

## Subscribe to events

Add `"posvel"` and `"quality"` to `EVENT_TYPES` in `dashboard.tsx`.

## Status card

A `PropertyCard` titled "Status" for timing-relevant data from the `quality` event. Placed at the top of the dashboard, before the sky view and signal graph.

| Field | Label | Formatting |
|---|---|---|
| `fixLevel` | Fix | plain string |
| `fixDim` | Dimension | plain string |
| `tdop` | TDOP | `x.xx` |
| `numSVUsed` | SVs used | plain number |
| `numSVTracked` | SVs tracked | plain number |
| `signalsUsed` | *per constellation* | Complex formatter: iterate the `{GNSS: [signal, ...]}` object and emit one row per constellation, e.g. `GPS signals: L1, L5` |

## Position card

A `PropertyCard` for position data from the `posvel` event.

| Field | Label | Formatting |
|---|---|---|
| `latLon` | Coordinates | Google Maps link, reuse `formatLL` from survey card |
| `height` | Height (WGS-84) | `x.xx m` |
| `heightMSL` | Height (MSL) | `x.xx m` |
| `posECEFX` | ECEF X | `x.xxxx m` |
| `posECEFY` | ECEF Y | `x.xxxx m` |
| `posECEFZ` | ECEF Z | `x.xxxx m` |

## Velocity card

A `PropertyCard` for velocity data from the same `posvel` event. Omit the entire card when ground speed is below 0.1 m/s (effectively stationary).

| Field | Label | Formatting |
|---|---|---|
| `groundSpeed` | Ground speed | `x.xx m/s` |
| `course` | Course | `x.x deg` |
| `velN` | Vel north | `x.xxx m/s` |
| `velE` | Vel east | `x.xxx m/s` |
| `velD` | Vel down | `x.xxx m/s` |

ECEF velocity and 3D speed omitted -- rarely interesting.

## Precise positioning card

A `PropertyCard` for correction and accuracy detail from the `quality` event. Relevant when doing RTK or other precise positioning.

| Field | Label | Formatting |
|---|---|---|
| `corrections` | Corrections | Join array with `, ` |
| `accHor` | Horizontal accuracy | `x.xxx m` |
| `accVert` | Vertical accuracy | `x.xxx m` |
| `accPos` | 3D accuracy | `x.xxx m` |
| `gdop` | GDOP | `x.xx` |
| `pdop` | PDOP | `x.xx` |
| `hdop` | HDOP | `x.xx` |
| `vdop` | VDOP | `x.xx` |
| `diffAge` | Differential age | `x.x s` |
| `rtcmRefBaseID` | RTCM base station | plain number |

All fields are optional and omitted from the card when absent. Speed and course accuracy omitted -- they belong more to velocity than positioning.

## Layout

```tsx
{events.quality && <PropertyCard title="Status" data={events.quality} format={statusFormat} />}
{events.satellites && haveLookAngles && <SkyViewCard svs={svs} />}
{events.satellites && <SignalGraphCard svs={svs} />}
{events.time && <PropertyCard ... />}
{events.phc && <PropertyCard ... />}
{events.receiver && <PropertyCard ... />}
{events.survey && <PropertyCard ... />}
{events.posvel && <PropertyCard title="Position" data={events.posvel} format={positionFormat} />}
{events.posvel && showVelocity(events.posvel) && <PropertyCard title="Velocity" data={events.posvel} format={velocityFormat} />}
{events.quality && <PropertyCard title="Precise Positioning" data={events.quality} format={precisePositioningFormat} />}
```

## Test dashboard

Add mock `posvel` and `quality` events to `test-dashboard.tsx` representing a typical RTK fixed solution with realistic values.

## Verify

All fields appear when a GPS receiver is connected and producing fixes. Fields are gracefully absent when the receiver or fix type doesn't provide them (e.g. no ECEF velocity, no differential age for standalone fix). Velocity card hidden when stationary. Signals used shows one line per constellation.
