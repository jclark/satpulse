# Web UI toolchain reorganisation (#283)

Separate frontend source from Go embed machinery, switch from esbuild to Vite, and set up an npm workspace to hold the frontend source. The workspace is the structure later work builds on -- satpulseweb.md imports the desktop frontend into it, and web-redesign.md (#284) extracts the shared components package -- but this change does not itself extract a shared package: the web dashboard is its only consumer today, so its components stay in the dashboard package until a second consumer exists.

No user-visible behaviour change. The web dashboard looks and works identically after this work.

## Current state

The `web/` directory mixes frontend source (Preact/TypeScript/Tailwind), the esbuild build script, built output (`app.js`, `style.css`), and Go embed code (`embed.go`). The desktop GUI in `satpulse-desktop/desktop/frontend/` independently develops overlapping components (sky view, time formatting, survey display) with no shared code.

`gps/ts/` already exists as the `@satpulse/gps` npm package. It contains TypeScript type definitions for the JSON wire format of `gpsprot`, `ptime`, and `gpsio` Go types, validated against actual Go JSON output via `go generate`. It owns the `typescript` devDependency.

## New structure

```
webui/                              # npm workspace root
  package.json                      # workspace config
  packages/
    dashboard/                      # web dashboard app (only package for now)
      package.json
      src/
        app.tsx                     # entry point
        dashboard.tsx               # SSE transport + layout
        svg.tsx                     # SkyView, SignalGraph
        timefmt.ts                  # time formatting utilities
        index.css
      index.html
      vite.config.ts

time/internal/web/                  # Go embed package (satpulsed only)
  embed.go                          # //go:generate npm ... run embed
  dist/                             # checked-in Vite build output
    index.html
    app.js
    style.css
```

## Migration from `web/`

| Current `web/` file | Destination |
|---|---|
| `svg.tsx` (SkyView, SignalGraph, simplifySignals) | `webui/packages/dashboard/src/svg.tsx` |
| `timefmt.ts` | `webui/packages/dashboard/src/timefmt.ts` |
| `dashboard.tsx` | `webui/packages/dashboard/src/` |
| `app.tsx` | `webui/packages/dashboard/src/` |
| `input.css`, `style.css` | `webui/packages/dashboard/src/index.css` (Tailwind source; built output is regenerated) |
| `index.html` | `webui/packages/dashboard/` |
| `build.mjs` | Remove; replaced by Vite build in dashboard package |
| `embed.go` | `time/internal/web/embed.go` |
| `test-dashboard.tsx`, `build-test-dashboard.mjs` | `webui/packages/dashboard/src/` + `test-dashboard.html` (mock dev server) |
| `dashboard.test.ts`, `timefmt.test.ts` | `webui/packages/dashboard/src/`, migrated jest -> vitest |

GPS message types (`SatellitesMsg`, `NavEpochMsg`, etc.) are **not** migrated — they are already defined in `@satpulse/gps` (`gps/ts/`). Code that currently uses inline type definitions for gpsprot JSON should import from `@satpulse/gps/gpsprot` instead.

## Vite build

The dashboard package uses Vite with the Preact preset (matching the desktop GUI's setup). The Vite config disables content hashing so filenames are stable (`app.js`, `style.css`). This keeps git diffs clean since the checked-in assets only change when actual content changes.

## Test tooling

Replace jest/ts-jest/babel-jest with vitest, the natural test runner for a
Vite project (it reuses the Vite config and the Preact preset, so there is no
separate ts-jest/babel transform to maintain). This also drops the entire
babel/istanbul dependency chain, which was the source of the js-yaml and
@babel/core Dependabot alerts (worked around in `web/` with npm `overrides`
pending this migration; the overrides go away with jest).

Tests live alongside their sources in the dashboard package
(`dashboard.test.ts`, `timefmt.test.ts`), run by the package's own Vitest
config. The workspace root exposes an aggregate `test` script.

## Go embed bridge

The embed package lives at `time/internal/web`: its only consumer is satpulsed's daemon (`time/app/daemon`), so it belongs in the `time/` hierarchy at the application layer, alongside the other daemon web-support packages (`time/internal/sseobs`, `promobs`). Placing it under the repo-root `internal/` (which `internals.md` reserves for satpulsetool subcommands, a hierarchy that sits *above* `time/`) would invert the dependency direction. A future `cmd/satpulseweb` serves a different frontend and embeds its own assets, so it does not need to import this package.

```go
//go:generate npm --prefix ../../../webui run embed
//go:embed dist
var dist embed.FS
```

The workspace `package.json` defines the `embed` script. The build output goes to a `dist/` subdirectory (the conventional Vite name, matching the local `npm run build` output) rather than the package root, because `--emptyOutDir` wipes the target before writing and would otherwise delete `embed.go`:

```json
"embed": "npm run build --workspace=packages/dashboard -- --outDir ../../../time/internal/web/dist --emptyOutDir"
```

Built assets remain checked in so `go build` works without npm. Existing Go code importing the `web` package changes to import `time/internal/web`.

## npm workspace

The workspace root `webui/package.json` declares a single package for now, `packages/dashboard`, which has the Vite build. Keeping `webui/` a workspace (rather than a lone package) is deliberate: satpulseweb.md's next phase imports the desktop frontend as a second workspace package, so the multi-package structure is imminent.

No `shared` package is created here. Extracting SkyView/timefmt into a shared package earns nothing while the dashboard is their only consumer. Creating `webui/packages/shared` and moving the reusable components into it is deferred to [web-redesign.md](web-redesign.md) (#284), which unifies the dashboard's components with the desktop GUI's -- the point at which a second consumer actually exists.

## TypeScript and `@satpulse/gps`

The workspace root `webui/package.json` depends on `@satpulse/gps` via a `file:` dependency:

```json
"dependencies": {
  "@satpulse/gps": "file:../gps/ts"
}
```

This makes GPS type imports available throughout the workspace (`import type { SatellitesMsg } from '@satpulse/gps/gpsprot'`).

A `file:` dependency does not install its target's devDependencies into the consuming workspace, nor put their binaries on the workspace's PATH, so `@satpulse/gps` cannot supply `tsc` to `webui`. The workspace therefore pins `typescript` as its own devDependency in `webui/package.json`, so `npm run typecheck` resolves the in-repo `tsc` from `webui/node_modules/.bin` rather than a global install. `gps/ts/` keeps its own `typescript` devDependency for standalone type-checking of `@satpulse/gps`.

## Devcontainer

- **Dockerfile**: cache npm dependencies the same way Go modules are cached today. Copy lockfile and workspace package.json files and run `npm ci` during image build.
- **postCreateCommand**: change from `npm install --prefix web` to `npm ci --prefix webui`.
- Use `npm ci` (not `npm install`) everywhere.

## Verify

- `npm run dev` in the dashboard package serves a working dev server
- `go generate ./time/internal/web` produces the embedded assets
- `go build` works without npm (uses checked-in assets)
- Web dashboard looks and works identically
