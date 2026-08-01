# NAME

satpulsetool-serial - list serial ports and detect their speeds

# SYNOPSIS

**satpulsetool** [*global options*] **serial** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-j**\|**\-\-jsonl**] [**\-s**\|**\-\-scan**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*] [*device*]

# DESCRIPTION

The **satpulsetool** **serial** command lists the serial ports of the host,
and detects the speed of a GPS receiver connected to one of them.

With no *device* and without **\-\-scan**, the command lists the serial ports it finds, one per line.
Each line is a display label beginning with the device path,
which may be followed by aliases and USB product information.
The ports are not opened, so this mode needs no permission to access them.

With a *device*, the command opens that path at the speed the port is already set to,
and determines the receiver's speed from the data that arrives.
Nothing is written to the receiver.
The detected speed in bits per second is written to standard output as a single integer,
so that it can be substituted into another command.
Detection succeeds only for a receiver sending packets of a GPS protocol that SatPulse recognizes;
a port carrying only RTCM corrections is not detected.
Ctrl-C interrupts detection.
The port settings in effect when the device was opened are restored before the command exits,
so the port is not left at the detected speed.
Opening a serial device usually requires membership of the `dialout` group.

With **\-\-scan**, every enumerated port is detected, in parallel.
Each detected port produces a line on standard output with the device path and the speed, separated by a space.
Every other port produces a line on standard error with the device path and a description.
A port that another program holds with a lock is reported as locked;
a port held without a lock is opened and probed like any other.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **serial** command.

**\-j**, **\-\-jsonl**
: Write one JSON object per port instead of a display label.
The object has `device` and `display` strings, and for a USB port a `usb` object with numeric `vid` and `pid` fields.
Cannot be combined with a *device* or **\-\-scan**.

**\-s**, **\-\-scan**
: Detect the speed of every enumerated serial port.
Each line is written as that port's detection finishes, so the order varies between runs.
The exit status reports the best outcome over all the ports.
Cannot be combined with a *device*.

**\-\-packet\-log** *path*
: Log to *path* a description of the packets received while detecting, including those received at the wrong speed.
The log is in `.jsonl` (JSON lines) format.
Requires a *device*.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no serial ports found, or no output received from the device

# EXAMPLES

List the serial ports:

    satpulsetool serial

List the serial ports as JSON Lines:

    satpulsetool serial --jsonl

Detect the speed of a receiver:

    satpulsetool serial /dev/ttyUSB0

Detect the speed of every serial port:

    satpulsetool serial --scan

Show receiver information at the detected speed:

    satpulsetool gps -d /dev/ttyUSB0 -s $(satpulsetool serial /dev/ttyUSB0) --show-receiver

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
