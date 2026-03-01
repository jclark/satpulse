# @satpulse/gps

TypeScript type definitions for the JSON wire format of `gps/gpsprot`, `gps/ptime`, and `gps/app/gpsio` Go types.

## Files

- `gpsprot.ts` -- interfaces for gpsprot message and value types
- `ptime.ts` -- Time and UTCTime type aliases
- `gpsio.ts` -- PacketLogEntry interface
- `validate.gen.ts` -- generated file: typed constants with JSON literals (checked in)
- `generate.go` -- Go code that produces validate.gen.ts
- `gen.go` -- go:generate entry point (writes validate.gen.ts, runs tsc)
- `gen_test.go` -- drift detection test

## How it works

The hand-written `.ts` files define TypeScript interfaces matching the JSON
serialisation of Go types. A Go program (`generate.go`) constructs sample
instances of each message type, marshals them to JSON, and writes
`validate.gen.ts` containing typed const initialisers:

```ts
const _satellitesMsg: SatellitesMsg = {"tag":"UBX","info":[...]};
```

Running `tsc --noEmit` on this file validates that the JSON output matches
the interfaces. If `tsc` fails, the interfaces need updating.

The generated `validate.gen.ts` is checked in. A Go test (`gen_test.go`)
re-generates the JSON in memory and compares it to the checked-in file.
If a Go type changes its JSON output, the test fails:

    gpsprot JSON serialisation has changed; run: go generate ./gps/ts

This test runs as part of `make test` and requires no TypeScript tooling.

## Workflow

- `make test` -- runs `gen_test.go`; fails if Go JSON has drifted.
- `go generate ./gps/ts` -- re-generates `validate.gen.ts` and runs
  `tsc --noEmit`. Requires `npm install` in this directory first.
- If `tsc` reports errors, update the hand-written interfaces, then
  re-run `go generate`.

## Consumers

Add a `file:` dependency in `package.json`:

```json
"@satpulse/gps": "file:../../gps/ts"
```

Import types:

```ts
import type { SatellitesMsg, SVInfo } from '@satpulse/gps/gpsprot';
import type { Time } from '@satpulse/gps/ptime';
```
