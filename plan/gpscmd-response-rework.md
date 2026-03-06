# Reworking response handling in gpscmd

## Context

`satpulsetool gps` sends configuration commands to a GPS receiver from a TOML message file. The desktop app (GUI) will do the same thing. When commands are sent, the user needs feedback on whether each command succeeded or failed.

## Problem

The current `responsePrinter` in `internal/gpscmd/response.go` is a simple formatter that parses ACK/NAK packets and prints them to stdout. It has two problems:

1. It doesn't correlate responses to sent messages. A UBX ACK-ACK prints as `UBX-ACK-ACK: 06-01` which is meaningless to the user. It should say something like `disable-nmea/3: OK`.

2. It's embedded in `internal/gpscmd` so the desktop app can't use it.

## Approach

Add a `PacketAnalyzer` to `gps/msgfile` that classifies incoming packets and correlates ACK/NAK responses to specific sent messages. For each incoming packet, the analyzer returns one of four results: definitely not a response, maybe a response, definitely a response, or a definite ACK/NAK of a specific sent message.

Internally, the analyzer uses one abstraction (unexported):

- **responseMatcher** -- given an incoming packet (tag + data), returns a `ResponseKind` saying how the packet relates to the specific sent message this matcher was created for. Created from the sent message by a responsePattern. Every sent message gets a matcher.

The existing unrecognized-packet line buffering stays in `internal/gpscmd` -- that's a CLI-specific concern for handling raw serial data.

## Changes to RawMsg

`RawMsg` gains two new fields:

- `Count int` -- total number of messages with this tag (set by `toRawMsgs`)
- `source responsePattern` -- unexported back-pointer to the original message struct (`*UBXMsg`, `*CASBINMsg`, etc.)

The `source` field uses the `responsePattern` interface (satisfied by all message types that implement `newMatcher()`).

## MsgID

```go
type MsgID struct {
    Tag   string // tag name
    Index int    // 0-based index among messages with this tag
    Count int    // total messages with this tag
}
```

`RawMsg` gains a `MsgID()` method that returns a `MsgID` from its `Tag`, `Index`, and `Count` fields. Formatting for display (e.g. `disable-nmea/3`) is the caller's responsibility.

## PacketAnalysis

The public output of `Analyze`:

```go
type ResponseKind int

const (
    NotResponse   ResponseKind = iota // definitely not a response to anything we sent
    MaybeResponse                     // might be a response, can't tell
    AckResponse                       // definite ACK/NAK of a specific sent message
    OtherResponse                     // definitely a response (not ACK/NAK)
)

const AckNak = "NAK" // default AckError when no detail available

type PacketAnalysis struct {
    Kind       ResponseKind
    AckError   string  // AckResponse only: empty = success, non-empty = failure
    RelatedMsg *RawMsg // AckResponse only: the sent message this responds to
}
```

- **NotResponse**: the incoming packet is definitely not a response to anything we sent. Periodic data (standard GNSS talker NMEA, UNCA/NOVA logs, periodic PQTM), and ACK/NAK packets whose correlation ID doesn't match any sent message.
- **MaybeResponse**: we can't tell whether this is a response. For example, a proprietary NMEA sentence when we sent a line command. The caller should display these since they might be relevant.
- **OtherResponse**: definitely a response to something we sent, but not an ACK/NAK. For example, a UBX CFG-TP5 data reply to a poll, a Unicore `$CONFIG,...` query result, a non-periodic PQTM reply. No correlation to a specific sent message -- the caller just knows it's relevant.
- **AckResponse**: definite ACK/NAK of a specific sent message. `RelatedMsg` identifies which. `AckError` empty = success, non-empty = failure. UBX ACK-ACK -> `AckError = ""`; UBX ACK-NAK -> `AckError = AckNak`. Unicore `$command,CMD,response: OK` -> `AckError = ""`; `$command,CMD,response: <error>` -> `AckError = <error>`.

`RelatedMsg` is non-nil only when `Kind == AckResponse`. Formatting for display is the caller's responsibility.

## responseMatcher (unexported)

A responseMatcher examines an incoming packet and returns how it relates to the specific sent message this matcher was created for.

```go
type responseMatcher interface {
    // match returns a ResponseKind and an ack error string.
    // The ack error is only meaningful when kind == AckResponse
    // (empty = success, non-empty = failure).
    match(tag gpsprot.Tag, data string) (ResponseKind, string)
}
```

Every sent message gets a matcher -- there are no nil matchers.

- `NotResponse`: the packet is definitely not a response to this sent message. Wrong protocol tag, recognized periodic data, parse failures, etc.
- `MaybeResponse`: the matcher can't tell whether this packet is a response to its sent message.
- `OtherResponse`: the packet is a data reply to this sent message's query.
- `AckResponse`: the packet is an ACK/NAK for this sent message (ack error empty = success, non-empty = failure).

Matchers are stateless -- they always answer based on the packet alone.  Ack tracking is handled by `PacketAnalyzer`, not by the matchers themselves. `Analyze` always calls all matchers, but ignores `AckResponse` from already-acked matchers (they can still contribute `OtherResponse` or other results). A matcher is marked acked only when it is the sole matcher returning `AckResponse` (i.e., when `Analyze` returns `AckResponse` for that matcher).

Matchers for different protocols share common helper functions where useful, but there is no separate classifier interface.

### responsePattern (unexported)

A responsePattern creates a responseMatcher for a specific sent message. It is implemented by each message type that knows what responses to expect.

```go
type responsePattern interface {
    newMatcher() responseMatcher
}
```

`*UBXMsg`, `*CASBINMsg`, and `*ASBINMsg` implement `responsePattern`. They create matchers that:
- Return `NotResponse` for packets with a different protocol tag.
- Extract the message class/ID from the raw packet header (this does not require full payload parsing).
- Attempt full parsing to detect ACK-ACK/ACK-NAK messages. Parse failures do not prevent class/ID matching.
- Return `AckResponse` when the incoming packet is an ACK-ACK or ACK-NAK whose echoed class/ID matches the sent message's class/ID.
- Return `OtherResponse` when the incoming message's class/ID matches the sent message (poll responses, even if the payload format is unrecognized).
- Return `NotResponse` otherwise (different class/ID, including periodic messages like NAV-PVT which never share class/IDs with config messages).

`*LineMsg` implements `responsePattern` when it has a `ResponsePattern` field. The `ResponsePattern` field selects the matcher behavior:

- `responsePattern = "unicore"`: creates a unicore matcher. The matcher extracts the command name from the first word of the sent line text (e.g. `CONFIG` from `CONFIG HEADING OFFSET 0.0`). It recognizes three packet patterns; everything else is `NotResponse`:

  1. **NMEA `$command,...` ack**: sentence starts with `$command,CMD,response`. If the command name matches the sent command, returns `AckResponse` (ack error empty if `response: OK`, otherwise the error text after `response`).
  2. **NMEA `$CONFIG,...` data reply**: sentence starts with `$CONFIG,`. Returns `OtherResponse`.
  3. **UNCA `#MODE,...` data reply**: a UNCA packet whose message name is MODE. Returns `OtherResponse`.

`*NMEAMsg` implements `responsePattern`. Its matcher returns `NotResponse` for binary protocol packets (UBX, CASBIN, ASBIN) and standard GNSS talker NMEA (periodic). For proprietary NMEA, it extracts the 3-letter vendor ID (e.g. "QTM" from "PQTM...") and dispatches via a vendor classifier map. Currently only the QTM classifier is registered; see [pqtm-response.md](pqtm-response.md) for details. Unrecognized vendors return `MaybeResponse`.

`*BinaryMsg` implements `responsePattern`. Its matcher returns `NotResponse` for all text-based packets (NMEA, UNCA, NOVA) since text is not a response to raw binary. For binary protocol packets (UBX, CASBIN, ASBIN) and unrecognized data, it returns `MaybeResponse`.

`*LineMsg` without a `ResponsePattern` field implements `responsePattern`. Its matcher returns `NotResponse` for binary protocol packets (UBX, CASBIN, ASBIN) and standard GNSS talker NMEA (periodic). For other text-based packets (proprietary NMEA, TXT, UNCA, NOVA, unrecognized), it returns `MaybeResponse`.

### Changes to LineMsg

`LineMsg` gains an optional TOML field:

```toml
[default.line]
responsePattern = "unicore"

[[line]]
text = "CONFIG HEADING OFFSET 0.0"
```

```go
type ResponsePattern int

const (
    ResponsePatternNone    ResponsePattern = iota // zero value: no response matching
    ResponsePatternUnicore                        // "unicore"
)
```

`ResponsePattern` implements `encoding.TextUnmarshaler`. `UnmarshalText` accepts `"unicore"` and rejects unknown values with an error during TOML loading. The zero value (`ResponsePatternNone`) means no response matching.

```go
type LineMsg struct {
    Text            string           `toml:"text"`
    EOL             *string          `toml:"eol"`
    ResponsePattern *ResponsePattern `toml:"responsePattern"`
    MsgCommon
}
```

The default can be set in `[default.line]` as shown above. If omitted, the value is inherited from the default (nil pointer filled in by `applyLineDefaults`). The default itself defaults to `ResponsePatternNone`.

## PacketAnalyzer

```go
type PacketAnalyzer struct {
    msgs     []RawMsg           // parallel with matchers
    matchers []responseMatcher  // one per sent message, never nil
    acked    []bool             // parallel: true if this matcher has been acked
}

func NewPacketAnalyzer() *PacketAnalyzer

// NotifySent records a sent message for future response matching.
func (pa *PacketAnalyzer) NotifySent(rm RawMsg)

// Analyze classifies an incoming packet and correlates acks to sent messages.
func (pa *PacketAnalyzer) Analyze(tag gpsprot.Tag, data string) PacketAnalysis
```

`NotifySent` appends to both parallel slices. It calls `newMatcher()` on the sent message's source to create the matcher (every message type provides one).

`Analyze` takes a protocol tag and packet data (not `scan.Packet`). For unrecognized packets (`pkt.Format == nil`), the caller passes an empty tag and the raw data.

### Analyze

1. **Call all matchers**: iterate all matchers, call `match(tag, data)` on each. Each returns a `(ResponseKind, string)`. For already-acked matchers, treat `AckResponse` as `NotResponse` (ignore further acks).

2. **If any non-acked matcher returned AckResponse**: if exactly one -> mark it acked, return `AckResponse` with `RelatedMsg` (from parallel `msgs` slice) and ack error. If multiple -> return `OtherResponse` (ambiguous; none are marked acked).

3. **If any matcher returned OtherResponse**: return `OtherResponse`.

4. **If all matchers returned NotResponse**: return `NotResponse`.

5. **Otherwise** (at least one `MaybeResponse`, no ack or response): return `MaybeResponse`.

## Changes to internal/gpscmd

`response.go` is replaced. The `responsePrinter` type is simplified to:

1. Create a `PacketAnalyzer` before sending messages.
2. Call `NotifySent` for each message as it's sent.
3. For each received packet:
   - If `pkt.Format == nil`: handle with existing unrecognized-data line buffering (stays in gpscmd). Line buffering only re-slices raw bytes into printable lines -- it does not print or display them.
   - Call `Analyze` (always, regardless of format), format and print based on the returned `PacketAnalysis`.

The `runMsgs`/`sendMsg` functions pass the analyzer through instead of the `responsePrinter`. The decision of whether to create an analyzer no longer depends on message type -- always create one when sending messages.

## What this doesn't cover

- **PUBX ack correlation**: A future PUBX response pattern on `*NMEAMsg` could create matchers for PUBX command responses.
- **Desktop integration**: the desktop app will use `PacketAnalyzer` the same way, but the specifics of Wails event emission and UI display are out of scope here.
