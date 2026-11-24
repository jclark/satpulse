# NAME

satpulsetool-gpio - list GPIO controllers and the GPIOs set up as PPS inputs

# SYNOPSIS

**satpulsetool** [*global options*] **gpio** [**\-h**\|**\-\-help**] [**\-j**\|**\-\-jsonl**]

# DESCRIPTION

The **satpulsetool** **gpio** command lists the GPIO controllers that have a device-tree node, one per line.
For each controller, it shows the gpiochip, such as `gpiochip0`, and the compatible strings of its device-tree node.

For a controller of a supported SoC family, it also shows the family,
the device through which the controller's registers are read,
and the length of that device's mapping.
It also shows each GPIO of the controller that a `pps-gpio` device-tree node, such as the one the `pps-gpio` overlay creates, uses as a pulse-per-second (PPS) input.
The GPIO is shown by the name the SoC's documentation gives it, such as `GPIO18` on a Raspberry Pi,
followed by the kernel PPS device created for it, such as `/dev/pps0`, when that device can be found.

The supported SoC families are `bcm2835` (Raspberry Pi 3 and 4) and `rp1` (Raspberry Pi 5).
The command needs no special privileges.
It is supported on 64-bit Arm Linux.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **gpio** command.

**\-j**, **\-\-jsonl**
: Write output in JSON Lines format.
A controller object has a `chip` string and a `compatible` array of strings.
For a controller of a supported family, it also has a `family` string, a `device` string, a `length` number,
and, if any of its GPIOs is a PPS input, a `pps` array.
A `pps` object has a `gpio` string giving the GPIO's name, a `line` number giving its line on the controller,
and, when the kernel PPS device can be found, a `device` string.

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no GPIO controllers with a device-tree node found

# EXAMPLES

List the GPIO controllers:

    satpulsetool gpio

On a Raspberry Pi 5 with the `pps-gpio` overlay on GPIO 18, the first line of output is:

    chip=gpiochip0 compatible=raspberrypi,rp1-gpio family=rp1 device=/dev/gpiomem0 length=0x30000 pps=GPIO18:/dev/pps0

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**
