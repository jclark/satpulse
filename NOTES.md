
## U-blox

* On F9T, using UBX-TIM-TP, tow seems to be wrong unless time base is set using TP5 to GPS
* Need TMODE3 on high-precision UBX receivers (e.g. F9P); TMODE2 on timing receivers.
* GNSS week number of past leap second event on nav-timels is wrong, because GPS transmits only bottom 8 bits

## Linux Serial

close system call on a serial output will wait for buffered output to drain. If there's no device, this is will wait for timeout, which is defaults to 30s. Timeout can be changed by setserial command. Uses TIOCGSERIAL/TIOCSSERIAL ioctl defined in linux/serial.h, which uses a serial_struct; closing_wait member controls how long to wait on closing.