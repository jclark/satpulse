---
title: Verify serial connection to GPS module
---

There will be a serial connection between the host computer and the GPS module.
SatPulse needs to know the device name on the host computer, and also the speed (baud rate).

SatPulse can work with a 1-way connection where it only receives data from the GPS module.
But for configuration it needs a 2-way connection where it can also send data to the GPS module.

## Determining the device name

### Linux

The device name depends on how the GPS module is connected:

* a connection to the pins on a Raspberry Pi CM4/CM5 will usually be `/dev/ttyAMA0`
* a USB-to-RS232 or USB-to-TTL converter will usually show up as `/dev/ttyUSB0`, but occasionally may show up as `/dev/ttyACM0`
* an RS232 connection using a DB9 port on the PC will usually show up as `/dev/ttyS0`
* a GPS in M.2 slot will usually be `/dev/ttyACM0`
* a GPS with a USB connection will be either `/dev/ttyACM0` or `/dev/ttyUSB0`

You may need to change 0 in the device name to a larger number.

`/dev/ttyUSB0` and `/dev/ttyACM0` are USB devices and they exist in `/dev` only when they are connected,
so you can use `ls` to find what you have available.
When you plug such a device in, there will be a kernel log message, which you can see using `dmesg | tail`.

### macOS

On macOS, a USB serial device shows up as a pair of device nodes,
for example `/dev/cu.usbmodem11301` and `/dev/tty.usbmodem11301`;
the one intended for outbound use is `/dev/cu.*`.
You can use `ls /dev/cu.*` to see what you have.
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

## Determining the speed

You also need to know the speed the GPS module is using.
This will often be included in the product description when you buy the module.
The default speed for every GPS module I have seen (and I have dozens) is 9600, 38400 or 115200.
Older modules use 9600.
Some newer u-blox modules use 38400.
Modern Chinese modules that are not using u-blox chips, especially higher-end models, often use 115200.

If you are not sure, then just try each of 9600, 38400 and 115200 until you find one that works.

SatPulse assumes serial connection uses 8 data bits, no parity and 1 stop bit (called 8N1);
many modules support only this and it is the default on all modules.

My understanding is that if the device is /dev/ttyACM0 any speed should work.
But you should still set it to match the speed of the module.

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
