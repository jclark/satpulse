---
title: Verify serial connection to GPS module
---

There will be a serial connection between the host computer and the GPS module.
SatPulse needs to know the device name on the host computer, and also the speed (baud rate).

SatPulse can work with a 1-way connection where it only receives data from the GPS module.
But for configuration it needs a 2-way connection where it can also send data to the GPS module.

## Determining the device name and speed

`satpulsetool serial` can find both the device name and the speed. {% include new-in-03.html %}
It only reads from the serial port; it never sends anything to the GPS module.

Run it with no arguments to list the serial ports:

```
$ satpulsetool serial
device=/dev/ttyACM0 vid=1546 pid=01a9 display="/dev/ttyACM0 (u-blox gen 9)"
device=/dev/ttyS0 display="/dev/ttyS0"
```

This lists the serial ports without opening them.
For a USB device, it shows the USB vendor and product IDs and the product name,
so the GPS is usually identifiable at a glance;
a u-blox receiver connected directly by USB is identified by its generation.

Then run it with `-d` and the device name to detect the speed:

```
$ satpulsetool serial -d /dev/ttyACM0
38400
```

It reads from the port at each of the usual speeds until it recognizes GPS output, and prints the speed it found.

If you cannot tell from the listing which port the GPS is on, use `-a` (for all) instead of `-d`:

```
$ satpulsetool serial -a
/dev/ttyACM0 38400
/dev/ttyS0: no output received from the device
```

This does the same for every port at the same time.
For each port on which it finds a GPS, it prints the device name and the speed:
here there is a GPS on `/dev/ttyACM0` running at 38400.
A port on which nothing was received is reported too.
A port locked by another program, such as satpulsed or gpsd, is reported as such and not disturbed.

The default speed for every GPS module I have seen (and I have dozens) is 9600, 38400 or 115200.
Older modules use 9600.
Some newer u-blox modules use 38400.
Modern Chinese modules that are not using u-blox chips, especially higher-end models, mostly use 115200.

SatPulse assumes serial connection uses 8 data bits, no parity and 1 stop bit (called 8N1);
many modules support only this and it is the default on all modules.

A native USB port on a GPS module does not have a serial speed.
It will typically appear as a `/dev/ttyACM0` device,
and the speed you specify for that device does not affect the speed at which the connection operates.

### Linux

The device name depends on how the GPS module is connected:

* a connection to the pins on a Raspberry Pi CM4/CM5 will usually be `/dev/ttyAMA0`
* a USB-to-RS232 or USB-to-TTL converter will usually show up as `/dev/ttyUSB0`, but occasionally may show up as `/dev/ttyACM0`
* an RS232 connection using a DB9 port on the PC will usually show up as `/dev/ttyS0`
* a GPS in M.2 slot will usually be `/dev/ttyACM0`
* a GPS with a USB connection will be either `/dev/ttyACM0` or `/dev/ttyUSB0`

You may need to change 0 in the device name to a larger number.

USB devices such as `/dev/ttyUSB0` and `/dev/ttyACM0` exist in `/dev` only while they are connected.
When you plug such a device in, there will be a kernel log message, which you can see using `dmesg | tail`.

### macOS

On macOS, a USB serial device shows up as a pair of device nodes,
for example `/dev/cu.usbmodem11301` and `/dev/tty.usbmodem11301`;
the one intended for outbound use is `/dev/cu.*`.
The device name changes depending on which USB port or hub the device is plugged into,
and macOS has no equivalent of the stable device names that udev provides on Linux.
SatPulse provides `find-serial` for dealing with this:
a macOS-specific utility written in C, which is included in the Homebrew tap. {% include new-in-03.html %}

Run `find-serial` with no arguments to show the USB serial devices currently plugged in.
It will print something like

```
device=/dev/cu.usbmodem11301 vid=1546 pid=01A9 model="u-blox GNSS receiver" vendor="u-blox AG - www.u-blox.com"
```

`model` and `vendor` are the device's own USB strings, so the GPS is usually identifiable at a glance.
If you have more than one USB serial device plugged in,
the `--vid` and `--pid` options (hexadecimal) narrow the matches by USB vendor and product ID.

With `--exec`, find-serial instead runs a command,
replacing one `{}` argument with the matched device path
(this requires exactly one device to match).
This means you do not have to look up the device name at all:

```
find-serial --exec -- satpulsetool gps -s 9600 -d {}
```

Adding `--wait` makes find-serial wait for a matching device to appear
(using hot-plug notifications, not polling)
instead of failing when none is present,
so you can run the command first and plug the receiver in afterwards.

The service installed by the Homebrew tap uses find-serial in the same way
to locate the GPS receiver when it starts,
so on macOS the device does not normally need to be configured in `satpulse.toml`.

### Windows

On Windows, use `COM1`, `COM2` etc. as serial port names.

## Serial device permissions

Typically on Linux, serial devices are configured to allow access by anybody in the `dialout` group.

It's convenient if you are in the `dialout` group so you can access the serial devices

```
sudo usermod -G dialout -a jjc
```

Here `jjc` is your username. You'll need to logout and then login again for this to take effect.

On macOS, no group membership or other setup is needed to access serial devices.

## Verifying with satpulsetool

Do, for example:

```
satpulsetool gps -s 9600 -d /dev/ttyAMA0
```

where 9600 is the speed and `/dev/ttyAMA0` is the device name.

With just those arguments, `satpulsetool gps` will read packets from the GPS,
detect what kind of receiver it is,
and probe whether it supports one of the high-level configuration protocols.

If it detects a GPS, it will tell you some information about what packets it received,
with more details if a probe succeeded.
