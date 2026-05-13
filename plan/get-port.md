# Get current port via read-only ConfigProps (#271)

Add the receiver port name as a read-only `gpsprot.ConfigProps`
property. This lets callers ask which receiver port SatPulse is
currently communicating over, without treating that port as something
SatPulse can set through `ConfigTarget`.

This is intentionally narrower than the older `get-port` branch and
older plan. Baud rate is already a normal `ConfigProps` property, so
port does not need to carry any speed-related metadata. The property
value is just a string such as `"USB"`, `"UART1"`, or `"COM1"`.

The same read-only-property mechanism should later support other
receiver facts that are useful to query but not set, especially
supported signal plans.

Existing reference implementation: branch
[`get-port`](https://github.com/jclark/satpulse/tree/get-port), commit
[`ad7fe32d`](https://github.com/jclark/satpulse/commit/ad7fe32d)
(`Implement get-port: surface receiver port info via ConfigProps`).
That branch is a good basis for the read-only property mechanics and
UBX polling/result plumbing, but its `Port` struct and `HasBaudRate`
field should be collapsed to the string property described here.

## Preliminary phase: drop the valPort UART1 fallback

Land this as its own commit before the rest of the plan.

`gps/internal/ubx/ubxcfg.go`'s `valPort()` currently falls back to
`UART1` when neither `c.portID` nor `c.raw.prt` is known (marked with
`// XXX what to do here`). That silent guess can leak into real writes
- UART baud-rate keys, per-port output-rate keys, per-port protocol
enables - on receivers we are actually talking to over USB. Change the
signature to:

```go
func (c *Configurator) valPort() (ucv.Port, bool)
```

returning `false` when the port has not been discovered, and remove
the fallback. Update the callers in the same file:

- `valBaudRate`: if `!ok`, return nil. We cannot target a UART
  baud-rate key without knowing which UART.
- `valGet` and `valSet`: propagate the unknown state through to
  `CfgVals.Transaction`. The simplest shape is to widen `Transaction`,
  `addGetKeys`, `msgChanges.items`, and `portBaudRateKey` to take
  `(port ucv.Port, ok bool)` (or accept `*ucv.Port`) and skip
  port-specific items - UART baud rate, per-port output rates,
  per-port protocol enables - when `!ok`.

Tests:

- `valBaudRate` is a no-op when neither `c.portID` nor `c.raw.prt` is
  set.
- `CfgVals.Transaction` / `addGetKeys` emit no UART1-specific items
  when the port has not been discovered.
- Existing val-based UBX tests that relied on the UART1 fallback are
  updated to populate `c.portID` or `c.raw.prt` explicitly (or to pass
  a known port to `Transaction` directly).

After this lands, the rest of the plan can rely on `valPort` honestly
reporting whether the port is known, and `currentPort` can share the
same sources without a separate carve-out.

## Design

Add `PropIDPort` as a read-only property on `gpsprot.ConfigProps`.

```go
// gps/gpsprot/configtarget.go

type ConfigProps struct {
    // ... existing fields ...
    port string
}

const (
    // ... existing PropIDs ...
    PropIDPort
)

const PropIDsReadOnly PropIDs = PropIDPort

func (cp *ConfigProps) GetPort() (string, bool) {
    if cp.valid&PropIDPort != 0 {
        return cp.port, true
    }
    return "", false
}

func (cp *ConfigProps) SetPort(port string) {
    cp.port = port
    cp.valid |= PropIDPort
}

func (cp *ConfigProps) ReadOnlyProps() PropIDs {
    return cp.valid & PropIDsReadOnly
}

func (cp *ConfigProps) ClearReadOnlyProps() {
    cp.valid &^= PropIDsReadOnly
}
```

`SetPort` is public because backend packages must populate result
properties, but callers must not put read-only properties in
`target.Props`.

## Read-Only Contract

A property is read-only iff its bit is in `PropIDsReadOnly`.

- Read-only properties may appear in a result `ConfigProps`.
- Read-only properties must not appear in `target.Props`.
- Callers request read-only properties through `target.Get`, e.g.
  `target.Get |= gpsprot.PropIDPort`.
- Backends populate read-only values on the result with the normal
  setter.

Enforce the contract once, at the public `gpscfg.Configure` entry
point:

```go
if ro := target.Props.ReadOnlyProps(); ro != 0 {
    panic(fmt.Sprintf("read-only properties in target.Props: %v", ro))
}
```

Backend `ConfigProtocol.Configure` implementations do not need their
own duplicate check. If called directly, they can naturally ignore
read-only bits in `target.Props` because no write path exists for
them.

`CopyFrom`, `Inconsistent`, and `Missing` stay content-preserving:
they operate on all valid properties, including read-only ones. If a
caller wants to turn a result-shaped `ConfigProps` into target props,
it must call `ClearReadOnlyProps()` first.

## JSON

Serialize port as a plain string:

```json
{ "port": "USB" }
```

Update:

- `propNames` with `"Port"`.
- `propIDJSON` and `propIDJSONNames` with `"port"`.
- `ConfigProps.UnmarshalJSON` with a `"port"` case that calls
  `SetPort`.
- `ConfigProps.serializableMap` to emit the string value.
- `CopyFrom`, `Inconsistent`, and `Missing` handling as needed.

## UBX Backend

The `get-port` branch has most of the useful UBX mechanics. Keep the
shape, but simplify the result to a string.

Port detection sources:

- `c.portID`, set from `UBX-MON-COMMS` when available.
- `c.raw.prt.PortID`, populated by `UBX-CFG-PRT` fallback.

After the preliminary phase, `valPort` already reads these same
sources and returns `(port, false)` when neither is set, so reporting
can reuse it directly. A reported port must mean the backend actually
learned the port.

Sketch:

```go
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
    cp := c.raw.Config(c.ver, c.portID)
    if cp != nil && c.target.Get&gpsprot.PropIDPort != 0 {
        if p, ok := c.valPort(); ok {
            if name, ok := ubxPortName(p); ok {
                cp.SetPort(name)
            }
        }
    }
    return cp
}
```

Map UBX port IDs to the names users see elsewhere:

```text
UART1
UART2
USB
I2C
SPI
```

Unknown UBX port IDs leave `PropIDPort` unset.

### Poll Gating

`pollMonComms` and `pollPrt` already run when the configurator needs
the current port for normal writes, such as message enablement or
baud-rate changes. Add the read-only query to the same gate. This is
also what lets `--show-port` ask for the existing `baudRate` property
without guessing which port-specific key to read.

Because `PropIDPort` is read-only, use `target.Get` directly rather
than `target.UsesAny(PropIDPort)`.

```go
func (c *Configurator) needsPort() bool {
    if c.target.Get&gpsprot.PropIDPort != 0 {
        return true
    }
    return c.target.UsesAny(gpsprot.PropIDBaudRate) || c.target.Opts.SetsMsgs()
}
```

Then use `needsPort()` in both `pollMonComms` and `pollPrt`.

### Baud Rate Query

`baudRate` is already a normal `ConfigProps` property. `--show-port`
should request it together with `PropIDPort`.

For legacy UBX, the existing `CFG-PRT` fallback already provides the
current baud rate for UART ports and `0` for ports where baud rate does
not apply.

For val-based UBX, finish the existing `CfgVals.addGetKeys` TODO for
`PropIDBaudRate`:

- Use the current port passed to `Transaction` (now `(port, ok)`
  after the preliminary phase).
- Add `KUart1Baudrate` or `KUart2Baudrate` for UART ports.
- Add no key for USB, I2C, or SPI; `CfgVals.getBaudRate` already
  reports those as `0` when the active port is known.
- Add no key when `!ok`; the result may then omit both `port` and
  `baudRate` rather than reporting a guessed speed.

## Other Backends

Other backends may leave `PropIDPort` unset until they have a
protocol-specific way to discover it.

UNC can later populate it from `LOGLIST` as a string such as `"COM1"`.
That work is separate because it depends on parsing the NOVA-style
`<...>` response.

## CLI

Add a `--show-port` flag to `satpulsetool gps`.

Behavior:

- `--show-port` sets
  `vars.configGet |= gpsprot.PropIDPort | gpsprot.PropIDBaudRate`.
- It triggers the configurator even if no write is requested.
- It composes with `--show-config`; `-c --show-port` asks for both.
- It prints the port when known, and prints serial speed only when the
  baud rate is known and non-zero:

```text
Port: UART1
Serial speed: 9600
```

When both are present, display port first and baud rate second.

For USB, I2C, and SPI, omit the `Serial speed` line rather than
printing `not applicable`.

Port is not included in `showProps` by default. `--show-config`
continues to show receiver configuration, while `--show-port` asks for
the connection port and its serial speed when applicable.

This gives users a straightforward way to discover the value to pass
to message-file `--port`, without making message-file mode probe or
guess.

## Tests

Add focused tests for:

- `ConfigProps` JSON round-trip with `"port": "USB"`.
- `PropIDs` JSON for `"port"`.
- `ReadOnlyProps` and `ClearReadOnlyProps`.
- `gpscfg.Configure` panics when `target.Props` contains a read-only
  port property.
- `CopyFrom`, `Inconsistent`, and `Missing` preserve port like other
  valid properties.
- UBX `target.Get&PropIDPort` triggers `MON-COMMS` or `CFG-PRT` even
  when no write is requested.
- UBX `target.Get&PropIDBaudRate` with an already discovered UART port
  requests the matching val-based baud-rate key.
- UBX `target.Get&PropIDBaudRate` with USB, I2C, or SPI does not add a
  val-based baud-rate key and reports baud rate as zero only when the
  active port was actually discovered.
- UBX result population from `c.portID`.
- UBX result population from `c.raw.prt` fallback.
- UBX leaves port unset when neither `c.portID` nor `c.raw.prt` is
  known (regression test for the removed UART1 fallback; complements
  the preliminary-phase tests on the write path).
- `--show-port` flag parsing requests both `PropIDPort` and
  `PropIDBaudRate`; `-c --show-port` composes with `showProps`.
- CLI output prints `Port: <name>` when the property is present and
  `Serial speed: <rate>` only when baud rate is non-zero.
- CLI output order is port first, baud rate second.
- `internal/gpscmd` replay coverage for `--show-port`, not just unit
  tests. Add a dedicated replay command/trace rather than changing the
  existing `noop` or `reload` traces.
- Existing replay tests already exercise `--show-config` through
  `internal/gpscmd/testdata/cmd/noop.sh` and
  `internal/gpscmd/testdata/cmd/reload.sh`; those should keep passing
  unchanged because port is not part of `showProps`.
- Include at least one UBX Gen 9 replay where `MON-COMMS` identifies
  the current port. If that receiver is on USB, the replay should also
  cover the zero-baud path. If a suitable UART/legacy UBX trace is
  available, add a second replay for the `CFG-PRT` fallback path and
  non-zero baud rate.

## Out Of Scope

- Reintroducing `HasBaudRate`; current speed and speed applicability
  belong to the existing `baudRate` property behavior.
- Implementing supported-signal read-only properties. This plan only
  establishes the read-only-property pattern they can use later.
- Message-file `--port`; that is covered by `plan/ubx-msg-port.md`.
