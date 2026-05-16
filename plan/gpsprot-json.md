# gpsprot JSON (#276)

Make `gpsprot.Msg` payloads serialize to stable, natural JSON independent of
any transport or event envelope.

This is a prerequisite for several separate plans:

- `gps/ts` TypeScript types and drift tests need to match actual Go JSON
  output.
- Packet log tests need deterministic expected decoded message output.
- The daemon event log format uses `gpsprot.Msg` as the `data` payload for GPS
  message records.
- SSE data can reuse canonical daemon observability payloads instead of
  web-specific GPS DTOs.
- gpsd compatibility analysis depends on the planned shape of `gpsprot`
  message JSON.

This plan only covers the JSON shape of `gpsprot.Msg` payloads. It does not
define SSE framing, the event-log envelope, or socket API framing.

## GPS message serialization cleanup

A few `gpsprot.Msg` types still need cleanup before they serialize cleanly
enough to be reused across packet testing, event logs, SSE data, and generated
TypeScript types.

### SurveyMsg geodetic fields

`SurveyMsg` has only ECEF `Position` (`Point3D`). The JSON shape should allow
both ECEF and geodetic coordinates without requiring every receiver protocol to
provide both:

- Make the ECEF position optional in `SurveyMsg`.
- Add optional `LatLon opt.Val[[2]Angle]` and `Height opt.Val[Length]` fields to `SurveyMsg`.
- Make `ObsTime` and `ObsCount` optional (`opt.Val`) -- different protocols provide different subsets.

### LeapSecondMsg cleanup

`LeapSecondMsg` embeds `ptime.LeapSecond` which does not serialize nicely:
`ptime.Time` fields render as raw `int64`, and the embedded fields lack JSON
tags. More broadly, GNSS satellites transmit leap second info as part of a
time-correction block that describes how UTC relates to the GNSS system time
(equivalent to `ptime.CorrectionParams`). The cleanup should consider
generalizing `LeapSecondMsg` to cover this GNSS-to-UTC conversion as a whole,
not just the leap second. Details to be fleshed out.

### TimeRef serialization

`TimeMsg.Ref` is a `TimeRef` (`int` enum: `NavSolution`, `PrePulse`,
`PostPulse`). It currently serializes as a bare integer (`"ref":1`), which is
opaque to consumers. Add `MarshalJSON`/`UnmarshalJSON` methods so it serializes
as a string (`"ref":"prePulse"`), matching how `GNSS` already serializes.

### Replace pointer optionality with `opt.Val`

- `TimeMsg.UTCTime *ptime.UTCTime` -> `opt.Val[ptime.UTCTime]`
- `TimeMsg.PulseOffset *float64` -> `opt.Val[float64]`
- `SVInfo.LookAngles *LookAngles` -> `opt.Val[LookAngles]`

Update all producers and consumers, including `TimeMsg.Merge` and
`ComputeTAITime`.

## Verify

- `gpsprot.Msg` payloads serialize cleanly for all message types used by packet testing, event logs, SSE data, and generated TypeScript types.
- `gps/ts` generated types remain in sync with Go GPS JSON output.
- Existing GPS decoder and message tests pass.
