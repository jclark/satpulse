# UBX message-file port support (#270)

Add a port-aware UBX Gen 9+ message-file helper for configuring
`CFG-MSGOUT` rates without hard-coding the active receiver port into
the message file.

This plan assumes the user supplies the active u-blox output port with
a new `--port` argument when sending the message file. Discovering that
port is deliberately out of scope here; the separate get-port work owns
that problem.

## Goal

Today a Gen 9+ message file has to spell out the complete
port-specific `CFG-MSGOUT` key in a raw `[[ubx]]` `CFG-VALSET`
payload:

```toml
[[ubx]]
tag = "ubx-nav-timeutc-usb"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x2091005E, 1]
```

That is inconvenient because the key embeds the output port. A message
file author should be able to copy any available port key from the
u-blox `CFG-MSGOUT` table and let the command-line `--port` argument
select the actual port.

Example target syntax:

```toml
[[ubxmsg]]
tag = "osnma-monitor"
description = "Enable UBX-NAV-TIMETRUSTED"
key.usb = 0x209103ab
rate = 1

[[ubxmsg]]
tag = "osnma-monitor"
description = "Enable UBX-SEC-OSNMA"
key.usb = 0x209106cd
rate = 1
```

Then:

```sh
satpulsetool gps -d /dev/ttyACM0 -m configs/gpsmsg/osnma.toml -t osnma-monitor --port usb
```

emits `UBX-CFG-VALSET` packets for the USB keys
`0x209103ab` and `0x209106cd`.

## Scope

In scope:

- A new message-file type, `[[ubxmsg]]`.
- A `satpulsetool gps --port` option accepted in message-file mode.
- A common message-file port argument, passed as a string.
- Gen 9+ `UBX-CFG-VALSET` encoding for `CFG-MSGOUT` `U1` rates.
- Message-file docs, JSON schema, and focused tests.

Out of scope:

- Discovering the active receiver port.
- Using a read-only port property.
- Probing before message-file sends.
- A Gen 8 `UBX-CFG-MSG` helper.
- A name-to-message-key map.
- Checking whether the receiver supports a particular message before
  sending the request.

Older u-blox receivers can continue to use raw `[[ubx]]` entries if
needed. This helper is specifically for Gen 9+ configuration databases
where `CFG-MSGOUT-*_<PORT>` keys exist.

## Message Type

Add `UBXMsgoutMsg` to `gps/msgfile` for `[[ubxmsg]]` entries.

Fields:

```go
type UBXMsgoutMsg struct {
    Key  UBXMsgoutKeys `toml:"key"`
    Rate uint8         `toml:"rate"`
    MsgCommon
}

type UBXMsgoutKeys struct {
    I2C   opt.Val[uint32] `toml:"i2c,omitzero"`
    UART1 opt.Val[uint32] `toml:"uart1,omitzero"`
    UART2 opt.Val[uint32] `toml:"uart2,omitzero"`
    USB   opt.Val[uint32] `toml:"usb,omitzero"`
    SPI   opt.Val[uint32] `toml:"spi,omitzero"`
}
```

The `key` values are complete `CFG-MSGOUT-..._<PORT>` Key IDs from
the u-blox spec, for example `0x209103a8` for I2C or `0x209103ab` for
USB. The message file can specify whichever port keys are convenient
to copy from the spec. Missing port keys are inferred using the normal
u-blox `CFG-MSGOUT` numbering pattern.

Validation:

- At least one `key.<port>` value must be present.
- If not all five port keys are supplied, each supplied key must look
  like a `CFG-MSGOUT` `U1` key:
  `key & 0xffff0000 == 0x20910000`.
- If not all five port keys are supplied, multiple supplied keys must
  imply the same port-neutral base key under the standard u-blox
  offset pattern.
- `rate` is encoded as `U1`; normal TOML numeric bounds should reject
  values outside `0..255`.
- `ubxmsg` conversion requires a supplied UBX port.

The port offset is the existing u-blox order:

```text
i2c   -> 0
uart1 -> 1
uart2 -> 2
usb   -> 3
spi   -> 4
```

The output key is:

```go
base := suppliedKey - uint32(suppliedPortOffset)
key := base + uint32(targetPortOffset)
```

If the target port key is explicitly supplied, this produces the same
value. The explicit keys are still checked for consistency with the
standard pattern unless all five port keys are supplied. If u-blox
ever ships an exception to this numbering scheme, a message file can
spell out all five keys and no inference is needed.

## Encoding

`[[ubxmsg]]` emits a `UBX-CFG-VALSET` packet:

- class/id: `0x06 0x8a`
- version: no transaction
- layers: RAM
- transaction: none
- one `CFG-MSGOUT` item with the port-adjusted key and the `U1` rate

Implementation should reuse the existing UBX config-value helpers:

- `gps/lib/ubxcfgval.KeyU` for the final `CFG-MSGOUT` key type
- `gps/lib/ubxcfgval.MarshalItems`
- `gps/lib/ubxbin.CfgValsetID`

The raw packet should be treated like other UBX CFG writes for
response matching: expect ACK or NAK for `UBX-CFG-VALSET`.

## CLI

Add a `--port` option to `satpulsetool gps`.

Rules:

- `--port` is only meaningful with `--msg-file`.
- Accepted values are case-insensitive:
  `i2c`, `uart1`, `uart2`, `usb`, `spi`.
- If selected messages include `ubxmsg` and `--port` is missing,
  fail before sending anything with a clear error.
- `--show-tags` must not require `--port`, because it does not encode
  messages.

This option is an explicit input. The command does not infer the port
from the serial device path and does not probe the receiver.

The value remains a string in the message-file layer. Port is a
common GPS receiver concept, but each protocol has its own port names
and numbering. Protocol-specific message types interpret the string
when they need it.

For this first implementation, `ubxmsg` is the only consumer of
`--port`, so flag parsing can validate the value up front against the
u-blox port names above. In the future, once other message types use
the common port argument, port-name validation should move closer to
the message type that consumes the value.

## Message-File Plumbing

Extend `gps/msgfile`:

- Add `Default.UBXMsgout` and `UBXMsgout []UBXMsgoutMsg` to `Parsed`
  with TOML tags for `ubxmsg`.
- Include `ubxmsg` in defaulting, tag indexing, tag validation,
  tag descriptions, and `TaggedMsgs`.
- Add a port argument to raw conversion:

  ```go
  func ToRaw(msgs any, port string) ([]RawMsg, error)
  ```

- Empty `port` means no port was supplied.

`internal/gpscmd` passes the parsed `--port` value through to
`ToRaw`.

For `ubxmsg`, valid port strings are mapped inside the UBX message
type to the u-blox offsets:

```text
i2c   -> 0
uart1 -> 1
uart2 -> 2
usb   -> 3
spi   -> 4
```

## Documentation And Examples

Update:

- `configs/gpsmsg/format.md`
- `configs/gpsmsg/gpsmsg-schema.json`
- `configs/gpsmsg/ubx9.toml`

The updated `ubx9.toml` should replace hard-coded USB `CFG-VALSET`
entries with `[[ubxmsg]]` entries where practical. The tags should
drop the port suffix when the port is now supplied by `--port`.

OSNMA-specific message-output enablement can then be expressed as
portable Gen 9+ entries:

```toml
[[ubxmsg]]
tag = "osnma-monitor"
description = "Enable UBX-NAV-TIMETRUSTED"
key.usb = 0x209103ab
rate = 1

[[ubxmsg]]
tag = "osnma-monitor"
description = "Enable UBX-SEC-OSNMA"
key.usb = 0x209106cd
rate = 1
```

## Tests

Add focused tests for:

- `ubxmsg` encodes the expected `CFG-VALSET` payload for each port.
- `ubxmsg` can infer the target port key from any supplied
  `key.<port>` value.
- Partial key tables reject multiple supplied `key.<port>` values that
  imply inconsistent base keys.
- Fully specified key tables do not require the standard offset
  pattern.
- Missing `--port` fails when `ubxmsg` messages are selected.
- `--show-tags` works without `--port`.
- Invalid `--port` values are rejected.
- Invalid `key.<port>` values are rejected.
- Existing message types still work through `ToRaw`.
- Tag validation catches `ubxmsg` mixed with other message types
  under the same selected tag, matching current message-file rules.
