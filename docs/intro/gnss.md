---
title: GPS and GNSS
sitemap: false
---

GPS (Global Positioning System) technically refers to the satellite constellation operated by the USA. There
are three other similar constellations with global coverage: Galileo, BeiDou and GLONASS operated by the
European Union, China and Russia respectively. The technically correct term for such a constellation is
GNSS (Global Navigation Satellite System). SatPulse is designed to work any GNSS.
The term GPS is used informally to refer to any GNSS system, and we will use it in that sense.

## Module features

### Protocol

A GNSS module uses one or more protocols to communicate with the host computer over a serial connection.
All GNSS modules support the NMEA protocol.
This is sufficient for a host computer to receive basic information about the navigation solutions computed
by the GNSS chip; this includes position, velocity and time.
A time server can operate using just the information provided by NMEA together with a PPS signal.
However, GNSS modules provide many capabilities that can be accessed only with a vendor-specific protocol,
which may be an extension to NMEA or may be a completely different binary protocol.

Use of a vendor-specific protocol is necessary in particular for configuring a GNSS module,
and for making use of the higher-end features described later in this section.

Use of a vendor-specific protocol allows one particular aspect of PTP to work more smoothly.
PTP uses a timescale based on TAI.
All GNSS systems other than GLONASS use a system time that is a fixed offset from TAI.
Using a vendor-specific protocol allows the system time to be converted directly to PTP's timescale.
NMEA provides time only in UTC.
The offset between TAI and UTC varies depending on the occurrence of leap seconds.
Using NMEA requires that the module convert the system time to UTC and
the host software convert from UTC to PTP's timescale,
which requires configuration of leap second occurrences to be maintained on the host.
Using a vendor-specific protocol avoids the need for this.

### Frequency Band

The feature that probably makes the most difference to the timing performance of a module is
whether it supports multiple frequency bands.

The signals broadcast by satellites are delayed as they pass through the ionosphere,
in ways that cannot be precisely predicted or modelled.
This is the principal source of error in GNSS measurements of time or position.
However, signals in different frequency bands are delayed by different amounts.
If a module simultaneously receives signals in different bands from the same satellite,
it can use the differences to compensate for the effects of the delays.

The details of the various signals broadcast by each GNSS are complex. For marketing purposes,
vendors typically divide frequencies into L1, L2, L5 and L6.
GPS and other GNSS systems started off by supporting the L1 band.
The L2 band came next, allowing for the first dual-band receivers.
L5 is a more modern signal,
L6 is used primarily for satellite-broadcast PPP, discussed below.

Modules typically support one of the following combinations:

* L1 - single-band
* L1/L2 - dual-band
* L1/L5 - dual-band
* L1/L2/L5 - triple-band
* L1/L2/L5/L6 - all-band

Newer inexpensive Chinese-made dual-band modules tend to support L1/L5 rather than L1/L2.

### Constellation

There are four major GNSS constellations:
* GPS, operated by the US
* GLONASS, operated by Russia
* Galileo, operated by the EU
* BeiDou, operated by China

Modern GNSS modules typically support all four, although a few have dropped support for GLONASS.

GLONASS made some different technical choices from the other three major GNSSs:
* it uses FDMA rather than CDMA, although it is in the course of rolling out new satellites that support CDMA
* its system time is based on UTC rather than being a fixed offset from TAI.

The latter makes it a slightly less good fit for PTP, since the PTP timescale is based on TAI.

BeiDou was the first to offer an operational L5 signal.
GPS has fallen a bit behind Galileo and BeiDou.
As of 2025Q3, it has not yet declared full operational capability for its L5 signal.
Both Galileo and BeiDou provide a satellite-broadcast PPP service (described below),
and Galileo also provides a navigation message authentication service (also described below);
GPS offers neither.

There are also regional GNSSs:
* QZSS, operated by Japan
* NavIC (sometimes known as IRNSS), operated by India

QZSS is technically quite advanced. NavIC's first signal is in the L5 band.

### High-end features

#### Timing mode

Timing mode is, as the name suggests, a mode intended for timing applications.
Normally, a GNSS receiver uses information from at least four satellites to compute its 3D position and the time.
In timing mode, it assumes a fixed position and then uses information from one satellite to compute the time.
There are usually two possibilities for determining the fixed position to be used.
* It can be explicitly specified by the user.
* The receiver can perform a survey-in process, where it computes its position once a second over a user-specified period of time, and then uses the average of the positions as the fixed position.

#### Quantization/sawtooth error reporting

When GNSS receivers generate a pulse to mark the start of a second, the pulse is constrained to be aligned to a tick of the receiver's internal clock, and so will not usually be precisely aligned to the true start of the second as determined by the receiver.
The error in the pulse caused by this constraint is called a quantization error
or sawtooth error.
Some timing-oriented receivers are able to report these errors,
thus allowing the host to correct for them.
This error in modern receivers is of the order of a few nanoseconds.

#### RTK

Real-Time Kinematic (RTK) positioning is a precision positioning technique, which uses two GNSS receivers: a base station with a precisely known position which provides correction data in the form of RTCM messages, and a rover, which can use this correction data to determine its position relative to the base station with centimeter-level accuracy.
The distance between the base and the rover is limited to about 10-20 kilometers.

GNSS receivers that perform well for RTK typically also provide good timing performance.
Base mode for RTK is similar to timing mode.
RTK is a much larger market than timing, and so RTK-capable receivers often offer better value for timing applications
than specialized timing receivers.

#### Raw data

Raw data provides access to the inputs used by the GNSS module's internal processing.
This includes the satellite measurements (pseudoranges, carrier phase, Doppler) and data from navigation messages such as ephemeris.

It's a bit like a camera that provides access to RAW data and not just JPEG.

For timing applications, the main use of raw measurement data is to obtain a precise position via post-processed PPP (Precise Point Positioning), which can then be used as the fixed position in timing mode.

Raw data can also be used to do software-based RTK.

#### Satellite-broadcast PPP

With satellite-broadcast PPP, satellites broadcast near-realtime correction data using the L6 band. These are SSR (State Space Representation) corrections and are globally applicable: the corrections are things like more precise satellite orbits or satellite clock offsets.
They are different from the corrections used by RTK, which are OSR (Observation Space Representation) corrections and are only locally applicable.

Galileo, BeiDou and QZSS each have their own PPP service: HAS, B2b-PPP and MADOCA, respectively. These services are all free to use.

#### Navigation message authentication

With navigation message authentication, navigation messages broadcast by a GNSS are cryptographically signed.
This allows you to detect some kinds of spoofing.
Galileo and QZSS have both deployed NMA services: OSNMA and QZNMA, respectively.
