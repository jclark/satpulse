# Web UI toolchain reorganisation

Separate frontend source from Go embed machinery, switch from esbuild to Vite, and set up an npm workspace that enables component sharing with the desktop GUI.

No user-visible behaviour change. The web dashboard looks and works identically after this work.

## Current state

The `web/` directory mixes frontend source (Preact/TypeScript/Tailwind), the esbuild build script, built output (`app.js`, `style.css`), and Go embed code (`embed.go`). The desktop GUI in `satpulse-desktop/desktop/frontend/` independently develops overlapping components (sky view, time formatting, survey display) with no shared code.

## New structure

```
webui/                              # npm workspace root
  package.json                      # workspace config
  packages/
    shared/                         # shared components, tokens, types
      package.json
      src/
        components/                 # (initially empty, populated by design-system.md)
        viz/                        # SkyView, SignalGraph
        types/                      # TypeScript interfaces matching gpsprot JSON
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

## Vite build

The dashboard package uses Vite with the Preact preset (matching the desktop GUI's setup). The Vite config disables content hashing so filenames are stable (`app.js`, `style.css`). This keeps git diffs clean since the checked-in assets only change when actual content changes.

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

## Devcontainer

- **Dockerfile**: cache npm dependencies the same way Go modules are cached today. Copy lockfile and workspace package.json files and run `npm ci` during image build.
- **postCreateCommand**: change from `npm install --prefix web` to `npm ci --prefix webui`.
- Use `npm ci` (not `npm install`) everywhere.

## Verify

- `npm run dev` in the dashboard package serves a working dev server
- `go generate ./internal/web` produces the embedded assets
- `go build` works without npm (uses checked-in assets)
- Web dashboard looks and works identically
