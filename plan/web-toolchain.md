# Web UI toolchain reorganisation (#283)

Separate frontend source from Go embed machinery, switch from esbuild to Vite, and set up an npm workspace that enables component sharing with the desktop GUI.

No user-visible behaviour change. The web dashboard looks and works identically after this work.

## Current state

The `web/` directory mixes frontend source (Preact/TypeScript/Tailwind), the esbuild build script, built output (`app.js`, `style.css`), and Go embed code (`embed.go`). The desktop GUI in `satpulse-desktop/desktop/frontend/` independently develops overlapping components (sky view, time formatting, survey display) with no shared code.

`gps/ts/` already exists as the `@satpulse/gps` npm package. It contains TypeScript type definitions for the JSON wire format of `gpsprot`, `ptime`, and `gpsio` Go types, validated against actual Go JSON output via `go generate`. It owns the `typescript` devDependency.

## New structure

```
webui/                              # npm workspace root
  package.json                      # workspace config
  packages/
    shared/                         # shared components, tokens, utilities
      package.json
      src/
        components/                 # (initially empty, populated by design-system.md)
        viz/                        # SkyView, SignalGraph
        timefmt.ts                  # time formatting utilities
    dashboard/                      # web dashboard app
      package.json
      src/
        app.tsx                     # entry point
        dashboard.tsx               # SSE transport + layout
      index.html
      vite.config.ts

internal/web/                       # Go embed package
  embed.go                          # //go:generate npm ... run embed
  content/                          # checked-in built artifacts
    index.html
    app.js
    style.css
```

## Migration from `web/`

| Current `web/` file | Destination |
|---|---|
| `svg.tsx` (SkyView, SignalGraph, simplifySignals) | `webui/packages/shared/src/viz/` |
| `timefmt.ts` | `webui/packages/shared/src/timefmt.ts` |
| `dashboard.tsx` | `webui/packages/dashboard/src/` |
| `app.tsx` | `webui/packages/dashboard/src/` |
| `input.css`, `style.css` | Split: tokens to shared, dashboard-specific to dashboard |
| `index.html` | `webui/packages/dashboard/` |
| `build.mjs` | Remove; replaced by Vite build in dashboard package |
| `embed.go` | `internal/web/embed.go` |
| `test-dashboard.tsx`, `build-test-dashboard.mjs` | `webui/packages/dashboard/src/` (mock dev server) |
| `dashboard.test.ts`, `timefmt.test.ts` | Move alongside their sources (dashboard test to dashboard package, timefmt test to shared package), migrated jest -> vitest |

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

Each package runs its own tests: `timefmt` and `viz` tests live in the shared
package, `dashboard.test.ts` in the dashboard package. The workspace root
exposes an aggregate `test` script.

## Go embed bridge

```go
//go:generate npm --prefix ../../webui run embed
//go:embed content
var content embed.FS
```

The workspace `package.json` defines the `embed` script:

```json
"embed": "npm run build --workspace=packages/dashboard -- --outDir ../../internal/web/content --emptyOutDir"
```

Built assets remain checked in so `go build` works without npm. Existing Go code importing the `web` package changes to import `internal/web`.

## npm workspace

The workspace root `webui/package.json` declares both packages. The shared package is source-only (no build step). The dashboard package has a Vite build that bundles shared components.

The desktop GUI's `desktop/frontend/package.json` references the shared package via a relative path dependency: `"@satpulse/shared": "file:../../webui/packages/shared"`.

## TypeScript and `@satpulse/gps`

`gps/ts/` is the canonical home for `typescript` as a devDependency. The workspace root `webui/package.json` depends on `@satpulse/gps` via a `file:` dependency:

```json
"dependencies": {
  "@satpulse/gps": "file:../gps/ts"
}
```

This makes GPS type imports available throughout the workspace (`import type { SatellitesMsg } from '@satpulse/gps/gpsprot'`) and provides `tsc` via `gps/ts/node_modules`. The workspace does not need its own `typescript` devDependency.

## Devcontainer

- **Dockerfile**: cache npm dependencies the same way Go modules are cached today. Copy lockfile and workspace package.json files and run `npm ci` during image build.
- **postCreateCommand**: change from `npm install --prefix web` to `npm ci --prefix webui`.
- Use `npm ci` (not `npm install`) everywhere.

## Verify

- `npm run dev` in the dashboard package serves a working dev server
- `go generate ./internal/web` produces the embedded assets
- `go build` works without npm (uses checked-in assets)
- Web dashboard looks and works identically
