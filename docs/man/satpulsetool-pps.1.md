# NAME

satpulsetool-pps - examine kernel PPS devices

# SYNOPSIS

**satpulsetool** [*global options*] **pps** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-d**\|**\-\-pps\-device** *path*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*] [**\-j**\|**\-\-jsonl**]

# DESCRIPTION

The **satpulsetool** **pps** command examines the kernel pulse-per-second (PPS) devices, such as `/dev/pps0`, through which the kernel reports the times of pulses on a GPIO pin, a serial modem-control line or a PTP hardware clock pin.

With the **\-d** option, it prints the timestamps captured by one device as they arrive.
Otherwise, it lists the PPS devices present, without opening them.
It changes nothing on the device.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **pps** command.

**\-d**, **\-\-pps\-device** *path*
: Print the assert timestamps of the PPS device at *path* as they arrive.
An edge captured before the command started is not printed.
Opening the device usually requires root privileges.

**\-t**, **\-\-timeout** *seconds*
: Stop printing timestamps after *seconds*.
A value of 0 means run until interrupted.
The default is 10.
Requires **\-d**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A device object has `device` and `name` strings,
a `path` string naming the source of the pulses when the kernel reports one,
a `capture` array of the edges the device captures (`assert`, `clear`),
and, for a device that can echo edges to an output, an `echo` array of the edges it can echo.
With **\-d**, a timestamp object has a `device` string, an RFC 3339 UTC timestamp `t` with nanoseconds,
and the device's sequence number `seq` for the edge.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no PPS devices present, or no timestamps received before the timeout

# EXAMPLES

List the PPS devices:

    satpulsetool pps

Print timestamps from `/dev/pps0` for 10 seconds:

    sudo satpulsetool pps -d /dev/pps0

Print timestamps as JSON Lines until interrupted:

    sudo satpulsetool pps -d /dev/pps0 -t 0 -j

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**, **satpulsed(8)**
