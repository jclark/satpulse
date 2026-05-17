# Get current-port info via ConfigProps (#271)

A desktop GUI needs to know which port the receiver is currently
communicating on, and whether the concept of a baud rate applies to
that port at all (UART yes; USB / I2C / SPI no), so that it can
decide whether to offer a speed-change control. The data is useful
both before any change is made (read-only query) and as part of the
result of a change. The receiver-side cost varies by backend, so it
must be opt-in.

## Design

Add port information as a new read-only property on `gpsprot.ConfigProps`.
It uses the same opt-in mechanism as other gettable properties
(`target.Get |= PropIDPort`) and the same validity-bit pattern. It has
a public `SetPort` (backends in other packages need to populate it),
but the read/write contract is enforced by the `PropIDsReadOnly` mask
plus an entry-point panic at `gpscfg.Configure`.

```go
// gps/gpsprot/configtarget.go

type Port struct {
    Name        string // vendor port name, e.g. "UART1", "USB", "I2C", "COM1"
    HasBaudRate bool   // true iff the port type has a baud rate at all
                       // (UART yes; USB / I2C / SPI no -- the concept
                       // does not apply, not just "fixed")
}

const PropIDPort PropIDs = 1 << iota // added to existing list

// PropIDsReadOnly is the bitmask of properties that can be retrieved
// but not set. Grows as more read-only props are added.
const PropIDsReadOnly PropIDs = PropIDPort

func (cp *ConfigProps) GetPort() (Port, bool) {
    if cp.valid&PropIDPort != 0 {
        return cp.port, true
    }
    return Port{}, false
}

// ReadOnlyProps returns the subset of read-only properties that are
// valid in cp. Used by gpscfg.Configure to detect API misuse
// (read-only properties in target.Props).
func (cp *ConfigProps) ReadOnlyProps() PropIDs {
    return cp.valid & PropIDsReadOnly
}
```

### Read/write contract

A property is read-only iff its bit is in `PropIDsReadOnly`. For
read-only properties:

1. **Direction.** Read-only properties may appear in a *result*
   `ConfigProps` (returned by `Configurator.ConfigProps()`); they must
   not appear in a *target* `ConfigProps` (`target.Props`).
2. **Asking.** A caller requests retrieval via
   `target.Get |= PropIDPort`, same as any other gettable property.
3. **Population.** A backend, when fulfilling a `target.Get` for a
   read-only property, populates it on the result via the public
   setter (`SetPort`).
4. **Violation.** API misuse. `gpscfg.Configure`
   (`gps/app/gpscfg/gpscfg.go:46`), the single entry point above all
   backends, panics when `target.Props.ReadOnlyProps() != 0`. The
   panic message names the offending bits (via `PropIDs.String()`) so
   the fix is obvious:

   ```go
   if ro := target.Props.ReadOnlyProps(); ro != 0 {
       panic(fmt.Sprintf("read-only properties in target.Props: %v", ro))
   }
   ```

   The check lives in `gpscfg` rather than each backend's
   `ConfigProtocol.Configure`, so no per-backend duplication is needed
   and the normal call path is fully covered.

   Backends' own `ConfigProtocol.Configure` implementations do not
   re-validate. They silently ignore any read-only bits that happen
   to be set in `target.Props` -- which falls out naturally from the
   fact that there's no write code path for read-only properties.
   This means a hypothetical caller that bypasses `gpscfg` and calls
   a backend directly does not crash; it just gets best-effort
   behaviour where the read-only bits have no effect. The single
   panic site stays in `gpscfg` where the API contract belongs.

```go
// SetPort sets the current port info. Intended for backend
// implementations of Configurator. Callers constructing a
// ConfigTarget must not set read-only properties; doing so will
// cause gpscfg.Configure to panic. See PropIDsReadOnly.
func (cp *ConfigProps) SetPort(p Port) {
    cp.port = p
    cp.valid |= PropIDPort
}

// ClearReadOnlyProps removes all read-only properties from the
// ConfigProps so it can safely be used as target.Props. Useful when
// reusing a previous result (or a deserialised one) as the basis for
// a new target.
func (cp *ConfigProps) ClearReadOnlyProps() {
    cp.valid &^= PropIDsReadOnly
}
```

The panic check at `gpscfg.Configure` enforces the contract at runtime
without doubling the API surface (no separate result type, no
parallel `InfoIDs` bitmask). The `PropIDsReadOnly` mask is the single
point of truth and grows as more read-only props are added (e.g.
supported signal plans).

### Behaviour of `CopyFrom` / `Inconsistent` / `Missing`

These methods stay content-preserving: they operate uniformly across
all valid bits, including read-only ones.

- `CopyFrom(other)`: copies port if `other` has it valid.
- `Inconsistent(other)`: includes port in the diff when both sides
  are valid and they differ. Useful for "did the port change between
  two snapshots?"
- `Missing(other)`: includes port if `other` has it and the receiver
  doesn't.

The output of these methods is a `*ConfigProps` whose intended use is
up to the caller. To use such an output as `target.Props`, call
`ClearReadOnlyProps()` first, otherwise `gpscfg.Configure` will panic.

All current callers of these methods have been audited and continue
to work unchanged: none of them transition a result-shaped
`ConfigProps` into `target.Props`.

`ReceiverInfo` is left alone. Vendor / firmware / hardware /
SupportedGNSS keep their existing "always populated by probe"
semantics. Port specifically does not fit that mould because for some
backends (UNC) populating it requires an extra request that should not
be paid for callers that don't care.

### Why not `opt.Val[bool]` for HasBaudRate

Validity for the entire `Port` is carried by the `PropIDPort` bit in
`ConfigProps.valid`. When that bit is unset, `GetPort` returns `false`
and the caller knows nothing was retrieved. When set, both `Name` and
`HasBaudRate` are meaningful. No need for per-field optionality.

### Why not a hard error on speed-change against `!HasBaudRate`

The configurator API is best-effort: individual requests can fail and
the rest of the configuration proceeds. A "this whole config target is
invalid" error path would be inconsistent with that. Instead, the GUI
reads `Port` up-front via `target.Get` and decides whether to offer the
speed control at all. If a speed change is asked for on a port without
a baud rate anyway, the backend simply doesn't generate the
speed-change request (UBX already does this silently for USB), and the
post-run `ConfigProps()` reflects what actually landed.

## UBX backend

Port detection already exists on the configure path:

- `pollMonComms` (`gps/internal/ubx/ubxcfg.go:837`) polls UBX-MON-COMMS
  on protVer >= 50.
- `pollPrt` (`gps/internal/ubx/ubxcfg.go:853`) polls UBX-CFG-PRT as
  fallback on older firmware.
- Both currently fire only when `Opts.BaudRate != 0 || Opts.SetsMsgs()`.
- The detected port is stored in `c.portID`.

Changes needed:

1. Add `target.Get & gpsprot.PropIDPort != 0` to the trigger conditions
   for both `pollMonComms` and `pollPrt`. Factor the shared condition
   into a helper. Note: use `target.Get` directly, *not*
   `target.UsesAny(...)`. `UsesAny` ORs in `Props.valid`, but for a
   read-only property `Props.valid` should never participate in
   retrieval decisions -- this matches the rule that backends
   silently ignore read-only bits in `target.Props`. No other gating
   changes; `pollPrt`'s existing version handling and
   `c.portID == nil` early-return are unchanged.
2. Map `MonCommsPortID` / `CfgPrt PortID` values to `Port`:
   - `UART1` -> `{ "UART1", true }`
   - `UART2` -> `{ "UART2", true }`
   - `USB`   -> `{ "USB",   false }`
   - `I2C`   -> `{ "I2C",   false }`
   - `SPI`   -> `{ "SPI",   false }`
3. In `Configurator.ConfigProps()` (`gps/internal/ubx/ubxcfg.go:326`),
   set `PropIDPort` on the result when `target.Get & PropIDPort != 0`
   and either `c.portID` or `c.raw.prt` is known. The two sources
   mirror `valPort()`'s existing fallback logic: `c.portID` is set by
   MON-COMMS, `c.raw.prt.PortID` by the CFG-PRT fallback. If neither
   is set, leave `PropIDPort` unset on the result. (Do not refactor
   `valPort()` itself; the population code uses an inline check or a
   small helper that mirrors the first two branches without the
   UART1 fallback.)
   Note: the result is *not* populated just
   because `Opts.BaudRate != 0`. Port behaves like every other
   property: it appears in the result iff the caller asked for it via
   `target.Get`. A GUI that wants to confirm whether a requested
   speed change applied must set `target.Get |= PropIDPort` itself --
   one line, entirely under the caller's control. Special-casing port
   to also appear on speed-change requests would make it the only
   property with that behaviour.

## UNC backend

Deferred. When implemented:

- Add a `LOGLIST` request, gated on
  `target.Get & PropIDPort != 0 || target.Opts.BaudRate != 0`.
  (Same reasoning as UBX: `target.Get` directly, not `UsesAny`,
  because port is read-only.)
- Parse the first line `<LOGLIST <PORT> ...` to extract the current
  port name.
- COM1/COM2/COM3 -> `HasBaudRate: true`; anything else -> `false`.
- LOGLIST uses the NOVA `<` packet format which the unc package does
  not currently parse. Adding it is part of the separate UNC
  speed-change work, not this plan.

## Other backends

Quectel, AS, CASIC, NMEA-only, etc. simply leave `PropIDPort` unset on
the result. Best-effort: callers see "not known" and don't show
port-specific UI.

## JSON round-trip

`ConfigProps.MarshalJSON` (`gps/gpsprot/configtarget.go:830`) gains an
entry for `port`:

```json
{ "port": { "name": "UART1", "hasBaudRate": true } }
```

`ConfigProps.UnmarshalJSON` (line 535) gains a `case "port":` that
calls `SetPort` (or writes the field directly, since it's in-package).

`propNames` (`gps/gpsprot/configtarget.go:64`) gains `"Port"` at the
position matching `PropIDPort`'s bit. This is what
`PropIDs.String()` (line 260) reads when formatting the bitmask, so
the panic message at `gpscfg.Configure` correctly names the
offending bit.

`PropIDs` JSON (`propIDJSON` and `propIDJSONNames` at lines 272 and
290) gains `"port"` -> `PropIDPort`.

`Inconsistent`, `Missing`, `CopyFrom` (lines 724, 769, 817) each gain a
port branch following the existing pattern.

## CLI display

`internal/gpscmd/gpscmd.go` `printProps` (line 397) gains a `printPort`
helper following the existing `Label: value[; modifier]*` style used by
`printTimePulse` and friends:

```
Port: UART1; speed configurable
Port: USB; speed not configurable
```

This way `satpulsetool gps` shows the port when the user requests it
via `target.Get |= PropIDPort`.
Port is not part of receiver configuration -- it's a property of the
connection -- so it gets its own `--show-port` flag rather than
riding on `--show-config`. The change is confined to
`internal/gpscmd/gpsflags.go`:

- Declare a `showPort bool` local; bind it with `flags.BoolVar` to
  `--show-port`.
- After flag parsing, OR `PropIDPort` into `vars.configGet` when
  `showPort` is true. This composes cleanly with the existing
  `if showConfig { vars.configGet = showProps }` -- the `|=` runs
  after the `=`, so `-c --show-port` retrieves both.
- Add `showPort` to the existing `doConfigure := save || saveAll ||
  ... || showConfig || configChanged` line, so the configurator is
  invoked when only `--show-port` is given.
- Update the inline usage string at line 56 to include
  `[--show-port]` alongside the other show-* flags.

`printProps` already runs `printPort` whenever `GetPort()` returns
ok, so no further wiring is needed.

This keeps `-c` behaviour identical to today: no extra MON-COMMS poll
on UBX, no new output line for callers that don't ask.

## Tests

- `gps/gpsprot/configtarget_test.go`:
  - round-trip JSON for a `ConfigProps` with port set
  - `PropIDs` JSON for `"port"`
  - `CopyFrom` / `Inconsistent` / `Missing` semantics for port (port
    is propagated like any other prop)
  - `ReadOnlyProps()` returns the read-only bits that are valid
  - `ClearReadOnlyProps()` clears port and leaves other props untouched
  - `gpscfg.Configure` panics when given `target.Props` with port
    set; backend `ConfigProtocol.Configure` implementations called
    directly do not panic and silently ignore read-only bits
- `gps/internal/ubx/ubxcfg_test.go` -- existing `TestMonCommsPort`
  already exercises port extraction. Add cases:
  - `target.Get & PropIDPort != 0` triggers a MON-COMMS or CFG-PRT
    poll even when no write is requested.
  - Resulting `ConfigProps` has `Port.HasBaudRate` correct for USB
    vs UART.
  - CFG-PRT fallback path: when `c.portID == nil` (MON-COMMS did
    not identify the output port) but `c.raw.prt` is populated,
    `ConfigProps()` still reports `PropIDPort`. This pins down the
    fallback that the `valPort()`-mirror logic enables.
- `internal/gpscmd/gpsflags_test.go`:
  - `--show-port` alone: `vars.configGet == PropIDPort` and the
    configurator runs.
  - `-c --show-port`: `vars.configGet == showProps | PropIDPort`.
  - No-flag invocation still defaults to `--show-receiver`
    (existing behaviour preserved).

## Out of scope

- Surfacing the current baud rate as a separate `PropIDBaudRate`. The
  GUI can ask for speed configurability via `PropIDPort` and infer
  what control to show; the actual current baud value is a separate
  follow-up if the GUI wants to display it.
- A typed error for "port not configurable". The best-effort model
  handles this implicitly.
- Generalising to a broader `InfoIDs` bitmask alongside `PropIDs`. The
  read-only-property-on-ConfigProps shape is enough for now and
  scales to other read-only properties (e.g. supported signal plans)
  by adding more `PropID*` constants.
