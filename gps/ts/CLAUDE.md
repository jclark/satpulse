# gps/ts

The `.ts` files here are HAND-WRITTEN, not generated. `gpsprot.ts`,
`configtarget.ts`, `ptime.ts`, and `gpsio.ts` are all maintained by hand.
Do not ask whether they are generated; do not look for a generator that
emits them.

The only generated file is `validate.gen.ts`, which is checked in.

## Changing a Go type's JSON

Changing the JSON serialisation of a `gpsprot` (or `ptime`/`gpsio`) type is a
two-sided change:

1. Update the Go struct tag.
2. Update the corresponding hand-written interface here.
3. Run `go generate ./gps/ts` to regenerate `validate.gen.ts`, and commit it.

`gen_test.go` runs under `make test` and fails if the Go JSON has drifted from
the checked-in `validate.gen.ts` -- so skipping step 3 breaks the build. Step 3
runs `tsc --noEmit` over the generated file, which is what actually validates
the hand-written interfaces against the real Go output; it needs `npm install`
in this directory first.

