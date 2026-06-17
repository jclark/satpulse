# SatPulse Desktop GUI

Experimental desktop GUI for GPS receiver configuration, built with [Wails](https://wails.io/).

## Prerequisites

All platforms need:

- Go 1.25+
- Node.js and npm (for the frontend build)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation):
  `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
  (ensure `$(go env GOPATH)/bin` is on your `PATH` so the `wails` command is found)

Then the platform-specific webview toolkit:

### macOS

- Xcode command line tools: `xcode-select --install`

### Linux (GTK / WebKitGTK)

Install GTK 3 and the WebKitGTK 4.1 (libsoup3) development headers, the variant
shipped by all current releases (Ubuntu 24.04+, Debian trixie+, Fedora 40+).

Debian, Ubuntu, Raspberry Pi OS:

```
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
```

Fedora:

```
sudo dnf install gtk3-devel webkit2gtk4.1-devel
```

The `Makefile` targets WebKitGTK 4.1 on Linux by default, so plain `make` works.
On an older 4.0-only distro, build with `make WEBKIT=4.0`. Run `wails doctor` to
check your system (it may report the 4.0 package as missing -- expected and
harmless when targeting 4.1).

### Windows

- WebView2 runtime (preinstalled on Windows 11; the Wails installer pulls it
  in otherwise). No extra build tag is needed.

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

- macOS: `open build/bin/SatPulse.app`
- Linux: `./build/bin/SatPulse`
- Windows: `build\bin\SatPulse.exe`

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
