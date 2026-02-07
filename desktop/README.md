# SatPulse Desktop GUI

Experimental desktop GUI for GPS receiver configuration, built with [Wails](https://wails.io/).

## Prerequisites

- Go 1.25+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation):
  `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Xcode command line tools: `xcode-select --install`

## Setup

After cloning the repo, create a `go.work` file in the repository root:

```
make setup
```

This creates a Go workspace that includes both the main satpulse module and
the desktop module, allowing the `replace` directive in `desktop/go.mod` to
resolve correctly.

## Build

```
cd desktop
make
```

Run the application:

```
open build/bin/satpulse-gps.app
```

## Development

For live reload during development:

```
cd desktop
make dev
```

## Bugs

Packet capture only works immediately after connecting. If you click Detect
(or any other config operation) before starting capture, the capture will
fail silently. See TODO.md for the architectural fix.
