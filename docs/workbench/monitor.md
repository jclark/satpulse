---
title: Monitor
toc: false
monitor_gallery:
  - url: /assets/images/wb-f9p-monitor.png
    image_path: /assets/images/wb-f9p-monitor.png
    alt: "Monitor tab with a ZED-F9P: clock, fix summary, map and sky view"
    title: "Monitor tab with a ZED-F9P"
  - url: /assets/images/wb-allystar-monitor-rtk.png
    image_path: /assets/images/wb-allystar-monitor-rtk.png
    alt: "Monitor tab with an Allystar TAU951M-P200 with an RTK fixed solution"
    title: "RTK fixed solution"
  - url: /assets/images/wb-allystar-monitor-scrolled.png
    image_path: /assets/images/wb-allystar-monitor-scrolled.png
    alt: "Monitor tab scrolled down to the Position Scatter and PVT Messages sections"
    title: "Position scatter and PVT messages"
---

The Monitor tab provides high-level monitoring,
providing the things you would expect like a map view and a sky view.
The top row is always visible: a clock, a summary of the current fix, a map and a sky view.
Below it are four sections which start closed: Position Scatter, PVT Messages, Satellite Signals and Survey.

{% include gallery id="monitor_gallery" %}

Points to note:

* The summary block next to the clock is abbreviated.
  On the DOP line, G is geometric, P position, T time, H horizontal and V vertical;
  the letters on the accuracy line mean the same.
  Ground speed and course accuracy appear only when the receiver is moving.
* In the sky view, colour is the constellation, fading shows the signal strength,
  and a hollow centre means the satellite is being tracked but is not used in the solution.
* Position Scatter is for a stationary receiver.
  It plots each fix against the running mean of the fixes so far, and reports CEP and RMS figures.
  When corrections are running, it also gives the distance from the base station.
  This is where you see what RTK is doing for you.
* PVT Messages has one row per receiver message that contributed to the fix.
  A value in bold was reported by the receiver;
  a value not in bold was derived by SatPulse from what the receiver did report.
  So you can see which message gave you the height and which one did not.
