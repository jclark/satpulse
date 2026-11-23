# JSON ConfigTarget: name-based option vocabulary and a satpulsetool argument

Issue: #372. Two related changes:

1. Give the `ConfigOptions` flag and enum types a name-based JSON
   vocabulary, replacing the raw integers the workbench wire format
   uses today, and update the webui to match.
2. Add a hidden satpulsetool gps argument that accepts a whole
   `ConfigTarget` as JSON, giving the CLI parity with the workbench's
   `POST /api/config/apply` and making every configtarget.go semantic
   expressible without per-flag CLI work.

Part 1 stands alone (it fixes a real fragility); part 2 builds on it.

## Background

The JSON `ConfigTarget` interface already exists end to end: the
workbench config panel builds `{Props: {...}, Opts: {...}}` and posts
it to `/api/config/apply`, which unmarshals into a `ConfigTarget` and
calls `session.ApplyConfig` (cmd/satpulsewb/server.go:282). The two
halves speak different vocabularies:

- `Props` has custom marshal/unmarshal (configtarget.go:589) in the
  readback vocabulary (`signalsEnabled`, `mode`, `timePulse`, ...),
  driving the setters so validity bits are set. `PropIDs` (`Get`)
  marshals as an array of property name strings (configtarget.go:312).
  `ClearReadOnlyProps` exists specifically so a deserialized result
  can be reused as `target.Props`.
- `Opts` uses default struct marshaling: Go field names and raw
  integer flag values. The webui hand-mirrors the bit assignments in
  webui/packages/workbench/src/msg-flags.ts ("keep in sync" comment)
  and hardcodes `Survey.Flags: 1` for SurveyAgain and numeric
  save/reset codes in config-panel.tsx. Any bit renumbering in
  configtarget.go silently corrupts webui requests.

satpulsetool gps predates gpshwtest and does not expose all of
configtarget.go (`SetStatic` is hidden, `NMEAMsgOther`/`RTCMMsgOther`
and unaligned time pulses are inexpressible, `--survey` always sets
`SurveyAgain`). Rather than adding flags one by one, a JSON target
argument exposes the whole structure at once; see the closing section
of gpshwtest-fixes.md for the semantics this unlocks for testing.

## Part 1: name-based JSON for ConfigOptions types

### Flag sets (gps/gpsprot/configtarget.go)

Add `MarshalJSON`/`UnmarshalJSON` marshaling as arrays of name
strings, modeled on `PropIDs` (configtarget.go:312-327), to:

| Type | Names |
|------|-------|
| `NMEAMsgFlags` | `RMC GGA GSA GSV ZDA VTG GLL other` |
| `RTCMMsgFlags` | `MSM4 MSM7 ARP lax other` |
| `PVTMsgFlags` | `pos vel time timePulse leapSecond survey tai ecef timePulseAfter quality epoch off` |
| `SatsMsgFlags` | `sat signal` |
| `RawMsgFlags` | `obs navData` |
| `SurveyFlags` | `again` |

Rules:

- Names are derived from the model's own constant names
  (`PVTMsgTimePulseAfter` -> `timePulseAfter`), following the naming
  style `PropIDs` and the readback vocabulary already established:
  camelCase, case-sensitive, defined in gpsprot with no reference to
  the CLI. The CLI keeps its own tokens (`tp`, `leap`, `sig`, ...) as
  its own surface spelling; the two layers name things independently.
- One name-to-bit table per type drives both directions (marshal
  iterates in bit order, like `propIDJSONNames`). Unknown names are
  errors, as in `PropIDs.UnmarshalJSON`.
- CLI conveniences have no JSON equivalent: `ptp`/`ntp` are exact
  flag-set abbreviations, `auto` is the RTCM expansion
  (ARP|lax|MSM4-or-MSM7). JSON speaks the model, so a caller writes
  the expansion out.
- The empty set marshals as `[]`. For the `opt.Val` fields this is
  load-bearing: set-but-empty (turn the group off) must round-trip as
  `[]`, distinct from absent (no request). Verify `opt.Val.IsZero`
  keeps that distinction under the `json:",omitzero"` tags, and test
  the round trip of both states.
- The CLI-level validity rules (`after` requires `tp`, MSM4/MSM7
  exclusive, ...) stay in the CLI parsers; the JSON layer is the raw
  model and does not enforce them.

### Enums

`SaveType` and `ResetType` get `MarshalText`/`UnmarshalText` (which
encoding/json picks up) with model-derived names:

- Save: `none`, `minimal`, `all`
- Reset: `none`, `reload`, `cold`, `factory`

No numeric fallback on unmarshal: the webui flips in the same change,
and there are no other producers (verified: the apply direction is
the only wire use of `ConfigOptions` JSON; nothing marshals it for
output today).

`Survey.MinDur` (nanoseconds) and `AccLimit` (Length, with its own
JSON methods) are unchanged; `TimeAssist` and `OSNMA` keep default
marshaling.

### Tests

Round-trip tests in configtarget_test.go: each flag set (empty, one
flag, all flags, unknown-name error), both enums, and a whole
`ConfigTarget` marshal/unmarshal round trip including a set-but-empty
`opt.Val` field.

### webui (same change)

- msg-flags.ts: bit constants become name constants (the "keep in
  sync" burden shrinks to spellings, which no longer silently break).
- config-panel.tsx `handleApply` and the `*WireValue` helpers: build
  arrays of names instead of OR-ing numbers (`opts.PVTMsg = ["survey"]`,
  `opts.Survey.Flags = surveyAgain ? ["again"] : []`); map the
  numeric save/reset UI state to the enum strings at the wire.
  UI state representation is otherwise untouched.
- Regenerate and commit the embedded assets: `go generate
  ./cmd/satpulsewb` (webui/CLAUDE.md; make does not catch stale dist).

## Part 2: satpulsetool gps --target-json

New hidden flag (like `--static`) on the gps command:

    satpulsetool gps -d /dev/ttyUSB0 --target-json '{"Props":{...},"Opts":{...}}'

- The value is the JSON text of a `ConfigTarget` (`Props`, `Get`,
  `Opts`, all optional), or `-` to read the JSON from stdin. No file
  form; the shell covers that.
- Mutually exclusive with the config-shaping flags (signals, mode,
  time pulse, message output, save/reset, `--static`, ...): error if
  combined. Connection and session flags (`-d`/`-s`/`-f`, `--json`,
  `--capture`, `--packet-log*`, `--show-receiver`/`--show-config`/
  `--show-port`) combine normally.
- Wiring: parseFlags stores the string; `createConfigTarget`
  (gpscmd.go:86), when it is set, unmarshals into
  `gpsprot.NewConfigTarget()` with `DisallowUnknownFields` (an expert
  interface should reject typos loudly), then calls
  `Props.ClearReadOnlyProps()` so a `--show-config` result can be fed
  back verbatim - the use case that method documents. The existing
  NoOp/Socket/ForceProbe logic then applies unchanged, and the target
  flows into `run()` as usual.
- Verify `ConfigProps.UnmarshalJSON` rejects unknown property keys
  (it switches on key names); if it silently ignores them, make it
  error - same typo argument.
- Policy-free by design: none of the flag layer's policy applies -
  no `SurveyAgain` injection, no save-required-with-reset rule, no
  capability pre-checks (`configSupportReq` stays empty; refusals
  surface from the backend or receiver). That is the point of the
  interface: it speaks raw configtarget.go.
- Hidden flag, so no man page entry, following the `--static`
  precedent (internal/gpscmd/CLAUDE.md's man-page rule applies to the
  documented surface). No NEWS entry: not user-facing.

### Tests

gpscmd unit tests on `createConfigTarget`: a target from JSON
(props + opts + get), unknown-field rejection, exclusivity with a
config flag, and a round trip of a readback `config` object fed back
as `Props`.

## Order of work

1. Part 1 Go types + tests.
2. webui flip + regenerated assets (same commit as 1, since the wire
   format changes).
3. Part 2 flag + tests.
