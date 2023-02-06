
## U-blox

* On F9T, using UBX-TIM-TP, tow seems to be wrong unless time base is set using TP5 to GPS
* GNSS week number of past leap second event on nav-timels is wrong, because GPS transmits only bottom 8 bits
* With ZED-F9P need to use firmware HPG 1.12 to get quantization error in UBX-TIM-TP message; neither HPG 1.13 nor HPG1 1.32 provide it.
* With HPG products (e.g. ZED-F9P), need to use
   * UBX-CFG-TMODE3 rather than UBX-CFG-TMODE2
   * UBX-NAV-SVIN rather than UBX-TIM_SVIN

## Linux Serial

close system call on a serial output will wait for buffered output to drain. If there's no device, this is will wait for timeout, which is defaults to 30s. Timeout can be changed by setserial command. Uses TIOCGSERIAL/TIOCSSERIAL ioctl defined in linux/serial.h, which uses a serial_struct; closing_wait member controls how long to wait on closing.