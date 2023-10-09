
## U-blox

* On F9T, using UBX-TIM-TP, tow seems to be wrong unless time base is set using TP5 to GPS
* GNSS week number of past leap second event on nav-timels is wrong, because GPS transmits only bottom 8 bits
* With ZED-F9P need to use firmware HPG 1.12 to get quantization error in UBX-TIM-TP message; neither HPG 1.13 nor HPG1 1.32 provide it.
* With HPG products (e.g. ZED-F9P), need to use
   * UBX-CFG-TMODE3 rather than UBX-CFG-TMODE2
   * UBX-NAV-SVIN rather than UBX-TIM-SVIN
* M8F outputs a pulse on timepulse 2

## Linux Serial

close system call on a serial output will wait for buffered output to drain. If there's no device, this is will wait for timeout, which is defaults to 30s. Timeout can be changed by setserial command. Uses TIOCGSERIAL/TIOCSSERIAL ioctl defined in linux/serial.h, which uses a serial_struct; closing_wait member controls how long to wait on closing.

## PTP Hardware Clock

ADJ_SETOFFSET does not work atomically (just does a read and a write) with most (all?) PHC drivers. We can measure the delay and
then use this to get things adjusted quicker. It was about 4.5us on a i210.

## Raspberry Pi CM4

There can be a delay between when the pulse on the SYNC pin occurs and when the driver delivers the PHC time for the pulse (up to a 0.25 seconds). This means that even though the GPS receiver emits the time pulse before the message with the GPS solution (including the time), the app may see the time pulse after the message.

## GLONASS

GLONASS time is UTC+3, specifically UTC as maintained by Russia.

Unlike all the other major GNSS systems, it's relative to UTC not TAI.

Transmission structure is

1. Superframe lasts 2.5 minutes and has 5 frames; so there are exactly 24 superframs every hour.
2. Frame lasts 30 seconds and has 15 strings; the first 4 frames of every superframe are the same
3. String last 2 seconds and consist of 85 data bits at 50bps lasting 1.7s, followed by a time mark lasting 0.3s; the first 4 strings of every frame are the same

Superframes are aligned so that a superframe starts at the start of day in GLONASS time.
Frames are aligned so that their start/end edge is an even number of seconds from the start of a day.

There are 4 timing-related words that are transmitted:

1. a 5-bit word called N<sub>4</sub>, transmitted once per superframe, which gives a 1-based 4-year interval number starting from 1996
2. an 11-bit word called N<sub>T</sub>, transmitted once per frame, which gives 1-based day number within the 4-year interval identified by N<sub>4</sub>
3. a 12-bit word called t<sub>k</sub>, transmitted once per frame, that gives the time of the start of frame relative to the start of the day identified by N<sub>T</sub>
  * 5 bits for number of hours
  * 6 bits for number of minutes
  * 1 bit for number of 30-second intervals
4. a 2-bit word called KP, transmitted once per superframe, which gives information about the leap second, if any, at the end of the current quarter, where
   * 00 means no leap second
   * 01 means positive leap second
   * 10 means not yet decided 
   * 11 means negative leap second

Any leap second will occur at midnight UTC, which will be immedately before 03:00:00 in GLONASS time. Immediately following the leap second i.e. at 03:00:00 it starts transmitting a sequence of superframes that are aligned with the new UTC. Since frames are two seconds long and the leap second changes alignment by one second, the frames from the old UTC and the new UTC will be differently aligned.

GLONASS receivers know when the leap second is coming up from the KP word.
They have to resynchronize themselves with the new time mark alignment.

This can cause [bugs](https://eos-gnss.com/knowledge-base/articles/technical-bulletin-1701-glonass-leap-second)










