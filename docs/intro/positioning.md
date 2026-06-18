---
title: "Precision positioning"
---

A basic GPS receiver without any special configuration can achieve an accuracy of perhaps 3 to 5m.
A dual-band or multi-band receiver can improve this to perhaps 1 to 2m.



## RTK


Need to distinguish hardware /software RTK

Real-Time Kinematic (RTK) positioning is a precision positioning technology, which uses two GNSS receivers: a base station with a precisely known position which provides correction data in the form of RTCM messages, and a rover, which can use this correction data to determine its position with centimeter-level accuracy.
The distance between the base and the rover is limited to about 10-20 kilometers.

## PPP

TODO: this needs much more emphasis on the RTK base station case. Determining a fixed antenna position matters as much for RTK as for timing, and more so - it is the main reason an RTK base user cares. Lead with the RTK base station's need for a known fixed position and present timing as the same requirement showing up again, rather than reaching the fixed-position/survey-in/PPP material entirely through timing mode.

The normal mode of operation of a GNSS receiver is to simultaneously solve for position, velocity and time using at least four satellites. GNSS receivers designed specifically for timing applications also offer a timing mode, where the position is fixed, the velocity is zero and the receiver solves only for time, which it can do using a single satellite. High-precision receivers designed for RTK such as the u-blox ZED-F9P also offer a timing mode.

In order to operate in timing mode, the position of the receiver must first be determined. Timing receivers typically offer a survey-in feature, where the receiver determines the position by averaging the position solutions over some user-configurable period. The SatPulse daemon will use this by default.

This is convenient, but there is a more precise way to determine the antenna position, which can lead to improved performance. This involves collecting raw observation data from the receiver for a number of hours and then submitting this data to an online Precise Point Positioning (PPP) service. The online service has access to additional data about the satellite orbits and clocks, which enable it to produce a much more precise position. Final data about the satellite orbits at a particular time only becomes available about 2 weeks after that time. So for the best possible results it is necessary to wait for 2 weeks after the data is collected before submitting it to the service. But good results can also be obtained using a more rapid service using data that becomes available after about 2 days.

Online PPP services typically expect the data to be in [RINEX](https://igs.org/wg/rinex/) format.