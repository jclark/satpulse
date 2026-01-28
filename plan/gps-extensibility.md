# GPS message extensibility using TOML

Part of #200.

We want to use TOML because our config file is in TOML format.
YAML will be too complex for our user base.
Challenge is to find a way to express things in TOML that is
- simple to understand
- reasonably convenient to write
- can be typed properly

Key ideas:
- use tag key to name things, rather than naming things using TOML table names
- instead using TOML table names for type of message (thus ensuring things are well typed)

## Motivation

`satpulsetool gps` provides protocol-agnostic configuration, but:

- there will always be protocol-specific details that cannot be abstracted
- only UBX and Unicore protocols are currently supported; adding new protocols requires significant work
- technical users are comfortable reading their receiver's manual and constructing exact commands

## CLI interface

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m um980.toml -t setup
```

**New flags:**

| Flag | Description |
|------|-------------|
| `-m`, `--msg-file PATH` | Path to TOML file containing message definitions |
| `-t`, `--tag NAME,...` | Comma-separated list of tags to send (in order); default is empty string |
| `--show-tags` | List all tags in the message file with their descriptions, then exit |

**Constraints:**

The `-m` flag cannot be combined with config flags like `--gnss`, `--pps`, `--save`, etc. This avoids ambiguity about ordering of manual messages versus higher-level configuration.

When `-m` is used without `--capture`, the tool defaults to `--capture 2` to show receiver responses.

`--show-tags` requires `-m` but does not require `--serial-device` or `--socket` (no connection needed). It prints to stderr (like `--help`) and exits after printing. Example output:

```
No tag: Configure PPP settings
Tags:
  version - Query firmware version
  query-pps
  set-pps - Configure PPS output
  save - Save configuration to NVM
```

Format mimics `--help` style:
- `No tag:` line appears only if there are messages with empty tag (omit line entirely if none)
- `Tags:` section lists named tags with two-space indent (omit section entirely if no named tags)
- Tags without descriptions show just the tag name

## TOML message file

Following is user-facing explanation of how TOML message file works.

### Message types

| Type | Keys | Framing |
|------|------|---------|
| `[[line]]` | `text`, `eol`, `delay`, `tag`, `description` | appends eol (default `\r\n`) |
| `[[binary]]` | `hex`, `delay`, `tag`, `description` | none |
| `[[nmea]]` | `text`, `delay`, `tag`, `description` | prepends `$`, appends `*XX\r\n` checksum |

### Simplest case

Example file called um980-ppp.toml

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This specifies two messages each of which are lines.
Using `-m um980-ppp.toml` will send all the messages (since all have empty tag, which is the default for `-t`).
Each line will be terminated with CR/LF.

### Delay key

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
delay = 0.1

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This will add delay 0.1 seconds after sending the first line.

### Defaults

```toml
[default.line]
delay = 0.1

[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This will add a delay of 0.1 seconds after every line.

### Line terminator

`eol` key is a string specifying the line terminator.

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
eol = "\n"
```

This is usually specified in default.line table:

```toml
[default.line]
eol = "\n"

[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

You can use `eol = ""` to send plain text with no line terminator.

### Binary

```toml
[[binary]]
hex = "B562068A0900000100000100321001DEED"
```

Hex string must have even length. Whitespace within the hex string is ignored.

### NMEA

```toml
[[nmea]]
text = "PCAS04,3"
```

This is like `line`, except:
- leading `$` is prepended if missing
- trailing `*XX` checksum is computed and appended if missing
- CRLF is always appended

Validates result using `internal/nmea` and returns error if invalid.

### Tags

Call the file `um980.toml`:

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
tag = "ppp"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
tag = "ppp"

[[line]]
text = "SIGNALGROUP 1"
tag = "signalgroup1"

[[line]]
text = "SIGNALGROUP 2"
tag = "signalgroup2"
```

Use `-m um980.toml -t ppp` to send messages with tag ppp.

Use `-m um980.toml -t signalgroup1,ppp` to send messages with tag signalgroup1 and then messages with tag ppp.

Default tag can be set:

```toml
[default.line]
tag = "setup"

[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"

[[line]]
text = "SIGNALGROUP 1"

[[line]]
text = "SIGNALGROUP 2"
tag = "signalgroup2"
```

If there is no default tag, messages have the empty tag `""` by default.

`-t foo,,bar` will send messages with foo tag, then empty tag, then bar tag.

The default value for `-t` option is the empty string `""`.

**Type homogeneity requirement:**

All messages with the same tag must have the same type. All messages selected by a single `-t` option must also have the same type. This is because response display differs by type: line/nmea messages show text responses; binary messages don't display responses.

### Description key

`description` is an optional string that documents what a tag does. It is displayed by `--show-tags`.

```toml
[[nmea]]
text = "PQTMVERNO"
tag = "version"
description = "Query firmware version"

[[nmea]]
text = "PQTMCFGPPS,R,1"
tag = "query-pps"
description = "Query PPS configuration"
```

When multiple messages share the same tag, each can have a `description`. The rule is: **all non-empty descriptions for a tag must be identical**. This allows flexibility:

```toml
# Option 1: description on first message only
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
tag = "nmea-satpulse"
description = "Enable NMEA messages for satpulse"

[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,1"
tag = "nmea-satpulse"

[[nmea]]
text = "PQTMCFGMSGRATE,W,GSV,1"
tag = "nmea-satpulse"

# Option 2: repeat for clarity (must match)
[[nmea]]
text = "PQTMRESTOREPAR"
tag = "factory-reset"
description = "Restore factory defaults and reboot"

[[nmea]]
text = "PQTMSRR"
tag = "factory-reset"
description = "Restore factory defaults and reboot"
```

Default is empty string `""`. Not allowed in `[default.line]` etc - validation error if specified there.

### Protocol specific message types

```toml
[[ubx]]
tag = "gps-l5-health"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10320001, 1]
```

## Implementation

### Go types

Optional fields use pointer types so we can distinguish "not specified" from "empty/zero":

```go
// LineMsg represents a [[line]] entry or [default.line].
type LineMsg struct {
	Text        string   `toml:"text"`
	EOL         *string  `toml:"eol"`
	Delay       *float64 `toml:"delay"`
	Tag         *string  `toml:"tag"`
	Description string   `toml:"description"`
}
```

Key points:
- Same type for default and messages; default is single, messages are slice
- Pointer fields allow distinguishing unset from zero/empty values
- `Description` is plain string (not pointer) - no default mechanism, validation error if set in `[default.*]`
- Validation: `Default.Line.Text` must be empty; each `Line[i].Text` must be non-empty
- Validation: all non-empty descriptions for the same tag must be identical

### Loading and defaulting

Uses `defaultMsgFile()` to pre-fill defaults, then TOML decoder overwrites only fields present in file.

For each message, `applyLineDefaults()` copies default pointer if message field is nil.

### Core functions

In `msgfile.go`:
- `LoadMsgFile(path string) (*MsgFile, error)` - parse TOML file
- `(mf *MsgFile) Validate() error` - validate structure
- `(mf *MsgFile) TaggedLineMsgs(tags []string) []LineMsg` - filter and apply defaults
- `LineMsgsToRaw(msgs []LineMsg) []rawMsg` - convert for sending

### gpscmd.go structure

The `run()` function handles both config mode and message file mode:
- Load message file in `Cmd()` before opening connection
- Branch after `startScan()`: call `runMsgs()` if message file provided, else `runConfig()`
- Shared cleanup: capture phase, stop scanner, wait for goroutines

### Response display

For line and nmea messages, display text responses from the receiver.

```go
type responsePrinter struct {
	w       io.Writer
	lineBuf []byte
}

func (rp *responsePrinter) HandlePacket(pkt scan.Packet)
func (rp *responsePrinter) Flush()
```

Packet handling:
- **Unrecognized packets:** Accumulate in line buffer. Print on EOL (LF or CRLF). Clear buffer on non-printable chars (outside 0x20-0x7E, tab, CR, LF).
- **Recognized packets:** Print if all printable, otherwise skip.
- **NMEA (when MsgTypeNMEA):** Skip normal GNSS talker sentences; show proprietary responses.

Binary messages do not display responses (pass nil responsePrinter).

## Implementation steps

### Step 1: Basic line messages (done)

**Files:**
- `internal/gpscmd/msgfile.go` - `LineMsg`, `MsgFile`, `LoadMsgFile`, `Validate`, `TaggedLineMsgs`, `LineMsgsToRaw`
- `internal/gpscmd/msgfile_test.go` - parsing tests
- `internal/gpscmd/gpsflags.go` - `-m`/`--msg-file` flag
- `internal/gpscmd/gpscmd.go` - `runMsgs()`, `runRawMsgs()`, `runConfig()`

**Done:**
- `LineMsg` with pointer fields for optional values
- `Default.Line` with built-in default EOL `"\r\n"`
- `TaggedLineMsgs(tags)` filters by tag and applies defaults
- `run()` branches after `startScan()` based on whether message file provided
- Message file loaded and validated in `Cmd()` before opening connection

### Step 2: Response display (done)

**Files:**
- `internal/gpscmd/gpscmd.go` - `responsePrinter` type and methods

**Details:**
- `responsePrinter` handles displaying text responses from the receiver
- `runMsgs()` reads packets from pCh during delays via `sendMsg()`
- `responsePrinter` passed to `keepReading` for capture phase

### Step 3: Defaults and EOL (done)

Implemented in Step 1.

### Step 4: Binary messages (done)

**Details:**
- Add `BinaryMsg` type with `Hex`, `Delay`, `Tag`
- Add `Binary []BinaryMsg` and `Default.Binary` to `MsgFile`
- Hex decoding: strip whitespace, validate even length and hex chars
- `runBinaryMsgs([]BinaryMsg)` converts to `[]rawMsg` and calls `runRawMsgs()` with nil responsePrinter
- Make `LineMsg` and `BinaryMsg` implement `UserMsg` interface
- At this stage: file must contain only one type (all line or all binary)

### Step 5: Tags (done)

**Files:**
- `internal/gpscmd/gpsflags.go` - add `-t`/`--tag` flag

**Details:**
- Add `Tag` field defaulting
- `TaggedMsgs(tags)` filters by tags, preserving order from tags argument
- Validate: all messages for each tag have same type
- Return typed slice (`[]LineMsg` or `[]BinaryMsg`) to control response display
- Add tests to msgfile_test.go

### Step 6: NMEA messages

**Details:**
- Add `NMEAMsg` type with `Text`, `Delay`, `Tag`
- Add `NMEA []NMEAMsg` and `Default.NMEA` to `MsgFile`
- Build: prepend `$` if missing, compute/append `*XX` checksum if missing, append `\r\n`
- Validate with `internal/nmea`
- Update `responsePrinter` for NMEA: use `nmea.CheckSyntax()` to get `SentenceSyntaxFlags`, display unless `IsValidGNSSTalkerNMEA()` is true

### Step 7: Show tags and descriptions

**Files:**
- `internal/gpscmd/msgfile.go` - add `Description` field to `MsgCommon`
- `internal/gpscmd/gpsflags.go` - add `--show-tags` flag
- `internal/gpscmd/gpscmd.go` - handle `--show-tags` in `Cmd()`

**Details:**
- Add `Description string` to `MsgCommon` with `toml:"description"` (not a pointer - no default mechanism)
- Add `showTags bool` to `flagVars`
- Add validation: for each tag, collect non-empty descriptions; error if not all equal
- Add validation: error if `description` is non-empty in any `[default.*]` section
- Add `(mf *MsgFile) TagDescriptions() []TagDesc` returning tag/description pairs in order of first occurrence
- `--show-tags` loads file, validates, prints tags, exits
- `--show-tags` requires `-m` but not `-d`/`--socket`

### Step 8: UBX/CASBIN messages (fire-and-forget)

**Files:**
- `internal/ubx/bin/common.go` - add `const Endian = binary.LittleEndian`
- `internal/casic/bin/common.go` - add `const Endian = binary.LittleEndian`, add ACK/NAK types
- `internal/gpscmd/payload.go` - new file
- `internal/gpscmd/payload_test.go` - tests
- `internal/gpscmd/gpscmd.go` - refactor `responsePrinter`

#### 8a: Add CASIC ACK/NAK types

Add `internal/casic/bin/ack.go`.

Per CASIC spec section 2.10, ACK/NAK payloads are 4 bytes: `clsID` (U1), `msgID` (U1), `res` (U2 reserved).
This differs from UBX where MsgID is a single uint16 field.

```go
package bin

const (
	AckNakID MsgID = clsAck | (0x00 << 8)
	AckAckID MsgID = clsAck | (0x01 << 8)
)

// AckNak is sent when a CFG message was not processed correctly.
// Payload: clsID (U1), msgID (U1), res (U2).
type AckNak struct {
	ClsID uint8
	MsgID uint8
	_     uint16 // reserved
}

func (m *AckNak) ID() MsgID { return AckNakID }

// AckedMsgID returns the MsgID of the message being NAK'd.
func (m *AckNak) AckedMsgID() MsgID { return makeMsgID(m.ClsID, m.MsgID) }

// AckAck is sent when a CFG message was processed correctly.
// Payload: clsID (U1), msgID (U1), res (U2).
type AckAck struct {
	ClsID uint8
	MsgID uint8
	_     uint16 // reserved
}

func (m *AckAck) ID() MsgID { return AckAckID }

// AckedMsgID returns the MsgID of the message being ACK'd.
func (m *AckAck) AckedMsgID() MsgID { return makeMsgID(m.ClsID, m.MsgID) }

func init() {
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
}
```

#### 8b: Register CASIC message ID names

Add `internal/casic/bin/msg.go` to register known message IDs without full implementations. Follow the pattern from `ubx/bin/msg.go` and `uncmsg/other.go`:

```go
package bin

const (
	CfgTPID MsgID = clsCfg | (0x03 << 8)
)

func init() {
	idNameMap[CfgTPID] = "TP"
	// ...
}
```

This ensures `MsgID.String()` returns e.g. "CFG-TP" instead of "CFG-0x03" when printing ACK/NAK responses.

#### 8c: Refactor responsePrinter for per-protocol handling

Current `responsePrinter` mixes concerns. Refactor to clearly separate protocol-specific logic:

```go
// responsePrinter handles displaying responses from the receiver.
type responsePrinter struct {
	w       io.Writer
	lineBuf []byte
}

func (rp *responsePrinter) handlePacket(pkt scan.Packet) {
	if pkt.Format == nil {
		rp.handleUnrecognized([]byte(pkt.Data))
		return
	}
	rp.flushLine()
	if s := rp.formatPacket(pkt); s != "" {
		io.WriteString(rp.w, s)
	}
}

// formatPacket returns the string to print for a recognized packet.
// Dispatches to protocol-specific formatters.
func (rp *responsePrinter) formatPacket(pkt scan.Packet) string {
	switch {
	case pkt.HasTag(ubx.Tag):
		return rp.formatUBX(pkt)
	case pkt.HasTag(casic.Tag):
		return rp.formatCASBIN(pkt)
	case pkt.HasTag(nmea.Tag):
		return rp.formatNMEA(pkt)
	default:
		return rp.formatText(pkt)
	}
}

func (rp *responsePrinter) formatUBX(pkt scan.Packet) string {
	msg, err := ubxbin.ParseMsg(pkt.Data)
	if err != nil {
		return ""
	}
	switch m := msg.(type) {
	case *ubxbin.AckAck:
		return fmt.Sprintf("UBX-ACK-ACK: %s\n", m.MsgID)
	case *ubxbin.AckNak:
		return fmt.Sprintf("UBX-ACK-NAK: %s\n", m.MsgID)
	}
	return ""
}

func (rp *responsePrinter) formatCASBIN(pkt scan.Packet) string {
	msg, err := casbin.ParseMsg(pkt.Data)
	if err != nil {
		return ""
	}
	switch m := msg.(type) {
	case *casbin.AckAck:
		return fmt.Sprintf("CASBIN-ACK-ACK: %s\n", m.AckedMsgID())
	case *casbin.AckNak:
		return fmt.Sprintf("CASBIN-ACK-NAK: %s\n", m.AckedMsgID())
	}
	return ""
}

func (rp *responsePrinter) formatNMEA(pkt scan.Packet) string {
	// Skip standard GNSS talker sentences but show TXT and proprietary
	if nmea.CheckSyntax(pkt.Data).IsValidGNSSTalkerNMEA() && pkt.Data[3:6] != "TXT" {
		return ""
	}
	return rp.formatText(pkt)
}

func (rp *responsePrinter) formatText(pkt scan.Packet) string {
	// Strip trailing EOL, check all chars printable
	s := strings.TrimRight(pkt.Data, "\r\n")
	if s == "" {
		return ""
	}
	for i := range len(s) {
		if !isPrintable(s[i]) {
			return ""
		}
	}
	return s + "\n"
}
```

This structure:
- Separates protocol detection from formatting logic
- Makes it easy to add new protocol handlers
- Keeps ACK/NAK formatting close to where it's used
- Each `format*` method returns empty string to skip printing

#### 8d: Payload encoding

**Details:**
- `ubx/bin` and `casic/bin` own endianness; gpscmd uses `bin.Endian` for encoding
- `EncodePayload(types string, values []any) ([]byte, error)` in gpscmd
- Type specifiers: U1, U2, U4, I1, I2, I4, R4, R8
- `UBXLikeMsg` struct in gpscmd with fields: `Class`, `ID`, `Payload` (with `Types` and `Values`), `Delay`, `Tag`
- `UBXMsg` and `CASBINMsg` embed `UBXLikeMsg`; each implements `GetBytes()` calling its `bin.PackMsg()`
- Build packet using `bin.PackMsg(mid, payload)` (already exists in both ubx/bin and casic/bin)
- Fire-and-forget (response display handled by refactored responsePrinter)

### Step 9: UBX Configurator integration

**Details:**
- Add method to `ConfigProtocol` for pre-built packets
- Use existing `Configurator` for ACK/NAK, retries, timeouts
- Update `runMsgs()` to use Configurator for protocol-specific messages

## File changes summary

| File | Change | Status |
|------|--------|--------|
| `internal/gpscmd/msgfile.go` | Types, parsing, TaggedLineMsgs, validation | done |
| `internal/gpscmd/msgfile_test.go` | Unit tests | done |
| `internal/gpscmd/gpsflags.go` | `-m` flag | done |
| `internal/gpscmd/gpscmd.go` | `runMsgs()`, `runConfig()`, `runRawMsgs()`, `responsePrinter` | done |
| `internal/gpscmd/gpsflags.go` | `-t` flag | done |
| `internal/casic/bin/ack.go` | CASIC ACK/NAK types (Step 8a) | |
| `internal/casic/bin/msg.go` | CASIC message ID name registry (Step 8b) | |
| `internal/gpscmd/gpscmd.go` | Refactor responsePrinter for per-protocol handling (Step 8c) | |
| `internal/gpscmd/payload.go` | Payload encoding (Step 8d) | |

## Dependencies

- `github.com/pelletier/go-toml/v2` (already in go.mod, used by daemon)
- `internal/nmea` for NMEA checksum and validation
- Issue #197 (`--capture` flag) for response display
