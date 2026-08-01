# NAME

satpulsetool-serial - list serial ports and detect their speeds

# SYNOPSIS

**satpulsetool** [*global options*] **serial** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-j**\|**\-\-jsonl**] [**\-s**\|**\-\-scan**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*] [*device*]

# DESCRIPTION

The **satpulsetool** **serial** command lists serial ports and detects the serial speed of connected Global Navigation Satellite System (GNSS) receivers.
It has enumeration, single-device detection, and scan modes.

With no *device* and without **\-\-scan**, the command lists serial ports without opening them.
Human-readable output contains one display label per port.
The label starts with the canonical device name and may include aliases and USB product information.

With a *device*, the command opens that path at its current speed and detects the speed from received data.
It validates packets from known GNSS protocols and does not accept correction-only output as a detection.
On success it writes the detected speed in bits per second to standard output as a single integer.
On failure or interruption it restores the speed that was in effect when the device was opened.
Detection fails before changing the port when its current speed cannot be represented as a supported numeric speed, since that speed could not be restored safely.

The default candidate order is 38400, 9600, 115200, the current speed, 460800, 230400, 57600, 19200, 4800, and 921600 bits per second.
Native USB devices try 115200 first.
Each candidate is observed for up to 1.25 seconds.
The command stops after five candidates when every candidate has been silent.

With **\-\-scan**, the command detects all enumerated ports in parallel.
Each detected port produces one standard-output line containing its canonical device name, a space, and its detected speed.
Each unsuccessful port produces one standard-error line containing its canonical device name and a description.
Lines are written as each detection completes, so their order is not fixed.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **serial** command.

**\-j**, **\-\-jsonl**
: Write one JSON object per enumerated port instead of human-readable display labels.
Each object contains `device` and `display` strings.
USB ports also contain a `usb` object with numeric `vid` and `pid` fields.
Applies only to enumeration mode.

**\-s**, **\-\-scan**
: Detect the speed of every enumerated serial port in parallel.
Cannot be combined with a *device* or **\-\-jsonl**.

**\-\-packet\-log** *path*
: Write received packets and serial speed changes to a JSONL packet log.
Requires a *device* and cannot be used with **\-\-scan**.

# EXIT STATUS

**0**
: At least one port was listed or detected.

**1**
: An error occurred and no port was listed or detected.
This includes usage errors, permission errors, locked devices, non-serial devices, unsupported current speeds, interruptions, and output that did not validate at any candidate speed.

**2**
: No ports were found, or at least one probed port was silent and no port was detected.

For **\-\-scan**, the status represents the best result: detected, then silent, then error.

# EXAMPLES

List serial ports:

    satpulsetool serial

List serial ports as JSON Lines:

    satpulsetool serial --jsonl

Detect the speed of one receiver:

    satpulsetool serial /dev/ttyACM0

Use a detected speed with the **gps** command:

    satpulsetool gps -d /dev/ttyS0 -s $(satpulsetool serial /dev/ttyS0) --show-receiver

Detect all enumerated ports:

    satpulsetool serial --scan

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
