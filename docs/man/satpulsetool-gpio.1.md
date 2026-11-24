# NAME

satpulsetool-gpio - examine PPS on GPIO pins

# SYNOPSIS

**satpulsetool** [*global options*] **gpio** [**\-h**\|**\-\-help**] [**\-j**\|**\-\-jsonl**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-pps**] [**\-\-cpu** *N*] [**\-\-priority** *N*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-timeout** *seconds*] [*name*]

# DESCRIPTION

The **satpulsetool** **gpio** command can perform the following operations related to a pulse-per-second (PPS) signal connected to a GPIO pin:

* show which GPIOs are set up as PPS inputs;
* detect PPS edges on a GPIO by polling it.

A GPIO is set up as a PPS input by a `pps-gpio` device-tree node, such as the one the `pps-gpio` overlay creates on a Raspberry Pi.
The kernel then provides the pulses through a kernel PPS device, such as `/dev/pps0`.

By default, the command shows the GPIO controllers of the system and the PPS inputs on each.
A PPS input is shown by the name of its GPIO and its kernel PPS device.
For example, on a Raspberry Pi 5 with the `pps-gpio` overlay on GPIO 18, the output starts:

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

The other lines of a controller's entry are mainly useful in problem reports.
On a controller that does not support polling, a PPS input is shown by the numbers that identify its GPIO in the device tree.
With a *name*, such as **GPIO18**, the command shows only the controller of that GPIO.

With **\-p**, the command instead detects PPS edges on a GPIO by repeatedly reading its level, and prints the time of each rising edge.
It polls the GPIO *name*, or, without a *name*, the only GPIO set up as a PPS input.
The GPIO must already be an input, as the `pps-gpio` overlay makes it; the command does not change it.
Polling works on the Raspberry Pi 3, 4 and 5.
It needs read access to `/dev/gpiomem` (Raspberry Pi 3 and 4) or `/dev/gpiomem0` (Raspberry Pi 5), which the `gpio` group has on Raspberry Pi OS.
The times are most precise when the poller runs at real-time priority on a CPU that does not handle the GPIO's interrupt, as listed in `/proc/interrupts`.

The command is available only on 64-bit Arm Linux.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **gpio** command.

**\-p**, **\-\-pps**
: Detect PPS edges on the GPIO instead of showing GPIO controllers.

**\-\-cpu** *N*
: Run the poller on CPU *N*.
Requires **\-p**.

**\-\-priority** *N*
: Run the poller at `SCHED_FIFO` real-time priority *N*, from 1 to 99.
Usually requires root privileges.
Requires **\-p**.

**\-t**, **\-\-timeout** *seconds*
: Stop polling after *seconds*.
A value of 0 means run until interrupted.
The default is 10.
Requires **\-p**.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A controller object has a `node` string, optional `compatible` and `chips` arrays,
for a controller that supports polling `backend` and `device` strings and a numeric `length`,
and, if any of its GPIOs is a PPS input, a `pps` array.
A `pps` object has a `gpio` string and a numeric `line`,
or, where the GPIO is not shown by name, a `cells` array of numbers,
and a `device` string if its kernel PPS device is found.
With **\-p**, an edge object has a `gpio` string, an RFC 3339 UTC timestamp `t`,
`uncertainty` and `pollWidths` arrays and an optional `reject` string,
with the same meanings as for **satpulsetool-serial(1)**.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no GPIO controllers found, or no PPS edges detected

# EXAMPLES

Show the GPIOs set up as PPS inputs:

    satpulsetool gpio

Detect PPS edges on the GPIO set up as a PPS input:

    satpulsetool gpio -p

Detect PPS edges on GPIO18 for 150 seconds, on CPU 3 at real-time priority 40:

    sudo satpulsetool gpio -p GPIO18 --cpu 3 --priority 40 -t 150

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**
