---
title: Design of SatPulse compared with GPSd
---

In version 0.1 SatPulse focused on a specialized use case:
transferring time from a GPS to a PTP hardware clock (PHC).
In version 0.2, SatPulse's scope is much broader.
It now includes a rich, general-purpose GPS subsystem,
which supports a wide range of vendor protocols.
I believe 0.2 already has everything needed to provide full support for
timing-oriented use of a GPS receiver on Linux.
It also includes most of the pieces that are needed to support
precision positioning using RTK and NTRIP; this will be rounded out in upcoming releases.

At the moment, support for GPS receivers in Linux is almost universally based on GPSd.
In this post I want to explain some key design choices I made for SatPulse
that are different from those made by GPSd.
My goal is to help people understand when it might be worth considering SatPulse
as an alternative to GPSd.
I want to emphasize that SatPulse is not attempting to be a replacement for GPSd.
GPSd does what it sets out to do very well, as evidenced by its popularity.

Let me start by giving a brief overview of how GPSd works.
GPSd is both a daemon, written in C, and a suite of related tools.
It is centered around a device-independent data model of the information
provided by periodic messages emitted by GPS receivers.
A single instance of the daemon acts as a multiplexer.
Streams of messages are input from multiple sources such as serial devices,
converted to the device-independent data model and provided to multiple clients.
GPSd's primary API is a service API: a client runs as a separate process and
interacts with GPSd over a TCP socket using a JSON protocol;
it also provides a client library API that wraps the service API.
In the GPSd architecture, the daemon does not perform application-level work on the data;
its role is restricted to multiplexing and conversion into the device-independent data model.
GPSd has a zero-configuration philosophy:
when a user plugs in a GPS receiver, a GPSd client should work without requiring
the user to perform any configuration.
GPSd provides only a very limited device-independent abstraction for GPS
receiver configuration; most importantly it has the concept of enabling a *binary* mode,
which configures the receiver to output messages using its vendor-specific binary protocol instead of NMEA.
Instead it provides a separate Python program, ubxtool, for configuring u-blox receivers using the UBX protocol.

Like GPSd, SatPulse is also both a daemon and a suite of related tools,
and it is also centered around a similar device-independent data model.
SatPulse is written in Go.
SatPulse compiles into two executables:
`satpulsed`, which is a daemon, and `satpulsetool`, which is a command line tool.
All the application-level functionality that SatPulse provides
is included in `satpulsed` or `satpulsetool` depending on whether that functionality
needs to work as a daemon or not.
The daemon is configured using a file in TOML syntax.
One instance of the daemon runs for each device to which a receiver is attached.
This allows each instance to have its own configuration file,
and for systemd to manage the daemon lifecycle.
The configuration file has a modular structure, with separate TOML tables to configure
each aspect of application-level functionality.
The second executable, `satpulsetool`, is a suite of command-line tools.
It uses a subcommand syntax, so for example `satpulsetool gps` runs the `gps` tool.
These separate tools are bundled into a single executable because the Go language has a runtime that makes executables
quite large.
The GPS subsystem of SatPulse is structured as a reusable library of Go packages.
Both `satpulsed` and `satpulsetool` use this library for their GPS-related functionality:
`satpulsetool` does not need `satpulsed` to be running.

The most significant GPS functionality in SatPulse that goes beyond what GPSd provides
is its device-independent abstraction for GPS receiver configuration.
It allows you to choose which specific aspects of the configuration should be changed.
So, for example, you can specify that a time pulse should be enabled with a specific pulse width,
that the time pulse should be enabled only when the receiver has a lock,
and that it should be aligned to the system time of a particular GNSS;
or you can specify that the receiver should operate in time mode with specific fixed ECEF coordinates.
The changes to be made are given to the configuration engine which figures out how to apply
them to a specific receiver. This often involves reading the existing configuration of the receiver,
so that only the specified aspects of the configuration are changed.
An important part of configuring a GPS receiver is enabling the right set of periodic messages.
Each receiver divides up information in different ways.
So instead of specifying particular named device-dependent messages (e.g. UBX-NAV-PVT), you specify the needed information in data-model terms (e.g. the time in UTC and position in geodetic coordinates).
The engine then enables the best set of messages that provide this information.
The configuration engine is part of the library and is used both by `satpulsed` and `satpulsetool`.
Having `satpulsed` perform receiver configuration works well because it can
infer some aspects of receiver configuration from the application-level configuration.
For example, if the configuration file specifies a PHC to be disciplined,
then the daemon will ensure a 1PPS time pulse is enabled.
But configuration changes that affect the receiver's non-volatile
memory or interrupt receiver operation (such as changing the enabled GNSS constellations) are only
done when specifically requested by the user with `satpulsetool`.

The device-independent data model provided by the GPS subsystem can be serialized as JSON.
The daemon uses this for its Web dashboard feature.
It includes an HTTP server with an endpoint that exposes this data model as JSON-encoded server-sent events (SSE).
Another endpoint serves an HTML page which uses JavaScript to connect to the SSE endpoint and display a dashboard.
The daemon does not yet expose this data model over a network socket for consumption by third-party applications.
This will be straightforward to do, but I want to stabilize the data model first.

However, SatPulse emphasizes a different approach to allowing multiple independent applications to share access to
a single receiver, based on providing network access to packet streams.
The daemon can be configured to make TCP ports or Unix domain sockets
proxy the packets emitted by the receiver,
optionally filtering packets by protocol,
and optionally also allowing writing to the receiver, with a configurable locking strategy to prevent conflicts between writers.
This provides functionality similar to ser2net, but is protocol-aware.
Streams of native-protocol packets are already a well-defined wire-format.
Exposing this directly on a network endpoint allows receiver sharing without the need to define
a new daemon-dependent wire-format.
Many applications already exist that can work with such packet streams.
Here are three examples.
1. All GPS vendors provide an application for configuring their receivers, typically Windows-only.
Many of these applications, notably u-center, allow the receiver to be accessed over a TCP socket.
These can be used with SatPulse by taking advantage of the read-write feature. 
2. IANA have [registered](https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml?search=nmea) service name nmea-0183 and port 10110 for NMEA over TCP or UDP. [GeoClue](https://gitlab.freedesktop.org/geoclue/geoclue/-/wikis/home) is a D-Bus service that provides location information. It includes an NMEA backend that uses DNS-based service discovery (RFC 6763 as implemented by Avahi) to discover nmea-0183 services on the network. SatPulse can be configured to expose the NMEA service on port 10110, Avahi can be configured to advertise it and then the GeoClue NMEA backend can discover it and expose it to desktop apps over DBus.
3. RTK works by providing the rover's receiver with a stream of RTCM packets emitted by a base station's receiver. In a production environment, NTRIP is typically used to move packets from a base to a rover. An RTK base station uses a NTRIP server to provide RTCM packets to an NTRIP caster; an RTK rover uses an NTRIP client to get RTCM packets from the caster. NTRIP support is planned for a future release of SatPulse. But one popular open-source caster, the [BKG NtripCaster](https://igs.bkg.bund.de/ntrip/bkgcaster) can [pull](https://igs.bkg.bund.de/root_ftp/NTRIP/software/caster/ntripcaster_manual.html#c5) RTCM data from a TCP port without using NTRIP. This allows SatPulse to be used today as the GPS component of a combined RTK and time server.

We can summarize the key design choices for SatPulse that are different from GPSd as follows:

1. it is written in Go
2. the primary GPS API is a library API
3. the daemon does application-level work
4. the daemon has a configuration file
5. the daemon has separately configured instance per receiver
6. it provides device-independent model for GPS configuration
7. it emphasizes packet streams as a foundational layer

So why did I make these choices? They were not independent.
The initial PTP use case requires a daemon to do substantial application-level work:
it uses timestamp events from the Linux PHC subsystem in combination
with messages from the GPS receiver to discipline the PHC and send metadata updates to the PTP daemon.
I didn't want this use case to require two independent daemons.
Once the daemon is doing application-level work, then there needs to be a way to configure it. 
Having one daemon instance per serial device makes things straightforward:
each instance can have its own configuration file
and systemd can ensure that the daemon is not started until the serial device is ready.

Even with the initial PTP use case, the daemon has significant internal concurrency:
reading from the serial device, reading timestamp events, and updating the PTP daemon
are naturally concurrent with the main PHC-disciplining loop.
Other features like packet proxying or HTTP monitoring involve additional concurrency.
A reliable implementation of a daemon with this level of concurrency would not be feasible
except in a modern language like Go, which is memory safe and has language support for concurrency.
The implementation of `satpulsetool` also uses concurrency, although to a lesser extent than `satpulsed`.
Go makes it relatively straightforward to have a library that supports concurrency in a flexible way.

I hope this post has made it clear that GPSd and SatPulse are very different programs,
which have made different design choices for good reasons.
SatPulse would not be a suitable replacement for many uses of GPSd.
But I think there are several use-cases, particularly on the server side, where
SatPulse's more integrated approach and support for GPS configuration offers significant advantages.

## TODO

1. Add AOSA book reference to GPSd overview paragraph (around line 23). Something like: "GPSd's design is eloquently explained in a [chapter](https://aosabook.org/en/v2/gpsd.html) from the Architecture of Open Source Applications book."
2. Around line 51-54, note that SatPulse's default configuration treads lightly: it will not touch a serial port or modify a receiver unless explicitly configured to do so.
3. Around line 65-66, briefly motivate *why* configuration matters: modern GPS features (RTK, constellation selection, OSNMA, satellite-based PPP) all require receiver configuration, and there is no vendor-independent standard for it.
4. Fix "are input from" (line 28) — e.g. "It receives message streams from multiple sources..."
