# webui

The frontend workspace. Its packages are built by vite into `dist` directories
that are checked in and compiled into the Go binaries with `//go:embed`:

- `packages/dashboard` is the `satpulsed` web UI, embedded via
  `time/internal/web/embed.go`.
- `packages/workbench` is a component library with no standalone app. It is
  consumed by the entry package `packages/workbench-http`, which is the
  `satpulsewb` UI, embedded via `cmd/satpulsewb/embed.go`.

Nothing else depends on `packages/workbench`, and `packages/dashboard` does not
depend on it, so the two frontends regenerate independently.

## Regenerating the embedded assets

After changing any source file under `packages/dashboard/`, run from the repo
root:

    go generate ./time/internal/web

After changing any source file under `packages/workbench/` or
`packages/workbench-http/`, run:

    go generate ./cmd/satpulsewb

Commit the regenerated `dist/` in the same change as the source edit. `make`
does not run `go generate`, so nothing catches a stale `dist/`: the binary
silently keeps serving the old frontend.

Both builds need the workspace dependencies installed once:

    npm --prefix webui ci

Content hashing is disabled in both vite configs, so the output filenames are
stable and regenerating without a source change produces no diff.

Regeneration is not needed for edits to markdown under `plan/`.

Changes to `gps/ts` need it for the workbench only. `packages/workbench` imports
the ConfigTarget name vocabularies from `@satpulse/gps/configtarget` as values,
so they are compiled into its bundle. `packages/dashboard` imports
`@satpulse/gps` only via `import type`, which vite erases, so those types never
reach its bundle.
