# UBX port-dependent CFG-VALSET helper (#270)

Add a port-aware UBX Gen 9+ message-file helper for one-byte
port-dependent CFG-VALSET writes (such as `CFG-MSGOUT-*_<PORT>` and
`CFG-INFMSG-*_<PORT>`) without hard-coding the active receiver port into
the message file.

This plan assumes the user supplies the active u-blox output port with
a new `--port` argument when sending the message file. Discovering that
port is deliberately out of scope here; the separate get-port work owns
that problem.

This plan builds on `plan/ubx-msg-save.md`. The save plan adds the
save-aware `CFG-VALSET` path and allows `--save` in message-file mode,
but it does not add `--port`. `[[ubxvalport]]` should resolve a
port-specific key and then use the same VALSET encoding and
layer-selection helper as `[[ubxval]]`.

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
u-blox port-dependent table and let the command-line `--port` argument
select the actual port.

Example target syntax for `CFG-MSGOUT`:

```toml
[[ubxvalport]]
tag = "osnma-monitor"
description = "Enable UBX-NAV-TIMETRUSTED"
key.usb = 0x209103ab
value = 1

[[ubxvalport]]
tag = "osnma-monitor"
description = "Enable UBX-SEC-OSNMA"
key.usb = 0x209106cd
value = 1
```

Then:

```sh
satpulsetool gps -d /dev/ttyACM0 -m configs/gpsmsg/osnma.toml -t osnma-monitor --port usb
```

emits `UBX-CFG-VALSET` packets for the USB keys
`0x209103ab` and `0x209106cd`.

Adding `--save` to the same invocation writes the resulting items to
`RAM|BBR|Flash` instead of RAM only.

The same helper covers `CFG-INFMSG`:

```toml
[[ubxvalport]]
tag = "ubx-inf-notice"
description = "Enable INF-NOTICE on USB"
key.usb = 0x20920004
value = 0x07
```

And, when all five port keys are supplied explicitly, port-dependent
keys that do not follow the standard offset pattern (for example
`CFG-USBOUTPROT-*` and `CFG-UART1OUTPROT-*`):

```toml
[[ubxvalport]]
tag = "ubx-outprot-ubx"
description = "Enable UBX output protocol"
key.i2c   = 0x10720001
key.uart1 = 0x10740001
key.uart2 = 0x10760001
key.usb   = 0x10780001
key.spi   = 0x107a0001
value = 1
```

## Scope

In scope:

- A new message-file type, `[[ubxvalport]]`.
- A `satpulsetool gps --port` option accepted in message-file mode.
- A common message-file port argument, passed as a string.
- Gen 9+ `UBX-CFG-VALSET` encoding for a single one-byte `value`.
- Use of the message-file `--save` behavior from
  `plan/ubx-msg-save.md`.
- Message-file docs, JSON schema, and focused tests.

Out of scope:

- Discovering the active receiver port.
- Using a read-only port property.
- Probing before message-file sends.
- A Gen 8 `UBX-CFG-MSG` helper.
- A name-to-message-key map.
- Multi-byte values (e.g. baud-rate keys). Those keys do not fit the
  port-output abstraction anyway.
- Checking whether the receiver supports a particular message before
  sending the request.

Older u-blox receivers can continue to use raw `[[ubx]]` entries if
needed. This helper is specifically for Gen 9+ configuration databases
where one-byte port-dependent CFG keys exist.

## Message type

Add `UBXValPortMsg` to `gps/msgfile` for `[[ubxvalport]]` entries.

Fields:

```go
type UBXValPortMsg struct {
    Key   UBXValPortKeys `toml:"key"`
    Value uint8          `toml:"value"`
    MsgCommon
}

type UBXValPortKeys struct {
    I2C   opt.Val[uint32] `toml:"i2c,omitzero"`
    UART1 opt.Val[uint32] `toml:"uart1,omitzero"`
    UART2 opt.Val[uint32] `toml:"uart2,omitzero"`
    USB   opt.Val[uint32] `toml:"usb,omitzero"`
    SPI   opt.Val[uint32] `toml:"spi,omitzero"`
}
```

The `key` values are complete `CFG-..._<PORT>` Key IDs from the u-blox
spec, for example `0x209103a8` for I2C or `0x209103ab` for USB. The
message file can specify whichever port keys are convenient to copy
from the spec.

Validation:

- At least one `key.<port>` value must be present.
- If fewer than five port keys are supplied, each supplied key must be
  in a known port-inferable family. The initial allow-list is:
  - `CFG-MSGOUT`: `key & 0xffff0000 == 0x20910000`
  - `CFG-INFMSG`: `key & 0xffff0000 == 0x20920000`
- If fewer than five port keys are supplied, multiple supplied keys
  must imply the same port-neutral base key under the standard u-blox
  offset pattern.
- If all five `key.<port>` values are supplied, no family check or
  offset-pattern check is performed; the helper simply looks up the
  key matching `--port`. This supports port-dependent keys that do
  not follow the offset pattern (for example
  `CFG-USB`/`CFG-UART1OUTPROT`/...`).
- The selected key must encode a one-byte value: enforced downstream by
  `ubxcfgval.Key.NValueBytes()`. No separate check is needed.
- `value` is encoded as `U1`; normal TOML numeric bounds reject values
  outside `0..255`.
- Conversion requires a supplied UBX port.

The port offset, used for inference and for selection from the five
port keys, is:

```text
i2c   -> 0
uart1 -> 1
uart2 -> 2
usb   -> 3
spi   -> 4
```

When fewer than five keys are supplied and the supplied keys are
inferable, the resolved key is:

```go
base := suppliedKey - uint32(suppliedPortOffset)
key := base + uint32(targetPortOffset)
```

If the target port key is explicitly supplied, the resolved key is the
explicit value (which under the offset pattern equals `base +
targetPortOffset`).

## Encoding

`[[ubxvalport]]` resolves the requested port key and emits the same
`UBX-CFG-VALSET` packet shape as `[[ubxval]]`:

- class/id: `0x06 0x8a`
- version: no transaction
- layers: determined by the message-file `--save` option
- transaction: none
- one item with the resolved key and the `U1` value

Implementation reuses the existing UBX config-value helpers:

- `gps/lib/ubxcfgval.KeyU` for the final key type
- `gps/lib/ubxcfgval.MarshalItems`
- `gps/lib/ubxbin.CfgValsetID`

`ubxvalport` calls `newUBXValRaw` with one key, `types = "U1"`, and a
single value.

The raw packet is treated like other UBX CFG writes for response
matching: expect ACK or NAK for `UBX-CFG-VALSET`.

## CLI

Add a `--port` option to `satpulsetool gps`. The `--save` option is
the one from `plan/ubx-msg-save.md` and applies to `ubxvalport` as
well as `ubxval`.

Rules:

- `--port` is only meaningful with `--msg-file`.
- `--save` is allowed with `--msg-file` only when the selected tags
  include a save-aware message type: `ubxval` from
  `plan/ubx-msg-save.md` or the new `ubxvalport` type from this plan.
- `--save-all` remains invalid with `--msg-file`.
- Accepted values are case-insensitive:
  `i2c`, `uart1`, `uart2`, `usb`, `spi`. Stored normalized to lower
  case.
- If selected messages include `ubxvalport` and `--port` is missing,
  conversion fails before sending anything with a clear error.
- `--show-tags` must not require `--port`, because it does not encode
  messages.

This option is an explicit input. The command does not infer the port
from the serial device path and does not probe the receiver.

The value remains a string in the message-file layer. Port is a
common GPS receiver concept, but each protocol has its own port names
and numbering. Protocol-specific message types interpret the string
when they need it.

For this first implementation, `ubxvalport` is the only consumer of
`--port`, so flag parsing validates the value up front against the
u-blox port names above. In the future, once other message types use
the common port argument, port-name validation should move closer to
the message type that consumes the value.

## Message-file plumbing

Extend `gps/msgfile`:

- Add `Default.UBXValPort` and `UBXValPort []UBXValPortMsg` to
  `Parsed` with TOML tags for `ubxvalport`.
- Include `ubxvalport` in defaulting, tag indexing, tag validation,
  tag descriptions, and `TaggedMsgs`.
- Extend the raw conversion signature from `plan/ubx-msg-save.md` to
  add the port argument:

  ```go
  func ToRaw(msgs any, port string, save bool) ([]RawMsg, error)
  ```

- Empty `port` means no port was supplied.
- `ubxvalport` uses both arguments: `port` chooses the key and `save`
  chooses the VALSET layers.
- Extend the save-aware message check from `plan/ubx-msg-save.md` so
  `save` is valid for either `[]UBXValMsg` or `[]UBXValPortMsg`.
  It is still an error for selected raw `ubx`, line, NMEA, or other
  non-save-aware message types.

`internal/gpscmd` passes the parsed `--port` value through to
`ToRaw` along with the parsed message-file `--save` value.

For `ubxvalport`, valid port strings are mapped inside the UBX message
type to the u-blox offsets:

```text
i2c   -> 0
uart1 -> 1
uart2 -> 2
usb   -> 3
spi   -> 4
```

## Documentation and examples

Update:

- `configs/gpsmsg/format.md`
- `configs/gpsmsg/gpsmsg-schema.json`
- `configs/gpsmsg/ubx9.toml`

The updated `ubx9.toml` replaces hard-coded USB `CFG-VALSET` entries
with `[[ubxvalport]]` entries where practical. The tags drop the
port suffix when the port is now supplied by `--port`. This supersedes
the intermediate `[[ubxval]]` migration described in
`plan/ubx-msg-save.md` for entries that are specifically
`CFG-MSGOUT` rates.

OSNMA-specific message-output enablement can then be expressed as
portable Gen 9+ entries:

```toml
[[ubxvalport]]
tag = "osnma-monitor"
description = "Enable UBX-NAV-TIMETRUSTED"
key.usb = 0x209103ab
value = 1

[[ubxvalport]]
tag = "osnma-monitor"
description = "Enable UBX-SEC-OSNMA"
key.usb = 0x209106cd
value = 1
```

## Tests

Add focused tests for:

- `ubxvalport` encodes the expected `CFG-VALSET` payload for each port.
- `ubxvalport` without `--save` writes RAM only.
- `ubxvalport` with `--save` writes `RAM|BBR|Flash`.
- `--save` with selected `ubxvalport` messages is accepted.
- `--save` with selected non-`ubxval`/non-`ubxvalport` messages is
  still rejected.
- `ubxvalport` can infer the target port key from any single supplied
  `key.<port>` value in an inferable family (MSGOUT or INFMSG).
- Partial key tables reject multiple supplied `key.<port>` values that
  imply inconsistent base keys.
- Partial key tables reject keys that are not in an inferable family.
- Fully specified key tables do not require the standard offset
  pattern (used for `CFG-*OUTPROT-*` style keys).
- Missing `--port` fails when `ubxvalport` messages are selected.
- `--show-tags` works without `--port`.
- Invalid `--port` values are rejected.
- `--port` is case-insensitive.
- Invalid `key.<port>` values are rejected (e.g., zero-size key).
- Existing message types still work through `ToRaw`.
- Tag validation catches `ubxvalport` mixed with other message types
  under the same selected tag, matching current message-file rules.
