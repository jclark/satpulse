# NAME

satpulsetool-pps - examine kernel PPS devices and poll GPIOs for PPS edges

# SYNOPSIS

**satpulsetool** [*global options*] **pps** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-d**\|**\-\-pps\-device** *path*] [**\-g**\|**\-\-gpio** *name*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-cpu** *N*] [**\-\-priority** *N*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*] [**\-j**\|**\-\-jsonl**]

# DESCRIPTION

The **satpulsetool** **pps** command examines the kernel pulse-per-second (PPS) devices, such as `/dev/pps0`, through which the kernel reports the times of pulses on a GPIO pin, a serial modem-control line or a PTP hardware clock pin.

With the **\-d** option, it prints the timestamps captured by one device as they arrive.
With the **\-g** option, it instead polls a GPIO for PPS edges directly, reading the GPIO controller's registers, and prints the time of each edge it catches;
the GPIO must already be an input, as the `pps-gpio` overlay makes it.
Otherwise, it lists the PPS devices present, without opening them.
It changes nothing on the device or GPIO.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **pps** command.

**\-d**, **\-\-pps\-device** *path*
: Print the assert timestamps of the PPS device at *path* as they arrive.
An edge captured before the command started is not printed.
Opening the device usually requires root privileges.

**\-g**, **\-\-gpio** *name*
: Poll the GPIO called *name* for PPS edges and print the time of each rising edge.
*name* is the GPIO's name in the documentation of the board or SoC, such as GPIO18 on a Raspberry Pi, not a header pin number.
The value is case-insensitive.
The edge is located between the last read of the GPIO before it and the first read after it.
Supported on 64-bit Linux on a Raspberry Pi 3, 4 or 5, and usually requires root privileges.
Cannot be combined with **\-d**.

**\-\-cpu** *N*
: Run the poller on CPU *N*.
Choose one that does not handle the GPIO's interrupt (see `/proc/interrupts`).
By default the poller is not pinned to a CPU.
Requires **\-g**.

**\-\-priority** *N*
: Run the poller at `SCHED_FIFO` real-time priority *N*, from 1 to 99.
The default is 0, which leaves it at normal priority.
Requires **\-g**.

**\-t**, **\-\-timeout** *seconds*
: Stop printing timestamps or edges after *seconds*.
The exit status is 2 if none arrived, however the command ended.
A value of 0 means run until interrupted.
The default is 10.
Requires **\-d** or **\-g**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A device object has `device` and `name` strings,
a `sourcePath` string naming the source of the pulses when the kernel reports one, such as the serial port of a line discipline source,
a `capture` array of the edges the device captures (`assert`, `clear`),
and, for a device that can echo edges to an output, an `echo` array of the edges it can echo.
With **\-d**, a timestamp object has a `device` string, an RFC 3339 UTC timestamp `t` with nanoseconds,
and the device's sequence number `seq` for the edge.
With **\-g**, an edge object has a `gpio` string, a timestamp `t` as for **\-d**,
`uncertainty` and `pollWidths` arrays and an optional `reject` string.
`uncertainty` gives two durations in seconds: from the start of the read before the edge to `t`, and from `t` to the end of the read after the edge.
`pollWidths` gives the durations in seconds of those two reads.
A `reject` string gives the reason the edge is unsuitable for timing, as for **satpulsetool-serial(1)**.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no PPS devices present, or no timestamps received or edges detected

# EXAMPLES

List the PPS devices:

    satpulsetool pps

Print timestamps from `/dev/pps0` for 10 seconds:

    sudo satpulsetool pps -d /dev/pps0

Print timestamps as JSON Lines until interrupted:

    sudo satpulsetool pps -d /dev/pps0 -t 0 -j

Poll GPIO18 for 150 seconds on CPU 3 at real-time priority 40, writing JSON Lines:

    sudo satpulsetool pps -g GPIO18 --cpu 3 --priority 40 -t 150 -j

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**, **satpulsed(8)**
