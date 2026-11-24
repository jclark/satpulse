# NAME

satpulsetool-gpio - list GPIO controllers and poll a GPIO for PPS edges

# SYNOPSIS

**satpulsetool** [*global options*] **gpio** [**\-h**\|**\-\-help**] [**\-j**\|**\-\-jsonl**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-pps**] [**\-\-cpu** *N*] [**\-\-priority** *N*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*] [*name*]

# DESCRIPTION

The **satpulsetool** **gpio** command lists the GPIO controllers that have a device-tree node, one per line.
For each controller, it shows the gpiochip, such as `gpiochip0`, and the compatible strings of its device-tree node.

For a controller of a supported SoC family, it also shows the family,
the device through which the controller's registers are read,
and the length of that device's mapping.
It also shows each GPIO of the controller that a `pps-gpio` device-tree node, such as the one the `pps-gpio` overlay creates, uses as a pulse-per-second (PPS) input.
The GPIO is shown by the name the SoC's documentation gives it, such as `GPIO18` on a Raspberry Pi,
followed by the kernel PPS device created for it, such as `/dev/pps0`, when that device can be found.
A *name* given to the command is a name of this form, not a header pin number, and is case-insensitive.
With a *name*, it shows only the controller that has that GPIO, and shows the GPIO only if it is a PPS input.
Listing reads only sysfs and needs no special privileges.

With the **\-p** option, it instead polls a GPIO for PPS edges, reading the GPIO controller's registers, and prints the time of each rising edge it catches.
It polls the GPIO called *name*, or, without a *name*, the one GPIO shown as a PPS input; it is an error if there is not exactly one.
The GPIO must already be an input, as the `pps-gpio` overlay makes it.
It changes nothing on the GPIO.
Polling requires read access to the device through which the controller's registers are read, which the `gpio` group has on Raspberry Pi OS.
It is most precise on a CPU that does not handle the GPIO's interrupt (see `/proc/interrupts`).

The supported SoC families are `bcm2835` (Raspberry Pi 3 and 4) and `rp1` (Raspberry Pi 5).
The command is supported on 64-bit Arm Linux.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **gpio** command.

**\-p**, **\-\-pps**
: Poll a GPIO for PPS edges and print the time of each rising edge.
The edge is located between the last read of the GPIO before it and the first read after it.

**\-\-cpu** *N*
: Run the poller on CPU *N*.
By default the poller is not pinned to a CPU.
Requires **\-p**.

**\-\-priority** *N*
: Run the poller at `SCHED_FIFO` real-time priority *N*, from 1 to 99.
The default is 0, which leaves it at normal priority.
Usually requires root privileges.
Requires **\-p**.

**\-t**, **\-\-timeout** *seconds*
: Stop polling after *seconds*.
A value of 0 means run until interrupted.
The default is 10.
Requires **\-p**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A controller object has a `chip` string and a `compatible` array of strings.
For a controller of a supported family, it also has a `family` string, a `device` string, a `length` number,
and, if any of its GPIOs is a PPS input, a `pps` array.
A `pps` object has a `gpio` string giving the GPIO's name, a `line` number giving its line on the controller,
and, when the kernel PPS device can be found, a `device` string.
With **\-p**, an edge object has a `gpio` string, an RFC 3339 UTC timestamp `t`,
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
: No data found: no GPIO controllers with a device-tree node found, or no PPS edges detected

# EXAMPLES

List the GPIO controllers:

    satpulsetool gpio

On a Raspberry Pi 5 with the `pps-gpio` overlay on GPIO 18, the first line of output is:

    chip=gpiochip0 compatible=raspberrypi,rp1-gpio family=rp1 device=/dev/gpiomem0 length=0x30000 pps=GPIO18:/dev/pps0

Show the controller that has GPIO18:

    satpulsetool gpio GPIO18

Poll the GPIO set up as a PPS input for 10 seconds:

    satpulsetool gpio -p

Poll GPIO18 for 150 seconds on CPU 3 at real-time priority 40, writing JSON Lines:

    sudo satpulsetool gpio -p GPIO18 --cpu 3 --priority 40 -t 150 -j

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**
