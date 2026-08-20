# NAME

satpulsetool-serial - examine serial ports

# SYNOPSIS

**satpulsetool** [*global options*] **serial** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-a**\|**\-\-all**] [**\-d**\|**\-\-serial\-device** *path*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-i**\|**\-\-info**] [**\-j**\|**\-\-jsonl**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-pps\-pin** **cts**\|**dcd**\|**dsr**\|**ri**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-m**\|**\-\-pps\-method** **poll**\|**wait**\|**kernel**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-max\-wakeup\-latency** *microseconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-s**\|**\-\-device\-speed** *bps*] [**\-t**\|**\-\-timeout** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*]

# DESCRIPTION

The **satpulsetool** **serial** command can perform the following operations related to serial ports:

* detect the speed of a connected GPS receiver by trying to read data from the serial port at different speeds;
* detect pulse-per-second (PPS) edges on a modem-control pin;
* discover the available serial ports;
* show information about a serial port without opening it (this is useful mainly with USB serial ports);
* log packets received from a serial port.

With the **\-d** option, it operates on a single specified serial port.
With the **\-a** option, it discovers serial ports and operates on all the discovered ports.
When either the **\-d** or **\-a** option is specified, the default operation is speed detection.
Otherwise, the default operation is to discover the available serial ports and show information about them.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **serial** command.

**\-a**, **\-\-all**
: Discover the available serial ports and perform operations on all discovered ports.

**\-d**, **\-\-serial\-device** *path*
: Operate on the specified serial port.

**\-i**, **\-\-info**
: Show information about serial ports without opening or reading from them.

**\-p**, **\-\-pps\-pin** **cts**\|**dcd**\|**dsr**\|**ri**
: Detect PPS edges on the specified modem-control pin.
This causes speed detection not to be performed.

**\-m**, **\-\-pps\-method** **poll**\|**wait**\|**kernel**
: Controls how the operating system is used to detect modem status changes that mark pulse edges:
**kernel** means the kernel timestamps the time of a status change;
**wait** means the kernel notifies the application of a status change;
**poll** means the application continually asks for the current status.
When this option is omitted, the best available method is used.
Requires **\-p**.

**\-\-max\-wakeup\-latency** *microseconds*
: Limit CPU wakeup latency while detecting PPS; `0` is maximally strict. Currently supported only on Linux. Requires **\-p**.

**\-\-packet\-log** *path*
: Write a log of packets received to *path*.
The log is in JSON lines format.
Requires **\-d**.

**\-s**, **\-\-device\-speed** *bps*
: Set the speed of the serial port.
This causes speed detection not to be performed.
A speed of 0 uses the current speed of the serial port.
Requires **\-d**, and **\-p** or **\-\-packet\-log**.

**\-t**, **\-\-timeout** *seconds*
: Stop detecting PPS edges or capturing packets after *seconds*.
A value of 0 means run until interrupted.
The default is 10 with **\-p**, and 0 otherwise.
Requires **\-p**, or **\-s** and **\-\-packet\-log**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A port description object has `device` and `display` strings,
for a USB port a `usb` object with numeric `vid` and `pid` fields,
for a port with a USB serial number a `serial` string,
and, for a port with aliases, an `aliases` array of paths.
A detected speed object has a `device` string and a numeric `speed`.
With **\-p**, an edge object has a `device` string, an RFC 3339 UTC timestamp `t`,
and, when the **poll** method is used, optional fields `uncertainty` in seconds and `settling`.
A `settling` value of true means the accuracy of subsequent edges is still expected to improve, during acquisition or while the polling window recovers from missed pulses; it is omitted once `uncertainty` reflects the resolution the hardware can achieve.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no serial ports found, the **\-d** *path* matched no discovered port, no output received from the device, no packets captured, or no PPS edges detected

# EXAMPLES

List the serial ports:

    satpulsetool serial

List the serial ports as JSON Lines:

    satpulsetool serial --jsonl

Show information about one port:

    satpulsetool serial -i -d /dev/ttyUSB0

Detect the speed of a receiver:

    satpulsetool serial -d /dev/ttyUSB0

Detect the speed of every serial port:

    satpulsetool serial -a

Detect the speed of a receiver, logging the packets received during detection:

    satpulsetool serial -d /dev/ttyUSB0 --packet-log capture.jsonl

Capture packets for 30 seconds at 38400 bits per second:

    satpulsetool serial -d /dev/ttyUSB0 -s 38400 -t 30 --packet-log capture.jsonl

Capture packets at the port's current speed until interrupted:

    satpulsetool serial -d /dev/ttyUSB0 -s 0 --packet-log capture.jsonl

Watch for PPS pulses on the CTS pin of every serial port:

    satpulsetool serial -p cts -a

Monitor PPS edges on the CTS pin for 30 seconds at 38400 bits per second, logging received packets:

    satpulsetool serial -p cts -s 38400 -t 30 -d /dev/ttyUSB0 --packet-log capture.jsonl

Show receiver information at the detected speed:

    satpulsetool gps -d /dev/ttyUSB0 -s $(satpulsetool serial -d /dev/ttyUSB0) --show-receiver

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
