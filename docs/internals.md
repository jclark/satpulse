---
title: SatPulse internals
---

## Go packages

The Go packages can be divided up into layers where each layer depends only on packages in the same or lower layers.
Each layer can also be divided into time sync, GPS and base groups, where time sync can depend on GPS, and time sync and GPS can depend on base.

|              | Time sync | GPS | Base |
|--------------|-----------|-----|--------|
| **Command**  | daemon, pmccmd, sdpcmd, syncsimcmd, cmd/ifwait | gpscmd, decodecmd | cmd |
| **Application** | ts, gpsevent, phcsync, timemsg, ptpgm, refclock, obs, sseobs, promobs, logobs, statsobs, syncsim, proxy, bcast | gpsio, gpscfg | logfile |
| **Domain** | phc, sockrefclock, clocksim, phctime | gpsprot, scan, scantest, ubx, nmea, rtcm, gpsreg, casic, unc, nov, sino, as, ptime, gpsdecode | |
| **Library** | pmc, circbuf, median, check, sse, ifwait, fuser, devnotify, allan | ubxbin, ubxcfgval, casbin, asbin, novmsg, uncmsg, nmeamsg, geopos, fieldenc | ntptime |

### Command-line layer

Provides the user interface to the programs, including the command-line interface and the configuration file.

#### Time sync

`cmd/satpulsetool` provides main for satpulsetool.

`cmd/satpulsed` provides main for satpulsed.

`internal/daemon` implements the satpulsed daemon. It orchestrates all the parts of the satpulsed program. It also handles the TOML config file and provides HTTP endpoints for the web interface and metrics.

`internal/pmccmd` implements `pmc` subcommand of satpulsetool.

`internal/syncsimcmd` implements the `syncsim` subcommand of satpulsetool. It parses configuration and command-line arguments, then orchestrates a discrete-event simulation of the synchronization system using `internal/syncsim`.

`internal/sdpcmd` implements the `sdp` subcommand of satpulsetool. It provides interfaces to manage software-defined pins (SDPs) on PTP hardware clocks, including listing available interfaces and pins, capturing external timestamps, configuring periodic output, and disabling pins.

`cmd/ifwait` provides a program that waits for a network interface to become ready. It exercises the functionality of the internal/ifwait package.

#### GPS

`internal/gpscmd` implements `gps` subcommand of satpulsetool.

`internal/decodecmd` implements `decode` subcommand of satpulsetool. It decodes binary GPS packets from hex strings or annotates JSONL packet logs with decoded payload fields.

#### Base

`internal/cmd` provides some common functionality used for command-line interfaces.

### Application layer

Provides the main blocks of the applications.

#### Time sync

`internal/ts` implements a goroutine that reads external timestamps from the PTP hardware clock and sends those to a channel. These external timestamps are the time pulses emitted by the GPS receiver.

`internal/gpsevent` provides the main event handling loop after GPS configuration is done. It receives GPS packets from `internal/gpsio` and then uses the appropriate protocol implementation to construct protocol-independent messages that it passes to `internal/phcsync`. It also receives timestamps from `internal/ts` and passes them to `internal/phcsync`.

`internal/phcsync` provides the core functionality of synchronizing the PTP hardware clock. It receives timestamps from gpsevent and accesses GPS messages via `internal/timemsg`.

`internal/timemsg` buffers recent GPS time messages from a receiver and provides methods to retrieve consecutive, second-aligned messages and pulse offset corrections for time synchronization. It recevies messages from `internal/gpsevent` and provides them to `internal/phcsync`. It isolates `internal/phcsync` from the complexities of internal/gpsprot`.

`internal/ptpgm` manages the PTP grandmaster state and synchronization status as seen by ptp4l. It provides a worker goroutine that sends updates to ptp4l via the PTP management protocol.

`internal/refclock` provides abstractions for sending clock synchronization samples to external time synchronization services like chrony. It includes a worker goroutine that processes samples from a channel and delivers them to configured refclock implementations.

`internal/syncsim` provides a discrete-event simulator for testing the phcsync controller with configurable error models for GPS PPS timing and PHC oscillator characteristics. It generates synthetic pulses, messages, and ticks under various fault conditions and measures controller performance.

`internal/statsobs` accumulates clock synchronization statistics including phase offset (maximum, mean, RMS), frequency deviation (mean, standard deviation), and frequency delta characteristics. It implements `phcsync.Sampler` for data collection.

`internal/proxy` implements proxying of GPS packets to TCP and Unix domain sockets.

`internal/obs` provides unified observability interfaces including `Observer` (which extends `phcsync.Sampler` and `gpsprot.Handler`) for receiving both clock synchronization samples and GPS protocol messages.

`internal/sseobs` implements the `Observer` interface to generate Server-Sent Events data that the daemon uses for the web interface.

`internal/promobs` implements the `Observer` interface to collect Prometheus metrics that the daemon exposes via HTTP handlers.

`internal/logobs` implements the `Observer` interface to record clock synchronization samples to log files and emit statistical summaries via structured logging. It provides `ClockLogObserver` for per-sample clock data and `StatsLogObserver` for interval-based summaries.

`web` embeds the HTML/JavaScript code for the web interface. This code is transpiled from TypeScript and uses Preact JavaScript library.

`internal/bcast` concurrency abstraction broadcasts a channel to multiple other channels.

#### GPS

`internal/gpsio` implements a goroutine that reads input from the GPS, splits it into packets and sends those packets to a channel. It provides an abstraction for performing IO on a GPS, which can work either over a serial connection (using `term`) or over a network connection. The input is split into packets using the `internal/scan` package.

`internal/gpscfg` drives the GPS configuration process. It combines `internal/gpsio` and `internal/gpsprot`.

#### Base

`internal/logfile` provides utility functions for opening and reopening log files.

### Domain layer

Provides packages using domain-specific abstractions that are used by the application layer. These packages have mutual dependencies. They do not make use of goroutines nor do they perform logging.

#### Time sync

`internal/phc` provides low level abstractions to access the PTP hardware clock. It is highly Linux dependent. It uses `internal/ptime`.

`internal/sockrefclock` implements the chrony refclock protocol. It uses `internal/ptime`.

`internal/clocksim` provides discrete-time simulation of PTP hardware clocks and GNSS PPS signals. It includes simulator functions for oscillators (modeling frequency errors like white noise, flicker noise, random walk, drift) and for GPS/PPS timing errors (jitter, sawtooth, sinusoids, colored noise).

`internal/phctime` provides an `Era` type used for managing stepping of a PHC and types that combine `Era` with `ptime.Time`.

#### GPS

`internal/ptime` provides a Time type that represents time in the PTP timescale (nanoseconds in TAI timescale since 1970-01-01T00:00:00 TAI). This is used throught the domain layer and higher level layers.

`internal/scan` provides a Packet type representing a packet of GPS data in some protocol. It also provides a scan function to split up a stream of bytes into packets. It does not interpret the content of the packet.

`internal/gpsprot` abstracts a GPS protocol such as UBX or NMEA. This operates in two phases: configuration and post-configuration. The post-configuration part of this defines types that represent the information contained in GPS messages in a protocol-independent way.

`internal/ubx` implements `internal/gpsprot` abstractions for the UBX protocol. It uses `internal/ubx/bin` and `internal/ubxcfgval` to do this.

`internal/nmea` implements `internal/gpsprot` abstractions for the NMEA protocol.

`internal/rtcm` implements `internal/gpsprot` abstractions for the RTCM protocol.

`internal/gpsreg` provides a registry for the various implementations of the `internal/gpsprot` layer. Higher layers avoid interacting with the implementations for specific protocols as much as possible. Generally the command-line layer interacts with `internal/gpsreg`, and passes the appropriate implementations into lower layers.

`internal/casic` implements `internal/gpsprot` abstractions for the CASIC binary protocol. It uses `internal/casic/bin` to do this.

`internal/unc` implements `internal/gpsprot` abstractions for the Unicore protocol. It uses `internal/uncmsg` to parse Unicore binary and ASCII message formats.

`internal/nov` implements `internal/gpsprot` abstractions for the NovAtel protocol. It uses `internal/novmsg` to parse binary and ASCII NovAtel packets.

`internal/sino` provides satellite numbering schemes for SinoGNSS receivers, defining NMEA satellite ID mappings for GLONASS, NavIC, Galileo, QZSS, BeiDou, and SBAS.

`internal/as` provides NMEA satellite numbering configuration for Allystar GPS receivers.

`internal/scantest` provides utility functions for testing GPS packet format implementations. It includes functions to find packets within buffers and insert random data prefixes for robustness testing.

`internal/gpsdecode` decodes binary GPS packets into JSON-serializable maps derived from the Go structs defined in the library layer packages.

### Library layer

Provides a library of packages, which are potentially useful outside satpulse. There are few mutual dependencies. These packages do not make use of goroutines nor do they perform no logging.

#### Time sync

`internal/pmc` implements a PTP management client.

`internal/ifwait` use the Linux kernel's netlink subsystem to wait for changes in a network interface's status.

`internal/devnotify` uses the Linux kernel's netlink subsystem to listen for creation of new devices by udev. (This is not used currently.)

`internal/sse` marshals data into the format of HTML SSE (server-sent events).

`internal/allan` computes Allan deviations. (This is not used currently.)

`internal/check` validates struct fields against constraints specified in struct tags using reflection. It supports numeric types with comparison operators (`>`, `>=`, `<`, `<=`) and recursively validates nested structs.

`internal/circbuf` provides a generic circular buffer that maintains a sliding window of recent samples. It supports appending with automatic overflow handling and reverse chronological iteration.

`internal/fuser` finds processes that have a specific file or directory open by examining the Linux /proc filesystem. It provides functionality similar to the Unix fuser utility.

`internal/median` provides efficient median computation for a fixed-size moving window using a circular buffer with a sorted index array. It supports 64-bit integers, floats, and time.Duration.

#### GPS

`internal/ubxbin` translates binary packets in the UBX protocol to and from Go structs.

`internal/ubxcfgval` handles the 9th generation UBX format for configuration data that is payload for UBX-CFG-VALGET/VALSET messages

`internal/ubxcfgval/cfgschema` contains a YAML schema for configuration data handled by `internal/ubxcfgval`. This is used to generate code in the `internal/ubxcfgval` package.

`internal/casbin` translates binary packets in the CASIC protocol to and from Go structs.

`internal/asbin` translates binary packets in the Allystar binary protocol to and from Go structs.

`internal/novmsg` provides parsing and serialization of NovAtel GPS receiver messages in binary and ASCII formats. It defines message header and body types and implements CRC32 validation.

`internal/uncmsg` parses Unicore protocol messages in binary and ASCII formats. It defines message structures and provides parsing/serialization using `internal/novmsg`.

`internal/geopos` converts between positions in the ECEF and LLH geodetic coordinate systems. This is used by the web interface to link to Google maps.

`internal/fieldenc` provides reflection-based encoding and decoding of Go structs to and from ordered string field arrays. It supports standard types and custom TextMarshaler/TextUnmarshaler implementations.

`internal/nmeamsg` analyzes NMEA sentence syntax and computes checksums.

`term` provides access to the Linux terminal interface, which provides access to serial devices. This is similar to [github.com/pkg/term](https://github.com/pkg/term), but provides additional Linux-specific functionality.

#### Base

`internal/ntptime` reads the Linux kernel's NTP synchronization state via the `adjtimex` syscall and exposes it as platform-independent types. It provides information about system clock synchronization and leap second status.



