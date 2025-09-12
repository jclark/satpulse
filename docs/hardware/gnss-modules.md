---
title: GNSS modules
---

A GNSS board is built around a *module*.
Most of its capabilities come from the module it uses.
A module is a small, thin, rectangular, metal‑shielded component with solder pads underneath, typically around 1–3 cm across and a few millimeters thick;
it is built around a GNSS chip, which is the silicon that does the actual GNSS processing,
and integrates other components such as flash memory and an oscillator.
Designing a GNSS board around a module is relatively straightforward.
Designing a module around a bare chip is significantly harder.
Designing a GNSS chip is orders of magnitude more difficult and expensive; only a few companies do it.
Some vendors make both modules and chips; others make modules only or chips only.

A GNSS module cannot be connected to computer directly. It needs first to be integrated into a board.
The board provides connections for antenna input, PPS output, serial IO and power.
The board may be designed to go inside a computer's case, or it may have its own separate enclosure.

## Features

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

The only vendor specific protocol supported by the first release of SatPulse is UBX,
which is the protocol used by u-blox chips.

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
Galileo has the only deployed service so far, which is called OSNMA, and it achieved operational status in 2025.

## u-blox

u-blox is a Swiss company, which makes both modules and chips.
It makes some of its chips available to other vendors to make modules.
Its proprietary protocol is UBX and is used by all of its products.
However, the details of the UBX messages used varies greatly between products.

u-blox provides a Windows application called u-center for configuring their products.
It can work over a TCP/IP connection and so can be used with SatPulse.

### Product categories

u-blox has three main product categories

* standard precision: firmware starts with `SPG`; these are basic products without high-end features
* high precision: firmware starts with `HPG`; these offer high-end features including RTK and raw data
* timing: firmware starts with `TIM`; these are specifically designed for timing and include timing mode and raw data

There is also a frequency and time synchronization category (firmware starts with `FTS`), but there has only ever been one product in that category (the LEA-M8F).

u-blox appears to have a policy of not making firmware upgrades for timing products available on their web site; you can only get them if you are a commercial partner with an NDA in place. This is a major disadvantage of timing products as compared to high precision products.

### Platforms

#### M8

M8 is the earliest platform that I think it is still worth considering. It is L1-only.

For standard precision, M10 platform is available very cheaply and that is a better choice than M8.

But for a timing mode product, the LEA-M8T is still a solid choice if you get it at a suitably cheap price.
As well as timing mode, it supports raw data.

The LEA-M8F is a unique product and there's nothing else like it. It is sort of a GPSDO and has unique performance characteristics.
However, it is now getting quite old.

#### F9

F9 is u-blox's first dual-frequency platform. There are two products: ZED-F9T, which is the timing mode product,
and ZED-F9P, which is the high-precision product. 

The first products were L1/L2-only (e.g. ZED-F9T-00). More recently, L1/L5 variants have become available (e.g. ZED-F9T-10).
In 2025, they also released a ZED-F9T variant (ZED-F9T-20) that can do either L1/L2 or L1/L5.
(Note that this is not technically triple-band, because it cannot do L1, L2 and L5 simultaneously.)

The latest ZED-F9P firmware supports OSNMA.

The ZED-F9T has two timing features that are not provided by the ZED-F9P: it has a proprietary differential timing mode
(rather like RTK but for timing), and it provides quantization error reporting. (Early firmware for ZED-F9P also did quantization
error reporting, but that was removed in later firmware versions.)

But so far I have not found that ZED-F9T provides better timing performance in SatPulse than the ZED-F9P (although this
could be due to limitations in my test equipment).

F9 and subsequent platforms use a completely different (and better) approach to configuration within the framework of the UBX protocol.

#### M10

M10 is u-blox's latest SPG platform. It is L1-only.

The M10050-KB chip is available to OEMs, and several Chinese OEMs make very inexpensive modules using this chip.
This is probably the best value choice today for an inexpensive L1 module.

It supports all constellations, but no high-end features.

#### F10

F10 is a dual-band L1/L5 platform.

There are both standard precision products (MAX-F10S, NEO-F10N) and a timing product (NEO-F10T).

The standard precision F10 is u-blox's cheapest dual-band product and I have got good results from it. I would recommend it.

In my testing so far, I have not found the timing mode product (NEO-F10T) provides any better performance,
and it has the major disadvantage of no firmware updates being available. So I wouldn't recommend it at this point.
The NEO-F10T is a cheaper alternative to the ZED-F9T: it is not an upgrade.

#### X20
 
X20 is u-blox's first all-band platform i.e. L1/L2/L5/L6.

So far they have released the high precision product, the ZED-X20P.
Judging by the Interface Manual that they have released, the 2.00 firmware is still lacking important features,
notably satellite-broadcast PPP.
So personally I am waiting till this matures a bit: I expect in due course it will be a good product.

#### F20

F20 is u-blox's triple band platform L1/L2/L5 (so like X20 but without satellite-broadcast PPP).

## Unicore

Unicore is a Chinese company, which makes both modules and chips.

Unicore provide a Windows application called UPrecise for configuring their modules.
It can work over a TCP/IP connection and so can be used with SatPulse.

Their current high precision products are the NebulasIV range, based on their UC9810 chip. It includes

* UM980 - base model
* UM981 - adds an inertial sensor
* UM982 - adds dual antennas
* UM960 - lower end model

These are all-band receivers and primarily oriented at the RTK market.
One notable feature of UM980 is that the latest firmware supports all three satellite-broadcast PPP services.
They are resold by companies in Europe and the USA.
SatPulse has support for these in development. These use a similar protocol to NovAtel receivers.

The NebulasIV range also includes a timing receiver, the UT986, but it uses a different protocol and is not widely available.

There is a separate range of standard precision products, including the UM620,
which also use a different protocol.

## Allystar

Allystar is a Chinese company, which makes both modules and chips.

Allystar provides a Windows application called Satrack for configuring their modules.
It can work over a TCP/IP connection and so can be used with SatPulse.

Allystar make the TAU1201 dual-band L1/L5 GNSS module,
which is one of the least expensive dual-band modules available.
The module itself is under $10 in China.
I've tested it and found its performance to be excellent.

They also make some modules with higher-end features, but I have not yet evaluated their performance.

Allystar have their own binary protocol, which is similar in style to UBX.

## Quectel

Quectel is Chinese company which makes modules, but not chips.

Quectel provides a Windows application called QGNSS for configuring their modules.
Unfortunately it does not support working over a TCP/IP connection.

Their flagship product is the LG290P, which is an all-band receiver.

They use a protocol based on NMEA, using proprietary sentences.





