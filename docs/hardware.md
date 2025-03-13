---
title: What hardware to get
layout: single
---
There are very few inexpensive ethernet controllers that support PPS input. At the time of writing (2025Q1), the best options are:

- the Intel i210, specifically the i210-T1 card; this can be used with any PC;
- Raspberry Pi Compute Module 4 (CM4) or Compute Module 5 (CM5), combined with the official CM4 or CM5 IO board

For more information (including suitable GPS receivers)

- for the i210 and other PC-based options, see my [pc-ptp-ntp-guide](https://github.com/jclark/pc-ptp-ntp-guide) project
- for the CM4/CM5 option, see my [rpi-cm4-ptp-guide](https://github.com/jclark/rpi-cm4-ptp-guide) project 

When choosing a GPS receiver for use with SatPulse, I recommend using a u-blox receiver.

For PTP to work well, clients need to have NICs with PTP hardware timestamping support. This is a common feature of modern NICs. The PTP features also need to be supported by the driver. Intel NICs generally have PTP hardware timestamping with Linux driver support.

For best results the network switches should also have PTP support. For a low-cost switch, I recommend the FS.com IES3110 series.
