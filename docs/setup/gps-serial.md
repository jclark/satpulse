---
title: Serial connection setup
---

## Required information

<!-- From quickstart.md lines 59-73 -->
The GPS receiver must have two connections to the computer on which SatPulse is running.

First, there will be a serial connection between the USB or serial port on the GPS receiver to a USB or serial port on the computer.
You need to know which device this is. On a Raspberry Pi CM4 it is typically `/dev/ttyAMA0`.
On a PC, it might be `/dev/ttyAMA0` or `/dev/ttyACM0` or `/dev/ttyUSB0`, with possibly a higher number than `0`.
You also need to know the speed the GPS receiver is using; 9600 is the most common speed, but some recent devices use 38400.

Second, there will be a PPS connection between the PPS output of the GPS receiver and the pin on the ethernet controller.
You need to know the name of the ethernet interface. On a Raspberry Pi CM4 using Raspberry Pi OS, it is typically `eth0`.
On a PC, it is typically something like `enp1s0` or `enp2s0`.
You also need to know the index of the pin; this is usually 0, but on a PC, could also be 1 or 2.

If you put the wrong values for these, there will be an error in the logs.

## Verify GPS connection

<!-- From rpi-cm4-ptp-guide/os.md lines 167-173 -->
You should verify that the GPS is properly connected. There are two connections

- the serial connection
- the PPS connection

### Serial connection

<!-- From rpi-cm4-ptp-guide/os.md lines 175-193 -->
Assuming you have specified `dtoverlay=disable-bt` above and you have connected the GPS
RX (white) and TX (green) pins to pins 8 and 10 respectively on the J8 HAT connector,
then the serial device will be `/dev/ttyAMA0`.

Do:

```
(stty 9600 -echo -icrnl; cat) </dev/ttyAMA0
```

Here 9600 is the speed. The most common default speed is 9600, but some receivers default to 38400 or 115200.

You should see  lines starting with `$`.
In particular look for a line starting with `$GPRMC` or `$GNRMC`. The number following that should be the current UTC time;
for example, `025713.00` means `02:57:13.00` UTC.
After another 8 commas, there will be a field that should have the current UTC date;
for example, `140923` means 14th Septemember 2023.

## PC-specific serial verification

<!-- From pc-ptp-ntp-guide/server-linux.md lines 67-98 -->
First determine the serial device name:

* a USB-RS232 or USB-TTL converter will usually show up as`/dev/ttyUSB0`, but occasionally may show up as `/dev/ttyACM0`
* an RS232 connection using a DB9 port on the PC will usually show up as `/dev/ttyS0`
* a GPS in M.2 slot will usually be `/dev/ttyACM0`
* a GPS with a USB connection will be either `/dev/ttyACM0` or `/dev/ttyUSB0`

Obviously you may need to change 0 in the device name to a larger number, if you have multiple such devices.

It's convenient if you are in the `dialout` group so you can access the serial devices

```
sudo usermod -G dialout -a jjc
```

Here `jjc` is your username. You'll need to logout and then login again for this to take effect.

Then do:

```
(stty 9600 -echo -icrnl; cat) </dev/ttyUSB0
```

Here 9600 is the speed. The most common default speed is 9600, but some receivers default to 38400.

You should see  lines starting with `$`.
In particular look for a line starting with `$GPRMC` or `$GNRMC`. The number following that should be the current UTC time;
for example, `025713.00` means `02:57:13.00` UTC.
After another 8 commas, there will be a field that should have the current UTC date;
for example, `140923` means 14th Septemember 2023.

## Advanced: ser2net for remote access

<!-- From rpi-cm4-ptp-guide/gps-config.md lines 19-37 -->
My preferred solution is to use `ser2net` on the CM4 to make the serial connection available on a TCP port. You can then connect to that TCP port using u-center from a Windows machine.  To do this


```
sudo apt install ser2net
```

Then create a new file (or replace existing file) `/etc/ser2net.yaml` containing:

```
---
connection: &con0
    accepter: tcp,2002
    options:
        kickolduser: true
    enable: on
    connector: serialdev,/dev/ttyAMA0,9600n81,local
```

In the above, you will need to change 9600 to whatever your GPS module expects.