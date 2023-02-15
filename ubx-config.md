# Notes on how to configure GPS receivers for time mode using UBX protocol

## Determine receiver version

Use UBX-MON-VER to get version.

Key info

* UBX Protocol version major.minor
* Product category
   * SPG - standard precision, e.g. M8N
   * HPG - high precision, e.g. F9T
   * TIM - timing, e.g. M8T
   * FTS - frequency and time sync, e.g. M8F
* Does it run from ROM or FLASH?

## GNSS Constellations

Should enable a single constellation (GPS by default)

## Time pulse

Use UBX-CFG-TP5. Need

* Align to top of second
* Lock to GNSS freq
* Use GNNS time-grid of single chosen constellation
* Rate of 1Hz
* When not locked, no pulse
* When locked, period of 1s and pulse of 0.1s
* Cable delay based on cable length and cable type
* Use rising edge
* What about sync mode? Only for FTS products.

## Time mode

Enable using UBX-CFG-TMODE2 or UBX-CFG-TMODE3

Seems to be TMODE2 on timing products, TMODE3 on high-precision products.

If time-mode is disabled, then we can start a survey to enable it.

Achievable accuracy seems to depend on lot on how good the sky view is.

Might want to do a series of surveys with progressively longer observation times and tighter deviations.

2000 points seems a good default survey time (used by Lady Heather and Skytraq)

## Enable messages

Need to enable the messages we use.

# Disable messages

If we are on a low baud rate, then we might need to disable messages to avoid overflowing capacity of link.

## New config system

UBX systems from 9th gen support a new config system.

