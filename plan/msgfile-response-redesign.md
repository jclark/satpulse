# Message file response handling redesign

Redesign how `gps/msgfile` correlates responses to sent messages,
fixing ambiguous ACK matching and enabling smarter send pacing.

Related: issue #249.

## Problems with current design

### Ambiguous ACK matching

When multiple messages have been sent and an ACK arrives that could
match more than one unacknowledged message, the analyzer treats it as
ambiguous and does not recognize it as an ACK. The ACK is still
displayed but without correlation to the sent message.

This happens because messages are sent as fast as possible (unless
`delay` is set), so multiple messages with the same ACK signature
can be in flight simultaneously.

### No way to know when responses are complete

After sending all messages, there is either no wait at all (responses
may be missed) or a fixed `--capture` duration (wastes time if
responses arrive quickly). The analyzer has no concept of whether all
expected responses have been received.

### ACK and data are mutually exclusive

The current `ResponseKind` enum forces a packet to be either
`AckResponse` or `OtherResponse`, never both. PQTM query responses
carry both the acknowledgement and the data in a single packet
(e.g. `PQTMCFGPPS,OK,1,1,100000,1000,0,0`). Currently this is
classified as `OtherResponse`, losing the ACK information.

## Design overview

### Two-level architecture

The design separates response handling into two levels:

- **Analyzers** (internal): per-protocol classifiers that examine
  individual packets and request messages, producing structured
  descriptions.

- **Correlator** (public): the top-level struct that combines
  request descriptions with response classifications to make state
  decisions (correlation, ambiguity, completion). It maintains
  per-request state and provides the public API.

### Response analysis

Each protocol tag has a `responseAnalyzer` that classifies incoming
packets independently of any sent request. It produces a
`responseAnalysis` with a `responseKind`: `responseMaybeData` (the
zero value, meaning uncertain), `responseNotData` (definitely not a
data response), `responseData` (confirmed data), `responseAck`,
`responseNak`, or `responseWait`. For ACK/NAK/wait, the analysis
includes the correlation fragment extracted from the packet and
any error string.

The response analyzer never looks at what was sent. It only knows
what protocol it handles and how to parse that protocol's packets.
For tags with no registered response analyzer, the correlator
treats the packet as `responseMaybeData`.

```go
type responseKind int
const (
    responseMaybeData responseKind = iota // zero value: might be data, uncertain
    responseNotData                       // definitely not data
    responseData                          // confirmed data response
    responseAck                           // positive acknowledgement
    responseNak                           // negative acknowledgement
    responseWait                          // interim "still processing"
)

type responseAnalysis struct {
    kind         responseKind
    ackCorrelate string // ack/nak/wait: correlation fragment from packet
    ackError     string // nak: error description
}

type responseAnalyzer interface {
    analyzeResponse(data string) responseAnalysis
}
```

### Request analysis

Each message type (UBXMsg, CASBINMsg, NMEAMsg, LineMsg, etc.)
implements `requestAnalyzer` to produce a `requestAnalysis` from
its outgoing message bytes. The `requestAnalysis` describes:

- Where ACKs arrive (tag + correlation fragment for matching) and
  what ACK behavior to expect (none, ack or nak, nak only).
- Where data arrives (tag, possibly different from ACK tag).
- What kind of data to expect (none, unknown, combined with ACK,
  ambiguous single, confirmable single, multiple).
- An optional data match function to confirm whether a candidate
  data packet belongs to this request. A nil `dataMatch` means
  any data (`responseData` or `responseMaybeData`) on the matching
  `dataTag` is accepted without further classification.

The request analyzer is tied to the message type, not to a protocol
tag. This is because the same message type may expect responses on
multiple tags (e.g. Unicore line messages expect ACKs on TagNMEA
and data on TagUnicoreAscii).

Each message type (UBXMsg, CASBINMsg, NMEAMsg, LineMsg, etc.)
implements the `requestAnalyzer` interface. `RawMsg.source`
changes from the old `responsePattern` interface to
`requestAnalyzer`. The `Correlator` calls `analyzeRequest` on
`RawMsg.source` when it needs the `requestAnalysis` (in
`ReadyToSend` and `NotifyMsgSent`).

```go
type AckExpectation int
const (
    ExpectAckNone    AckExpectation = iota // no ack or nak expected
    ExpectAckOrNak                         // expect ack on success, nak on failure
    ExpectAckNakOnly                       // no ack on success, may get nak on failure
)

type dataExpectation int
const (
    expectDataUnknown  dataExpectation = iota // zero value: don't know what to expect
    expectDataNone                            // no data expected
    expectDataWithAck                         // data combined in ack packet
    expectDataAmbig                           // single data, indistinguishable from periodic
    expectDataSingle                          // one separate data response, confirmable
    expectDataMultiple                        // multiple data responses
)

type requestAnalysis struct {
    ackTag       gpsprot.Tag            // tag where ACK/NAK arrives; empty if ExpectAckNone
    ackCorrelate string                 // fragment for matching against response ackCorrelate
    expectAck    AckExpectation
    dataTag      gpsprot.Tag            // tag where data arrives; empty = any tag
    dataExpect   dataExpectation
    dataMatch    func(data string) bool // nil: accept all on matching tag; non-nil: true = matches
}

type requestAnalyzer interface {
    analyzeRequest(data string) requestAnalysis
}
```

### Correlation

The `Correlator` combines request and response analyses:

1. Receives a packet with a tag.
2. Looks up the `responseAnalyzer` for that tag. If none exists,
   the response kind is `responseMaybeData` (the default).
3. Calls `analyzeResponse` to get a `responseAnalysis`.
4. Based on the response kind:
   - **responseAck/responseNak/responseWait**: scans pending
     requests where `ackTag` matches this tag and `ackCorrelate`
     matches. If exactly one matches, updates that request's ack
     state and attributes the ACK. If multiple match (ambiguous),
     does not update any request's ack state; returns
     `Ack = AckNone`, `Relevance = LevelAmbigResponse`.
   - **responseData/responseMaybeData**: scans pending requests
     where `dataTag` matches this tag (or `dataTag` is empty),
     then calls `dataMatch` (if non-nil) to confirm. If exactly
     one matches, updates that request's data state and sets
     relevance based on `dataExpect`. If multiple match
     (ambiguous), does not update any request's data state;
     returns `Relevance = LevelAmbigResponse`.
   - **responseNotData**: no correlation action.
5. Updates per-request state and returns a `Correlation` for the
   caller to use for display.

### Per-request state

The correlator tracks two independent aspects per request:

- **ACK status**: not expected, waiting, waiting for more (after
  a wait/interim response), succeeded, or failed. Initial state
  is `ackNotExpected` for `ExpectAckNone`, `ackWait` for both
  `ExpectAckOrNak` and `ExpectAckNakOnly`.
- **Data status**: not expected, waiting, or received.

```go
type ackStatus int
const (
    ackNotExpected ackStatus = iota // zero value: no ack expected
    ackWait                         // expecting ack
    ackWaitMore                     // got responseWait, real ack coming
    ackSuccess                      // got responseAck
    ackFailed                       // got responseNak
)

type dataStatus int
const (
    dataNotExpected dataStatus = iota // zero value: no data expected
    dataWait                          // expecting data
    dataReceived                      // got data
)

type requestState struct {
    analysis requestAnalysis
    ack      ackStatus
    ackError string
    data     dataStatus
}
```

A request's completion is known when:
- `expectDataNone` + `ExpectAckOrNak`: ACK succeeded or failed.
- `expectDataNone` + `ExpectAckNakOnly`: NAK received. On success
  no response arrives; completion is never confirmed (caller
  times out, but this is not reported as a problem).
- `expectDataNone` + `ExpectAckNone`: completion known immediately
  (e.g. UBX CFG-RST, no response expected).
- `expectDataUnknown`: completion is never known (e.g. line
  messages, binary messages -- gpscmd shows plausible responses
  until timeout). Only valid with `ExpectAckNone`.
- `expectDataWithAck` + `ExpectAckOrNak`: ACK succeeded (data
  included) or ACK failed. (Only valid with `ExpectAckOrNak`;
  data in the ACK implies an ACK is expected.)
- `expectDataAmbig`: completion is never known (cannot confirm
  the data response is actually a reply to this request). Valid
  with `ExpectAckNone` (e.g. UBX non-CFG poll) or
  `ExpectAckOrNak` (e.g. UBX CFG poll where periodic messages
  of the same type also exist); in the latter case, ACK failure
  makes it complete.
- `expectDataSingle` + `ExpectAckOrNak`: ACK succeeded + data
  received, or ACK failed.
- `expectDataSingle` + `ExpectAckNakOnly`: NAK received, or data
  received. On silent success, completion requires data.
- `expectDataSingle` + `ExpectAckNone`: data received.
- `expectDataMultiple` + `ExpectAckOrNak`/`ExpectAckNakOnly`:
  NAK received (no data coming). On success, completion is never
  known (cannot determine when the last response arrives).
- `expectDataMultiple` + `ExpectAckNone`: completion is never
  known.

### Public API

```go
type RelevanceLevel int
const (
    LevelAckOnly       RelevanceLevel = iota // ack/nak/wait packet, no content to display
    LevelNotResponse                          // not a response to anything sent
    LevelMaybeResponse                        // might be a response, uncertain
    LevelAmbigResponse                        // definitely a response, but ambiguous which request
    LevelMultiResponse                        // one of multiple data responses
    LevelSoleResponse                         // the single expected data response
)

type AckKind int
const (
    AckNone  AckKind = iota // not an ack/nak/wait
    AckAck                  // positive acknowledgement
    AckNak                  // negative acknowledgement
    AckOther                // other ack-related (e.g. interim "still processing")
)

type Correlation struct {
    Ack          AckKind
    NakError     string           // meaningful when Ack == AckNak; non-empty = error detail
    InResponseTo *RawMsg          // non-nil when Ack != AckNone
    Relevance    RelevanceLevel
}

type Correlator struct {
    responseAnalyzers map[gpsprot.Tag]responseAnalyzer
    requests          []requestState
}

func NewCorrelator(...) *Correlator
func (c *Correlator) ReadyToSend(rm RawMsg) bool
func (c *Correlator) NotifyMsgSent(rm RawMsg)
func (c *Correlator) CorrelatePacket(tag gpsprot.Tag, data string) Correlation
func (c *Correlator) CanAcceptMore() bool
func (c *Correlator) Missing() (missingAck, missingData []*RawMsg)
```

`CanAcceptMore` returns true when some requests could still receive
ack or data responses. Returns false when all requests have reached
known completion and there is nothing left to wait for.

`Missing` returns requests whose firm expectations were not met:
- `missingAck`: requests with `ExpectAckOrNak` where no ack/nak
  was received. (`ExpectAckNakOnly` is not included since silent
  success is normal.)
- `missingData`: requests where data was firmly expected
  (`expectDataSingle`, `expectDataWithAck`, `expectDataMultiple`)
  but no data was received.

The caller displays packet content when
`Relevance >= LevelMaybeResponse`. The `Ack`/`NakError`/
`InResponseTo` fields are orthogonal to relevance and used to
display the ack/nak indicator line.

### Relevance level determination

- `LevelAckOnly`: the packet was matched as an ACK, NAK, or wait
  and carries no displayable data content. This covers all
  ACK/NAK/wait packets except a successful ACK (`AckAck`) on a
  request with `expectDataWithAck` (which gets `LevelSoleResponse`
  because the packet carries data).
- `LevelNotResponse`: the packet was not matched to any request.
- `LevelMaybeResponse`: the packet could be a data response but
  confirmation is uncertain. This includes `expectDataAmbig`
  (e.g. non-CFG UBX poll indistinguishable from periodic) and
  `expectDataUnknown` (e.g. line/binary messages).
- `LevelAmbigResponse`: the packet is definitely a response to
  something we sent, but it matched multiple pending requests and
  cannot be attributed to one. Used for both ambiguous ACK/NAK
  packets (where `Ack = AckNone` to avoid false attribution) and
  ambiguous data packets. No request state is updated. Pacing
  makes ACK ambiguity rare; practical usage makes data ambiguity
  near-nonexistent.
- `LevelMultiResponse`: the packet is a confirmed data response for
  a request with `expectDataMultiple`.
- `LevelSoleResponse`: the packet is a confirmed data response for
  a request with `expectDataSingle` or `expectDataWithAck`.

### Send pacing

```go
func (c *Correlator) ReadyToSend(rm RawMsg) bool
```

`ReadyToSend` calls `analyzeRequest` internally and checks whether
any pending request has a conflicting ACK correlation fragment
(same `ackTag` and `ackCorrelate` with ack still pending). Returns
true if safe to send.

### delay, waitLimit, and --capture

Three independent timing controls govern message sending:

**`delay`** is a per-message minimum pause before the next message
is sent. It controls inter-message spacing (e.g. giving the
receiver time to process a command before sending the next). It
does not extend the response wait deadline. Packets are processed
during the delay.

**`waitLimit`** is a per-message property on `MsgCommon` (TOML key
`waitLimit`), a float64 in seconds with a default of 1.2 seconds.
It is stored as `RawMsg.WaitLimit` (a `time.Duration`). It
controls how long the sender is willing to wait for responses.
After sending each message, the sender extends a response deadline
to `max(deadline, now + waitLimit)`. A message with a longer
`waitLimit` extends the deadline but a subsequent message with a
shorter one does not reduce it.

**`--capture`** is a CLI flag that adds additional packet capture
time *after* all message sending and response waiting is complete.
It has no default in message file mode. When specified, packets
are displayed for the full capture duration with no early exit.
`--capture` is independent of `waitLimit`.

### Send loop

After sending each message, the sender waits for the message's
`delay`, then checks `ReadyToSend` for the next message. If
`ReadyToSend` returns false, the sender processes incoming packets
(which may clear the conflicting ACK) until `ReadyToSend` returns
true or the response deadline expires.

### Knowing when to stop waiting

After all messages have been sent, the sender enters a response
wait loop. On each packet it checks `CanAcceptMore()`: if false,
all requests have reached known completion and the sender stops
immediately. Otherwise it keeps processing packets until the
response deadline expires.

After the deadline (or early stop), the caller calls `Missing()` to
get requests whose firm expectations were not met, and warns the
user about each one.

### Changes to response.go

The `responseHandler` in `internal/gpscmd/response.go` replaces
its current `PacketAnalyzer` with a `Correlator`. The display
logic changes:

**Per-packet display.** For each packet, `CorrelatePacket` returns
a `Correlation`. The handler produces output in two independent
parts:

- **Ack indicator line**: if `Ack` is `AckAck`, print
  "tag: OK". If `AckNak`, print "tag: receiver rejected
  message: error". `AckOther` (wait) may optionally print
  "tag: processing...". Uses `InResponseTo` for the tag/index.
- **Packet content**: if `Relevance >= LevelMaybeResponse`,
  display the packet content (text or hex). This is independent
  of the ack line -- a PQTM query response with `AckAck` +
  `LevelSoleResponse` prints both.

**Read loop.** After sending all messages, the handler enters a
read loop consuming packets and calling `CorrelatePacket`. It
checks `CanAcceptMore()` after each packet; if false, it stops
immediately. Otherwise it reads until the capture timeout.

**Post-timeout reporting.** After the loop, the handler calls
`Missing()` and warns about each message in `missingAck`
("no response received for message X") and each in `missingData`
("no data response received for message X").

**Line buffering.** Unrecognized packets (no `PacketFormat`) are
still line-buffered. Completed lines are passed to
`CorrelatePacket` with `EmptyTag`. Since there is no response
analyzer for `EmptyTag`, the correlator treats these as
`responseMaybeData`. Requests with empty `dataTag` (line/binary
messages) can match via their `dataMatch` closure.

## Per-protocol details

### UBX (u-blox)

#### Protocol behavior

UBX is a binary protocol. Each message has a 2-byte identity
(class + ID). Two rules govern responses:

1. Any CFG-class message sent to the receiver produces ACK-ACK
   (class 0x05, ID 0x01) on success or ACK-NAK (class 0x05,
   ID 0x00) on failure. The ACK/NAK payload contains the class/ID
   of the original message.

2. Any message can be polled by sending it with an empty payload.
   The receiver replies with the same message populated with data.

These rules combine to give three request/response patterns:

**CFG set.** Send a CFG message with a payload to set configuration.
The receiver replies with ACK-ACK or ACK-NAK (rule 1). For example,
sending CFG-PRT (class 0x06, ID 0x00) with port configuration data
produces an ACK-ACK containing `0x06 0x00`. This is the only
response.

**CFG poll.** Send a CFG message with an empty payload to query
the current value. The receiver replies with both an ACK-ACK or
ACK-NAK (rule 1) and the same message populated with data (rule 2).
For example, polling CFG-TP5 produces an ACK-ACK for CFG-TP5 and
a CFG-TP5 message containing the current timepulse configuration.

**Non-CFG poll (periodic).** Send a non-CFG message with an empty
payload. The receiver replies with the same message populated with
data (rule 2) only -- no ACK/NAK since it is not CFG-class. For
example, polling NAV-PVT (class 0x01, ID 0x07) produces a NAV-PVT
message with the current navigation solution. Most NAV, RXM, MON,
and TIM messages are periodic: the receiver may output them
independently of any poll, so the response is indistinguishable
from periodic output.

**Non-CFG poll (poll-only).** Some non-CFG messages are poll-only
(Type "Polled" in the u-blox spec): the receiver only outputs them
in response to a poll, never periodically. For these messages the
response is definitively a reply to the poll. Currently MON-VER
(0x0A 0x04) and MON-GNSS (0x0A 0x28) are handled as poll-only.

**Exception:** CFG-RST (reset) does not produce a response because
the receiver resets before it can reply.

#### Request analyzer

Implemented by UBXMsg. Produces a requestAnalysis:
- `ackTag`: TagUBX.
- `ackCorrelate`: class/ID (2-byte string, substring of message).
- `expectAck`: `ExpectAckOrNak` if CFG-class, `ExpectAckNone`
  otherwise. Exception: CFG-RST uses `ExpectAckNone` (receiver
  resets before it can reply).
- `dataTag`: TagUBX.
- `expectData`: `expectDataNone` for CFG set, `expectDataSingle`
  for CFG poll, `expectDataAmbig` for non-CFG periodic poll,
  `expectDataSingle` for non-CFG poll-only (MON-VER, MON-GNSS).
  CFG-RST uses `expectDataNone`.
- `dataMatch`: non-nil closure that checks the incoming packet's
  class/ID against the sent message's class/ID. (Without this,
  any UBX data packet would match.)

#### Response analyzer

Registered for TagUBX. Classifies incoming UBX packets:
- ACK-ACK: `responseAck`, `ackCorrelate` = class/ID from payload.
- ACK-NAK: `responseNak`, `ackCorrelate` = class/ID from payload.
- Other UBX: `responseMaybeData`.

### CASIC binary

#### Protocol behavior

CASIC is a binary protocol structurally similar to UBX. Each message
has a 2-byte identity (class + ID). The same two rules as UBX apply:
any CFG-class message gets ACK-ACK or ACK-NAK, and any message can
be polled by sending it with an empty payload. This gives the same
three basic patterns (CFG set, CFG poll, non-CFG poll).

CASIC also has a specific polling mechanism using CFG-MSG:

**Single-message poll.** CFG-MSG (class 0x06, ID 0x01) with the
target message's class/ID in the payload and rate set to 0xFFFF.
For example, to poll NAV-DOP (class 0x01, ID 0x02), send CFG-MSG
with payload `0x01 0x02 0xFF 0xFF`. The receiver replies with an
ACK-ACK for the CFG-MSG (since CFG-MSG is CFG-class), then the
requested NAV-DOP message with data.

**All-rates query.** Sending CFG-MSG with an empty payload requests
the output rates for all configured message types. The receiver
replies with an ACK-ACK for the CFG-MSG, followed by multiple
CFG-MSG response packets, one per message type. Each response
contains a class/ID and its output rate. The number of responses
depends on how many message types the receiver has configured and
is not known in advance. (This behavior is observed but not
documented in the CASIC protocol specification.)

#### Request analyzer

Implemented by CASBINMsg. Same structure as UBX, with additions:
- Single-message poll (CFG-MSG with rate=0xFFFF):
  `ackCorrelate` = CFG-MSG class/ID. `dataExpect` =
  `expectDataSingle`. `dataMatch` = non-nil closure that checks
  whether the incoming packet's class/ID matches the polled
  message's class/ID (which differs from the ACK's class/ID).
- All-rates query (CFG-MSG with empty payload):
  `dataExpect` = `expectDataMultiple`. `dataMatch` = non-nil
  closure that checks the incoming packet's class/ID is CFG-MSG.

#### Response analyzer

Registered for TagCASICBin. Same structure as UBX:
- ACK-ACK: `responseAck`, `ackCorrelate` = class/ID from payload.
- ACK-NAK: `responseNak`, `ackCorrelate` = class/ID from payload.
- Other CASIC: `responseMaybeData`.

### Allystar binary (ASBIN)

#### Protocol behavior

ASBIN is a binary protocol with different wire format from UBX
(different sync bytes and checksum algorithm) but the same
request/response rules: any CFG-class message gets ACK/NAK, and
any message can be polled with an empty payload. This gives the
same three patterns: CFG set (ACK only), CFG poll (ACK + data),
non-CFG poll (data only). As with UBX, some CFG messages that
cause a reset (e.g. CFG-SIMPLERST) do not produce a response.

#### Request analyzer

Implemented by ASBINMsg. Same as UBX.

#### Response analyzer

Registered for TagAllystarBin. Same as UBX.

### SDBP (Techtotop)

#### Protocol behavior

SDBP is a binary protocol with 2-byte message identity (class + ID).
There are several request/response patterns:

**Set command.** Send SDBP-CFG-UART-I2 (class 0x03, ID 0x21) with
configuration payload (6+ bytes). Receiver replies with PubAck
(class 0x01, ID 0x01) on success or PubNak (class 0x01, ID 0x02)
on failure. The response payload contains the class/ID of the
original message (`0x03 0x21`). This is the only response.

**Query.** Send SDBP-CFG-UART-I1 (class 0x03, ID 0x21) with a
short payload (0-1 bytes). Receiver replies with PubAck followed by
SDBP-CFG-UART-O, a data message with the same class/ID but a
different payload containing the current configuration. Similarly,
SDBP-QUE-VER (class 0x05, ID 0x01) with empty payload returns
PubAck followed by a version string.

**Commands with no response.** SDBP-CTL-RESTART (class 0x02,
ID 0x01) and SDBP-CTL-STANDBY (class 0x02, ID 0x04) produce no
response on success. On failure they reply with PubNak.

#### Request analyzer

Implemented by SDBPMsg.
- Set commands: `expectAck` `ExpectAckOrNak`, `dataExpect` =
  `expectDataNone`.
- Query commands: `expectAck` `ExpectAckOrNak`, `dataExpect` =
  `expectDataSingle`, `dataMatch` = non-nil closure checking the
  packet's class/ID against the sent message's class/ID.
- Restart/standby: `expectAck` `ExpectAckNakOnly`, `dataExpect` =
  `expectDataNone`. (PubNak arrives on failure; on success no
  response arrives, so completion is never confirmed.)

#### Response analyzer

Registered for TagSDBP.
- PubAck: `responseAck`, `ackCorrelate` = class/ID from payload.
- PubNak: `responseNak`, `ackCorrelate` = class/ID from payload.
- Other SDBP: `responseMaybeData`.

### PQTM (Quectel proprietary NMEA)

#### Protocol behavior

PQTM is a proprietary NMEA protocol used by Quectel receivers.
Commands and responses use the same sentence name. The response is
always a single sentence. There are several patterns:

**Write command.** Send `$PQTMCFGPPS,W,1,1,100,1,1,0*XX`.
Receiver replies `$PQTMCFGPPS,OK*XX`.

**Query.** Send `$PQTMCFGPPS,R,1*XX`. Receiver replies with the
current values in the same sentence:
`$PQTMCFGPPS,OK,1,1,100,1,1,0*XX`. The `OK` and the data fields
are in the same sentence -- there is no separate success/failure
packet.

**Error.** If a command fails, the receiver replies
`$PQTMCFGPPS,ERROR,1*XX` where the number is an error code
(1 = invalid parameters, 2 = execution failed, 3 = unsupported).

**Version query.** Send `$PQTMVERNO*XX`. The response contains data
fields directly without an OK prefix:
`$PQTMVERNO,LG290P03...,2024/04/30,...*XX`. This pattern (data
without OK) is used by several command types, not just PQTMVERNO.

Every command produces exactly one response sentence.

#### Request analyzer

Implemented by NMEAMsg (when vendor is QTM).
- `ackTag`: TagNMEA.
- `ackCorrelate`: sentence name (e.g. "PQTMCFGPPS").
- `expectAck`: `ExpectAckOrNak`.
- `dataTag`: TagNMEA.
- `dataExpect`: `expectDataWithAck` for queries, `expectDataNone`
  for write commands. For sentences like PQTMVERNO that return data
  without OK, `expectAck` `ExpectAckNakOnly` (ERROR is still
  possible), `dataExpect` = `expectDataSingle`.
- `dataMatch`: nil for queries and write commands (data is combined
  with the ACK or not expected). For PQTMVERNO, non-nil closure
  that checks the sentence name matches "PQTMVERNO".

#### Response analyzer

Registered for TagNMEA (shared with PAIR and other NMEA protocols).
For PQTM sentences:
- `PQTMCFGPPS,OK`: `responseAck`, `ackCorrelate` = "PQTMCFGPPS".
- `PQTMCFGPPS,OK,1,1,...`: `responseAck`, `ackCorrelate` =
  "PQTMCFGPPS" (data is carried with the ack).
- `PQTMCFGPPS,ERROR,1`: `responseNak`, `ackCorrelate` =
  "PQTMCFGPPS", `ackError` = "invalid parameters".
- `PQTMVERNO,...`: `responseData`.

### PAIR (Airoha proprietary NMEA)

#### Protocol behavior

PAIR is a proprietary NMEA protocol used by Airoha-based receivers
(including some Quectel modules). Unlike PQTM, PAIR has a dedicated
universal response sentence (PAIR001) separate from data.

**Set command.** Send `$PAIR062,1,1*XX` (set NMEA output). Receiver
replies with `$PAIR001,062,0*XX` where `062` is the original
command ID and `0` means success.

**Query.** Send `$PAIR073*XX`. Receiver replies with two separate
sentences: first `$PAIR001,073,0*XX` (success), then
`$PAIR073,5*XX` (the data, using the original command's sentence
name with the queried value).

**Error.** If a command fails, the PAIR001 result code indicates the
reason: `$PAIR001,062,3*XX` means unsupported command. Result codes
are 2 (failed), 3 (unsupported), 4 (parameter error), 5 (busy).

**Wait.** Some commands reply initially with result code 1, meaning
"still processing": `$PAIR001,062,1*XX`. The final success or error
PAIR001 follows later.

#### Request analyzer

Implemented by NMEAMsg (when vendor is AIR).
- `ackTag`: TagNMEA.
- `ackCorrelate`: command ID (e.g. "073").
- `expectAck`: `ExpectAckOrNak`.
- `dataTag`: TagNMEA.
- `dataExpect`: `expectDataSingle` for queries, `expectDataNone`
  for set commands.
- `dataMatch`: non-nil closure that checks whether the sentence
  name matches the original command (e.g. "PAIR073").

#### Response analyzer

Registered for TagNMEA (shared). For PAIR sentences:
- `PAIR001,073,0`: `responseAck`, `ackCorrelate` = "073".
- `PAIR001,073,1`: `responseWait`, `ackCorrelate` = "073".
- `PAIR001,073,N` (N >= 2): `responseNak`, `ackCorrelate` = "073",
  `ackError` = error description.
- `PAIR073,data`: `responseData`.

### Unicore

#### Protocol behavior

Unicore uses ASCII text commands terminated by CR/LF. Every command
produces a response in NMEA framing that echoes the full command
text. There are several distinct request/response patterns:

**Set command.** Send `CONFIG PPS ENABLE GPS POSITIVE 500000 1000
0 0\r\n`. Receiver replies with a single NMEA-framed sentence:
`$command,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0,response: OK*XX`.
On failure: `$command,CONFIG PPS ...,response: not support*XX`.
The sentence identifier is literally `command`; the response
includes the full original command text between the first and
second commas. (This response format is observed behavior handled
by the existing code; it is not documented in the Unicore protocol
specification.)

**CONFIG query.** Send `CONFIG\r\n` (bare command, no parameters).
Receiver replies with the success/failure sentence first:
`$command,CONFIG,response: OK*XX`. Then the receiver sends multiple
`$CONFIG,...` NMEA sentences, one per configuration property:

    $CONFIG,COM1,CONFIG COM1 460800*65
    $CONFIG,COM2,CONFIG COM2 115200*23
    $CONFIG,PPS,CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0*6E
    $CONFIG,SIGNALGROUP,CONFIG SIGNALGROUP 2*74

The number of `$CONFIG` sentences depends on the receiver's
configuration and is not known in advance.

**MASK query.** Send `MASK\r\n`. Same pattern as CONFIG: a
success/failure sentence, then multiple `$CONFIG,MASK,...` sentences:

    $CONFIG,MASK,MASK 5.000000*15
    $CONFIG,MASK,MASK GPS*4A

Both CONFIG and MASK data responses use `$CONFIG,...` framing. The
key field (second field) distinguishes them: keys starting with
"MASK" belong to the MASK query, everything else to CONFIG.

**MODE query.** Send `MODE\r\n`. The receiver sends a success/failure
sentence, then a single response in UNCA (Unicore ASCII) framing
(not NMEA):

    #MODE,81,GPS,FINE,2230,547967000,0,0,18,518;MODE ROVER SURVEY,*1B

Unlike CONFIG/MASK, this is always exactly one response.

#### Request analyzer

Implemented by LineMsg with ResponsePatternUnicore.
- `ackTag`: TagNMEA (the `$command,...` ACK arrives as NMEA).
- `ackCorrelate`: full command text (e.g.
  "CONFIG PPS ENABLE GPS POSITIVE 500000 1000 0 0"). The ACK
  echoes this text exactly.
- `expectAck`: `ExpectAckOrNak`.
- `dataTag`: TagNMEA for CONFIG/MASK queries (data arrives as
  `$CONFIG,...` NMEA sentences). TagUnicoreAscii for MODE query
  (data arrives as UNCA `#MODE,...`).
- `dataExpect`: `expectDataNone` for set commands.
  `expectDataMultiple` for CONFIG and MASK queries.
  `expectDataSingle` for MODE query.
- `dataMatch`: non-nil closure. For CONFIG queries, matches
  `$CONFIG,...` sentences where the key field does not start with
  "MASK". For MASK queries, matches `$CONFIG,...` sentences where
  the key does start with "MASK". For MODE, matches the UNCA
  message name against the command name.

#### Response analyzer (TagNMEA)

The TagNMEA response analyzer handles Unicore ACKs and CONFIG/MASK
data responses alongside PQTM and PAIR classification. For Unicore
responses:
- `$command,CMD_TEXT,response: OK*XX`: `responseAck`,
  `ackCorrelate` = CMD_TEXT.
- `$command,CMD_TEXT,response: <text>*XX` (where text is not OK):
  `responseNak`, `ackCorrelate` = CMD_TEXT, `ackError` = text
  (e.g. "not support", "unknown command").
- `$CONFIG,...`: `responseData`.

#### Response analyzer (TagUnicoreAscii)

Registered for TagUnicoreAscii. Classifies UNCA packets:
- Any UNCA packet: `responseData`.

### Line messages (generic text)

#### Protocol behavior

Unstructured text sent as raw bytes. There is no defined
request/response protocol, no framing, and no way to know whether
any received data is a response to the sent message or unrelated.

#### Request analyzer

Implemented by LineMsg with ResponsePatternNone.
- `expectAck`: `ExpectAckNone`.
- `dataTag`: empty (any tag).
- `dataExpect`: `expectDataUnknown`.
- `dataMatch`: non-nil closure that checks whether the packet data
  is printable ASCII with the expected line ending.

No response analyzer (no dedicated tag). Completion is never known.
The correlator shows packets matching `dataMatch` as
`LevelMaybeResponse` until timeout.

### Binary messages (generic raw)

#### Protocol behavior

Unstructured binary data sent as raw bytes. Same situation as line
messages: no defined protocol, no way to correlate responses.

#### Request analyzer

Implemented by BinaryMsg.
- `expectAck`: `ExpectAckNone`.
- `dataTag`: empty (any tag).
- `dataExpect`: `expectDataUnknown`.
- `dataMatch`: non-nil closure that checks whether the first two
  bytes of the packet data match the first two bytes of the sent
  message.

No response analyzer (no dedicated tag). Completion is never known.
The correlator shows packets matching `dataMatch` as
`LevelMaybeResponse` until timeout.

## TagNMEA response analyzer dispatch

The TagNMEA response analyzer handles multiple proprietary
protocols via a registry keyed by 3-letter vendor code. Both
request and response classification are dispatched through the
same interface.

```go
type proprietaryNMEA interface {
    classifyRequest(payload string) requestAnalysis
    classifyResponse(payload string) responseAnalysis
}

var proprietaryClassifiers = map[string]proprietaryNMEA{
    "QTM": pqtmClassifier{},
    "AIR": pairClassifier{},
}
```

Each classifier is an empty struct that calls the corresponding
lib package (`qtmmsg.ClassifyRequest`/`ClassifyResponse`,
`airmsg.ClassifyRequest`/`ClassifyResponse`) and maps the
package-specific types to `requestAnalysis`/`responseAnalysis`.

**Response dispatch** (TagNMEA `analyzeResponse`):
1. Extract NMEA payload.
2. Check for Unicore `$command,...` prefix → handle ACK/NAK.
3. Check for `$CONFIG,...` → `responseData`.
4. If standard GNSS talker NMEA (e.g. GPRMC) → `responseNotData`.
5. If proprietary, extract 3-letter vendor code.
6. Look up vendor in `proprietaryClassifiers`.
7. If found, call `classifyResponse(payload)` → return result.
8. If not found → `responseMaybeData`.

**Request dispatch** (NMEAMsg `analyzeRequest`):
1. Extract NMEA payload from sent message.
2. If proprietary, extract 3-letter vendor code.
3. Look up vendor in `proprietaryClassifiers`.
4. If found, call `classifyRequest(payload)` → return result.
5. If not found → default `requestAnalysis` with
   `ExpectAckNone`, `expectDataUnknown`.

## Library API changes

### qtmmsg

The existing `CheckResponse(sent, recv)` conflates response
classification with request correlation. It is replaced by two
functions: `ClassifyRequest` (examines sent payload) and
`ClassifyResponse` (examines received payload, request-independent).
The old `CheckResponse` is removed.

#### ClassifyResponse

```go
type ResponseKind int
const (
    ResponseNotPQTM ResponseKind = iota // not a PQTM sentence
    ResponseOK                           // OK, no data (e.g. PQTMCFGPPS,OK)
    ResponseOKData                       // OK with data (e.g. PQTMCFGPPS,OK,1,1,...)
    ResponseError                        // ERROR with error code
    ResponseData                         // data without OK (e.g. PQTMVERNO,...)
)

type ResponseClass struct {
    Kind     ResponseKind
    Sentence string // sentence name (e.g. "PQTMCFGPPS")
    Error    string // non-empty for ResponseError
}

func ClassifyResponse(recv string) ResponseClass
```

The TagNMEA response analyzer maps these to `responseAnalysis`:
- `ResponseOK` / `ResponseOKData` -> `responseAck`,
  `ackCorrelate` = Sentence.
- `ResponseError` -> `responseNak`, `ackCorrelate` = Sentence,
  `ackError` = Error.
- `ResponseData` -> `responseData`.
- `ResponseNotPQTM` -> not handled by PQTM (fall through to other
  classifiers).

#### ClassifyRequest

```go
type RequestKind int
const (
    RequestCommand RequestKind = iota // expects ResponseOK or ResponseError
    RequestQuery                      // expects ResponseOKData or ResponseError
    RequestVerno                      // expects ResponseData or ResponseError
)

type RequestClass struct {
    Kind     RequestKind
    Sentence string // sentence name (e.g. "PQTMCFGPPS")
}

func ClassifyRequest(sent string) RequestClass
```

Classification logic:
- Contains `,W,` -> `RequestCommand`.
- Contains `,R,` -> `RequestQuery`.
- Sentence is `PQTMVERNO` -> `RequestVerno`.
- Otherwise -> `RequestCommand` (default).

The NMEAMsg request analyzer maps these to `requestAnalysis`:
- `RequestCommand` -> `ExpectAckOrNak`, `expectDataNone`.
- `RequestQuery` -> `ExpectAckOrNak`, `expectDataWithAck`.
- `RequestVerno` -> `ExpectAckNakOnly`, `expectDataSingle`.

In all cases: `ackTag` = TagNMEA, `ackCorrelate` = Sentence,
`dataTag` = TagNMEA. `dataMatch` = nil for commands and queries;
non-nil for `RequestVerno` (checks sentence name).

### airmsg

The existing `CheckResponse(sent, recv)` is replaced by
`ClassifyRequest` and `ClassifyResponse`. The old `CheckResponse`
is removed.

#### ClassifyResponse

```go
type ResponseKind int
const (
    ResponseNotPAIR ResponseKind = iota // not a PAIR sentence
    ResponseOK                           // PAIR001 with result 0
    ResponseWait                         // PAIR001 with result 1
    ResponseError                        // PAIR001 with result >= 2
    ResponseData                         // data echoing command ID (e.g. PAIR073,5)
)

type ResponseClass struct {
    Kind      ResponseKind
    CommandID string // 3-digit command ID (e.g. "073")
    Error     string // non-empty for ResponseError
}

func ClassifyResponse(recv string) ResponseClass
```

The TagNMEA response analyzer maps these to `responseAnalysis`:
- `ResponseOK` -> `responseAck`, `ackCorrelate` = CommandID.
- `ResponseWait` -> `responseWait`, `ackCorrelate` = CommandID.
- `ResponseError` -> `responseNak`, `ackCorrelate` = CommandID,
  `ackError` = Error.
- `ResponseData` -> `responseData`.
- `ResponseNotPAIR` -> not handled by PAIR (fall through to other
  classifiers).

#### ClassifyRequest

```go
type RequestKind int
const (
    RequestCommand RequestKind = iota // expects PAIR001 only
    RequestQuery                      // expects PAIR001 + data
)

type RequestClass struct {
    Kind      RequestKind
    CommandID string // 3-digit command ID (e.g. "073")
}

func ClassifyRequest(sent string) RequestClass
```

Classification logic: the default is even = command, odd = query.
An internal sorted exception list of command IDs overrides this
for cases where parity does not match the actual behavior.

IDs where parity gives the wrong answer (sorted):

    3, 5, 7, 8, 11, 20, 23, 30, 412, 507, 508, 511, 513,
    596, 753, 755, 756, 757, 758, 906, 907, 908

The NMEAMsg request analyzer maps these to `requestAnalysis`:
- `RequestCommand` -> `ExpectAckOrNak`, `expectDataNone`.
- `RequestQuery` -> `ExpectAckOrNak`, `expectDataSingle`.

In all cases: `ackTag` = TagNMEA, `ackCorrelate` = CommandID,
`dataTag` = TagNMEA. For queries, `dataMatch` = non-nil closure
that checks whether the received sentence name matches the
original command (e.g. "PAIR073").

## Testing

### Approach

Tests are table-driven and focused on the `Correlator` public API.
Each test case sends messages parsed from real TOML message files
and receives constructed response packets, checking the returned
`Correlation` at each step.

Message files live in `gps/msgfile/testdata/` and use the same
format as `configs/gpsmsg/*.toml`. Definitions can be copied
directly from production message files. A test helper function
takes a filename (relative to testdata) and a list of test cases:

```go
func runCorrelatorTests(t *testing.T, file string, tests []correlatorTest)
```

### Test case structure

```go
type correlatorTest struct {
    name   string
    tags   []string
    events []event
}
```

The `tags` field works like the `-t` flag of `satpulsetool gps`:
it resolves to a sequence of `RawMsg` values from the parsed TOML
file. Multiple tags produce messages in tag order. Multiple
messages per tag produce messages in file order within that tag.

### Events

`event` is an interface. The test runner processes events in order:

- `sendEvent{}`: calls `NotifyMsgSent` with the next message in
  the resolved sequence (advancing an internal cursor).
- `readyToSend{want: bool}`: calls `ReadyToSend` on the next
  message and asserts the result.
- recv helpers (see below): call `CorrelatePacket` with the
  constructed packet and stash the returned `Correlation`.
- `expect{...}`: asserts against the most recent `Correlation`.
  Zero-valued fields are not checked.
- `checkDone{canAcceptMore: bool}`: asserts `CanAcceptMore()`.
- `checkMissing{ack: []int, data: []int}`: asserts `Missing()`,
  where ints are indices into the resolved message sequence.

```go
type expect struct {
    ack       AckKind
    relevance RelevanceLevel
    msgIndex  *int // index into resolved messages; nil = InResponseTo is nil
}
```

### Recv helpers

Each protocol has helper functions that construct minimal valid
response packets -- just enough structure for the response analyzer
to classify. Payload content beyond what the analyzer inspects is
left empty.

**UBX:**

```go
recvUBX(ubxbin.CfgTp5ID)       // data packet with class/ID of CfgTp5
recvUBXAck(ubxbin.CfgTp5ID)    // ACK-ACK acknowledging CfgTp5
recvUBXNak(ubxbin.CfgTp5ID)    // ACK-NAK for CfgTp5
```

The MsgID argument is always the message being talked about. For
ACK/NAK, the helper constructs an ACK-ACK/ACK-NAK packet with the
given class/ID in the payload. For data, it constructs a packet
with that class/ID and an empty payload.

**CASIC binary:** same pattern as UBX.

```go
recvCASBIN(casicbin.MsgID)
recvCASBINAck(casicbin.MsgID)
recvCASBINNak(casicbin.MsgID)
```

**Allystar binary:** same pattern as UBX.

```go
recvASBIN(asbinID)
recvASBINAck(asbinID)
recvASBINNak(asbinID)
```

**SDBP:**

```go
recvSDBP(sdbpID)               // data packet
recvSDBPAck(sdbpID)            // PubAck
recvSDBPNak(sdbpID)            // PubNak
```

**NMEA (PQTM, PAIR, Unicore, generic):**

```go
recvNMEA("PQTMCFGPPS,OK")                         // PQTM OK (write ack)
recvNMEA("PQTMCFGPPS,OK,1,1,100,2,1,0")           // PQTM OK+data (query ack)
recvNMEA("PQTMCFGPPS,ERROR,1")                     // PQTM error
recvNMEA("PQTMVERNO,LG290P03...,2024/04/30,...")   // PQTM data without OK
recvNMEA("PAIR001,073,0")                           // PAIR OK
recvNMEA("PAIR001,073,1")                           // PAIR wait
recvNMEA("PAIR001,073,3")                           // PAIR error
recvNMEA("PAIR073,5")                               // PAIR data
```

NMEA responses are just text. The helper wraps the payload in
`$...*XX\r\n` framing with a computed checksum.

**Unicore:**

```go
recvNMEA("command,CONFIG PPS ...,response: OK")     // Unicore ACK
recvNMEA("CONFIG,PPS,CONFIG PPS ...")               // Unicore data
recvUnicoreAscii("#MODE,...;MODE ROVER SURVEY,")    // UNCA data
```

Unicore ACKs and CONFIG data use NMEA framing (TagNMEA), so they
use `recvNMEA`. MODE responses use UNCA framing (TagUnicoreAscii).

### Example test cases

```go
runCorrelatorTests(t, "ubx-test.toml", []correlatorTest{
    {
        name: "CFG poll ACK then data",
        tags: []string{"get-tp5"},
        events: []event{
            sendEvent{},
            recvUBXAck(ubxbin.CfgTp5ID),
            expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: ptr(0)},
            recvUBX(ubxbin.CfgTp5ID),
            expect{relevance: LevelSoleResponse},
            checkDone{canAcceptMore: false},
        },
    },
    {
        name: "CFG poll data arrives before ACK",
        tags: []string{"get-tp5"},
        events: []event{
            sendEvent{},
            recvUBX(ubxbin.CfgTp5ID),
            expect{relevance: LevelSoleResponse},
            recvUBXAck(ubxbin.CfgTp5ID),
            expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: ptr(0)},
            checkDone{canAcceptMore: false},
        },
    },
    {
        name: "two CFG sets with pacing",
        tags: []string{"set-tp5", "set-prt"},
        events: []event{
            sendEvent{},
            readyToSend{want: true}, // different class/ID, no conflict
            sendEvent{},
            recvUBXAck(ubxbin.CfgTp5ID),
            expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: ptr(0)},
            recvUBXAck(ubxbin.CfgPrtID),
            expect{ack: AckAck, relevance: LevelAckOnly, msgIndex: ptr(1)},
            checkDone{canAcceptMore: false},
        },
    },
    {
        name: "no response, missing ACK reported",
        tags: []string{"set-tp5"},
        events: []event{
            sendEvent{},
            checkDone{canAcceptMore: true},
            checkMissing{ack: []int{0}},
        },
    },
})
```

### What to test

Key scenarios per protocol:

- ACK, NAK, and no-response cases for each ACK expectation type
  (`ExpectAckOrNak`, `ExpectAckNakOnly`, `ExpectAckNone`).
- Data responses: sole, multiple, with-ack, ambiguous, unknown.
- Ordering: ACK before data, data before ACK, interleaved.
- Pacing: `readyToSend` true/false with conflicting and
  non-conflicting ACK correlations.
- Ambiguity: ACK matching multiple requests produces
  `LevelAmbigResponse` with no state update.
- Completion: `CanAcceptMore` transitions at the right time.
- `Missing` reports: correct sets for missing ACK and data.
- Unrelated packets: periodic data gets `LevelNotResponse`.

## Implementation phases

### Phase 1: Correlator skeleton

Public interface and core implementation of the `Correlator`:
`NewCorrelator`, `ReadyToSend`, `NotifyMsgSent`, `CorrelatePacket`,
`CanAcceptMore`, `Missing`. All public types (`Correlation`,
`RelevanceLevel`, `AckKind`, etc.) and internal types
(`requestAnalysis`, `responseAnalysis`, `requestState`, etc.).
No protocol-specific request or response analyzers yet. Test
helper infrastructure (`runCorrelatorTests`, event types, `expect`,
`checkDone`, `checkMissing`).

### Phase 2: gpscmd changes

Replace `PacketAnalyzer` usage in `internal/gpscmd/response.go`
with `Correlator`. Implement `ReadyToSend` pacing in the send
loop, `CanAcceptMore` early stop in the read loop, and `Missing`
post-timeout warnings.

### Phase 3: UBX

a) Write test cases and testdata TOML for UBX scenarios: CFG set,
   CFG poll, non-CFG poll, CFG-RST, pacing, ambiguity, missing.
b) Implement UBX request analyzer (`UBXMsg.analyzeRequest`) and
   UBX response analyzer (registered for TagUBX).
c) Run test cases.
d) Test against real hardware.

### Phase 4: Unicore

a) Write test cases and testdata TOML for Unicore scenarios: set
   command, CONFIG query, MASK query, MODE query.
b) Implement Unicore request analyzer (`LineMsg` with
   `ResponsePatternUnicore`), TagNMEA response analyzer (Unicore
   dispatch: `$command,...` ACKs, `$CONFIG,...` data), and
   TagUnicoreAscii response analyzer (UNCA data).
c) Run test cases.
d) Test against real hardware.

### Phase 5: PQTM

a) Write test cases and testdata TOML for PQTM scenarios: write
   command, query (OK+data), PQTMVERNO (data without OK), error.
b) Implement PQTM request/response classifiers in `qtmmsg`
   (`ClassifyRequest`, `ClassifyResponse`), replacing
   `CheckResponse`. Wire into TagNMEA response analyzer via
   `proprietaryNMEA` registry.
c) Run test cases.
d) Test against real hardware.

### Phase 6: PAIR

a) Write test cases and testdata TOML for PAIR scenarios: set
   command, query (PAIR001 + data), wait, error.
b) Implement PAIR request/response classifiers in `airmsg`
   (`ClassifyRequest`, `ClassifyResponse`), replacing
   `CheckResponse`. Wire into TagNMEA response analyzer via
   `proprietaryNMEA` registry.
c) Run test cases.
d) Test against real hardware.

### Phase 7: CASIC

a) Write test cases and testdata TOML for CASIC scenarios: CFG
   set, CFG poll, non-CFG poll, single-message poll (CFG-MSG),
   all-rates query.
b) Implement CASIC request and response analyzers.
c) Run test cases.
d) Test against real hardware.

### Phase 8: Allystar

a) Write test cases and testdata TOML for Allystar scenarios:
   same patterns as UBX (CFG set, CFG poll, non-CFG poll, reset).
b) Implement Allystar request and response analyzers.
c) Run test cases.
d) Test against real hardware.

### Phase 9: SDBP

a) Write test cases and testdata TOML for SDBP scenarios: set
   command, query (PubAck + data), restart/standby (PubNak only,
   no response on success).
b) Implement SDBP request and response analyzers.
c) Run test cases.
d) Test against real hardware.

## Follow-ups

Issues discovered during implementation of phases 7-9 that
should be addressed as follow-up work.

### Fix ExpectAckNone for reset messages

A NAK is always possible for any message in any protocol (as a
generic "unsupported message" rejection), so `ExpectAckNone`
should never be used for binary protocol messages. The question
for reset/restart commands is whether the protocol specifies an
ACK before resetting (`ExpectAckOrNak`) or no ACK on success
(`ExpectAckNakOnly`). This needs to be verified per-protocol by
checking protocol docs and testing with hardware. PQTM and PAIR
reset commands should also be checked.

The current UBX and CASIC analyzers use `ExpectAckNone` for
their reset messages, which should be changed to
`ExpectAckNakOnly` at minimum.

### Wait-for-ACK message property

Add a boolean `MsgCommon` property (TOML key `waitForAck` or
similar) that forces the sender to wait for the ACK/NAK before
sending the next message, even when `ReadyToSend` would allow
it. Currently the sender only waits when the next message's ACK
correlation would conflict with a pending request. The UBX spec
explicitly requires waiting for each ACK before sending the
next message; the current conflict-only pacing is an
optimization that works in practice but deviates from the spec.
This property would restore spec-compliant behaviour, and could
be the default for protocols where the spec mandates it.

### Unknown proprietary NMEA responses

For proprietary NMEA protocols where we don't understand the
vendor code (PXYZ where XYZ is not in `proprietaryClassifiers`),
we should accept as responses only:

- PXYZ sentences with matching vendor code XYZ.
- NMEA TXT sentences (any valid talker ID, e.g. GPTXT,
  GNTXT), since some proprietary protocols respond using
  TXT messages rather than their own sentence format.

Currently all unknown proprietary sentences get
`responseMaybeData` regardless of vendor code, which is too
broad.

### UBX-INF informational messages

UBX-INF-* messages (ERROR, WARNING, NOTICE, etc.) can
accompany ACK/NAK responses to provide additional error context.
The UBX response analyzer returns `responseMaybeData` for these,
but the correlator's data matching then fails (INF class 0x04
doesn't match any request's class/ID), so they end up as
`LevelNotResponse` and are silently dropped. The INF message
classes are already defined in `ubxbin/inf.go`.

These are spontaneous human-readable messages produced by the
receiver that could be related to what was sent. May need a new
response kind to handle this — distinct from data responses
(not correlated to a specific request) and from ACK/NAK (no
state update). NMEA TXT messages might use this same response
kind. Needs experimentation with hardware.

### Documentation updates

The message file format documentation and schema need updating
for the new `waitLimit` key and any other format additions.
The man page needs updating for the changed `--capture`
behavior with `-m` and `-t` (early stop when all responses
received, `waitLimit`-based deadlines).

- `configs/gpsmsg/format.md`: document `waitLimit` key and
  updated response handling behavior.
- `configs/gpsmsg/gpsmsg-schema.json`: add `waitLimit` to the
  schema definitions.
- `docs/man/satpulsetool-gps.1.md`: update `--capture`
  description to reflect that it now adds capture time *after*
  response waiting is complete, rather than being the sole
  timeout.

## Key files

- This plan: `plan/msgfile-response-redesign.md`
- Current matchers: `gps/msgfile/binary.go`, `gps/msgfile/text.go`
- Current analyzer: `gps/msgfile/msgfile.go` (PacketAnalyzer)
- Current send loop: `internal/gpscmd/gpscmd.go` (sendMsg)
- Current response handler: `internal/gpscmd/response.go`
- Unicore configurator (inspiration): `gps/internal/unc/config.go`
- UBX configurator (inspiration): `gps/internal/ubx/ubxcfg.go`
- Issue: #249
