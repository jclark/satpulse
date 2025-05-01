---
title: SatPulse internals
---

## Go packages

The Go packages can be divided up into layers, where packages in each layer depend on only packages in the same and lower layers.

### Command-line layer

Provides the user interface to the programs, including the command-line interface and the configuration file.

`cmd/satpulsetool` provides main for satpulsetool.

`cmd/satpulsed` provides main for satpulsed.

`internal/daemon` implements the satpulsed daemon. It orchestrates all the parts of the satpulsed program. It also handles the TOML config file.

`internal/pmccmd` implements `pmc` subcommand of satpulsetool.

`internal/gpscmd` implements `gps` command of satpulsetool.

`cmd/ifwait` provides a program that waits for a network interface to become ready. It exercises the functionality of the internal/ifwait package.

`internal/cmd` provides some common functionality used for command-line interfaces.

### Application layer

Provides the main blocks of the applications.

`internal/gpsio` implements a goroutine that reads input from the GPS, splits it into packets and sends those packets to a channel. It provides an abstraction for performing IO on a GPS, which can work either over a serial connection (using `term`) or over a network connection. The input is split into packets using the `internal/scan` package.

`internal/gpscfg` drives the GPS configuration process. It combines `internal/gpsio` and `internal/gpsprot`.

`internal/ts` implements a goroutine that reads external timestamps from the PTP hardware clock and sends those to a channel. These external timestamps are the time pulses emitted by the GPS receiver.

`internal/gpsevent` provides the main event handling loop after GPS configuration is done. It receives GPS packets from `internal/gpsio` and then uses the appropriate protocol implementation to construct a protocol-independent messages that it passes to `internal/combine` and `internal/mon`. It also receives timestamps from `internal/ts` and passes them to `internal/combine.

`internal/combine` is called by `internal/gpsevent` with events for time pulses and with events for GPS messages giving the current time. It generates samples that combine the PHC time of a time pulse with the correct time of that time pulse in the PTP timescale. It then passes these samples to `internal/mon`.

`internal/mon` is called by `internal/combine` and `internal/gpsevent`. It removes outliers from the samples it receives before passing them to `internal/servo`. It monitors the  synchronization status and reports it to the PTP grandmaster using `internal/pmc`. It also sends samples to chrony using `internal/sockrefclock`.

`internal/servo` is called by `internal/mon`. It uses the samples it receives to drive a Proportional-Integral servo that adjusts the frequency of the PHC to bring it into phase with the GPS time pulses.

`internal/proxy` implements proxying of GPS packets to TCP and Unix domain sockets.

`web` embeds the HTML/JavaScript code for the web interface. This code is transpiled from TypeScript and uses Preact JavaScript library.

`internal/bcast` concurrency abstraction broadcasts a channel to multiple other channels.

`internal/logfile` provides utility functions for opening and reopening log files.

### Domain layer

Provides packages using domain-specific abstractions that are used by the application layer. These packages have mutual dependencies. They do not make use of goroutines nor do they perform no logging.

`internal/ptime` provides a Time type that represents time in the PTP timescale (nanoseconds in TAI timescale since 1970-01-01T00:00:00 TAI). This is used throught the domain layer and higher level layers.

`internal/scan` provides a Packet type representing a packet of GPS data in some protocol. It also provides a scan function to split up a stream of bytes into packets. It does not interpret the content of the packet.

`internal/gpsprot` abstracts a GPS protocol such as UBX or NMEA. This operates in two phases: configuration and post-configuration. The post-configuration part of this defines types that represent the information contained in GPS messages in a protocol-independent way.

`internal/ubx` implements `internal/gpsprot` abstractions for the UBX protocol. It uses `internal/ubx/bin` and `internal/ubxcfgval` to do this.

`internal/nmea` implements `internal/gpsprot` abstractions for the NMEA protocol.

`internal/rtcm` implements `internal/gpsprot` abstractions for the RTCM protocol.

`internal/gpsreg` provides a registry for the various implementations of the `internal/gpsprot` layer. Higher layers avoid interacting with the implementations for specific protocols as much as possible. Generally the command-line layer interacts with `internal/gpsreg`, and passes the appropriate implementations into lower layers.

`internal/phc` provides low level abstractions to access the PTP hardware clock. It is highly Linux dependent. It uses `internal/ptime`.

`internal/sockrefclock` implements the chrony refclock protocol. It uses `internal/ptime`.

### Library layer

Provides a library of packages, which are potentially useful outside satpulse. There are no mutual dependencies. These packages do not make use of goroutines nor do they perform no logging.

`internal/ubx/bin` translates binary packets in the UBX protocol to and from Go structs.

`internal/ubxcfgval` handles the 9th generation UBX format for configuration data that is payload for UBX-CFG-VALGET/VALSET messages

`internal/ubxcfgval/cfgschema` contains a YAML schema for configuration data handled by `internal/ubxcfgval`. This is used to generate code in the `internal/ubxcfgval` package.

`internal/ifwait` use the Linux kernel's netlink subsystem to wait for changes in a network interface's status.

`internal/devnotify` uses the Linux kernel's netlink subsystem to listen for creation of new devices by udev. (This is not used currently.)

`internal/pmc` implements a PTP management client.

`internal/sse` marshals data into the format of HTML SSE (server-sent events).

`internal/allan` computes Allan deviations. (This is not used currently.)

`internal/geopos` converts between positions in the ECEF and LLA geodetic coordinate systems. This is used by the web interface to link to Google makes.

`term` provides access to the Linux terminal interface, which provides access to serial devices. This is similar to [github.com/pkg/term](https://github.com/pkg/term), but provides additional Linux-specific functionality.











