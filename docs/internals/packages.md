---
title: Packages
redirect_from:
  - /internals.html
---

## Go packages

The Go packages are organized into three main hierarchies: `gps/` for GPS processing (no external dependencies), `time/` for time synchronization (depends on `gps/`), and `internal/` for satpulsetool subcommands (depends on both).

Within each hierarchy, packages are organized into layers where each layer depends only on packages in the same or lower layers.

**Command-line layer** provides the user interface to the programs, including the command-line interface and the configuration file.

**Application layer** provides the main blocks of the applications.

**Domain layer** provides packages using domain-specific abstractions that are used by the application layer. These packages have mutual dependencies. They do not make use of goroutines nor do they perform logging.

**Library layer** provides a library of packages, which are potentially useful outside satpulse. There are few mutual dependencies. These packages do not make use of goroutines nor do they perform logging.

The following sections describe each directory that contains packages.

### cmd/

These packages provide entry points for the SatPulse executables. They are in the command-line layer.

`cmd/satpulsed` provides main for satpulsed.

`cmd/satpulsetool` provides main for satpulsetool.

`cmd/satpulsewb` provides main for satpulsewb, which serves SatPulse Workbench, the browser GUI for interactive GPS receiver configuration and monitoring. It adapts `gps/app/session` to HTTP: session methods become POST endpoints, snapshots GET endpoints, and session events an SSE stream, with a latest-event-per-name cache priming late-joining clients and the high-rate packet stream gated on a subscriber being connected. A per-run token guards the API and event stream, while a newest-window-wins write seat limits mutating operations to one browser window at a time: the per-claim seat value is carried on writer POSTs and broadcast as a sticky `writer` SSE event, so every other window is a live read-only viewer and no stream is ever terminated. Message files are chosen from the library (`msgfile.go`): the catalog endpoint lists the names `msgfile.ListNames` finds in `SATPULSE_GPSMSG_PATH` directories followed by the built-in library, preselecting the session vendor; the select endpoint resolves a name with `msgfile.FindName`, whose validation is the traversal guard, and loads it through the directory's `Load` method. The frontend is the Vite build output of `webui/packages/workbench-http`, embedded as checked-in assets under `dist/` (rebuilt by go generate, like `time/internal/web`).

`cmd/ifwait` provides a program that waits for a network interface to become ready. It exercises the functionality of the `time/lib/ifwait` package.

### gps/

These packages provide the public API for GPS processing. They are in the domain layer.

`gps/ptime` provides a Time type that represents time in the PTP timescale (nanoseconds in TAI timescale since 1970-01-01T00:00:00 TAI). This is used throughout the domain layer and higher level layers.

`gps/scan` provides a Packet type representing a packet of GPS data in some protocol. It also provides a scan function to split up a stream of bytes into packets. It does not interpret the content of the packet.

`gps/gpsprot` abstracts a GPS protocol such as UBX or NMEA. This operates in two phases: configuration and post-configuration. The post-configuration part of this defines types that represent the information contained in GPS messages in a protocol-independent way.

`gps/gpsreg` provides a registry for the various implementations of the `gps/gpsprot` layer. Higher layers avoid interacting with the implementations for specific protocols as much as possible. Generally the command-line layer interacts with `gps/gpsreg`, and passes the appropriate implementations into lower layers.

`gps/gpsdecode` decodes binary GPS packets into JSON-serializable maps derived from the Go structs defined in the library layer packages.

`gps/nmeasyn` synthesizes NMEA sentences from gpsprot messages.

`gps/msgfile` parses TOML message files that describe GPS messages to send to a receiver. It handles multiple protocol types (UBX, CASBIN, ASBIN, NMEA, line, binary), applies per-type defaults, and converts typed messages into raw bytes ready to send. Messages are organized by tags for selective sending. It also implements the message-file library search path over `fs.FS` directories: a `Name` (vendor directory plus file name) resolves to the first `vendor/file.toml` along a directory list (`FindName`, `ListNames`, and `EnvDirs` for `SATPULSE_GPSMSG_PATH`). The default library is embedded as a deterministic compressed zip and exposed by `Builtin`. Each `Dir` has a `Load` method that reads a file and resolves its includes: an on-disk directory loads through the path-based `Load`, so includes resolve natively and are not confined to the directory (as with satpulsetool); the embedded zip resolves them within the archive via `LoadFS`.

`gps/ts` generates TypeScript type definitions for the JSON values serialized from types in the `gps/*` packages.

### gps/app/

These packages provide GPS orchestration and CLI infrastructure. They are in the application layer.

`gps/app/gpsio` implements a goroutine that reads input from the GPS, splits it into packets and sends those packets to a channel. It provides an abstraction for performing IO on a GPS, which can work either over a serial connection (using `gps/lib/term`) or over a network connection. The input is split into packets using the `gps/scan` package.

`gps/app/gpscfg` drives the GPS configuration process. It combines `gps/app/gpsio` and `gps/gpsprot`.

`gps/app/cmd` provides some common functionality used for command-line interfaces.

`gps/app/logfile` provides utility functions for opening and reopening log files.

`gps/app/bcast` provides a concurrency abstraction that broadcasts a channel to multiple other channels. This is used for routing packets inside the application. At the moment it is used by `satpulsed` rather than `satpulsetool`, but it is useful for applications dealing with GPS packets.

`gps/app/stream` manages correction and packet streams. It pulls RTCM streams from an Ntrip or TCP network endpoint and feeds them to the GPS receiver over the serial port, pushes streams from the GPS receiver to a remote Ntrip network endpoint, and provides selected-GGA helpers for consumers that need current receiver position.

`gps/app/ntrip` implements an Ntrip caster for serving RTCM packet streams from a GPS receiver to Ntrip clients. It includes an STR record generation capability, which is also used by `gps/app/stream`.

`gps/app/session` implements an interactive session with a GPS receiver -- connect, probe, configure, send message files, monitor, disconnect -- as the application core shared by GUI shells (the Wails desktop app, `cmd/satpulsewb`). It owns the packet pipeline goroutines, delivers events to the shell through a `Sink` interface, opens its transport through an `Opener` (serial device or a running satpulsed's proxy socket, with reset operations gated off over the proxy), and reconnects and re-probes when a reset re-enumerates a USB device. It was extracted from the desktop app's `app.go`.

`gps/app/ubxsim` implements a hardware-free fake u-blox receiver for smoke-testing configuration wiring. It answers the configuration interface with the ACK/NAK semantics of the interface description and replays a recorded packet log as nav output gated by its own MSGOUT configuration.

### gps/internal/

These packages implement the `gpsprot` interface for specific protocols. They are in the domain layer and are not importable outside `gps/`.

`gps/internal/ubx` implements `gps/gpsprot` abstractions for the UBX protocol. It uses `gps/lib/ubxbin` and `gps/lib/ubxcfgval` to do this.

`gps/internal/nmea` implements `gps/gpsprot` abstractions for the NMEA protocol.

`gps/internal/rtcm` implements `gps/gpsprot` abstractions for the RTCM protocol. It uses `gps/lib/rtcmbin` for field extraction.

`gps/internal/spartn` implements `gps/gpsprot` abstractions for the SPARTN protocol. It uses `gps/lib/spartnbin` for field extraction.

`gps/internal/septentrio` implements `gps/gpsprot` abstractions for the Septentrio SBF protocol. It uses `gps/lib/sbfbin` to frame and parse SBF blocks.

`gps/internal/casic` implements `gps/gpsprot` abstractions for the CASIC binary protocol. It uses `gps/lib/casbin` to do this.

`gps/internal/unc` implements `gps/gpsprot` abstractions for the Unicore protocol. It uses `gps/lib/uncmsg` to parse Unicore binary and ASCII message formats.

`gps/internal/nov` implements `gps/gpsprot` abstractions for the NovAtel protocol. It uses `gps/lib/novmsg` to parse binary and ASCII NovAtel packets.

`gps/internal/sino` provides satellite numbering schemes for SinoGNSS receivers, defining NMEA satellite ID mappings for GLONASS, NavIC, Galileo, QZSS, BeiDou, and SBAS.

`gps/internal/as` provides NMEA satellite numbering configuration for Allystar GPS receivers.

`gps/internal/quectel` converts PQTM NMEA messages from Quectel GPS receivers into `gps/gpsprot` message format.

`gps/internal/scantest` provides utility functions for testing GPS packet format implementations. It includes functions to find packets within buffers and insert random data prefixes for robustness testing.

### gps/lib/

These packages are reusable libraries for GPS processing. They are in the library layer.

`gps/lib/ubxbin` translates binary packets in the UBX protocol to and from Go structs.

`gps/lib/ubxcfgval` handles the 9th generation UBX format for configuration data that is payload for UBX-CFG-VALGET/VALSET messages.

`gps/lib/ubxcfgval/cfgschema` contains a YAML schema for configuration data handled by `gps/lib/ubxcfgval`. This is used to generate code in the `gps/lib/ubxcfgval` package.

`gps/lib/casbin` translates binary packets in the CASIC protocol to and from Go structs.

`gps/lib/asbin` translates binary packets in the Allystar binary protocol to and from Go structs.

`gps/lib/bitsenc` provides reflection-based decoding of bit-packed binary data into Go structs. It supports unsigned and signed integers, bools, and embedded structs.

`gps/lib/rtcmbin` parses and serializes RTCM binary packets using `gps/lib/bitsenc`, including message types 1005/1006, 1230, and MSM. It also provides MSM7-to-MSM4 conversion.

`gps/lib/spartnbin` parses the SPARTN transport frame envelope and computes its CRCs using `gps/lib/bitsenc`, returning the (possibly encrypted) message payload as opaque bytes.

`gps/lib/sbfbin` translates binary blocks in the Septentrio SBF protocol to and from Go structs.

`gps/lib/rinex` defines an intermediate, RINEX-adjacent representation of observation data as JSON-serializable Go types, and reads and writes it as RINEX observation files.

`gps/lib/rnxrtcm` converts RTCM MSM7 observation messages to `gps/lib/rinex` records. It uses `gps/lib/rtcmbin` to decode the source messages.

`gps/lib/rnxubx` converts u-blox raw observation messages to `gps/lib/rinex` records. It uses `gps/lib/ubxbin` to decode the source messages.

`gps/lib/rnxunc` converts Unicore raw observation messages to `gps/lib/rinex` records. It uses `gps/lib/uncmsg` to decode the source messages.

`gps/lib/novmsg` provides parsing and serialization of NovAtel GPS receiver messages in binary and ASCII formats. It defines message header and body types and implements CRC32 validation.

`gps/lib/uncmsg` parses Unicore protocol messages in binary and ASCII formats. It defines message structures and provides parsing/serialization using `gps/lib/novmsg`.

`gps/lib/nmeamsg` analyzes NMEA sentence syntax, computes checksums, and decodes and serializes typed approved GNSS-talker sentences such as GGA and RMC.

`gps/lib/airmsg` classifies responses to Airoha proprietary PAIR NMEA commands.

`gps/lib/qtmmsg` parses Quectel NMEA PQTM messages.

`gps/lib/opt` provides a generic optional value type for use in serialized structs.

`gps/lib/geopos` converts between positions in the ECEF and LLH geodetic coordinate systems. This is used by the web interface to link to Google maps.

`gps/lib/fieldenc` provides reflection-based encoding and decoding of Go structs to and from ordered string field arrays. It supports standard types and custom TextMarshaler/TextUnmarshaler implementations.

`gps/lib/ntptime` reads the Linux kernel's NTP synchronization state via the `adjtimex` syscall and exposes it as platform-independent types. It provides information about system clock synchronization and leap second status.

`gps/lib/term` provides access to the Linux terminal interface, which provides access to serial devices. This is similar to [github.com/pkg/term](https://github.com/pkg/term), but provides additional Linux-specific functionality.

`gps/lib/serialenum` enumerates serial ports with human-readable display names, for the device dropdown in GUI shells (satpulsewb, the desktop app). It uses go.bug.st/serial/enumerator; satpulsed and satpulsetool must not import it, so their Linux builds stay CGO-free.

`gps/lib/decconv` converts between base-10 decimal strings and int64 scaled integers without floating point. It is used for exact parsing and formatting of physical quantities like angles and lengths.

`gps/lib/latin1z` provides fixed-size byte-array types (`StringZ5`, `StringZ10`, ...) for nul-terminated ISO 8859-1 (Latin-1) strings, with JSON serialization. The typed arrays are generated by `mksizes.go`.

`gps/lib/ascii` provides ctypes-style classification and conversion of ASCII bytes (the C `<ctype.h>` repertoire restricted to ASCII), using lookup tables built in `init()`. It supersedes the ad-hoc digit/hex/alpha/printable helpers that were scattered across the GPS protocol parsers.

### time/

These packages provide the public API for time synchronization. They are in the domain layer.

`time/phc` provides low level abstractions to access the PTP hardware clock. It is highly Linux dependent. It uses `gps/ptime`.

`time/sockrefclock` implements the chrony refclock protocol. It uses `gps/ptime`.

`time/lib/ntpshm` implements the ntpd/NTPsec SHM refclock writer. It uses `gps/ptime`.

`time/clocksim` provides discrete-time simulation of PTP hardware clocks and GNSS PPS signals. It includes simulator functions for oscillators (modeling frequency errors like white noise, flicker noise, random walk, drift) and for GPS/PPS timing errors (jitter, sawtooth, sinusoids, colored noise).

`time/phctime` provides an `Era` type used for managing stepping of a PHC and types that combine `Era` with `ptime.Time`.

### time/app/

These packages provide daemon orchestration and CLI. They are in the command layer.

`time/app/daemon` implements the satpulsed daemon. It orchestrates all the parts of the satpulsed program. It also handles the TOML config file and provides HTTP endpoints for the web interface and metrics.

`time/app/syncsimcmd` implements the `syncsim` subcommand of satpulsetool. It parses configuration and command-line arguments, then orchestrates a discrete-event simulation of the synchronization system using `time/internal/syncsim`.

### time/internal/

These packages are the main building blocks for satpulsed; they are in the application layer and are not importable outside `time/`.

`time/internal/ts` implements a goroutine that reads external timestamps from the PTP hardware clock and sends those to a channel. These external timestamps are the time pulses emitted by the GPS receiver.

`time/internal/gpsevent` provides the main event handling loop after GPS configuration is done. It receives GPS packets from `gps/app/gpsio` and then uses the appropriate protocol implementation to construct protocol-independent messages that it passes to `time/internal/phcsync`. It also receives timestamps from `time/internal/ts` and passes them to `time/internal/phcsync`.

`time/internal/phcsync` provides the core functionality of synchronizing the PTP hardware clock. It receives timestamps from gpsevent and accesses GPS messages via `time/internal/timemsg`.

`time/internal/timemsg` buffers recent GPS time messages from a receiver and provides methods to retrieve consecutive, second-aligned messages and pulse offset corrections for time synchronization. It receives messages from `time/internal/gpsevent` and provides them to `time/internal/phcsync`. It isolates `time/internal/phcsync` from the complexities of `gps/gpsprot`.

`time/internal/ptpgm` manages the PTP grandmaster state and synchronization status as seen by ptp4l. It provides a worker goroutine that sends updates to ptp4l via the PTP management protocol.

`time/internal/refclock` provides abstractions for sending clock synchronization samples to external time synchronization services like chrony. It includes a worker goroutine that processes samples from a channel and delivers them to configured refclock implementations.

`time/internal/syncsim` provides a discrete-event simulator for testing the phcsync controller with configurable error models for GPS PPS timing and PHC oscillator characteristics. It generates synthetic pulses, messages, and ticks under various fault conditions and measures controller performance.

`time/internal/obs` provides unified observability interfaces including `Observer` (which extends `phcsync.Sampler` and `gpsprot.Handler`) for receiving both clock synchronization samples and GPS protocol messages.

`time/internal/sseobs` implements the `Observer` interface to generate Server-Sent Events data that the daemon uses for the web interface.

`time/internal/promobs` implements the `Observer` interface to collect Prometheus metrics that the daemon exposes via HTTP handlers.

`time/internal/logobs` implements the `Observer` interface to record clock synchronization samples to log files and emit statistical summaries via structured logging. It provides `ClockLogObserver` for per-sample clock data and `StatsLogObserver` for interval-based summaries.

`time/internal/statsobs` accumulates clock synchronization statistics including phase offset (maximum, mean, RMS), frequency deviation (mean, standard deviation), and frequency delta characteristics. It implements `phcsync.Sampler` for data collection.

`time/internal/proxy` implements proxying of GPS packets to TCP and Unix domain sockets.

`time/internal/web` embeds the built web interface assets (index.html, app.js, style.css) that satpulsed serves. The assets are generated by `go generate ./time/internal/web`, which runs the Vite build in the `webui/` npm workspace via `npm run embed`; they are checked in so `go build` never needs npm.

### time/lib/

These packages are reusable libraries for time synchronization. They are in the library layer.

`time/lib/pmc` implements a PTP management client.

`time/lib/ifwait` uses the Linux kernel's netlink subsystem to wait for changes in a network interface's status.

`time/lib/devnotify` uses the Linux kernel's netlink subsystem to listen for creation of new devices by udev. (This is not used currently.)

`time/lib/sse` marshals data into the format of HTML SSE (server-sent events).

`time/lib/allan` computes Allan deviations. (This is not used currently.)

`time/lib/check` validates struct fields against constraints specified in struct tags using reflection. It supports numeric types with comparison operators (`>`, `>=`, `<`, `<=`) and recursively validates nested structs.

`time/lib/circbuf` provides a generic circular buffer that maintains a sliding window of recent samples. It supports appending with automatic overflow handling and reverse chronological iteration.

`time/lib/fuser` finds processes that have a specific file or directory open by examining the Linux /proc filesystem. It provides functionality similar to the Unix fuser utility.

`time/lib/median` provides efficient median computation for a fixed-size moving window using a circular buffer with a sorted index array. It supports 64-bit integers, floats, and time.Duration.

`time/lib/ntime` provides a domain-neutral nanosecond timestamp type for the refclock sample path. The domain of a value (UTC, TAI, PHC-raw) is determined by the producer and consumer, not by the type. It depends only on the standard library.

### internal/

These packages implement subcommands of satpulsetool. They are in the command-line layer and can import from both `gps/` and `time/`.

`internal/gpscmd` implements `gps` subcommand of satpulsetool.

`internal/annotatecmd` implements `annotate` subcommand of satpulsetool. It annotates JSONL packet logs with decoded payload fields (header, payload, cfgData).

`internal/convobscmd` implements `convobs` subcommand of satpulsetool. It converts raw and JSON observation streams.

`internal/decodecmd` implements `decode` subcommand of satpulsetool. It decodes a single GPS packet from hex or ASCII data into JSON.

`internal/ntripcmd` implements `ntrip` subcommand of satpulsetool. It fetches data from an Ntrip caster and writes either a JSONL packet log or raw bytes to stdout.

`internal/packcmd` implements `pack` subcommand of satpulsetool. It reads a JSONL packet log and writes selected packets as a raw byte stream, optionally preserving inter-packet timing.

`internal/scancmd` implements `scan` subcommand of satpulsetool. It reads a raw packet byte stream, splits it using the GPS packet scanner, and writes a JSONL packet log.

`internal/ubxsimcmd` implements the `ubxsim` subcommand of satpulsetool (Linux and macOS). It hosts the u-blox receiver simulator (`gps/app/ubxsim`) behind a pty for black-box testing without GPS hardware.

`internal/pmccmd` implements `pmc` subcommand of satpulsetool.

`internal/replaycmd` implements `replay` subcommand of satpulsetool. It replays a JSONL packet log, generating JSONL events similar to an event log.

`internal/sdpcmd` implements the `sdp` subcommand of satpulsetool. It provides interfaces to manage software-defined pins (SDPs) on PTP hardware clocks, including listing available interfaces and pins, capturing external timestamps, configuring periodic output, and disabling pins.

## npm packages

These hold the web frontend source. They are not Go packages: they are built with npm, and the build output is embedded into the Go binaries (see `time/internal/web`). GPS JSON wire types are published from `gps/ts` as `@satpulse/gps` and consumed here rather than redefined.

### webui/

`webui/` is the npm workspace holding the web frontend source (TypeScript, Preact, Tailwind). Three of its packages are bundled by Vite with content hashing disabled so the embedded filenames stay `app.js` and `style.css`. `@satpulse/dashboard` (`packages/dashboard`) is the satpulsed web dashboard app. `@satpulse/workbench` (`packages/workbench`) holds the SatPulse Workbench components and app, originally the desktop GUI frontend; its `src/transport.ts` defines the transport interface the components talk to their backend through -- a universal core plus optional connection-management and message-file capabilities. `@satpulse/workbench-http` (`packages/workbench-http`) is the satpulsewb entry point: token handling plus the fetch+SSE transport implementation; its build output is embedded by `cmd/satpulsewb`. `@satpulse/e2e` (`packages/e2e`) is the Playwright browser-test suite for the two embedded frontends; it builds nothing, launching the real `satpulsed`/`satpulsewb` binaries from `out/<arch>` and driving them with the smoketest's hardware-free packet sources (a FIFO packet-log replay, the `satpulsetool ubxsim` simulator). Its `harness.ts` provides the launch fixtures, and its two Playwright projects (`dashboard`, `workbench`) mirror the two frontends; it has no `test` script, so `npm test` does not run it (the runner is `npm run e2e`). The workspace imports GPS wire types from `@satpulse/gps` (`gps/ts`).

## Python packages

These hold the test harnesses that drive the built binaries from outside. They are Python 3.11 or later and use only the standard library at runtime; each has a `pyproject.toml` that declares no dependencies and exists to pin mypy strict under uv (`make typecheck`). Neither directory is installed: they are run in place from the repository root.

### gpshwtest/

`gpshwtest` tests GPS high-level configuration against real receivers, and is run as a directory (`python3 gpshwtest`) through its `__main__.py`. Because receivers are diverse and high-level configuration has best-effort semantics, it characterizes how device-independent configuration is realized on each receiver rather than rendering pass/fail verdicts; vetted characterizations are checked in under `baselines/` and compared on later runs. `probes.py` drives the receiver, `model.py` holds the configuration model under test, and `characterize.py` and `analyze.py` derive the two offline outputs, failures and characterization. All receiver I/O goes through `satpulsetool gps --json`. The goal and success criteria are defined in `gpshwtest/GOAL.md` (#310), the semantics under test in `SEMANTICS.md`.

### smoketest/

`smoketest` runs black-box scenarios against the real `satpulsed` and `satpulsewb` binaries, with no root and no GPS hardware, and is entered through `run.py`. It exercises program behaviour -- configuration wiring, startup, observability endpoints, logging, Ntrip, corrections, and shutdown -- not packet decoding. `run.py` is the engine, and three seams keep it independent of what it tests and where: the packet provider (`provider_api.py`, with `provider_replay.py` streaming a recorded packet log and `provider_ubxsim.py` running the simulator), the program under test (`program_api.py`, with `program_satpulsed.py` and `program_satpulsewb.py`), and the per-OS transport primitives (`platform_api.py`, with `platform_unix.py` today). `common.py` holds the shared checks, `ntpshm.py` and `ntpsock.py` stand in for the NTP consumers of the SHM and SOCK refclock protocols, and `system.py` is a separate runner for the package-installed systemd service. The scenarios themselves are the one real package here: `scenarios/` and a subpackage per group (basic, config, http, logging, ntp, ntrip, proxy, shutdown, stream).
