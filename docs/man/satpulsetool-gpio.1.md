# NAME

satpulsetool-gpio - list GPIO controllers and the GPIOs set up as PPS inputs

# SYNOPSIS

**satpulsetool** [*global options*] **gpio** [**\-h**\|**\-\-help**] [**\-j**\|**\-\-jsonl**]

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

The command needs no special privileges.
It is supported on 64-bit Arm Linux.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **gpio** command.

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

# EXIT STATUS

**0**
: Success

**1**
: Error

**2**
: No data found: no GPIO controllers found in the device tree

# EXAMPLES

List the GPIO controllers:

    satpulsetool gpio

List them in JSON Lines format:

    satpulsetool gpio -j

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-serial(1)**, **satpulsetool-sdp(1)**
