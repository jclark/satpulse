# GPS message extensibility using TOML

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

**Constraints:**

The `-m` flag cannot be combined with config flags like `--gnss`, `--pps`, `--save`, etc. This avoids ambiguity about ordering of manual messages versus higher-level configuration.

When `-m` is used without `--capture`, the tool defaults to `--capture 2` to show receiver responses.

## TOML message file

Following is user-facing explanation of how TOML message file works.

### Message types

| Type | Keys | Framing |
|------|------|---------|
| `[[line]]` | `text`, `eol`, `delay`, `tag` | appends eol (default `\r\n`) |
| `[[binary]]` | `hex`, `delay`, `tag` | none |
| `[[nmea]]` | `text`, `delay`, `tag` | prepends `$`, appends `*XX\r\n` checksum |

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


### Protocol specific message types

```toml
[[ubx]]
tag=gps-l5-health
class= 0x06
id = 0x8A
pack=U1U1U2U4U1
payload=[0, 1, 0, 0x10320001, 1]
```

## Implementation

### Go types

```go
// LineMsg represents a [[line]] entry or [default.line].
type LineMsg struct {
	Text  string  `toml:"text"`
	EOL   string  `toml:"eol"`
	Delay float64 `toml:"delay"`
	Tag   string  `toml:"tag"`
}

// BinaryMsg represents a [[binary]] entry or [default.binary].
type BinaryMsg struct {
	Hex   string  `toml:"hex"`
	Delay float64 `toml:"delay"`
	Tag   string  `toml:"tag"`
}

// NMEAMsg represents an [[nmea]] entry or [default.nmea].
type NMEAMsg struct {
	Text  string  `toml:"text"`
	Delay float64 `toml:"delay"`
	Tag   string  `toml:"tag"`
}

// MsgFile represents a parsed message file.
type MsgFile struct {
	Default struct {
		Line   LineMsg
		Binary BinaryMsg
		NMEA   NMEAMsg
	}
	Line   []LineMsg
	Binary []BinaryMsg
	NMEA   []NMEAMsg
}
```

Key points:
- Per-protocol packages (like ubx) define their own message types added to MsgFile
- Same type for default and messages; default is single, messages are slice
- Validation: `Default.Line.Text` must be empty; each `Line[i].Text` must be non-empty
- The ubx package does not import gpscmd; we rely on structural typing

### Loading and defaulting

Follow the pattern from `internal/daemon/config.go` and `internal/daemon/daemon.go`:

```go
func defaultMsgFile() *MsgFile {
	mf := new(MsgFile)
	mf.Default.Line.EOL = "\r\n"
	return mf
}

func LoadMsgFile(path string) (*MsgFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	mf := defaultMsgFile()
	err = toml.NewDecoder(f).DisallowUnknownFields().Decode(mf)
	if err != nil {
		return nil, err
	}
	return mf, nil
}

// msgFileErrorDetail extracts detailed error info from TOML parsing errors.
func msgFileErrorDetail(err error) string {
	if s, ok := err.(fmt.Stringer); ok {
		return s.String()
	}
	return ""
}
```

In `gpscmd.go`, report errors with detail like daemon does:

```go
mf, err := LoadMsgFile(path)
if err != nil {
	s := msgFileErrorDetail(err)
	if s != "" {
		fmt.Fprintln(os.Stderr, s)
	}
	return err
}
```

Pre-fill `Default.Line.EOL = "\r\n"` before decoding. TOML only overwrites fields present in the file.

For each message, apply defaults from `Default` field-by-field:
- For `string` fields: use default if message field is empty (`""`)
- For `float64` fields: use default if message field is zero (`0`)

### rawMsg and UserMsg interface

```go
// rawMsg is an internal type for sending messages.
type rawMsg struct {
	bytes []byte
	delay time.Duration
}

// UserMsg is implemented by LineMsg, BinaryMsg, NMEAMsg to convert to rawMsg.
type UserMsg interface {
	GetBytes() ([]byte, error)
	GetTag() string
	GetDelay() float64
}
```

### Core functions

```go
// LoadMsgFile reads and parses a TOML message file.
func LoadMsgFile(path string) (*MsgFile, error)

// Validate checks that the message file is valid.
func (f *MsgFile) Validate() error

// TaggedMsgs returns messages for the given tags.
// The returned any is a typed slice: []LineMsg, []BinaryMsg, or []NMEAMsg.
// All selected messages must have the same type.
func (f *MsgFile) TaggedMsgs(tags []string) (any, error)
```

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

### Step 1: Basic line messages

**Files:**
- `internal/gpscmd/msgfile.go` - new file with `LineMsg`, `MsgFile` (Line only), `LoadMsgFile`, `TaggedMsgs`
- `internal/gpscmd/msgfile_test.go` - parsing tests
- `internal/gpscmd/gpsflags.go` - add `-m`/`--msg-file` flag
- `internal/gpscmd/gpscmd.go` - add `runLineMsgs()`, `runRawMsgs()`, branch on `-m`

**Details:**
- `MsgFile` initially has only `Line []LineMsg`
- `TaggedMsgs(tags)` ignores tags for now, returns `[]LineMsg`
- Line framing: append `"\r\n"` (hardcoded initially)
- `runLineMsgs([]LineMsg)` converts to `[]rawMsg` and calls `runRawMsgs()`

**Tests:**
- Table-driven tests in `msgfile_test.go`
- Each test case: TOML input string, expected `[]rawMsg` output
- Compare using `reflect.DeepEqual`

### Step 2: Response display

**Files:**
- `internal/gpscmd/response.go` - new file with `responsePrinter`
- `internal/gpscmd/response_test.go` - tests

**Details:**
- Update `runMsgs()` to read packets from pCh during delays
- Pass `responsePrinter` to `keepReading` for capture phase

### Step 3: Defaults and EOL

**Details:**
- Add `Default.Line` to `MsgFile`
- Implement field-by-field defaulting in `TaggedMsgs()`
- Add `EOL` field to `LineMsg`
- Built-in default for EOL is `"\r\n"`
- Validate `Default.Line.Text` is empty

### Step 4: Binary messages

**Details:**
- Add `BinaryMsg` type with `Hex`, `Delay`, `Tag`
- Add `Binary []BinaryMsg` and `Default.Binary` to `MsgFile`
- Hex decoding: strip whitespace, validate even length and hex chars
- `runBinaryMsgs([]BinaryMsg)` converts to `[]rawMsg` and calls `runRawMsgs()` with nil responsePrinter
- Make `LineMsg` and `BinaryMsg` implement `UserMsg` interface
- At this stage: file must contain only one type (all line or all binary)

### Step 5: Tags

**Files:**
- `internal/gpscmd/gpsflags.go` - add `-t`/`--tag` flag

**Details:**
- Add `Tag` field defaulting
- `TaggedMsgs(tags)` filters by tags, preserving order from tags argument
- Validate: all messages for each tag have same type
- Return typed slice (`[]LineMsg` or `[]BinaryMsg`) to control response display

### Step 6: NMEA messages

**Details:**
- Add `NMEAMsg` type with `Text`, `Delay`, `Tag`
- Add `NMEA []NMEAMsg` and `Default.NMEA` to `MsgFile`
- Build: prepend `$` if missing, compute/append `*XX` checksum if missing, append `\r\n`
- Validate with `internal/nmea`
- Update `responsePrinter` for NMEA: use `nmea.CheckSyntax()` to get `SentenceSyntaxFlags`, display unless `IsValidGNSSTalkerNMEA()` is true

### Step 7: UBX/CASIC messages (fire-and-forget)

**Files:**
- `internal/ubx/bin/common.go` - add `const Endian = binary.LittleEndian`
- `internal/casic/bin/common.go` - add `const Endian = binary.LittleEndian`
- `internal/gpscmd/pack.go` - new file
- `internal/gpscmd/pack_test.go` - tests

**Details:**
- `ubx/bin` and `casic/bin` own endianness; gpscmd uses `bin.Endian` for encoding
- `Pack(format string, values []any) ([]byte, error)` in gpscmd
- Specifiers: U1, U2, U4, I1, I2, I4, X1, X2, X4, R4, R8
- `UBXLikeMsg` struct in gpscmd with fields: `Class`, `ID`, `Pack`, `Payload`, `Delay`, `Tag`
- `UBXMsg` and `CASMsg` embed `UBXLikeMsg`; each implements `GetBytes()` calling its `bin.PackMsg()`
- Build packet using `bin.PackMsg(mid, payload)` (already exists in both ubx/bin and casic/bin)
- Fire-and-forget (no ACK/NAK handling)

### Step 8: UBX Configurator integration

**Details:**
- Add method to `ConfigProtocol` for pre-built packets
- Use existing `Configurator` for ACK/NAK, retries, timeouts
- Update `runMsgs()` to use Configurator for protocol-specific messages

## File changes summary

| File | Change |
|------|--------|
| `internal/gpscmd/msgfile.go` | New: types, parsing, TaggedMsgs, validation |
| `internal/gpscmd/msgfile_test.go` | New: unit tests |
| `internal/gpscmd/response.go` | New: response display |
| `internal/gpscmd/response_test.go` | New: tests |
| `internal/gpscmd/gpsflags.go` | Add `-m` and `-t` flags |
| `internal/gpscmd/gpscmd.go` | Add `runMsgs()`, branch on `-m` |
| `internal/gpscmd/pack.go` | New: pack format encoding (Step 7) |

## Dependencies

- `github.com/pelletier/go-toml/v2` (already in go.mod, used by daemon)
- `internal/nmea` for NMEA checksum and validation
- Issue #197 (`--capture` flag) for response display
