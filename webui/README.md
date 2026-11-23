# Web UI workspace

npm workspace for the satpulse web frontend. One package for now:

- `packages/dashboard` (`@satpulse/dashboard`) -- the satpulsed web dashboard,
  built with Vite.

GPS wire types come from `@satpulse/gps` (`../gps/ts`).

## Setup

Install [Node.js](https://github.com/nodesource/distributions) (v24), then from
this directory:

    npm ci

## Development

    npm run dev        # dashboard dev server (mock data at /test-dashboard.html)
    npm test           # run all package tests (vitest)
    npm run typecheck  # type-check all packages

`npm run dev` serves the real dashboard at `/` (needs a satpulsed SSE feed) and a
mock dashboard at `/test-dashboard.html`. The mock accepts query parameters:
`?g=GE` shows only satellites whose SVID starts with G or E, and `?angles=0`
simulates a receiver reporting signal strength but no look angles.

## Embedding into satpulsed

    go generate ./time/internal/web

runs `npm run embed`, which builds the dashboard into `time/internal/web/dist`.
The built assets are checked in, so `go build` never needs npm.
