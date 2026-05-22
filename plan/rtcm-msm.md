# RTCM MSM observability

Extend `gpsprot.CorReportMsg` with detailed MSM satellite and
signal data for pulled RTCM packets, structured similarly to
`gpsprot.SatellitesMsg`.  Extend `RTCMSSE` to carry the same data
through to the web dashboard.

## Prerequisite

- `plan/rtcm-obs.md` (`CorReportMsg` and `RTCMSSE` in place).

This plan assumes it lands before `plan/sse-data.md`.  Once
`sse-data.md` Phase 2 removes the GPS `*SSE` types, `RTCMSSE` goes
with them and the dashboard consumes `CorReportMsg` directly; the
MSM field on `CorReportMsg` survives unchanged.

## CorReportMsg extension

Add `MSM opt.Val[RTCMMSM]` to `CorReportMsg`.  Set when:

- `Source == CorReportSourcePull`,
- `Tag == RTCM`, and
- the parsed `rtcmbin.Msg` is an MSM message (1071-1137).

Absent for non-MSM RTCM and for `CorReportSourceReceiver` (the
receiver-source path does not carry per-satellite detail).

```go
type RTCMMSM struct {
    GNSS        GNSS
    Level       int  // MSM level: 4, 5, 6, or 7
    MultipleMsg bool
    Sats        []RTCMMSMSat
}

type RTCMMSMSat struct {
    ID      SVID
    Signals []RTCMMSMSignal
}

type RTCMMSMSignal struct {
    ID  SignalID
    CN0 uint16   // tenths of dB-Hz (hi-res MSM6/7); units TBD
}
```

- `GNSS` from `rtcmbin.MSMHeader.GNSS()`, converted to
  `gpsprot.GNSS`.
- `SVID` constructed from GNSS + satellite ID from SatMask.
- `SignalID` mapped per constellation from RTCM signal IDs to
  `gpsprot.SignalID`.
- `CN0` from signal data (MSM4/5 use 6-bit, MSM6/7 use 10-bit).
- Cell mask determines which satellite-signal combinations are
  present.
- No look angles (MSM has observations only, not a receiver
  solution).
- No used/unused flags.

## Conversion

The stream-pull conversion API in `gps/app/stream` (added by
rtcm-obs.md) already has the parsed `rtcmbin.Msg` in hand when it
builds `*gpsprot.CorReportMsg`.  Extend it to populate `MSM` from
`rtcmbin.MSM` and `rtcmbin.MSMHiRes` when the message is MSM.

Per-constellation signal ID mapping tables live in
`gps/internal/rtcm`.

## SSE

Extend `RTCMSSE` with an MSM field:

```go
type RTCMSSE struct {
    MsgID  string                   `json:"msgID"`
    Source gpsprot.CorReportSource  `json:"source"`
    Used   opt.Val[bool]            `json:"used,omitzero"`
    MSM    opt.Val[gpsprot.RTCMMSM] `json:"msm,omitzero"`
}
```

Copy `MSM` straight from `CorReportMsg`.  All existing filtering
rules (Tag==RTCM, non-empty MsgID, drop on explicit checksum
failure, receiver-mode suppression) apply unchanged; MSM detail
rides along on pull-source events.

Because rtcm-obs.md switches the SSE observer to receiver mode on
the first valid receiver report and stays there, MSM detail is only
visible to the dashboard while it is in pull mode.

## Details still to be fleshed out

- Signal ID mapping tables per GNSS constellation.
- CN0 units and resolution: normalize MSM4/5 (6-bit) and MSM6/7
  (10-bit) to a common representation.
- Frontend display (parallels the satellite card but for MSM
  observations).
