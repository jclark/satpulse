---
title: Raspberry Pi OS installation
---

<!-- From rpi-cm4-ptp-guide/os.md lines 3-10 -->
These instructions are for the Raspberry Pi OS Lite. Raspberry Pi OS used to be called Raspbian.

As of the time of writing (early 2025), the current version of Raspberry Pi OS is based on Debian Bookworm.
The legacy version of Raspberry Pi OS is based on Debian Bullseye.
The CM5 requires Bookworm.
We are using Raspberry Pi OS, since it is optimized for the Raspberry Pi hardware.
We are using the Lite version, since we do not need or want a desktop environment for this application. We are also using the 64-bit version, since this takes best advantage of the CM4 hardware (particularly with 8Gb RAM).

## OS installation

<!-- From rpi-cm4-ptp-guide/os.md lines 12-21 -->
Install Raspberry Pi OS Lite 64-bit.

If your CM4/CM5 has eMMC, follow these [instructions](https://www.raspberrypi.com/documentation/computers/compute-module.html#flashing-the-compute-module-emmc).
When using the Raspberry Pi Imager, select `Raspberry Pi OS (other)` and then  `Raspberry Pi OS Lite (64-bit)`.

TODO: installation without eMMC

TODO: What is the minimum amount of RAM for 64-bit to be a better choice than 32-bit? Not sure if 1Gb RAM would work better with a 32-bit OS. 

## OS configuration

<!-- From rpi-cm4-ptp-guide/os.md lines 23-37 -->
If you want to do this using SSH from your main machine, then

* run `raspi-config` to enable SSH (under Interfacing)
* find the current IP address using `ifconfig`

Update packages

```
sudo apt update
sudo apt upgrade
```

and reboot.

<!-- From rpi-cm4-ptp-guide/os.md lines 38-44 -->
On the CM5, it is necessary to have at least kernel version 6.12 for PTP hardware timestamping to work.
You can check your kernel version with `uname -r`.
If that says you are running something earlier than 6.12 (e.g. 6.6.x), then you will need to update to a more recent kernel.
At the time of writing (March 2025), this can be done using the command `sudo rpi-update`. See
this [forum thread](https://forums.raspberrypi.com/viewtopic.php?t=379745).
This is not necessary nor recommended for the CM4. 

<!-- From rpi-cm4-ptp-guide/os.md lines 45-50 -->
Run `raspi-config`:

* enable serial port (under Interface/Serial Port); answer
   * No to login shell accessible over serial
   * Yes to enable serial port hardware

<!-- From rpi-cm4-ptp-guide/os.md lines 51-63 -->
For the CM4, but not the CM5, add the following
at the end of `/boot/firmware/config.txt` (on Bullseye, it's `/boot/config.txt`).
This should not 

```
# realtime clock
dtoverlay=i2c-rtc,pcf85063a,i2c_csi_dsi
# fan
dtoverlay=i2c-fan,emc2301,i2c_csi_dsi
# Make /dev/ttyAMA0 be connected to GPIO header pins 8 and 10
# This always disables Bluetooth
dtoverlay=disable-bt
```

<!-- From rpi-cm4-ptp-guide/os.md lines 64-68 -->
Disable the system service that initialises the modem:
```
sudo systemctl disable hciuart
```

<!-- From rpi-cm4-ptp-guide/os.md lines 69-74 -->
Set the timezone:

```
sudo dpkg-reconfigure tzdata
```

<!-- From rpi-cm4-ptp-guide/os.md lines 75-80 -->
Use raspi-config to set
* wifi country (under System Options > Wireless LAN)
* hostname (under System Options)

Reboot.



## Verify OS setup


<!-- From rpi-cm4-ptp-guide/os.md lines 143-148 -->
Check the RTC

```
sudo hwclock --show
```

<!-- From rpi-cm4-ptp-guide/os.md lines 149-154 -->
Check that the current date is correct:

```
date
```

<!-- From rpi-cm4-ptp-guide/os.md lines 155-165 -->
Check support for the fan controller. Do

```
ls /sys/class/thermal
```

You should see:

```
cooling_device0  thermal_zone0
```