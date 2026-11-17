---
title: GPS and GNSS basics
toc: false
redirect_from:
  - /intro/gnss.html
---

On this page, we will explain the basics of GNSS in the context of computer systems.
What is GNSS? How does it relate to GPS? How can it be used with a computer?

GPS (Global Positioning System) technically refers to the satellite constellation operated by the USA. There
are three other similar constellations with global coverage: Galileo, BeiDou, and GLONASS, operated by the
European Union, China, and Russia, respectively. The technically correct term for such a constellation is
GNSS (Global Navigation Satellite System).
However, the term GPS is often used informally to refer to any GNSS system, and we will use it in that sense.
Arguably the most advanced GNSS system today is Galileo, followed by BeiDou.
GPS has fallen a bit behind. GLONASS made some different technical choices from the other three,
which has put it at a disadvantage.
There are also constellations with regional coverage: QZSS and NavIC, operated by Japan and India, respectively.

A constellation means a system of satellites whose orbits are coordinated.
The satellites of a GNSS constellation are in medium Earth orbit,
about 20,000 km above the Earth's surface.
The global constellations are designed so that all points on Earth always have at least four satellites in view.
This requires a minimum of 24 satellites, but most of the constellations have significantly more than that.

The main function of GNSS is to allow a user to locate their position on Earth.
This uses a device called a GNSS receiver.
The receiver estimates its distance from a satellite based on the travel time of signals from the satellite.
This estimate is called a *pseudorange*.
The position being computed has three unknowns: the X, Y and Z coordinates.
But the receiver's internal clock is not perfectly aligned with the satellites' clocks
and the pseudorange does not compensate for the difference.
The receiver therefore has to compute an additional unknown:
the difference between its clock and the satellites' clocks.
This implies that the GNSS receiver needs to use at least four satellites to compute its position.
It also implies that the GNSS receiver is computing not only its precise position
but also the precise time.

Modern GNSS receivers can make use of multiple constellations at once.
This improves accuracy and allows them to obtain a fix when they do not have a full view of the sky.

A basic GNSS receiver can achieve an accuracy in good conditions of about 3-5 m.
The dominant source of error is disturbances in the ionosphere that are not modelled by GNSS.
The way the ionosphere affects a signal is dependent on the signal's frequency.
Originally, each GNSS broadcast signals on a single frequency.
All the constellations used frequencies in a part of the upper L-band
known as the L1 band.
Later, each GNSS started broadcasting on multiple bands,
and receivers were developed that could receive signals on two bands simultaneously.
This enables receivers to compensate for the ionospheric error,
which can improve accuracy to about 1-2 m.
The earliest dual-band receivers are L1/L2, meaning they use the L2 band in addition to the L1 band.
More modern dual-band receivers use the L5 band rather than the L2 band.
High-end receivers can use all three bands simultaneously.
BeiDou pioneered the use of L5. Galileo also fully supports it.
As of 2026Q3, GPS has not yet declared full operational capability for its L5 signal.

GNSS signals can be quite easily spoofed, and this is becoming an increasing problem.
A partial solution is navigation message authentication (NMA),
which cryptographically signs some of the information in the signals.
Galileo and QZSS have both deployed NMA services: OSNMA and QZNMA, respectively.

When a GNSS receiver is used with a computer system, it almost always has some sort of serial connection,
such as a UART, USB or I2C.
The GNSS receiver emits messages over the serial connection describing the position and time it has computed,
as well as other information about its computations, such as the satellites that it has in view.
Typically, these messages are emitted once per second, but they can be emitted more frequently if needed.
By default, GNSS receivers emit messages following the industry-standard NMEA protocol.
But almost all GNSS receivers support an additional vendor-specific protocol,
which can provide richer information than NMEA.
Perhaps the most important role of the vendor-specific protocol is receiver configuration:
allowing the host computer to configure aspects of the receiver's operation.
NMEA does not deal with this, and there is no industry-standard protocol for receivers.
Configuration includes such things as the speed of the serial interface,
which constellations and bands are enabled, which messages should be emitted, and the rate at which they should be emitted.
Sophisticated receivers have many hundreds of configurable parameters.

Physically, a GNSS receiver suitable for use with a computer system is built around a [module]({% link hardware/gnss-modules.md %}).
A module is a small, thin, rectangular, metal-shielded component with solder pads underneath, typically around 1-3 cm across and a few millimeters thick;
it is built around a GNSS chip, which is the silicon that does the actual GNSS processing,
and integrates other components such as flash memory and an oscillator.
A GNSS module cannot be connected to a computer directly. It first needs to be integrated into a board.
The board supports connections for serial IO to the host, for the antenna and for power.
The board may be designed to [go inside a computer's case]({% link hardware/gnss-boards.md %}), or it may have [its own separate enclosure]({% link hardware/gnss-enclosed.md %}).

