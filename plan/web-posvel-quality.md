# Dashboard position and quality cards

Add position/velocity and solution quality information to the web dashboard. The backend already sends `posvel` and `quality` SSE events on every navigation epoch; the frontend just doesn't subscribe to them yet.

Works against the current `PosVelSSE` and `QualitySSE` shapes. Small backend change needed to flatten array fields.

## Flatten array fields in `PosVelSSE`

The current `EventFormat` / `addFields` system maps top-level keys to rows 1:1. It has no mechanism to expand a 3-element array into separate labelled rows. Flatten the array fields in `PosVelSSE` into separate scalar fields, matching the pattern `SurveySSE` already uses for X/Y/Z:

- `PosECEF opt.Val[[3]gpsprot.Length]` -> `PosECEFX`, `PosECEFY`, `PosECEFZ opt.Val[gpsprot.Length]`
- `VelNED opt.Val[[3]gpsprot.Speed]` -> `VelN`, `VelE`, `VelD opt.Val[gpsprot.Speed]`
- `VelECEF opt.Val[[3]gpsprot.Speed]` -> `VelECEFX`, `VelECEFY`, `VelECEFZ opt.Val[gpsprot.Speed]`

Update `buildPosVelSSE` in `sseobs` to populate the new fields.

## Subscribe to events

Add `"posvel"` and `"quality"` to `EVENT_TYPES` in `dashboard.tsx`.

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

A separate `PropertyCard` for velocity data from the same `posvel` event. Omit the entire card when ground speed is below 0.1 m/s (effectively stationary -- the desktop GUI uses this same threshold in its map component to decide arrow vs dot).

| Field | Label | Formatting |
|---|---|---|
| `groundSpeed` | Ground speed | `x.xx m/s` |
| `speed3D` | 3D speed | `x.xx m/s` |
| `course` | Course | `x.x deg` |
| `velN` | Vel north | `x.xxx m/s` |
| `velE` | Vel east | `x.xxx m/s` |
| `velD` | Vel down | `x.xxx m/s` |
| `velECEFX` | Vel X | `x.xxx m/s` |
| `velECEFY` | Vel Y | `x.xxx m/s` |
| `velECEFZ` | Vel Z | `x.xxx m/s` |

Some of these fields may be omitted from the initial implementation if the card feels too dense. Ground speed, course, and NED components are the most useful; ECEF velocity is rarely interesting.

## Solution quality card

A `PropertyCard` for data from the `quality` event. Consider splitting across two cards if it gets too long. A natural split: fix status / accuracy in one, DOP / satellite detail in another.

| Field | Label | Formatting |
|---|---|---|
| `fix` | Fix | Join array with space (e.g. `carrierFixed 3D`) |
| `corrections` | Corrections | Join array with `, ` |
| `accHor` | Horizontal accuracy | `x.xxx m` |
| `accVert` | Vertical accuracy | `x.xxx m` |
| `accPos` | 3D accuracy | `x.xxx m` |
| `accSpeed` | Speed accuracy | `x.xxx m/s` |
| `accGroundSpeed` | Ground speed accuracy | `x.xxx m/s` |
| `accCourse` | Course accuracy | `x.x deg` |
| `gdop` | GDOP | `x.xx` |
| `pdop` | PDOP | `x.xx` |
| `hdop` | HDOP | `x.xx` |
| `vdop` | VDOP | `x.xx` |
| `tdop` | TDOP | `x.xx` |
| `numSVUsed` | SVs used | plain number |
| `numSVTracked` | SVs tracked | plain number |
| `signalsUsed` | Signals used | Iterate `{GNSS: [signal, ...]}`, one line per constellation |
| `diffAge` | Differential age | `x.x s` |
| `rtcmRefBaseID` | RTCM base station | plain number |

All fields are optional (`opt.Val` with `omitzero` on the backend) and omitted from the card when absent.

## Layout

Add the new cards to the `Dashboard` component before the existing time/PHC/receiver/survey cards -- position and quality are the most important information:

```tsx
{events.posvel && <PropertyCard title="Position" data={events.posvel} format={positionFormat} />}
{events.posvel && showVelocity(events.posvel) && <PropertyCard title="Velocity" data={events.posvel} format={velocityFormat} />}
{events.quality && <PropertyCard title="Solution Quality" data={events.quality} format={qualityFormat} />}
```

## Verify

All fields appear when a GPS receiver is connected and producing fixes. Fields are gracefully absent when the receiver or fix type doesn't provide them (e.g. no ECEF velocity, no differential age for standalone fix). Velocity card hidden when stationary.
