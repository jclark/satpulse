# webui

The frontend workspace. Its entry packages are built by vite into `dist`
directories. Native frontend assets are checked in and compiled into the Go
binaries with `//go:embed`:

- `packages/dashboard` is the `satpulsed` web UI, embedded via
  `time/internal/web/embed.go`.
- `packages/workbench` is a component library with no standalone app. It is
  consumed by `packages/workbench-http` and `packages/workbench-wasm`.
  The HTTP entry package is the `satpulsewb` UI, embedded via
  `cmd/satpulsewb/embed.go`.
- `packages/workbench-wasm` is the browser-only entry package. Its generated
  assets are ignored under `cmd/satpulsewbwasm/dist`; `Makefile.wasm` assembles them with
  the wasm module and matching Go runtime support into a static site.

`packages/dashboard` does not depend on `packages/workbench`, so it regenerates
independently of the two Workbench frontends.

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

After changing `packages/workbench/` or `packages/workbench-wasm/`, regenerate
the standalone browser assets too:

    npm --prefix webui run embed-workbench-wasm

Use `make -f Makefile.wasm` to build the complete browser site, and `make -f
Makefile.wasm test-browser` to test it against the hardware-free simulator. The optional cold-start
benchmark is `make -f Makefile.wasm benchmark`; it is excluded from normal e2e.
`test-assets` prepares the browser tests without running them, and `test-go`
runs the shell tests under Node.

For a full Playwright run, prepare both native binaries and WASM test assets
from the repo root:

    make
    make -f Makefile.wasm test-assets
    npm --prefix webui/packages/e2e run e2e

The suite builds nothing itself. The WASM project needs the browser site in
`out/workbench-wasm` and the simulator at `out/wasm-test/wasmsim`.

Frontend builds need the workspace dependencies installed once:

    npm --prefix webui ci

Content hashing is disabled in the entry package vite configs, so output filenames are
stable and regenerating without a source change produces no diff.

Regeneration is not needed for edits to markdown under `plan/`.

## Live development with HMR (workbench)

To develop `packages/workbench` / `packages/workbench-http` with hot module
reloading instead of regenerating and restarting, serve the assets from vite's
dev server and proxy the API and event stream to a running `satpulsewb`. The
frontend talks to it over relative paths (`/api`, `/sse`), and the vite config
proxies those to a fixed loopback port.

Start satpulsewb on that port with the token disabled:

    satpulsewb --listen 127.0.0.1:15754 --no-open-browser -d <device> -s <speed>

Then run the dev server and open the URL it prints (http://localhost:5173):

    npm --prefix webui/packages/workbench-http run dev

Assets hot-reload from vite; events come from the live session. This is
dev-only: the proxy lives in `server.proxy` in the vite config, which `vite
build` ignores, so the embedded `dist/` is unaffected.

Because the token is disabled and the port is fixed, an agent can drive this
with Playwright by navigating to http://localhost:5173 -- no token to thread
through and no browser auto-opened (`--no-open-browser`).

Changes to `gps/ts` need it for the workbench only. `packages/workbench` imports
the ConfigTarget name vocabularies from `@satpulse/gps/configtarget` as values,
so they are compiled into its bundle. `packages/dashboard` imports
`@satpulse/gps` only via `import type`, which vite erases, so those types never
reach its bundle.
