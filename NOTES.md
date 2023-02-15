
## U-blox

* On F9T, using UBX-TIM-TP, tow seems to be wrong unless time base is set using TP5 to GPS
* GNSS week number of past leap second event on nav-timels is wrong, because GPS transmits only bottom 8 bits
* With ZED-F9P need to use firmware HPG 1.12 to get quantization error in UBX-TIM-TP message; neither HPG 1.13 nor HPG1 1.32 provide it.
* With HPG products (e.g. ZED-F9P), need to use
   * UBX-CFG-TMODE3 rather than UBX-CFG-TMODE2
   * UBX-NAV-SVIN rather than UBX-TIM_SVIN

## Linux Serial

close system call on a serial output will wait for buffered output to drain. If there's no device, this is will wait for timeout, which is defaults to 30s. Timeout can be changed by setserial command. Uses TIOCGSERIAL/TIOCSSERIAL ioctl defined in linux/serial.h, which uses a serial_struct; closing_wait member controls how long to wait on closing.

## PTP Hardware Clock

ADJ_SETOFFSET does not work atomically (just does a read and a write) with most (all?) PHC drivers. We can measure the delay and
then use this to get things adjusted quicker. It was about 4.5us on a i210.

## Raspberry Pi CM4

There can be a delay between when the pulse on the SYNC pin occurs and when the driver delivers the PHC time for the pulse (up to a 0.25 seconds). This means that even though the GPS receiver emits the time pulse before the message with the GPS solution (including the time), the app may see the time pulse after the message.

