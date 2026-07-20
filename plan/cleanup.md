# Cleanup opportunities

Concrete, behaviour-preserving tidies: dead-code removals and small local
dedups that stand on their own merits. Line numbers are approximate and should
be re-checked against the current tree. Run the affected package's tests after
each change.

## High value: dead code in `gps/internal/ubx/ubxcfgvals.go`

About 84 deletable lines behind two dev feature-flags whose branches never run.

- `inferTimegridTp1` contains an `if false { ... }` block (~738-747) that never
  executes. Its only use of `rateTimeRefToTimegridTp1` is inside that block, so
  that function (~805-818) becomes dead once the block goes.
- `messagesBuild` is selected by `if true { messagesBuild() } else { messagesBuildOld(tg) }`
  (~321-327); the `else` is unreachable, making `messagesBuildOld` (~334-374,
  ~41 lines) dead. Its only caller of `timegridTp1ToMsgRateKey` is inside it, so
  that helper (~886-899) dies too.
- After removing `messagesBuildOld`, the `tg` value returned by `timePulseBuild`
  loses its only real consumer, so `timePulseBuild` can drop its return value
  (simplifying its call sites).

Before deleting, grep the package's `*_test.go` for `messagesBuildOld`,
`timegridTp1ToMsgRateKey`, and `rateTimeRefToTimegridTp1` to confirm no test
references them.

## Low-risk micro-tidies

These are each a handful of lines, verbatim duplicates or trivial inlines.

### `time/internal/promobs/prometheus.go`
- `setOrDeleteOptLabel` (~570-579) is byte-for-byte identical to `setOrDeleteOpt`
  (~557-566). Delete it and repoint its two callers (~443, ~445). (~11 lines)
- `clearInfoGauge` (~612-619) and `clearBitmaskGauge` (~645-652) have identical
  bodies; merge into one `clearGauge`. (~8 lines)
- The block of 21 `reg.MustRegister(...)` calls (~205-226) can fold into a single
  variadic `reg.MustRegister(...)` call (it takes `...Collector`). (~20 lines)

### `gps/msgfile/msgfile.go`
- `ToRaw` repeats `if save { return nil, errSaveNotApplicable }` seven times
  (~808-856). Handle the two save-aware types first, then hoist a single guard.
  (~12 lines)
- `TaggedMsgs` (~597-656) has a fat 6-line block per message type; a small
  generic `pick[T]` helper collapses it to ~2 lines per type and reads better.
  (~30 lines)

### `gps/internal/unc/cfgprops.go`
- `coordsIsECEF` (~797-804) is `coordsIsLLH` (~788) with one negation; inline it
  as `!coordsIsLLH(...)` at the single call site (~777) and delete it. (~7 lines)

### `gps/internal/ubx/ubxcfgold.go`
- `getTmodeConfig` (~145-157) and `changeTmode` (~162-171) replicate the same
  tmode3 -> tmode2 -> tmode selection chain. Extract `currentTmodeMsg()` and
  route both through the existing `fromTmodeMsg` dispatch (ubxcfgtmode.go). (~6 lines)

### `gps/gpsprot/configtarget.go` (optional)
- `UnmarshalJSON` (~598-691) and `unmarshalTimePulse` (~693-731) repeat
  `json.Unmarshal(...); fmt.Errorf("key: %w", err)` ~15 times. A type-safe
  generic `unmarshalField[T](name, raw)` helper collapses each pair to one line
  without losing type safety. Modest (~20-30 lines) but clean.

## Enum String/Parse twin tables

Several enums in the GPS message libraries carry a `String()` switch and a
`Parse*()` switch that are exact mirrors over the same value set, so the
value<->name mapping is written twice and can drift. Collapse each to one typed
table that drives both directions, via a small generic helper in `novmsg` (the
lowest layer):

    type nameEntry[T comparable] struct { val T; name string }
    func enumName[T comparable](v T, tab []nameEntry[T]) (string, bool)
    func enumValue[T comparable](s string, tab []nameEntry[T]) (T, bool)

The table column is the concrete enum type (compile-time checked, no reflection
or `any`); the type-specific default (numeric fallback or error text) stays in
the thin `String()`/`Parse*()` wrappers. Behaviour is identical.

- `gps/lib/novmsg/navtypes.go`: `SolStatus` (105-178), `PosType` (241-394),
  `SinoPosType` (423-492). The const blocks stay; the value 18 collision
  (`PosWAAS` vs `PosSBAS`) is fine since they live in separate tables. (~215 lines)
- `gps/lib/uncmsg/navtypes.go`: `SolStatus` (22-55), `PosVelType` (101-202).
  Preserve the `"ppp"` alias with a second `{PosVelPPP, "ppp"}` row. (~100 lines)
- `gps/lib/uncmsg/version.go`: `ProductModel` (40-125), 17 values. (~60 lines)
- Opportunistic, once the helper exists: smaller twins like `TimeStatus`
  (`gps/lib/novmsg/common.go`) and `ClockStatus`/`UTCStatus`
  (`gps/lib/novmsg/time.go`).

## Overly complex functions

Flagged by cognitive complexity (gocognit) and nesting depth, not by
duplication. Flat dispatch switches are excluded: a long `switch` with
one-line cases is not itself complexity. In rough priority order; the
score is shown so the sort is visible.

- `gps/scan/scan.go` `Scan` (line 98) - cognitive 75, nesting depth 7 (the
  deepest in the repo), 113 lines. A packet-scanning state machine whose I/O
  refill and error classification (timeout / temporary / mid-packet restart)
  nests up to seven levels, with two `goto Loop` jumps and several interleaved
  concerns (refill, format detection, rescan-on-bad-checksum, restart).

- `gps/gpsprot/configprotocol.go` `ConfigDirector.Actions` (line 395) -
  cognitive 63, 91 lines. An unbounded `for` loop containing a nested per-request
  `switch` with `fallthrough`, retry counting, deadline tracking, two `panic`
  paths, and multiple `yield`/`return` exit points; the control flow is hard to
  trace.

- `time/app/daemon/daemon.go` `run` (line 92) - cognitive 61, 260 lines (second
  longest), nesting depth only 3. A god-function: one function sequentially wires
  up logging, config, GPS, observers, HTTP, and shutdown. Low nesting but very
  low cohesion - many unrelated responsibilities in one body.

- `time/internal/syncsim/syncsim.go` `Simulate` (line 167) - cognitive 61, 296
  lines (longest after `parseFlags`), nesting depth 6. Both long and nested: the
  simulation driver carries the whole run loop in one function.

- `gps/internal/ubx/ubxcfgold.go` `changeGNSS` (line 501) - cognitive 65, nesting
  depth 7, 100 lines. Nested loops over GNSS blocks and per-signal bit masks with
  version-gated branches, then a second corrective pass to drop GLONASS under
  frequency / major-GNSS limits. Densely nested, though much of the complexity is
  intrinsic to the UBX protocol's constraints.

- `gps/lib/bitsenc/bitsenc.go` `readStruct` (line 131) / `writeStruct` (line 321)
  - cognitive 80 / 70, nesting depth 6. A reflection-driven bit-field codec that
  branches on field kind (struct / slice / var / tagged) with recursion and inner
  loops, returning errors from six levels deep; `writeStruct` mirrors
  `readStruct`. Complexity is partly intrinsic to a reflection-based codec.

- `gps/internal/casic/casproc.go` `PacketProcessor.dispatch` (line 105) -
  cognitive 61, 158 lines. A message dispatch switch, which is normally fine, but
  several cases (`NavSol` / `Nav2Sol` / `NavPv` / `Nav2Pvh`) carry repeated
  multi-line pos+vel handling (set `Tag` and `Priority`, nil-check the handler,
  call the Pos/Vel methods) rather than a one-line dispatch; that per-case work
  is what pushes the score to 61. Worth checking whether it belongs in the switch.
