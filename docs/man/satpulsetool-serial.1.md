# NAME

satpulsetool-serial - describe serial ports and detect receiver speeds

# SYNOPSIS

**satpulsetool** [*global options*] **serial** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-j**\|**\-\-jsonl**] [**\-s**\|**\-\-detect\-speed**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*] [*port*]

# DESCRIPTION

The **satpulsetool** **serial** command reports on the serial ports of the host.
It has two modes.
Without **\-\-detect\-speed**, it describes the ports it discovers, without opening them.
With **\-\-detect\-speed**, it opens ports and detects the speed of a connected GPS receiver from the data the receiver sends.
In either mode, the optional *port* operand selects a single port;
without it, the command covers every discovered port.

## Describing ports

Each discovered port produces one line on standard output.
The line is a display label beginning with the device path,
which may be followed by aliases and USB product information.
When a USB serial number is available, the label is followed by `serial=` and the quoted serial number.
**\-\-jsonl** replaces the labels with JSON objects.
Since the ports are not opened, this mode needs no permission to access them.

With a *port*, only the selected port is described.
The selector is matched against each discovered port's device path and aliases,
first as given and then with symlinks resolved,
so a path like `/dev/serial/by-id/...` selects the port it points to.
A selector that matches no discovered port is an error.

## Detecting speeds

Detection opens a port at the speed the port is already set to,
and determines the receiver's speed from the data that arrives.
Nothing is written to the receiver.
Detection succeeds only for a receiver sending packets of a GPS protocol that SatPulse recognizes;
a port carrying only RTCM corrections is not detected.
Ctrl-C interrupts detection.
The port settings in effect when the port was opened are restored before the command exits,
so the port is not left at the detected speed.
Opening a serial port usually requires membership of the `dialout` group.

With a *port*, exactly the path given is opened, whether or not it is a discovered port,
so a symlink like `/dev/serial/by-id/...`, or a port that discovery misses, can be detected.
The detected speed in bits per second is written to standard output as a single integer,
so that it can be substituted into another command.

Without a *port*, every discovered port is opened, in parallel.
Each detected port produces a line on standard output with the device path and the speed, separated by a space.
Each line is written as that port's detection finishes, so the order varies between runs.
Every other port produces a line on standard error with the device path and a description.
A port that another program holds with a lock is reported as locked;
a port held without a lock is opened and probed like any other.
The exit status reports the best outcome over all the ports.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **serial** command.

**\-j**, **\-\-jsonl**
: Write one JSON object per described port instead of a display label.
The object has `device` and `display` strings,
for a USB port a `usb` object with numeric `vid` and `pid` fields,
for a port with a USB serial number a `serial` string,
and, for a port with aliases, an `aliases` array of paths.
Cannot be combined with **\-\-detect\-speed**.

**\-s**, **\-\-detect\-speed**
: Detect receiver speeds instead of describing ports.
Cannot be combined with **\-\-jsonl**.

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
