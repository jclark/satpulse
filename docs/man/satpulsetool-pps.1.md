# NAME

satpulsetool-pps - examine kernel PPS devices and poll GPIO pins for PPS edges

# SYNOPSIS

**satpulsetool** [*global options*] **pps** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-d**\|**\-\-pps\-device** *path*] [**\-g**\|**\-\-gpio\-pin** *N*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-cpu** *N*] [**\-\-priority** *N*] [**\-\-max\-bracket** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*] [**\-j**\|**\-\-jsonl**]

# DESCRIPTION

The **satpulsetool** **pps** command examines the kernel pulse-per-second (PPS) devices, such as `/dev/pps0`, through which the kernel reports the times of pulses on a GPIO pin, a serial modem-control line or a PTP hardware clock pin.

With the **\-d** option, it prints the timestamps captured by one device as they arrive.
With the **\-g** option, it instead polls a Raspberry Pi GPIO pin for PPS edges directly, as **satpulsed** does with the `[pps]` table, and prints the time of each edge it catches;
the pin must already be an input, as the `pps-gpio` overlay makes it.
Otherwise, it lists the PPS devices present, without opening them.
It changes nothing on the device or pin.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **pps** command.

**\-d**, **\-\-pps\-device** *path*
: Print the assert timestamps of the PPS device at *path* as they arrive.
An edge captured before the command started is not printed.
Opening the device usually requires root privileges.

**\-g**, **\-\-gpio\-pin** *N*
: Poll GPIO *N* for PPS edges and print the time of each rising edge.
*N* is the GPIO number, not the header pin number.
The edge is located between the last read of the pin before it and the first read after it; the time printed is the midpoint of the two.
Currently supported on 64-bit Linux on a Raspberry Pi only.
Cannot be combined with **\-d**.

**\-\-cpu** *N*
: Run the poller on CPU *N*.
Choose one that does not handle the pin's interrupt (see `/proc/interrupts`).
By default the poller is not pinned to a CPU.
Requires **\-g**.

**\-\-priority** *N*
: Run the poller at `SCHED_FIFO` real-time priority *N*, from 1 to 99.
The default is 0, which leaves it at normal priority.
Requires **\-g**.

**\-\-max\-bracket** *seconds*
: Report an edge whose two bracketing reads are more than *seconds* apart as settling rather than settled, since an interruption between the reads mislocates it.
The default is 5e-6; 0 reports every edge as the poller finds it.
Requires **\-g**.

**\-t**, **\-\-timeout** *seconds*
: Stop printing timestamps or edges after *seconds*.
A value of 0 means run until interrupted.
The default is 10.
Requires **\-d** or **\-g**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A device object has `device` and `name` strings,
a `path` string naming the source of the pulses when the kernel reports one,
a `capture` array of the edges the device captures (`assert`, `clear`),
and, for a device that can echo edges to an output, an `echo` array of the edges it can echo.
With **\-d**, a timestamp object has a `device` string, an RFC 3339 UTC timestamp `t` with nanoseconds,
and the device's sequence number `seq` for the edge.
With **\-g**, an edge object has a numeric `gpio`, a timestamp `t` as for **\-d**,
and optional fields `uncertainty` in seconds and `settling`.
`uncertainty` is half the interval between the two reads bracketing the edge.
A `settling` value of true means the edge is not to be relied on: the accuracy of subsequent edges is still expected to improve, during acquisition or while the polling window recovers from missed pulses, or this edge's bracket exceeded **\-\-max\-bracket**.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no PPS devices present, or no timestamps received or edges detected before the timeout

# EXAMPLES

List the PPS devices:

    satpulsetool pps

Print timestamps from `/dev/pps0` for 10 seconds:

    sudo satpulsetool pps -d /dev/pps0

Print timestamps as JSON Lines until interrupted:

    sudo satpulsetool pps -d /dev/pps0 -t 0 -j

Poll GPIO 18 for 30 seconds on CPU 3 at real-time priority, as **satpulsed** would with the same `[sample.pps.gpio]` settings:

    sudo satpulsetool pps -g 18 --cpu 3 --priority 40 -t 30

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**, **satpulsed(8)**
