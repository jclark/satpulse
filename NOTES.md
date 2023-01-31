
## U-blox

* On F9T, using UBX-TIM-TP, tow seems to be wrong unless time base is set using TP5 to GPS
* Need TMODE3 on high-precision UBX receivers (e.g. F9P); TMODE2 on timing receivers.
* GNSS week number of past leap second event on nav-timels is wrong, because GPS transmits only bottom 8 bits
