# Move baud rate from Opts to Props

A desktop GUI deciding whether to offer a speed-change control needs
to know whether the receiver port has a baud rate at all (UART yes;
USB / I2C / SPI no). The cleanest way to surface that signal -- and
the receiver's current speed alongside it -- is to make the baud rate
a `gpsprot.ConfigProps` property: the validity bit handles "didn't
ask / not known", and a value of 0 in a result means "this port type
has no baud rate".

This plan only handles the move. The implementation deliberately
does not go out of its way to surface the actual current UART speed
on a plain query: where the value falls out for free (e.g. legacy
firmware where detection already polls CFG-PRT), the result reflects
it; where surfacing would require new poll requests (e.g. val-based
firmware would have to ask for `KUart{N}Baudrate`), it does not.

This plan assumes any earlier port-related changes have been reverted;
it does not depend on a port-name property and is independent of any
later read-only-properties work (e.g. surfacing port name or supported
signal plans).

## Design

Replace `gpsprot.ConfigOptions.BaudRate` with a new
`gpsprot.ConfigProps` property `PropIDBaudRate` (`uint32`).

```go
// gps/gpsprot/configtarget.go

type ConfigProps struct {
    // ... existing fields ...
    baudRate uint32
}

const (
    // ... existing PropIDs ...
    PropIDBaudRate
)

// GetBaudRate returns the baud rate and whether it's set.
// In a result, value 0 means the port type has no baud rate (USB / I2C / SPI).
func (cp *ConfigProps) GetBaudRate() (uint32, bool) {
    if cp.valid&PropIDBaudRate != 0 {
        return cp.baudRate, true
    }
    return 0, false
}

// SetBaudRate sets the baud rate. In a target, the value is the new
// speed to configure.
func (cp *ConfigProps) SetBaudRate(val uint32) {
    cp.baudRate = val
    cp.valid |= PropIDBaudRate
}
```

`ConfigOptions.BaudRate` is removed. All internal call sites that
currently read `target.Opts.BaudRate` instead read
`target.Props.GetBaudRate()`.

### Read/write semantics

`PropIDBaudRate` is a normal r/w property, like `SignalsEnabled` and
the others. There is no read-only mask involvement.

- **Target.** Caller sets `target.Props.SetBaudRate(N)` to request a
  new speed `N`. Caller sets `target.Get |= PropIDBaudRate` to read
  back the current speed.
- **Result.** Backend populates `Props.BaudRate` on the result when
  `target.UsesAny(PropIDBaudRate)` is true (i.e. the caller either
  asked via `target.Get` or supplied a write via `target.Props`) and
  the active port is known:
  1. Port has no baud rate concept (USB / I2C / SPI) -> value `0`.
  2. Port is a UART and a current value is known to the backend
     without extra work -> that value. This naturally covers the
     "we just successfully wrote `N`" case (the write path already
     records the new value on internal config state) and any case
     where existing detection polls happen to have surfaced the
     current speed.
  3. Otherwise -> leave unset. Specifically: do not add new poll
     requests just to learn the current UART speed.

### Migration of existing `Opts.BaudRate` writes

Today:
```
target.Opts.BaudRate = 9600
```
becomes:
```
target.Props.SetBaudRate(9600)
```

The "0 means do not change" sentinel that `Opts.BaudRate` used is
replaced by the validity bit: bit unset means "do not change".

Backends translate validity-bit-set into a request to change speed,
exactly as `Opts.BaudRate != 0` did before.

### `Opts` callers to migrate

All non-test references to `Opts.BaudRate` in the tree are:

- `internal/gpscmd/gpsflags.go`: the `--speed` flag writes
  `vars.configOpts.BaudRate`; the validation block reads it.
- `gps/internal/ubx/ubxcfg.go`: `saveMinimal()` checks
  `Opts.BaudRate != 0` to OR `CfgCfgIOPort` into `saveMask`;
  `pollMonComms`, `pollPrt` gate on
  `Opts.BaudRate != 0 || Opts.SetsMsgs()`; `setBaudRate()` reads
  `int(c.target.Opts.BaudRate)`.
- `gps/internal/ubx/ubxcfgold.go`: `changePrtBaudRate(opts)` reads
  `opts.BaudRate`.
- `gps/internal/ubx/ubxcfgvals.go`: `BaudRate(target, port)` reads
  `target.Opts.BaudRate`.

Each site changes to `target.Props.GetBaudRate()`. Where the existing
code distinguishes "set" from "zero", use the bool returned by
`GetBaudRate`. Where the existing code uses `Opts.BaudRate != 0`, the
new condition is `_, ok := target.Props.GetBaudRate(); ok`.

The single caller that sets the property is the `--speed` CLI flag,
which rejects `0`. The property is therefore always non-zero when
the validity bit is set, so `ok` matches today's `!= 0` semantics
exactly.

`changePrtBaudRate` takes `*gpsprot.ConfigOptions` today; change its
signature to take whatever it actually needs (the value plus the
"set" bool, or the whole `*ConfigTarget`).

## UBX backend

### Result population

`RawConfig.Config(ver)` / `CfgVals.Cook` are *not* changed to emit
baud rate on their own. Instead, an overlay in
`Configurator.ConfigProps()` adds the property after `c.raw.Config`
has built the rest:

```go
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
    cp := c.raw.Config(c.ver)
    if cp != nil && c.target != nil && c.target.UsesAny(gpsprot.PropIDBaudRate) {
        if port, ok := c.activePort(); ok {
            switch {
            case !portHasBaudRate(port):
                cp.SetBaudRate(0)
            case c.knownUartBaudRate(port) != 0:
                cp.SetBaudRate(c.knownUartBaudRate(port))
            }
        }
    }
    return cp
}
```

`activePort()` mirrors the first two branches of `valPort()`:
`c.portID` first, then `c.raw.prt.PortID`. (No UART1 fallback here:
"unknown" is a meaningful answer.)

`knownUartBaudRate(port)` returns the speed if it is already
available for free, otherwise `0`:

- Legacy: `c.raw.prt.BaudRate` if `c.raw.prt != nil`. This is set by
  both `pollPrt` and the `Done()` hook of a successful speed-change
  request (legacy goes through `msgSetRequest.Done() -> raw.AddMsg`
  with the new CFG-PRT), so the polled and just-written cases are
  handled uniformly.
- Val-based: the relevant `KUart{N}Baudrate` from `c.raw.CfgVals`
  if present. The val-set ack path (`raw.AddMsg` for `*CfgValset`
  with `LayerRAM`) populates the cfgvals on a successful write, so
  the just-written case works without extra plumbing. Plain queries
  of the current speed are *not* served (we do not add a `valGet`
  for `KUart{N}Baudrate`); see "Out of scope".

`portHasBaudRate(port)` returns `true` only for `UART1`/`UART2`.

### Trigger for active-port detection

`pollMonComms` and `pollPrt` gate today on
`Opts.BaudRate != 0 || Opts.SetsMsgs()`. Replace the
`Opts.BaudRate != 0` half with `target.UsesAny(PropIDBaudRate)`,
which covers both directions: the caller setting a new speed on
`target.Props` and the caller asking for the current speed via
`target.Get`. `UsesAny` is the correct helper because
`PropIDBaudRate` is a normal r/w property -- unlike the read-only
properties tracked separately (e.g. a future `PortName`), where
`target.Props.valid` must not participate in retrieval decisions.

### Write path

`setBaudRate()` (legacy) and `valBaudRate()` (new) read
`target.Props.GetBaudRate()` instead of `target.Opts.BaudRate`. The
existing speed-change machinery (`addMsgSetSpeedRequest`,
`WriteThenChangeSpeed`) is unchanged.

The "value lands in the result" behaviour is automatic: legacy
goes through `msgSetRequest.Done() -> raw.AddMsg`, so
`c.raw.prt.BaudRate` reflects the new value; val-based goes through
the `CfgValset` ack handling in `raw.AddMsg`, so the relevant key
ends up in `c.raw.CfgVals`. The overlay in `ConfigProps()`
(described above) reads from these and surfaces the value with no
additional bookkeeping.

`saveMinimal()` ORs `CfgCfgIOPort` into `saveMask` when the target
has a non-zero baud rate write (was: when `Opts.BaudRate != 0`).

## UNC and other backends

UNC currently has no speed-change support and does not read
`Opts.BaudRate`; nothing to do. Other backends ignore the property.

## CLI

`internal/gpscmd/gpsflags.go` `--speed` flag:

- Replace `flags.Uint32Var(&vars.configOpts.BaudRate, "speed", ...)`
  with a local `var baudRate uint32` and `flags.Uint32Var(&baudRate, "speed", ...)`.
- After validation, call `vars.configProps.SetBaudRate(baudRate)` (or
  call the setter on `target.Props` after target construction --
  whatever fits the existing flow).
- The `--speed` validation block continues to reject `0`.

`vars.configOpts` loses its `BaudRate` field along with
`ConfigOptions.BaudRate`.

Add `PropIDBaudRate` to the `showProps` constant so `--show-config`
asks for it. No new CLI flag.

`printProps` gains a small helper that prints the speed when
`PropIDBaudRate` is set on the result:

```
Serial speed: 9600
Serial speed: not applicable
```

The "not applicable" form is used when the value is `0` (port has no
baud rate concept). For UART ports whose speed has not been
populated, the line is omitted.

## Test log

`internal/gpscmd/testlog.go` `TestLogConfigEntry.BaudRate *uint32`
already exists but `writeTestLogConfigProps` does not populate it.
Add the obvious branch alongside `RTCMBaseID`:

```go
if br, ok := props.GetBaudRate(); ok {
    entry.BaudRate = &br
}
```

The replay-test goldens are not regenerated. `replay_test.go`
verify() is one-sided -- it checks golden fields against actual
props but does not flag actual fields that the golden omits, so
nothing breaks.

## JSON round-trip

`ConfigProps.MarshalJSON` and `UnmarshalJSON` gain `baudRate`:

```json
{ "baudRate": 9600 }
```

`propIDJSON` and `propIDJSONNames` gain `"baudRate" -> PropIDBaudRate`.

`propNames` gains `"BaudRate"` at the position matching
`PropIDBaudRate`'s bit, so `PropIDs.String()` formats it correctly.

`Inconsistent` and `CopyFrom` each gain a `baudRate` branch
following the existing pattern. `Missing` does not need a branch:
it copies the whole struct and masks `valid`, so new fields ride
along for free.

## Tests

- `gps/gpsprot/configtarget_test.go`:
  - JSON round-trip for a `ConfigProps` with `baudRate` set
  - `PropIDs` JSON for `"baudRate"`
  - `CopyFrom` / `Inconsistent` / `Missing` semantics for `baudRate`
- `gps/internal/ubx/ubxcfg_test.go`:
  - `target.Props.SetBaudRate(N)` triggers the same speed-change
    behaviour the old `target.Opts.BaudRate = N` did (regression
    coverage for the migration).
  - `target.Get |= PropIDBaudRate` with USB / I2C / SPI populates
    the result with `BaudRate = 0`.
  - Legacy UART with `target.Get |= PropIDBaudRate`: result has
    `BaudRate` = the speed `pollPrt` returned (free via
    `c.raw.prt.BaudRate`).
  - Val-based UART with `target.Get |= PropIDBaudRate` and no
    `KUart{N}Baudrate` in cfgvals: result-side `PropIDBaudRate` is
    unset.
- `internal/gpscmd/gpsflags_test.go`:
  - `--speed 9600` results in `target.Props.GetBaudRate() == (9600, true)`
    (the existing `--speed` test cases just rename their expected
    field).
  - `--speed 0` is still rejected.

## Out of scope

- Adding new poll requests just to learn the current UART baud
  rate. Specifically, no new `valGet` step is added for
  `KUart{N}Baudrate`; val-based runs that neither write the speed
  nor incidentally have the value already populated will leave
  `PropIDBaudRate` unset on the result. Legacy runs do not need new
  polls -- `pollPrt` already returns `CfgPrt.BaudRate`, so the
  current UART speed surfaces for free.

  A desktop GUI handling the val-based unset case can fall back to
  the host-side `SerialConn` speed -- the receiver-side speed must
  equal it or the conversation would not have worked. When the
  val-based query is wired up later, that fallback becomes
  unnecessary.
- A read-only `PortName` property and the read-only-property
  scaffolding it would need. Independent change; tracked separately.
- UNC speed-change support. Independent.
