# NAME

satpulsetool-serial - discover and analyze serial ports

# SYNOPSIS

**satpulsetool** [*global options*] **serial** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-j**\|**\-\-jsonl**] [**\-s**\|**\-\-detect\-speed**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-detect\-pps** *cts*\|*dcd*\|*dsr*\|*ri*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-poll**] [**\-\-speed** *baud*] [**\-t**\|**\-\-timeout** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*] [*port*]

# DESCRIPTION

The **satpulsetool** **serial** command reports on the serial ports of the host.
By default it discovers all serial ports and prints a line describing each discovered port on standard output;
it does not open the serial ports.
If the *port* argument is supplied, it describes only the specified port.
With **\-\-detect\-pps**, it instead opens the specified *port* and prints the wall-clock timestamp of each pulse-per-second (PPS) edge detected on the selected modem-control pin.
Each timestamp is shown as UTC time of day with microsecond precision.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **serial** command.

**\-s**, **\-\-detect\-speed**
: Detect the speed of a connected GPS receiver from the data being emitted by the receiver.
If the *port* argument is supplied, it detects the speed of only that port and prints its speed on standard output.
Otherwise, it detects the speed of all discovered ports in parallel and prints a line for each port.

**\-p**, **\-\-detect\-pps** *cts*\|*dcd*\|*dsr*\|*ri*
: Detect PPS edges on the specified serial modem-control input pin and print each edge's wall-clock timestamp.
The *port* argument is required.
This option is mutually exclusive with **\-\-detect\-speed** and **\-\-packet\-log**.
On platforms where edge detection is implemented by polling, edges are reported while polling adapts to the port.
Early timestamps can therefore have greater uncertainty.

**\-\-poll**
: Force polling for PPS edge detection even when the platform can wait for modem-control changes.
Applies only with **\-\-detect\-pps**.

**\-\-speed** *baud*
: Set the serial-port speed while detecting PPS so that receiver traffic flows during the measurement.
A value of 0 leaves the speed unchanged; the default is 0.
Applies only with **\-\-detect\-pps**.

**\-t**, **\-\-timeout** *seconds*
: Stop detecting PPS after this many seconds.
The default is 10; 0 runs until interrupted.
Applies only with **\-\-detect\-pps**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A port description object has `device` and `display` strings,
for a USB port a `usb` object with numeric `vid` and `pid` fields,
for a port with a USB serial number a `serial` string,
and, for a port with aliases, an `aliases` array of paths.
A detected speed object has a `device` string and a numeric `speed`.
With **\-\-detect\-pps**, each edge object has an RFC 3339 UTC timestamp `t`,
an optional polling `uncertainty` in seconds, and an optional `settling` boolean.

**\-\-packet\-log** *path*
: Log to *path* a description of the packets received while detecting, including those received at the wrong speed.
The log is in `.jsonl` (JSON lines) format.
Requires **\-\-detect\-speed** and a *port*.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no serial ports found, the *port* selector matched no discovered port, no output was received from the device, or no PPS edge was detected

# EXAMPLES

List the serial ports:

    satpulsetool serial

List the serial ports as JSON Lines:

    satpulsetool serial --jsonl

Describe one port:

    satpulsetool serial /dev/ttyUSB0

Detect the speed of a receiver:

    satpulsetool serial -s /dev/ttyUSB0

Detect the speed of every serial port:

    satpulsetool serial --detect-speed

Monitor PPS edges on the CTS pin for 30 seconds while receiving at 38400 baud:

    satpulsetool serial -p cts --speed 38400 --timeout 30 /dev/ttyUSB0

Show receiver information at the detected speed:

    satpulsetool gps -d /dev/ttyUSB0 -s $(satpulsetool serial -s /dev/ttyUSB0) --show-receiver

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
