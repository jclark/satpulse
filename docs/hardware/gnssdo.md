---
title: GNSS disciplined oscillators
---

A GNSS disciplined oscillator (GNSSDO) uses the PPS signal from a GNSS module to discipline a local oscillator,
and outputs a signal at a reference frequency, usually 10MHz.  
This reference frequency is useful for other equipment that requires a frequency input,
such as test instruments, radios, or audio devices.

A GNSSDO is potentially useful for a time server in two ways:

- if the GNSSDO provides a PPS output, and the PPS output is also disciplined,
 then it should have less jitter than the GNSS's PPS output;
- if the GNSS receiver loses its lock and stops generating PPS output,
 the oscillator should enable the GNSSDO to maintain PPS output for a period with a useful level of accuracy,
 thus providing a holdover capability for the time server

Note that SatPulse does not yet support holdover properly: see [issue #152](https://github.com/jclark/satpulse/issues/152).

For a GNSSDO to work well with a time server, it must satisfy several requirements:

1. it must have a PPS output (not all GNSSDOs do)
2. the PPS output must be disciplined not merely passed through from the GNSS module
3. the PPS output must be aligned to the top of the second; it is not sufficient to produce a 1Hz output
4. the PPS output must be 0-3.3V for compatibility with network cards
5. it must have a serial output that provides at least the current time, and
   ideally provides a rich set of messages including data like satellite visibility

I have only found one GNSSDO that satisfies the above requirements.

## BG7TBL GNSSDO-CM55

BG7TBL released a new GNSSDO in the second half of 2025.
As of September 2025, it is available only at BG7TBL's Taobao shop and
one AliExpress [store](https://th.aliexpress.com/item/1005008811959897.html);
it is not yet available on eBay.
This model says GNSSDO-CM55 on it and has SMA connectors.

BG7TBL has an older GNSSDO, which is much larger and has BNC connectors,
which says "GNSS Disciplined Oscillator" on it.
This is a completely different product, which I do not recommend.

The GNSSDO-CM55 has SMA female connectors for the following:

- Antenna input
- PPS output at 0-3.3V
- 10MHz sine wave output
- 10MHz square wave output

It also has an RS-232 DB-9 female connector and 12V DC input using a 5.5x2.1 barrel jack.

Its GNSS module is a u-blox LEA-M8T.
The serial output of the GNSS module is passed through directly to the RS-232 port.

It is easy to tell that the PPS output is disciplined, because on an oscilloscope the pulse width is 10ms,
but the pulse width reported using UBX is the usual 100ms.

 
