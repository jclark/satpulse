---
title: "Precision positioning"
---

A GNSS receiver using multiple bands can achieve an accuracy in the range of 1-2m.
It is in fact possible to achieve accuracy in the range of 1-2cm,
but it is not as simple as just plugging the GNSS receiver in and reading out the position.
This page explains the techniques involved.

## Raw observations

One of the key enabling capabilities is for a GNSS receiver to be able to output the measurements it is making, not just the final position.
These measurements are called raw observations.
Raw observations include measurements for each satellite and each signal that the receiver is tracking.
Support for raw observation output is typically found only in receivers intended for timing or high-precision positioning.

The fundamental measurement is the pseudorange.
To achieve centimeter accuracy, another kind of measurement is needed called *carrier phase*,
which we will describe below.

## Precise Point Positioning

After correcting for ionospheric error using multiple bands, the dominant remaining sources of error relate to satellite position and clocks.
When a satellite transmits its position, it is not transmitting its actual position but its predicted position; its actual position will be slightly different from its predicted position.
The clocks of satellites in a constellation are all synchronized to a common time,
but the synchronization is not perfect; an error of even 1 nanosecond translates into a position error of 30cm.

It is possible to estimate these and other similar errors after the fact by collecting and analyzing raw observations from a network of ground stations, located in precisely known positions across the world.
These error estimates are called *corrections*, specifically State-Space Representation (SSR) corrections.
The idea of Precise Point Positioning (PPP) is to use SSR corrections to perform a better calculation of position.

The classic form of PPP is post-processed PPP.
The workflow for post-processed PPP is to collect raw observations from a static receiver for a period of hours.
The raw observations are then converted into [RINEX](https://igs.org/wg/rinex/) format,
which is the standard interchange format for raw observations.
The RINEX file is then submitted to an online post-processed PPP service, such as CSRS-PPP.
The service has access to the error estimates produced by the ground stations and uses these in conjunction with the submitted RINEX file to produce a position estimate.

A more recent innovation is satellite-broadcast PPP.
In this form of PPP, SSR corrections are broadcast by satellites in near real time,
and the receiver itself applies the corrections in its position calculations.
This works because errors accumulate slowly:
satellites drift gradually from their predicted positions;
clocks similarly drift gradually from the correct time.
Thus corrections remain applicable for many minutes.
Satellite-broadcast PPP typically uses the L6 band.
Receivers that support the L6 band typically also support the L1, L2 and L5 bands, and are called all-band receivers.
The corrections can be broadcast by the GNSS satellites themselves.
The most notable example of this service is the Galileo HAS (High Accuracy Service),
which has global coverage.
There is also MADOCA from QZSS and PPP-B2b from BeiDou, but both have only regional coverage.
PPP-B2b uses the B2b band rather than the L6 band.
Access to these services is free.
It is also possible for the corrections to be broadcast by separate satellites as a commercial service,
such as the u-blox PointPerfect Global Service.

## Real-Time Kinematic

Real-Time Kinematic (RTK) is a precision positioning technique, which uses two GNSS receivers:
a static base station with a precisely known position and a moving rover.
The base emits correction data in the form of RTCM 3 messages, which are fed to the rover.
The rover uses the messages in conjunction with its own measurements to determine its position.
RTCM corrections are categorized as Observation Space Representation (OSR) corrections,
and are only applicable locally.
RTK works reliably when the base and rover are within 10-20km of each other;
in optimum conditions it can work up to 100km or so.

The most common protocol used to transfer corrections from the base to the rover is Ntrip.
The Ntrip protocol defines three roles. An Ntrip caster acts as an intermediary.
An RTK base station acts as an Ntrip server pushing RTCM corrections to an Ntrip caster.
An RTK rover acts as an Ntrip client pulling RTCM corrections from an Ntrip caster.

In modern RTK, the correction messages emitted by the base consist mostly of raw observations made by the base.
The RTCM messages with this information are called Multiple Signal Messages (MSM).
There are multiple levels of MSM messages, from 1 to 7, which have increasingly detailed information.
The two most common levels are MSM4 and MSM7.
MSM7 contains complete raw observation data and can also be converted to RINEX and used for PPP.
The other key piece of correction information is the position of the base.
The most common RTCM message with this information is the Stationary Base Station Antenna Reference Point (ARP) message.

RTK is a differential technique, which means that it uses differences between measurements made by the base
and measurements made by the rover.
The vector from the base to the rover is known as a baseline.
RTK estimates the position of the rover by estimating the baseline and then adding the baseline to the position of the base.
The key idea is that when a rover and base are relatively close, then for a satellite observed by both the base and the rover,
the difference between the distance from the rover to the satellite and the distance from the base to the satellite
is very close to the projection of the baseline onto the line of sight from the rover to the satellite.
Each such satellite thus gives a constraint on the baseline.
Combining the constraints from multiple satellites allows the rover to estimate the baseline and thus its position.
Note that the position of the rover is only as accurate as the position of the base.

This method of doing RTK where the rover receiver combines its measurements with the base's measurements is known as hardware RTK.
An alternative method is for the rover receiver to emit raw observations and for the host computer to use software
to combine the raw observations from the base and rover and compute the rover's position.
Receivers that support RTK are now inexpensive, so hardware RTK is now the preferred approach.

The receiver used as the RTK base must be configured with its fixed position.
This can be done in two ways.
The first way is to use a survey. This means that the receiver makes repeated observations of its position over a period of time,
and then uses their average as its position.
The alternative is to explicitly configure the coordinates of the fixed position.
The most accurate way to determine these coordinates is to use post-processed PPP.
Note that for a GNSS receiver used as an RTK base, the capability to emit MSM7 messages serves a dual purpose:
MSM7 messages both provide corrections to the rover and enable the absolute position of the base to be established using PPP.

Vendors often like to sell receivers for rovers and bases in matched pairs, but it should be clear from the above that these two roles
require different capabilities. A base receiver does not need to have the capability to perform RTK calculations
and a rover receiver using hardware RTK does not need to have the capability to emit MSM RTCM messages.
The requirements for an RTK base are in fact closer to those for a receiver used for [precision timing]({% link intro/timing.md %}): both need a stationary antenna with a precisely known position.

## Carrier phase

Raw observations include not only pseudorange but also carrier phase and this is what enables centimeter-level precision.
GNSS signals are transmitted by modulating a sine-wave carrier; the wavelength of the carrier is between about 19 and 25cm.
While a receiver is locked onto a GNSS signal, it keeps track of the number of cycles that have elapsed since it acquired lock.
This is measured to a small fraction of a cycle.
The important thing to understand is that the integer part of a carrier phase observation is not absolute.
It is measured with respect to a receiver-dependent origin.
The origin is guaranteed consistent only within a period during which the receiver maintained a continuous lock.

Both PPP and RTK use carrier phase observations to estimate the distance from the satellite to the receiver in cycles.
This process is called ambiguity resolution (AR),
because it is resolving the integer ambiguity in the origin of the carrier phase observations.
RTK normally resolves integer ambiguities in seconds, whereas PPP can take minutes or hours.
An RTK solution where integer ambiguities have been resolved is called a fixed solution.

