---
title: GNSS antennas
---

## Basics

A GNSS antenna is designed to handle a specific range of frequencies.
You should buy an antenna that supports at least the bands that your GNSS module supports.
The cheapest antennas are typically L1-band only.
Although the L1 frequencies of the various GNSS constellations differ slightly,
most L1-band antennas can handle all constellations.
Antennas that support two or more bands are significantly more expensive than L1-only antennas.
In evaluating whether a dual-band system is cost-effective, you need to account not only for the receiver price but also the higher cost of a dual-band antenna.

A GNSS antenna is almost always *active*, meaning that it contains an amplifier
DC-powered through the antenna cable from the receiver,
typically using 3.3-5V.
Signal-to-noise ratio is crucial for processing GNSS signals.
As a signal passes through the antenna cable, the strength of the signal is reduced
but the noise is not.
Amplification is necessary at the antenna end to achieve an adequate signal-to-noise ratio at the receiver end.

## Antenna positioning

The ideal position for an antenna is, of course, to be outside, on a roof with a 360 degree sky view and no neighbouring higher buildings, but this ideal is rarely achievable.
Good results can be achieved with a position that is far from ideal.
However, it is essential that the antenna has some view of the sky: more is obviously better.
It is possible for it to work inside through a window, but it depends on the type of glass.
Ordinary single-glazed windows only slightly reduce the signal, but the coatings in low-E or heat reflective can reduce the signal to point of unusability.

It is worth noting that a higher signal level is required for acquisition than for tracking.
I have occasionally, while travelling, opened the window and held the antenna outside until the PPS light comes on, then brought it inside and closed the window.

Once you know where the antenna will go, you can determine the cable length needed.

## Puck antennas

The most common form factor for an antenna is the so-called hockey puck.
This comes with a relatively thin non-detachable cable with an SMA male connector.
The cable is typically limited in length to 5m.
If that is long enough for you, then a hockey puck antenna is a good choice.

Puck antennas typically have a magnetic base.
They are designed to attach to a metal surface (such as car roof) larger than the antenna, which provides a ground plane.
If you are not using it on such a surface, the performance can be improved by putting a metal plate underneath it.
SparkFun sell an [antenna ground plate](https://www.sparkfun.com/gps-antenna-ground-plate.html) which is a metal plate, suitable for use as a ground plane for a puck antenna, with a nut welded to the bottom, compatible with 1/4" camera tripods and other mounts.
This is a great option for mounting a puck antenna.

Puck antennas vary widely in quality.
It's hard to be confident of the quality unless you go with a known brand name.
u-blox make a variety of models covering different frequency bands,
which are expensive.

Quectel is also a reputable brand, and they have some puck antennas,
which are available on AliExpress or Taobao at a significantly lower price-point.
For example, the YEGD006U1A is their all-band model and is CNY130 on Taobao
(it's a lot more on AliExpress).

I would also recommend Beitian on AliExpress: it's a step down from Quectel,
but I have found them to make high-quality products at a reasonable price.

## Survey antennas

If you need a longer cable, then the next step up is a survey antenna. 
Survey antennas are designed for the RTK market as well as survey applications;
since RTK has become a large market, they offer good value.
They work well for timing applications also.
These look like a mushroom and have a mounting hole in the center and an antenna connector on one side.

These use a TNC female antenna connector. So you will need separately to buy a cable with a TNC male connector on the antenna end and an SMA male connector on the receiver end. The most common type of cable for these is RG58.
For longer lengths, you might want to consider a more expensive, lower-loss cable such as LMR-200.
For really long lengths, you might want a cable such as LMR-400 which is too thick to be terminated with SMA;
in this case you would need a long cable with TNC on both ends with a TNC to SMA pigtail to connect to the receiver.

The thread of the center hole on survey antennas is 5/8-11 UNC, which is different from what cameras use.
There are survey tripods and poles designed for this thread, but they are expensive, and not necessary for non-survey applications.
There are a couple of inexpensive alternatives:

- you can get an adapter that converts from this thread to the 1/4" thread used by cameras, and then you can use mounts designed for cameras; I have an antenna clamped to a tree using a camera mount;
- you can get a tripod designed for use in the construction industry for laser-levels; these typically use the same thread, and are available inexpensively.

The US National Geodetic Survey
maintains an [Antenna Calibration](https://www.ngs.noaa.gov/ANTCAL/) database,
which has lots of mostly extremely expensive survey antennas.

Beitian have some antennas that are in this database and are available inexpensively from AliExpress: BT-300, BT-300D, BT-300S, BT-800, BT-800D. I have the BT-800D, which is advertised as triple-band, but I found it worked for L6 also.

I also have a Harxon antenna that is in the database, the HX-CSX627A, which I also got from AliExpress at a reasonable price.

## Antenna splitters

To feed one antenna into multiple receivers, you need a GNSS splitter. A simple T-connector is not safe: both receivers attempting to supply DC power will conflict, which can lead to receiver damage.
A GNSS splitter passes DC power from one port and blocks DC power on the others.

There are two main kinds of GNSS splitter:
* *Passive* splitters divide the RF signal without amplification; each output is weaker by about 3–4 dB.  
* *Active* splitters include amplification so that each output is close to the original signal level.

The amplifier (LNA) of a survey antenna is often in the 38-40dB range.
With a short cable, this has sufficient power that a passive splitter can work fine.
(I use a 6 way passive splitter with 10m cable.)

Antenna splitters are designed to cover a specific frequency range.
So you also need to pick a splitter to match the frequency coverage of your antenna.

BG7TBL make an inexpensive two-way, passive, all-band antenna splitter, PS-GNSS-2,
which I own.

Leo Bodnar make the LBE-2002, which is similar, but three-way and considerably more expensive.