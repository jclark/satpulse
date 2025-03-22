---
layout: single
title: "Comparing GPS performance with SatPulse"
date: 2025-03-23
---
I have been doing some more systematic testing on SatPulse. I have 6 different systems set up for testing. I ran SatPulse on them for a couple of days, with SatPulse producing its "clock" log, which includes the offsets measured between the PTP hardware clock (PHC) and the pulses emitted every second by the GPS. SatPulse feeds this into a PI servo that continually adjusts the frequency of the PHC. These offsets give an objective indicator of the performance of the GPS and the PHC.

Here is a table of the results:
 * GPS is the GPS used (configuration detailed below)
 * Mean is the mean of the absolute value of the offsets in nanoseconds
 * 95% is the 95th percentile, i.e. 95% of the absolute values of the offsets are within this
 * Std dev is the standard deviation of the offsets
 * Cost is the approximate cost in US$ without shipping or tax

| GPS | Mean | Std dev | 95% | Cost US$ |
|-----|-----|-----|----|----|
| M8F | 4.2 | 5.2 | 10 | 300 |
| F9P | 6.2 | 7.6 | 14 | 200 |
| F9T | 6.2 | 7.8 | 15 | 240 |
| GPSDO | 9.0 | 11.0 | 21 | 200 |
| M8T | 14.5 | 18.0 | 34 | 30 |
| M10  | 16.7 | 26.4 | 64 | 7 |

The systems are as follows:

* F9T is u-blox ZED-F9T in a [USB dongle from gnss.store](https://gnss.store/zed-f9t-timing-gnss-modules/108-elt0095.html) attached to Lenovo M720q Tiny PC (using Intel i3-9100T) with an Intel i210-T1 card, running Fedora 41
* F9P is a u-blox ZED-F9P in a [simpleRTK2B M.2 card from Ardusimple](https://www.ardusimple.com/product/simplertk2b-m-2/), in an Asus S500SD PC (using an Intel i5-12400) with an Intel i225-T1 card, running Debian Bookworm
* GPSDO is a BG7TBL GPSDO with an RS232 connection to a Raspberry Pi CM4 in a Waveshare PoE Board B, running Debian Bookworm
* M8F is a u-blox LEA-M8F in a [Timebeat](https://www.timebeat.app/) sandwich board (which they don't seem to sell any more), in a CM4 running Debian Bullseye
* M8T is a u-blox LEA-M8T in a Huawei board from ebay, in a CM4 running Fedora 41
* M10 is a [Star River](http://sragps.com/) SR1612U10 module (using a u-blox UBX-M10050-KB chip) in a SR1723-U10 board, in a Raspberry Pi CM5 running Debian Bookworm

SatPulse did its automatic configuration on all of these, apart from the GPSDO. The systems share two antennas (using GPS splitters). The antennas are both survey antennas, which can see about 50% of the sky. 

Some thoughts:
* The M8F is a bit different. It effectively is a mini-GPSDO. I was surprised to see it beat the F9P and F9T. I am also surprised that it is so much better than the BG7TBL GPSDO, which is physically much bigger.
* The dual-band high-precision models (F9P and F9T) do better than the single-band models, which is to be expected.
* The single-band timing model (M8T) does a bit better than the single-band standard precision model (M10), which is also to be expected.
* The M10 model is pretty decent for its price.
* The F9P, which is designed for RTK (precision positioning), and F9T, which is designed for precision timing, have essentially the same performance. I should mention that I flashed the F9P with older firmware that supports reporting the quantization error (sawtooth correction) in the UBX-TIM-TP message. I haven't tested whether this makes a difference. The F9T supports a differential timing where multiple F9Ts can work together to provide better precision. But if you don't need this, the F9P is the more cost-effective option. These are both dual-band. They have separate models which support L1+L2 and L1+L5.

