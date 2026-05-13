# RTCM MSM observability (#237)

Extend `gpsprot.RTCMMsg` with detailed MSM satellite and signal
data, structured similarly to `gpsprot.SatellitesMsg`.

## Prerequisite

- `plan/rtcm-obs.md` (basic `RTCMMsg` with MsgType and StationID).

## RTCMMsg extension

Add `MSM opt.Val[RTCMMSM]` to `RTCMMsg`.  Set for MSM messages
(1071-1137), absent for non-MSM (1005, 1006, 1230, etc.).

```go
type RTCMMSM struct {
    GNSS        GNSS
    Level       int
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
- `SignalID` converted from RTCM signal IDs (per constellation)
  to `gpsprot.SignalID`.
- `CN0` from signal data (MSM4/5 use 6-bit, MSM6/7 use 10-bit).
- Cell mask determines which satellite-signal combinations are
  present.
- No look angles (MSM has observations only, not receiver
  solution).
- No used/unused flags.

## Conversion

Extend the conversion function in `gps/internal/rtcm` to populate
`RTCMMSM` from `rtcmbin.MSM` and `rtcmbin.MSMHiRes`.  Needs a
signal ID mapping table per constellation (RTCM signal IDs to
`gpsprot.SignalID`).

## Details still to be fleshed out

- Signal ID mapping tables per GNSS constellation.
- CN0 units and resolution (normalize MSM4/5 and MSM6/7 to a
  common representation).
- Frontend display (parallels satellite card but for MSM
  observations).
