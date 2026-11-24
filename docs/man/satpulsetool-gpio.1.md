# NAME

satpulsetool-gpio - list GPIO controllers and poll a GPIO for PPS edges

# SYNOPSIS

**satpulsetool** [*global options*] **gpio** [**\-h**\|**\-\-help**] [**\-j**\|**\-\-jsonl**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-pps**] [**\-\-cpu** *N*] [**\-\-priority** *N*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*] [*name*]

# DESCRIPTION

The **satpulsetool** **gpio** command lists the GPIO controllers of the system's device tree:
the enabled device-tree nodes with the `gpio-controller` property.
For each controller, it shows the path of its node and the compatible strings of the node.
It also shows the gpiochips through which the kernel provides the controller's GPIOs, such as `gpiochip0`, when they can be found.
A node can have more than one gpiochip.

For a controller that one of its backends handles, it also shows the backend,
the device through which the controller's registers are read,
and the length of that device's mapping.
The backends are `bcm2835` (Raspberry Pi 3 and 4) and `rp1` (Raspberry Pi 5).

It also shows each GPIO of the controller that a `pps-gpio` device-tree node, such as the one the `pps-gpio` overlay creates, uses as a pulse-per-second (PPS) input,
with the kernel PPS device created for it, such as `/dev/pps0`, when that device can be found.
On a controller that a backend handles, the GPIO is shown by the name the SoC's documentation gives it, such as `GPIO18` on a Raspberry Pi, and its line on the controller.
On any other controller, or if the line is not one the backend names, it is shown by the cells of its GPIO specifier in the device tree, whose meaning the controller's device-tree binding defines.

Each controller is shown as an indented block.
On a Raspberry Pi 5 with the `pps-gpio` overlay on GPIO 18, the first controller is shown as:

    /axi/pcie@1000120000/rp1/gpio@d0000
      backend: rp1
      compatible: raspberrypi,rp1-gpio
      chips: gpiochip0
      device: /dev/gpiomem0
      length: 0x30000
      pps:
        gpio: GPIO18
        line: 18
        device: /dev/pps0

A *name* given to the command is a GPIO name of this form, not a header pin number, and is case-insensitive.
With a *name*, it shows only the controller that has that GPIO, and shows the GPIO only if it is a PPS input.
Listing needs no special privileges.

With the **\-p** option, it instead polls a GPIO for PPS edges, reading the GPIO controller's registers, and prints the time of each rising edge it catches.
It polls the GPIO called *name*, or, without a *name*, the one GPIO shown by name as a PPS input; it is an error if there is not exactly one.
The GPIO must already be an input, as the `pps-gpio` overlay makes it.
It changes nothing on the GPIO.
Polling requires read access to the device through which the controller's registers are read, which the `gpio` group has on Raspberry Pi OS.
It is most precise on a CPU that does not handle the GPIO's interrupt (see `/proc/interrupts`).

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
: Write output in JSON Lines format, one object per controller.
A controller object has a `node` string,
a `compatible` array of strings if the node has compatible strings,
and a `chips` array of strings if any gpiochips are found.
For a controller that a backend handles, it also has a `backend` string, a `device` string and a `length` number.
If any of its GPIOs is a PPS input, it has a `pps` array.
A `pps` object has a `gpio` string giving the GPIO's name and a `line` number giving its line on the controller,
or, where the GPIO is not shown by name, a `cells` array of numbers giving the cells of its GPIO specifier,
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
: No data found: no GPIO controllers found in the device tree, or no PPS edges detected

# EXAMPLES

List the GPIO controllers:

    satpulsetool gpio

List them in JSON Lines format:

    satpulsetool gpio -j

Show the controller that has GPIO18:

    satpulsetool gpio GPIO18

Poll the GPIO set up as a PPS input for 10 seconds:

    satpulsetool gpio -p

Poll GPIO18 for 150 seconds on CPU 3 at real-time priority 40, writing JSON Lines:

    sudo satpulsetool gpio -p GPIO18 --cpu 3 --priority 40 -t 150 -j

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**
