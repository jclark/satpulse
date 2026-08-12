# NAME

satpulsetool-serial - describe serial ports and detect GPS receiver serial speeds

# SYNOPSIS

**satpulsetool** [*global options*] **serial** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-j**\|**\-\-jsonl**] [**\-s**\|**\-\-detect\-speed**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*] [*port*]

# DESCRIPTION

The **satpulsetool** **serial** command reports on the serial ports of the host.
By default it discovers all serial ports and prints a line describing each discovered port on standard output;
it does not open the serial ports.
If the *port* argument is supplied, it describes only the specified port.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **serial** command.

**\-s**, **\-\-detect\-speed**
: Detect the speed of a connected GPS receiver from the data being emitted by the receiver.
If the *port* argument is supplied, it detects the speed of only that port and prints its speed on standard output.
Otherwise, it detects the speed of all discovered ports in parallel and prints a line for each port.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A port description object has `device` and `display` strings,
for a USB port a `usb` object with numeric `vid` and `pid` fields,
for a port with a USB serial number a `serial` string,
and, for a port with aliases, an `aliases` array of paths.
A detected speed object has a `device` string and a numeric `speed`.

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
: No data found: no serial ports found, the *port* selector matched no discovered port, or no output received from the device

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

Show receiver information at the detected speed:

    satpulsetool gps -d /dev/ttyUSB0 -s $(satpulsetool serial -s /dev/ttyUSB0) --show-receiver

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
