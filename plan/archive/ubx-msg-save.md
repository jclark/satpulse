# UBX message-file save support (#272)

Add a save-aware UBX Gen 9+ message-file type for sending one or more
`CFG-VALSET` items in one packet. This fixes the current message-file
problem where raw `[[ubx]]` `CFG-VALSET` entries always encode the
RAM layer and therefore ignore the command-line save intent.

This plan does not change `gps/internal/ubx` `Configure` behavior.
The existing configurator already chooses the right `CFG-VALSET`
layers when configuration is driven through `Configure`. The problem
is specific to message files.

## Goal

Raw `[[ubx]]` `CFG-VALSET` entries have to spell out the VALSET fixed
fields and hard-code the layer byte. For example, the `osnma` branch's
`configs/gpsmsg/osnma.toml` has non-message-output config writes like:

```toml
[[ubx]]
tag = "osnma-on"
description = "Enable Galileo OSNMA authentication"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10350005, 1]
```

The second payload byte is the layer mask, currently hard-coded to
`1` for RAM. There is no way for `satpulsetool gps --msg-file ...`
to make this persistent with `--save`.

Add a typed message-file form:

```toml
[[ubxval]]
tag = "osnma-on"
description = "Enable Galileo OSNMA authentication"
keys = [0x10350005]
types = "U1"
values = [1]
```

Without `--save`, this emits `UBX-CFG-VALSET` to RAM only. With
`--save`, it emits the same key/value items to `RAM|BBR|Flash`.

## Scope

In scope:

- A new message-file type, `[[ubxval]]`.
- A `--save` option accepted in message-file mode.
- `--save` choosing `CFG-VALSET` layers:
  - no `--save`: `ubxbin.CfgValsetLayerRAM`
  - `--save`: `ubxbin.CfgValsetLayerRAM | ubxbin.CfgValsetLayerBBR | ubxbin.CfgValsetLayerFlash`
- Updating Gen 9+ UBX message files that currently use raw
  `CFG-VALSET` writes.
- Message-file docs, JSON schema, and focused tests.

Out of scope:

- `--save-all` in message-file mode.
- Rewriting or interpreting arbitrary raw `[[ubx]]` payloads.
- Port discovery.
- A message-file `--port` option.
- Port-aware `CFG-MSGOUT` key inference. That remains in
  `plan/ubx-msg-port.md`.
- Receiver capability probing or recovery from `ACK-NAK`.
- Changing the existing `Configure` save semantics.

## Message Type

Add `UBXValMsg` to `gps/msgfile` for `[[ubxval]]` entries.

User-facing fields:

```go
type UBXValMsg struct {
    Keys   []uint32 `toml:"keys"`
    Types  string   `toml:"types"`
    Values []any    `toml:"values"`
    MsgCommon
}
```

The file format mirrors the existing raw payload format, with one
parallel `keys` entry for each typed value:

```toml
keys = [0x10350005]
types = "U1"
values = [1]

keys = [0x10350005, 0x101100dd, 0x10350009]
types = "U1U1U1"
values = [1, 1, 1]

keys = [0x40050002]
types = "U4"
values = [0xdeadbeef]
```

This avoids custom TOML unmarshalling and gives `[[ubxval]]` the
same numeric semantics as `payload.types` / `payload.values`. The
TOML decoder fills `[]any`; integer literals, including TOML hex
integers, decode as `int64`, and floating-point literals decode as
`float64`.

Use the existing payload type grammar for `types`: `U1`, `U2`, `U4`,
`I1`, `I2`, `I4`, `R4`, and `R8`. This is a subset of the u-blox
configuration scalar types, but it is enough for the values we need
now and keeps the message-file encoding model consistent. Do not add
`ubxval`-only type names.

Do not add 8-byte integer types to this format now. TOML integer
literals decoded into `[]any` arrive as signed `int64`, so `U8` could
not represent the full unsigned 64-bit range. `I8` is also out of
scope until there is a concrete need to extend the shared payload type
grammar. Full-width unsigned or raw 64-bit `X8` values remain out of
scope for this message-file path unless a later design adds an
explicit representation for them.

Message-file authors should choose the type that mirrors the scalar
type in the u-blox specification:

- `Un`, `Xn`, and `E1` -> the corresponding `U1`, `U2`, or `U4`
- `In` -> the corresponding `I1`, `I2`, or `I4`
- `Rn` -> `R4` or `R8`
- `L` -> `U1`, using `0` or `1`

This prefix mapping is the convention for supplied SatPulse UBX TOML
files, but validation should enforce only what can be checked from the
numeric Key ID and selected type. The numeric Key ID encodes value
width, but it does not preserve the documentation distinction between
`Xn` and `Un`.

Validation:

- The number of parsed `types` entries must equal both `len(keys)` and
  `len(values)`.
- Each encoded value width must match the value width encoded in its
  Key ID.
- Existing payload integer range checks apply to `U*` and `I*`.
- Existing payload float handling applies to `R4` and `R8`: TOML
  floats and integers are numeric values, not raw bit patterns.
- Invalid Key IDs with no value width fail before sending.

Do the key/value conversion by sharing or refactoring the existing
payload type parsing and value encoding helpers. For each item, encode
the typed value to little-endian bytes, validate that byte count
against the key's value size, copy those bytes into a `uint64`, and
build a `ubxcfgval.Item`. Add an exported helper in
`gps/lib/ubxcfgval` if needed for checking a numeric key's value byte
count.

## Encoding

`[[ubxval]]` emits a `UBX-CFG-VALSET` packet:

- class/id: `0x06 0x8a`
- version: no transaction
- transaction: none
- layers: determined by the message-file `--save` option
- one or more config items with the supplied keys and converted values

Add a helper in `gps/msgfile`, not `gps/internal/ubx`, because message
files cannot import internal UBX configurator code:

```go
func ubxValLayers(save bool) ubxbin.CfgValsetLayer
func newUBXValRaw(keys []uint32, types string, values []any, save bool) ([]byte, error)
```

The implementation should reuse the existing UBX config-value helpers:

- `gps/lib/ubxcfgval.Key`
- `gps/lib/ubxcfgval.Item`
- `gps/lib/ubxcfgval.MarshalItems`
- `gps/lib/ubxbin.CfgValset`
- `gps/lib/ubxbin.CfgValsetID`

`[[ubxval]]` should still use the no-transaction VALSET mode. A
single entry can set multiple keys in one packet; it does not need to
use the multi-packet UBX transaction mechanism.

The raw packet should be treated like other UBX CFG writes for
response matching: expect ACK or NAK for `UBX-CFG-VALSET`.

## CLI

Allow `--save` with `--msg-file`.

Rules:

- `--msg-file --save` is valid only when the selected tags resolve to
  `ubxval` messages.
- If `--save` is used and the selected tags do not include
  `ubxval`, fail before sending anything with a clear error. This
  avoids making `--save` a silent no-op for raw `[[ubx]]`, line, NMEA,
  or other message types.
- `--msg-file --save-all` remains invalid.
- `--msg-file` still cannot be combined with higher-level
  configuration changes, `--reset`, `--reload`, or `--factory-reset`.
- In normal configuration mode, the current `--save` behavior is
  unchanged: it still requires configuration changes and sets
  `gpsprot.SaveMinimal`.
- In message-file mode, `--save` is passed directly to raw message
  conversion and does not create a `ConfigTarget`.

The `--save`/`ubxval` validation happens after loading the message
file and selecting tags, because flag parsing alone cannot know the
selected message type.

This keeps the two save paths separate:

- Configure path: `ConfigOptions.Save` drives protocol-specific
  configurators.
- Message-file path: a plain boolean controls save-aware message-file
  encoders.

## Message-File Plumbing

Extend `gps/msgfile`:

- Add `Default.UBXVal` and `UBXVal []UBXValMsg` to `Parsed`
  with TOML tags for `ubxval`.
- Include `ubxval` in defaulting, tag indexing, tag validation,
  tag descriptions, and `TaggedMsgs`.
- Treat `ubxval` as the only save-aware message type in this plan.
- Extend raw conversion to pass the new save argument:

  ```go
  func ToRaw(msgs any, save bool) ([]RawMsg, error)
  ```

- Existing message types continue through the old path when `save` is
  false.
- `ubxval` uses `save`.
- If `save` is true and `msgs` is not `[]UBXValMsg`, return an
  error such as `--save requires selected ubxval messages`.

This deliberately uses a direct argument; do not add a raw conversion
options struct.

`internal/gpscmd` should pass the message-file `--save` boolean
through to `ToRaw`.

## Config File Migration

The `osnma` branch's `configs/gpsmsg/osnma.toml` has suitable
non-message-output VALSET tags for this migration:

- `osnma-on` / `osnma-off`: `CFG-GAL-USE_OSNMA`
- `osnma-only-auth` / `osnma-only-auth-off`:
  `CFG-NAVSPG-ONLY_AUTHDATA`
- `osnma-timesync` / `osnma-timesync-off`:
  `CFG-GAL-OSNMA_TIMESYNC`

`configs/gpsmsg/ubx9.toml` currently has no good non-message-output
tag to use as a `[[ubxval]]` example. Its raw `CFG-VALSET` entries
are all USB `CFG-MSGOUT` enable/disable settings, with tags such as
`ubx-nav-timegps-usb` and `ubx-nav-timegps-usb-off`.

Those entries are ultimately a better fit for `[[ubxmsg]]` in
`plan/ubx-msg-port.md`, because they embed the output port in the key.
Since this save plan lands before the port-aware plan, convert the
existing `CFG-MSGOUT` entries in `ubx9.toml` mechanically to
`[[ubxval]]` as an intermediate step so `--save` works with them.
The later port plan should then convert those entries to `[[ubxmsg]]`.

For a concrete non-message-output migration, convert OSNMA entries
like:

```toml
[[ubx]]
tag = "osnma-on"
description = "Enable Galileo OSNMA authentication"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10350005, 1]
```

to:

```toml
[[ubxval]]
tag = "osnma-on"
description = "Enable Galileo OSNMA authentication"
keys = [0x10350005]
types = "U1"
values = [1]
```

For the intermediate `CFG-MSGOUT` migration in `ubx9.toml`, use
`types = "U1"` and `values = [1]` or `values = [0]`, because
`CFG-MSGOUT` keys are `U1`.

Keep `configs/gpsmsg/ubx8.toml` as raw `[[ubx]]` for now, because
Gen 8 message enablement uses `UBX-CFG-MSG`, not `CFG-VALSET`.

`plan/ubx-msg-port.md` can later replace the USB-specific Gen 9+
entries with `[[ubxmsg]]`; both message types should share the same
save-aware VALSET encoding helper.

## Documentation

Update:

- `configs/gpsmsg/format.md`
- `configs/gpsmsg/README.md`
- `configs/gpsmsg/gpsmsg-schema.json`
- `configs/gpsmsg/osnma.toml` from the OSNMA branch
- `configs/gpsmsg/ubx9.toml`, as an intermediate `[[ubxval]]`
  migration before `[[ubxmsg]]` exists

The docs should explain:

- `[[ubxval]]` is for u-blox Gen 9+ configuration database writes.
- `--save` changes VALSET layers from RAM to `RAM|BBR|Flash`.
- Raw `[[ubx]]` packets are not interpreted, so `--save` does not
  rewrite hard-coded raw VALSET payloads.
- `[[ubxval]]` uses `keys`, `types`, and `values`; `types` and
  `values` follow the same grammar and numeric semantics as existing
  `payload.types` / `payload.values`.
- Supplied UBX TOML files should use the type matching the scalar
  type and width in the u-blox spec.
- The `[[ubxval]]` examples should use non-`CFG-MSGOUT` keys;
  `CFG-MSGOUT` examples belong with `[[ubxmsg]]`.

## Tests

Add focused tests for:

- `ubxval` without `--save` encodes the same bytes as the old raw
  RAM-only `CFG-VALSET` form.
- `ubxval` with `--save` changes only the layer byte to
  `RAM|BBR|Flash`.
- Multiple keys in one `[[ubxval]]` entry encode as multiple
  config items in one VALSET packet.
- `keys`, parsed `types`, and `values` counts must match.
- TOML hex integer values such as `values = [0xdeadbeef]` decode and
  encode correctly for unsigned integer-like keys.
- Boolean-style `L` keys use `types = "U1"` with values `0` or `1`,
  for example `CFG-GAL-USE_OSNMA`.
- Unsigned and signed value bounds match existing payload behavior.
- 8-byte integer types are not accepted.
- Float conversion for `R4` and `R8` matches existing payload
  behavior, including numeric rather than raw-bit semantics.
- Invalid key/type/value width combinations fail before sending.
- `ToRaw(msgs, save)` keeps existing message types working when
  `save` is false.
- `ToRaw(msgs, true)` rejects selected line, NMEA, raw `ubx`, or
  other non-`ubxval` message types.
- `internal/gpscmd` flag parsing accepts `--msg-file --save` and
  still rejects `--msg-file --save-all`.
- Existing replay tests are unchanged unless they intentionally cover
  message-file `--save`.

Add at least one message-file unit test that loads a converted
`osnma.toml` entry, such as `osnma-on`, and verifies the emitted
`CFG-VALSET` item. Add a second test that loads a converted
`ubx9.toml` `CFG-MSGOUT` entry as `[[ubxval]]`, because that is
the intermediate migration before `[[ubxmsg]]` exists.
