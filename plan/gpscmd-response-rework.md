# Reworking response handling in gpscmd

## Problem

The current `responsePrinter` in `internal/gpscmd/response.go` is a simple formatter that parses ACK/NAK packets and prints them to stdout. It has two problems:

1. It doesn't correlate responses to sent messages. A UBX ACK-ACK prints as `UBX-ACK-ACK: 06-01` which is meaningless to the user. It should say something like `disable-nmea/3: ACK`.

2. It's embedded in `internal/gpscmd` so the desktop app can't use it.

## Approach

Add a `PacketAnalyzer` to `gps/msgfile` that knows what messages have been sent and can classify incoming packets relative to those sent messages. The analysis includes both the protocol-level detail (ACK vs NAK, which class/ID) and the user-level identity (tag, index within tag, description).

The existing unrecognized-packet line buffering stays in `internal/gpscmd` -- that's a CLI-specific concern for handling raw serial data.

## Changes to RawMsg

`RawMsg` gains two new fields:

- `Count int` -- total number of messages with this tag (set by `toRawMsgs`)
- `source tagDescGetter` -- unexported back-pointer to the original message struct (`*UBXMsg`, `*CASBINMsg`, etc.)

The `source` field uses the existing `tagDescGetter` interface (satisfied by `*MsgCommon`, which is embedded in all message types). When the `PacketAnalyzer` needs protocol-level detail (class/ID for ACK matching), it type-switches on `source` to get the concrete type.

`RawMsg` gains a `Label()` method for user-friendly display:

- Single message with tag "set-rate": `set-rate`
- Third of five messages with tag "disable-nmea": `disable-nmea/3`

## PacketAnalyzer

```go
type PacketAnalyzer struct {
    // tracks sent messages keyed by (protocol, class, ID) for ACK matching
}

func NewPacketAnalyzer() *PacketAnalyzer

// NotifySent records a sent message for future ACK/NAK matching.
func (pa *PacketAnalyzer) NotifySent(rm RawMsg)

// Analyze classifies an incoming packet. The packet must be recognized
// (pkt.Format != nil). Returns a PacketAnalysis describing what the
// packet is and whether it's a response to a sent message.
func (pa *PacketAnalyzer) Analyze(pkt scan.Packet) PacketAnalysis
```

The caller is responsible for filtering out unrecognized packets (`pkt.Format == nil`) before calling `Analyze`. Unrecognized packet handling remains in `internal/gpscmd`.

### NotifySent

When `NotifySent` is called, the analyzer type-switches on `rm.source`:

- `*UBXMsg`: records pending (class, ID) keyed for UBX ACK matching
- `*CASBINMsg`: records pending (class, ID) keyed for CASBIN ACK matching
- `*ASBINMsg`: records pending (class, ID) keyed for ASBIN ACK matching
- `*NMEAMsg`, `*LineMsg`, `*BinaryMsg`: no ACK mechanism, nothing to record for matching

In all cases, the `RawMsg` is stored so it can be returned via `ResponseTo`.

### Pending entry tracking

Each pending sent entry tracks two independent flags: got ACK/NAK, got PollResponse. A UBX CFG poll produces both an ACK and a PollResponse for the same sent message. Matching is a linear scan from the start: find the first entry with matching class/ID that doesn't already have the relevant flag set.

### Analyze

Dispatches on packet protocol tag:

- **UBX**: parses with `ubxbin.ParseMsg`. If ACK-ACK or ACK-NAK, extracts echoed class/ID, looks up in pending sent messages. Returns `Ack`/`Nak` with `ResponseTo` if matched, `UnmatchedAck`/`UnmatchedNak` if not. For CFG-class messages: if class/ID matches a pending sent message, returns `PollResponse` with `ResponseTo`. All other UBX messages return `Background`.
- **CASBIN**: same pattern with `casbin.ParseMsg`.
- **ASBIN**: same pattern with `asbin.ParseMsg`.
- **NMEA**: checks syntax. Standard GNSS talker sentences (except TXT) return `Background`. TXT and proprietary sentences return `PossibleReply`.
- **Other**: returns `Background`.

## PacketAnalysis and PacketKind

```go
type PacketKind int

const (
    Ack          PacketKind = iota // matched ACK for a sent message
    Nak                            // matched NAK for a sent message
    PollResponse                   // data packet matching a sent poll request
    UnmatchedAck                   // ACK but no matching sent message
    UnmatchedNak                   // NAK but no matching sent message
    PossibleReply                  // NMEA TXT/proprietary, might be a response
    Background                     // normal traffic, not a response
)

type PacketAnalysis struct {
    Kind       PacketKind
    Binary     bool     // is this a binary protocol packet
    ResponseTo *RawMsg  // sent message this is a response to, if matched
}
```

`ResponseTo` is non-nil when the analyzer successfully matches the packet to a sent message (for `Ack`, `Nak`, or `PollResponse`). It is nil for `UnmatchedAck`, `UnmatchedNak`, `PossibleReply`, and `Background`. Formatting the analysis for display is the caller's responsibility.

## Changes to internal/gpscmd

`response.go` is replaced. The `responsePrinter` type is simplified to:

1. Create a `PacketAnalyzer` before sending messages.
2. Call `NotifySent` for each message as it's sent.
3. For each received packet:
   - If `pkt.Format == nil`: handle with existing unrecognized-data line buffering (stays in gpscmd).
   - Otherwise: call `Analyze`, format and print based on the returned `Kind` and `ResponseTo`.

The `runMsgs`/`sendMsg` functions pass the analyzer through instead of the `responsePrinter`. The decision of whether to create an analyzer no longer depends on message type -- always create one when sending messages.

## What this doesn't cover

- **NMEA response correlation**: NMEA has no ACK mechanism. TXT and proprietary sentences are flagged as `PossibleReply` but not correlated to specific sent messages. Smarter NMEA matching (e.g. recognizing that a PUBX response matches a sent PUBX command) is future work.
- **Desktop integration**: the desktop app will use `PacketAnalyzer` the same way, but the specifics of Wails event emission and UI display are out of scope here.

