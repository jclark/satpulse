# Extended NMEA sentence handlers

## Problem

Some GPS receivers (e.g. Quectel LG290P) don't have their own binary protocol but instead use non-standard NMEA sentences for configuration responses and additional navigation data. Quectel uses proprietary sentences starting with `P` (e.g. PQTMPVT, PQTMVEL), but other vendors may use NMEA-like sentences that don't even follow the proprietary `P` convention. Currently, the NMEA `PacketProcessor` handles standard approved GNSS talker sentences (RMC, ZDA, GSV, GSA) via `Dispatch`, and everything else falls through to `NativeMsgHandler` as raw `Sentence` objects. This means the rich data in messages like PQTMPVT, PQTMVEL, PQTMEPE is not parsed into protocol-agnostic messages (`TimeMsg`, `PosGeoMsg`, `VelGeoMsg`, etc.) and is unavailable to the daemon, desktop app, or any other `MsgHandler` consumer.

The Quectel LG290P is the immediate motivating case, but this is a general problem. MediaTek (PMTK), SkyTraq (PAIR/PSTI), Septentrio, and others all define non-standard NMEA sentences that carry navigation data. We need an extensible mechanism that lets vendor-specific code register handlers for these sentences without modifying the core NMEA package.

## Design

### MsgBundle in gpsprot

A general-purpose struct in `gps/gpsprot` that bundles all possible protocol-agnostic messages. Any combination of fields may be non-nil.

```go
// MsgBundle holds a set of protocol-agnostic messages. Any combination
// of fields may be non-nil.
type MsgBundle struct {
    Time       *TimeMsg
    PosGeo     *PosGeoMsg
    PosECEF    *PosECEFMsg
    VelGeo     *VelGeoMsg
    VelECEF    *VelECEFMsg
    LeapSecond *LeapSecondMsg
    Survey     *SurveyMsg
}

// Dispatch calls the corresponding MsgHandler methods for each
// non-nil field.
func (b *MsgBundle) Dispatch(h MsgHandler, tRead time.Time) {
    if b.Time != nil {
        h.Time(b.Time, tRead)
    }
    if b.PosGeo != nil {
        h.PosGeo(b.PosGeo, tRead)
    }
    // ... etc for each field
}
```

`MsgBundle` is not specific to NMEA -- it's a general-purpose type that any protocol processor could use when a single packet produces multiple messages.

`NavEpochMsg` is deliberately excluded from `MsgBundle`. Unlike the other messages which have a 1:1 relationship with a raw sentence, `NavEpochMsg` is accumulated across multiple sentences within a navigation epoch (e.g. fix quality from PQTMPVT, DOPs from PQTMDOP, accuracy from PQTMEPE). The handler receives a `*NavEpochMsg` for the current epoch and contributes fields to it; the NMEA `PacketProcessor` owns the epoch lifecycle and dispatches the completed message. See "NavEpochMsg accumulation" below.

### ExtSentenceHandler interface in gps/internal/nmea

The handler interface lives in `gps/internal/nmea`, not in `gpsprot`. This is an internal concern of the NMEA processing pipeline; external consumers don't need to know about it. `gpsreg` imports `nmea` and can wire things up.

```go
// ExtSentenceHandler handles non-standard NMEA sentences that are not
// approved GNSS talker sentences. This covers proprietary sentences
// (starting with P) and any other vendor-specific NMEA-like sentences
// that don't conform to the standard address format.
//
// Handlers are called for any sentence that is not handled by the
// standard approved-sentence processing. Each registered handler gets
// a chance to look at the sentence and decide whether to handle it.
// A handler returns a non-nil *MsgBundle if it handled the sentence,
// or nil to pass.
//
// epoch is the NavEpochMsg being accumulated for the current navigation
// epoch. The handler may set fields on it (e.g. DOPs, fix quality) but
// does not dispatch it -- the PacketProcessor handles epoch lifecycle.
type ExtSentenceHandler interface {
    // HandleSentence attempts to handle a non-standard NMEA sentence.
    // flags contains the syntax flags from the packet scanner.
    // payload is the NMEA payload between $ and *XX (e.g. "PQTMPVT,1,...")
    // epoch is the NavEpochMsg for the current epoch; the handler may
    // contribute fields to it.
    // Returns a non-nil MsgBundle if the sentence was handled (the bundle
    // may have no message fields set, e.g. for epoch-only contributions).
    // Returns eoe=true if the sentence marks an end-of-epoch boundary
    // (e.g. PQTMEOE).
    // Returns (nil, false) if the handler does not recognize the sentence.
    HandleSentence(flags nmeamsg.SentenceSyntaxFlags, payload string, epoch *gpsprot.NavEpochMsg) (bundle *gpsprot.MsgBundle, eoe bool)
}
```

Key design choices:

- **Returns (MsgBundle, bool)**: The handler returns a `*gpsprot.MsgBundle` and an epoch-complete flag rather than calling `MsgHandler` methods directly. This keeps the handler pure and testable -- it doesn't need to know about the downstream consumer. The NMEA `PacketProcessor` dispatches the results and flushes the epoch when signalled. A non-nil bundle means the handler claimed the sentence; nil means it didn't recognize it.

- **Payload string, not parsed fields**: The handler receives the raw payload string (everything between `$` and `*XX`), not pre-split fields. Different vendors have different conventions for field layout, and the handler knows best how to parse its own sentences. The payload includes the address field (e.g. `PQTMPVT,1,...`).

- **Syntax flags included**: The handler receives `SentenceSyntaxFlags` so it can check structural properties (e.g. `IsValidProprietaryNMEA()` to confirm a conforming proprietary address format before extracting the manufacturer ID).

- **Not limited to proprietary sentences**: The handler chain runs for any sentence not handled as an approved GNSS talker sentence. This covers both proper proprietary sentences (`P` prefix) and non-conforming vendor sentences that don't follow any NMEA convention.

- **No vendor prefix filtering**: Every registered handler gets a chance to look at every non-standard sentence and decides for itself whether it recognizes it. This avoids building a prefix-to-handler registry and keeps registration simple.

- **Multiple messages per sentence**: A single sentence (e.g. PQTMPVT) may carry time, position, and velocity. The `MsgBundle` allows returning all of them at once.

- **Only periodic output messages**: Handlers must only claim sentences that carry periodic navigation data (e.g. PQTMPVT, PQTMVEL, PQTMSVINSTATUS). Configuration responses (e.g. PQTMCFGPPS, PQTMCFGSVIN, PQTMVERNO) and command acknowledgments must return nil so they fall through to `NativeMsgHandler`. This is essential because `NativeMsgHandler` is how the configuration/command pipeline (satpulsetool, desktop app) sees responses to commands it sent. A handler that claimed configuration responses would break the command-response flow.

### Integration into NMEA PacketProcessor

The NMEA `PacketProcessor` gains a slice of registered handlers:

```go
type PacketProcessor struct {
    gpsprot.DefaultPacketProcessor
    mh         gpsprot.MsgHandler
    sb         satellitesBuffer
    extHandlers []ExtSentenceHandler
}
```

A new method to register handlers:

```go
func (p *PacketProcessor) AddExtHandler(h ExtSentenceHandler)
```

In `ProcessPacket`, after the approved-sentence path and before the `NativeMsgHandler` fallback, try extension handlers. The handler chain runs for any sentence that wasn't handled as an approved GNSS talker sentence -- no gating on `IsValidProprietaryNMEA()`, since some vendors don't conform to the proprietary convention:

```go
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
    sen := NewSentence(data)
    if sen == nil {
        return "", fmt.Errorf("not a valid NMEA packet: %s", data)
    }
    msgID := sen.AddressField()
    approvSen := sen.ApprovedSentence()
    if approvSen != nil {
        handled, err := p.Dispatch(approvSen, tRead, p.mh)
        if err != nil || handled {
            return msgID, err
        }
    }
    // Try extension handlers for non-standard sentences
    for _, h := range p.extHandlers {
        if result, eoe := h.HandleSentence(sen.SyntaxFlags, sen.Payload, &p.navEpoch); result != nil {
            result.Dispatch(p.mh, tRead)
            if eoe {
                p.flushNavEpoch(tRead)
            }
            return msgID, nil
        }
    }
    nmh := p.GetNativeMsgHandler()
    if nmh != nil {
        return msgID, nmh.NativeMsg(Tag, msgID, sen, tRead)
    }
    return msgID, nil
}
```

A sentence not handled by any `ExtSentenceHandler` still falls through to `NativeMsgHandler`, preserving the existing behavior for configuration responses and unrecognized messages.

### Registration via gpsreg

The `gpsreg` package handles registration. A new function:

```go
func CreateExtSentenceHandlers() []nmea.ExtSentenceHandler
```

This returns handlers for all known vendors. Initially just Quectel.

In `CreatePacketProcessors`, the NMEA processor gets the extension handlers registered:

```go
func CreatePacketProcessors(nmeaNumbering []gpsprot.NMEASVNumberingRange) map[gpsprot.Tag]gpsprot.PacketProcessor {
    nmeaPP := nmea.NewPacketProcessor()
    if nmeaNumbering != nil {
        nmeaPP.SetSVNumbering(nmeaNumbering)
    }
    for _, h := range CreateExtSentenceHandlers() {
        nmeaPP.AddExtHandler(h)
    }
    // ...
}
```

### Quectel handler: gps/internal/quectel

A new package `gps/internal/quectel` implements `nmea.ExtSentenceHandler` for Quectel's PQTM sentences.

```go
package quectel

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) HandleSentence(
    flags nmeamsg.SentenceSyntaxFlags,
    payload string,
    epoch *gpsprot.NavEpochMsg,
) (*gpsprot.MsgBundle, bool) {
    // Must be a valid proprietary NMEA sentence (P + 3+ char manufacturer ID)
    if !flags.IsValidProprietaryNMEA() {
        return nil, false
    }
    msg, err := qtmmsg.ParsePeriodicMsg(payload)
    if msg == nil {
        return nil, false
    }
    // ... type-switch on msg, return (bundle, false) for data messages,
    // (&MsgBundle{}, true) for EOE
}
```

The `default: return nil` is critical: any PQTM sentence not explicitly listed (including all `PQTMCFG*` configuration responses, `PQTMVERNO` version responses, acknowledgments) falls through to `NativeMsgHandler` where the command/response pipeline handles it.

#### Initial message support

The handler only claims periodic output messages. Quectel's PQTM messages fall into two categories:

**Periodic output** (handled -- parsed into protocol-agnostic messages):

| PQTM message | MsgBundle fields | NavEpochMsg fields | Notes |
|---|---|---|---|
| PQTMPVT | Time, PosGeo, VelGeo | Quality, Dim, NumSVUsed, HDOP, PDOP | Primary PVT: date, time, lat, lon, alt, vel N/E/D, speed, heading, fix type |
| PQTMVEL | VelGeo | | Velocity: N/E/D, ground speed, 3D speed, COG, accuracies |
| PQTMEPE | | (position accuracy) | Position error: N/E/D/2D/3D accuracy |
| PQTMSVINSTATUS | Survey | | Survey-in status: mean ECEF position, accuracy, obs count, valid |
| PQTMNAV | Time, PosGeo, VelGeo | Quality, Dim, NumSVUsed, NumSVTracked | Extended PVT with standard deviations, time status, sat counts |
| PQTMDOP | | GDOP, PDOP, TDOP, VDOP, HDOP | All DOP values |

**Configuration/command responses** (not handled -- fall through to NativeMsgHandler):

PQTMCFGPPS, PQTMCFGSVIN, PQTMCFGCNST, PQTMCFGMSGRATE, PQTMCFGUART, PQTMCFGRCVRMODE, PQTMCFGELETHD, PQTMCFGPROT, PQTMCFGFIXRATE, PQTMCFGRTCM, PQTMVERNO, PQTMSAVEPAR, PQTMRESTOREPAR, etc.

PQTMPVT is the most important since it provides time, position, and velocity in a single message and is available on all LG290P firmware versions. PQTMNAV is more comprehensive but was added in protocol spec v1.1 and requires newer firmware.

#### Tag and NativeMsgID

Messages returned by the Quectel handler use `Tag: nmea.Tag` (since they are NMEA packets) and `NativeMsgID` set to the PQTM address field (e.g. `"PQTMPVT"`).

### NavEpochMsg accumulation

Unlike the messages in `MsgBundle`, `NavEpochMsg` is built up across multiple sentences within a navigation epoch. The NMEA `PacketProcessor` owns a `NavEpochMsg` for the current epoch. Each `ExtSentenceHandler.HandleSentence` call receives a pointer to it and may set fields:

- PQTMPVT sets Quality, Dim, NumSVUsed, HDOP, PDOP
- PQTMDOP sets GDOP, PDOP, TDOP, VDOP, HDOP
- PQTMEPE contributes position accuracy
- PQTMNAV sets Quality, Dim, NumSVUsed, NumSVTracked

The `PacketProcessor` decides when to flush the accumulated `NavEpochMsg` and dispatch it via `MsgHandler.NavEpoch`. The handler signals end-of-epoch via the `eoe` return value (e.g. when it sees PQTMEOE). For receivers without an explicit EOE message, epoch boundaries are detected by time-of-day change when the next epoch's first message arrives.

## What this doesn't cover

- **ConfigProtocol for Quectel**: The LG290P uses PQTM commands for configuration (baud rate, constellation, PPS, etc.), with PQTM OK/ERROR responses. These configuration sentences share the same `PQTM` prefix as the periodic output sentences but are deliberately not claimed by the `ExtSentenceHandler`, so they continue to reach `NativeMsgHandler` for the command/response pipeline. Adding a `ConfigProtocol` implementation for Quectel is a separate concern. The existing message file system (`configs/gpsmsg/lg290p.toml`) already handles sending these commands.

- **NMEA SV numbering for Quectel**: If Quectel uses non-standard SV numbering in GSV sentences, that would be handled separately via `SetSVNumbering` (same mechanism as Allystar/SinoGNSS/u-blox).

- **Moving NMEAPacketProcessor out of gpsprot**: The `NMEAPacketProcessor` interface currently lives in `gpsprot` but is also an NMEA-internal concern that only `gpsreg` needs. Moving it to `gps/internal/nmea` alongside `ExtSentenceHandler` would be a good cleanup but is not part of this change.
