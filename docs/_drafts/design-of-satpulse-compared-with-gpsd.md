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
`satpulsed`, which is a daemon and `satpulsetool` which is a command line tool.
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
The configuration engine is part of the library and is used both by `satpulsed` and `satpulsetool`:
configuration changes that affect the receiver's non-volatile
memory or interrupt receiver operation (such as changing the enabled GNSS constellations) are only
done when specifically requested by the user with `satpulsetool`.
The daemon infers some aspects of receiver configuration
from the application-level configuration.
For example, if the configuration file specifies a PHC to be disciplined,
then the daemon will ensure a 1PPS time pulse is enabled.

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
XXX


The daemon has significant internal concurrency:
reading from the serial device, reading timestamp events, and updating the PTP daemon
are naturally concurrent with the main PHC-disciplining loop.



The starting point was that the daemon needed to do application-level work.
This fo

XXX interrelated
XXX when is SatPulse design choice a better fit




-----

GPSd's design choices are eloquently explained in a [chapter](https://aosabook.org/en/v2/gpsd.html) from the Architecture of Open Source Applications book.
In this post, I will describe the key design choices made by SatPulse,
which lead to an architecture very different from GPSd's.
I hope this will allow potential users to assess whether SatPulse
offers any advantages for their use cases.


GPSd has been around in roughly its present form since about [2006](https://gpsd.gitlab.io/gpsd/history.html).
There have been significant hardware advances since that time.
On the positioning side, hardware RTK allowing centimeter positioning accuracy
has become a mass market product used for drones and autonomous driving.
On the timing side, PTP hardware clocks with software defined pins (SDPs) supporting PPS input
have also become available at hobbyist prices,
making nanosecond-level timing accuracy possible.
These are both enabled by multiband receivers which have become ubiquitous
and inexpensive: these allow receivers to compensate for
ionospheric interference, which had previously been the most
significant source of inaccuracy.

## Scope

The first set of design choice relates to scope.
SatPulse is focused on modern GPS hardware that takes advantage
of the possibilities of the state of the art in GPS technology.
SatPulse also emphasizes GPS hardware that is available at affordable prices:
this covers a broad range from a dollar or two up to $500 or so.
In concrete terms, this means that SatPulse will support a vendor-specific protocol
only when the vendor offers affordable dual-band receivers.
I make an exception to this rule for single-band timing receivers,
since specialist timing receivers are still relatively uncommon.
I also exclude vendors whose GNSS chips are not available as a separate module
that can be attached to a host computer.
The most important vendor that SatPulse supports is u-blox.
The other vendors that SatPulse supports are all Chinese.
This reflects the current reality of the GPS market.

The other key choice related to scope is GPS receiver configuration.
SatPulse takes the view that configuring a GPS receiver is a core part
of what is involved in providing support for a receiver.
NMEA provides a standard that receivers can use for communicating
information about the navigation solutions being computed periodically by the receiver
But there is no vendor-independent standard for configuring a receiver.
But many key features of modern GPS receivers require configuration.
For example, use of RTK requires a receiver to configured as a rover
or a base station.
A receiver acting as an RTK base station needs to emit RTCM packets.
Almost all modern receivers support all the major GNSS constellations
(GPS, GLONASS, Galileo and BeiDou), and many users wish to control
which constellations are being used.
More recently, receivers can support satellite-broadcast PPP
like Galileo HAS, BeiDou B2b-PPP or QZSS Madoca,
which allow accuracies of a few centimeters without an RTK base station:
this again needs configuration.
Indeed all precision positioning technologies rely on supplying
the receiver with corrections, which augment
the standard GNSS positioning information.
The information may be broadcast over satellites
or it may be received over the internet.
It may be specific to a particular location (OSR), o
it may be regional/global (SSR).
In all cases, som configuration is needed to get the receiver
to use the corrections.
Another important technology is network message authentication,
which digitally signs the navigation messages broadcast by a GNSS system,
thus giving a degree of protection against spoofing.
Galileo's implementation of this, OSNMA, became operational in 2026.
And again this is a technology that requires configuration of the receiver.


## Architecture

GPSd is implemented in C and its basic function is to work as a multiplexer
from GPS devices to clients.
It reads device-dependent data from multiple serial devices connected to GPS receivers,
transforms it into a device-independent form and makes it available to multiple clients.
Clients read the device-independent data over a socket in JSON format.
This gpsd is itself intentionally very thin.
GPSd aims to offer a zero-configuration experience.
The user can plug in a new device and GPSd client can automatically get
information from it without having to perform any configuration.

SatPulse is implemented in Go and the overall structure is rather different.
Its GPS subsystem includes similar functionality to GPSd:
it can read device-independent data from a serial device connected to a GPS receiver
and transform it into a device-independent form.
But the main purpose of satpulsed, the daemon included in SatPulse, is not
to enable other clients to do useful things, but to do useful things itself.

There is a library of Go packages that handle reading device-dependendent
data from a serial device and transforming it to a device-independent form.
The library is designed to take advantage of Go's concurrency.

The satpulsed daemon is itself a very functional application.

SatPulse is implemented in Go, and its GPS functionality is a collection of reusable Go packages

Like GPSd, SatPulse has a device-independent data model
for the information reported by a GPS receiver.
With GPSd, the GPSd daemon itself does not do anything with this information
other than make it available to applications in JSON format over a socket.
The daemon itself is very thin and lightweight.

With SatPulse, the pipeline that prp is structured as a library.
The SatPulse daemon is an application built using that library.

GPSd and SatPulse are similar in one respect:
they both have a device-independent data model for the information
reported by a GPS receiver, covering things like time, position and velocity,
and they both can serialize this as JSON.
There are many differences in the details.
But the more important point is that the main function of GPSd is to deliver
this information to applications. Is

### Service API vs library API

* GPSd is not just a daemon - it's a daemon plus a fleet of clients (cgps, xgps, gpspipe, gpsprof, ubxtool, gpsdecode, gpsmon, etc.)
* These clients all talk to the daemon over a **service API** (inter-process): JSON over TCP socket, shared memory, or D-Bus
* The daemon owns the device; clients get data by connecting to the daemon
* Even libgps, the C client library, is just a convenience wrapper around the IPC protocol - it doesn't talk to GPS hardware directly

* SatPulse provides a **library API** (in-process): a collection of Go packages that any program can import and call directly
* Both satpulsed and satpulsetool are applications built on the same library
* They share code at the source level, not by talking to each other over a socket

* The language choice makes this viable, not just a stylistic preference
* In C, writing a concurrent library that manages a serial device, fans out packets to multiple consumers, and handles configuration - all thread-safely - is impractical
* GPSd's service architecture is the right answer in C: put concurrency in the daemon, hide it behind a socket, let clients be simple single-threaded programs
* In Go, goroutines and channels make a concurrent library API natural
* gps/app/bcast broadcasts a channel to multiple goroutines; gps/app/gpsio runs a goroutine that reads packets and sends them to a channel
* Callers don't think about threads or locking - they receive from a channel

### Multiplexing at different levels

* GPSd **must** multiplex because of the service API model
* A serial port can only have one owner, so if multiple clients need GPS data, you need a broker process
* Multiplexing isn't just a feature of GPSd - it's the reason GPSd exists as a daemon
* GPSd multiplexes at the **data layer**: parses raw packets, translates to device-independent JSON, serves to clients
* Clients must speak GPSd's protocol - you can't point u-center at GPSd

* SatPulse doesn't need cross-process multiplexing for its own consumers
* In-process fan-out via goroutines and channels (gps/app/bcast) serves the PTP sync, RTCM router, web dashboard, Prometheus exporter, packet logger, etc.

* SatPulse does support multiplexing to external clients, but at the **packet layer** via TCP proxy
* The proxy forwards raw native protocol (UBX, NMEA, CASIC, etc.) over TCP
* External clients don't need to know SatPulse exists
* u-center on a Windows machine connects to the proxy and thinks it's talking directly to a u-blox receiver
* All vendors provide Windows apps for configuring their receivers, and most allow access over TCP - this just works
* Streaming NMEA over TCP is an established convention with an IANA registered port (nmea-0183, 10110/tcp) - SatPulse's packet proxy is a natural extension of this to other protocols
* SatPulse allows filtering packets by proxy port - you can expose different ports with different protocols, e.g. a read-only port serving just NMEA data on 10110
* Can be made auto-discoverable on the local network with Avahi using the _nmea-0183._tcp service type - GeoClue already understands this
* More general than GPSd's multiplexing: works with any app that speaks the receiver's native protocol, not just SatPulse-aware clients

### Consequences

* satpulsetool can do real work with GPS hardware (configure receivers, decode packets, capture data) without a daemon running - it uses the same library directly
* A GPSd client like ubxtool can't do anything without the GPSd daemon running (or reimplementing device handling)
* The SatPulse daemon is not a thin multiplexer relaying data to the real applications - it is the application, doing the work itself (PTP clock discipline, RTCM routing, correction streams)
* One instance per receiver falls out naturally: no brokering to do, the daemon is the application
* Chain: C -> service API -> daemon as multiplexer -> one daemon for all devices
* Chain: Go -> library API -> in-process fan-out -> one instance per receiver, daemon does the work itself

Per-receiver daemon configuration
Device management
* gpsd
  * aims for zero-config experience: user plugs in a GPS receiver and a GPSd application can get location info without the user 
  * one instance for all GPS receivers
  * discovers GPS receivers on serial ports
  * expects to own 
* satpulse
  * requires explicit daemon configuration per receiver
  * does not discover devices
  * one instance per receiver
  * relies on udev and systemd to start up the per-receiver instance
  * less intrusive: will not touch a serial port unless instructed to
  * will not modify a GPS receiver unless instructed to
  * for server scenarios, user needs to make a decision to run a server
  * configuration is simpler
  * more reliable, deterministic faster service startup
  * enables per-receiver configuration



Outline
1. SatPulse has evolved from just PTP to more general-purpose suite of tools
2. How does this compare to GPSd? Refer to AOSA book and GPSD architecture
3. How GPS world has have changed since AOSA
   - PTP now accessible
   - Dual band cheap: gives PTP level precision
   - Hardware RTK now cheap
4. SatPulse scope choices
   * Emphasize taking advantage of capabilties of modern hardware for precision timing and positioning
   * Receiver configuration is key part of the problem
5. SatPulse architecture choices
   * Where similar to GPSd: device-independent JSON-serializable representation of data reported by the GPS receiver
   * packet layer
   * high and low level configuration
   * daemon does work itself
   * explicit per-receiver daemon configuration
   * one instance per receiver; no multiplexing
6 Implementation choices
   * Use Go
   * GPS functionality is exposed as a reusable library
   * satpulsetool uses same library as the daemon
     * Should be possible to build a GUI app with it
   * Linux is primary platform but other platforms supported to varying extent
7. Status





## Bulletpoints

**1. SatPulse has evolved from just PTP to more general-purpose suite of tools**

1. SatPulse is a system for controlling, observing, and integrating GNSS receivers — not just a daemon
2. SatPulse treats a GNSS receiver as a configured component with a defined role in a system — not as a peripheral to be discovered and shared
3. It understands roles: timing server, RTK base, mobile rover. The role drives configuration, processing, and routing.
4. It comprises satpulsed (daemon), satpulsetool (standalone CLI), and a desktop GUI (future)
5. It started as a narrow PTP timing tool but has evolved into a general-purpose GNSS system
6. It supports receivers from 8 vendors: u-blox, Unicore, Quectel, Zhongke/CASIC, Allystar, Techtotop, ByNav, ComNav/SinoGNSS

**2. How does this compare to GPSd?**

7. gpsd is the established player and works well for its use cases — this is not a replacement
8. gpsd is a client-side multiplexer: auto-discover devices, share access, normalise data for multiple applications
9. SatPulse is a server-side/infrastructure system: run a receiver as a controlled, reliable service
10. gpsd's zero-configuration philosophy is a principled and correct choice for its problem — but it's the wrong choice for infrastructure deployments where you need deterministic, auditable behaviour
11. gpsd consumes the packet layer internally and exposes only the data layer (as JSON on a TCP port); it does not expose the packet layer and does not provide a configuration layer
12. gpsd deliberately keeps the daemon thin and pushes responsibility to clients — that's right for a multiplexer, wrong for a system controller

**3. How the GPS world has changed since AOSA**

13. It targets modern dual-band, RTK-capable receivers — not trying to support legacy hardware
14. Modern GNSS modules are powerful and inexpensive — dual-band, built-in RTK engines, often under $50 — but the surrounding software ecosystem is fragmented and often oriented around legacy assumptions. SatPulse addresses this gap.
15. Vendor configuration tools are typically Windows-only GUIs that cannot be automated or integrated into a Linux system. SatPulse provides unified, scriptable, integratable control across vendors.
16. SatPulse embraces hardware RTK (computed in the receiver) rather than software RTK (RTKLIB model) — this aligns with the capabilities of modern GNSS modules

**4. SatPulse scope choices**

*Emphasize taking advantage of capabilities of modern hardware for precision timing and positioning:*

17. This is not arbitrary — precision timing (PTP) and precision positioning (RTK) require stateful, coordinated behaviour that cannot be pushed to clients
18. PTP requires the daemon to discipline the PTP hardware clock directly — this can't be done from a client
19. RTK requires the daemon to manage correction flows, configure the receiver's RTK mode, and route packets

*Receiver configuration is key part of the problem:*

20. GPS configuration is entirely vendor-specific — there is no standard (unlike NMEA for data)
21. SatPulse provides a device-independent interface to configuration — this is central to its mission
22. Device-independent configuration is much harder than device-independent data representation, because configuration is interactive (request/response) and deeply vendor-specific

**5. SatPulse architecture choices**

*Where similar to GPSd: device-independent JSON-serializable representation:*

23. SatPulse has three layers: packet layer, data layer, and high-level configuration layer
24. The **data layer** sits on top of the packet layer — it provides a device-independent semantic representation of what the receiver reports: timing, position, velocity, satellite info, solution characteristics

*Packet layer:*

25. The **packet layer** is the foundation — it deals with native GNSS protocols (UBX, NMEA, RTCM, CASIC, NovAtel-style, etc.), is bidirectional, and supports routing and proxying
26. The packet layer is not just internal plumbing — it is exposed and directly useful
27. It enables: RTCM routing for RTK base and rover operation, TCP proxying of native protocols, integration with vendor tools like u-center running on a remote Windows machine
28. Incoming correction data (RTCM, SPARTN) also passes through the packet layer — it is parsed, appears in the packet log, and is visible in the observability layer alongside receiver output

*High and low level configuration:*

29. The **high-level configuration layer** sits on top of the data layer — it uses the same vocabulary as the data layer to express configuration intent ("enable timing mode", "set constellations to GPS+Galileo L1+L5", "set pulse width to 0.1s")
30. There is also **low-level configuration**, which is a separate path built directly on the packet layer — protocol-aware message files that understand request/response patterns and acknowledgements, not blind byte-sending
31. High-level configuration means SatPulse can auto-configure a receiver optimally for its role (timing, base station) without the user needing to know protocol details
32. Low-level configuration provides an escape hatch for vendor-specific features not yet covered by the high-level model, and works even for receivers without full high-level support (tier 2 vendors)
33. The two tiers of vendor support reflect this: tier 1 (u-blox, Unicore) has full high-level configuration; tier 2 (Quectel, Zhongke, Allystar, Techtotop, ByNav, ComNav) uses message files with common tag conventions for a degree of vendor-independence

*Daemon does work itself:*

34. SatPulse does substantial work inside the daemon: configuration, packet routing, correction handling, role management
35. SatPulse owns the full chain from configuration through to packet routing and observability — no external glue required, fewer moving parts, predictable behaviour
36. The daemon's role and its configuration of the receiver are coupled: because the daemon knows what it is being asked to do, it can configure the receiver appropriately — for example, enabling velocity messages and disabling time mode for a mobile rover, or the opposite for a fixed timing server. This is only possible because the daemon owns both the role and the device-independent configuration interface. It is an example of how the architectural decisions are mutually reinforcing — configuration, role management, and daemon-centric processing are not independent features but parts of a single coherent design.
37. SatPulse has a unified stream abstraction for correction data with pull (inbound) and push (outbound) directions, plus a built-in NTRIP caster
38. **Stream pull** brings correction data into the receiver — transport-agnostic (plain TCP, NTRIP client, with MQTT as a future option), with adaptive backoff and reconnection (future)
39. **Stream push** sends RTCM from the receiver to an external caster (e.g. rtk2go.com) using the NTRIP server (SOURCE) protocol (future)
40. **NTRIP caster** — SatPulse acts as the caster itself, accepting connections from rovers and streaming RTCM directly from the receiver, with per-mountpoint authentication and optional MSM7→MSM4 conversion (future)
41. All three roles share common infrastructure: the same packet broadcast, connection abstraction, pruning queue, backoff logic, and MSM conversion
42. All configured in satpulse.toml alongside receiver configuration, timing, and everything else — one coherent configuration for the whole system
43. Contrast with the typical DIY base station where you glue together separate programs (str2str, ntripserver, etc.) with manual plumbing
44. SatPulse provides a web dashboard, Prometheus metrics endpoint, and structured event logging
45. RTCM packets from both directions (receiver and network) go through the same observability pipeline — tagged by source, visible in the same event log and metrics (future for network direction)
46. Observability is important for infrastructure: you need to know that your timing source or base station is working correctly

*Explicit per-receiver daemon configuration:*

47. Taking configuration seriously requires explicit configuration — you must tell SatPulse what receiver it's talking to and what you want
48. This makes SatPulse a well-behaved systemd service: it touches nothing you don't tell it to touch
49. If you disable configuration, SatPulse will not alter the receiver at all — it will only observe
50. These are not independent design choices — they are consequences of a single founding decision, and they reinforce each other. The result is deterministic startup behaviour, reproducible configuration, and auditable system state.

*One instance per receiver; no multiplexing:*

51. Explicit configuration leads naturally to one daemon instance per receiver — clean lifecycle, clean isolation
52. One-per-receiver means no auto-discovery — SatPulse doesn't scan ports or probe devices

**6. Implementation**

*Use Go:*

53. It is written in Go — memory-safe, easy to deploy as a single binary, no runtime dependencies
54. Go means a single static binary — no runtime dependencies, easy cross-compilation, simple deployment on embedded systems like Raspberry Pi

*Ecosystem:*

55. SatPulse plays nicely with the Linux timing ecosystem: ptp4l for PTP, chrony or ntpd-rs for NTP
56. The combination of SatPulse + ntpd-rs gives a fully memory-safe timing stack
57. SatPulse works with PHC hardware for nanosecond-class PTP precision, and also without PHC for standard NTP use cases

**Unclassified**

58. The data layer will become a proper external API for clients (future)
59. Base station operation — timing + RTK corrections from one receiver, one antenna — is a natural and unusual combined capability (future)
60. Desktop GUI using Wails (future)
61. The vision is a system layer for modern GNSS receivers — enabling others to build survey tools, monitoring systems, and mobile apps on top
