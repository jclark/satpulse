---
title: Raspberry Pi OS installation
---

These instructions are for the Raspberry Pi OS Lite. Raspberry Pi OS used to be called Raspbian.

As of the time of writing (mid-September 2025), the current version of Raspberry Pi OS is based on Debian Bookworm.
Debian Trixie, the next version of Debian, has been released; a release of Raspberry Pi OS based on Debian Trixie
is expected in a few months.
The CM5 requires at least Bookworm.
I recommend Raspberry Pi OS, since it is optimized for the Raspberry Pi hardware.
I recommend using the Lite version, since the desktop environment is not needed for this application.
On the CM4, we are also using the 64-bit version, since this takes best advantage of the CM4 hardware.
On the CM5, there is only a 64-bit version.

## OS installation

Install Raspberry Pi OS Lite 64-bit.

If your CM4/CM5 has eMMC, follow these [instructions](https://www.raspberrypi.com/documentation/computers/compute-module.html#flashing-the-compute-module-emmc).
When using the Raspberry Pi Imager, select `Raspberry Pi OS (other)` and then `Raspberry Pi OS Lite (64-bit)`.

TODO: installation without eMMC

TODO: What is the minimum amount of RAM for 64-bit to be a better choice than 32-bit? Not sure if 1Gb RAM would work better with a 32-bit OS. 

## Using full version of Raspberry Pi OS

If you want to have a graphical desktop as well, then you can install the full version of Raspberry Pi OS.
In this case, I recommend that after installation you use raspi-config to configure it to boot into the command line,
and then explicitly start a desktop when you need it, by doing:

```
sudo systemctl start graphical.target
```

If you want to switch back to text, connect over ssh and do:

```
sudo systemctl isolate multi-user.target
```

## Managing OS versions

SatPulse and PTP use some relatively obscure parts of the kernel, which get broken from time to time.

You can check your kernel version with `uname -r`. On a CM4, you will see something like

```
6.12.20+rpt-rpi-v8
```

On a CM5, you will see something like

```
6.12.20+rpt-rpi-2712
```

The first part, `6.12.20`, is the upstream Linux kernel on which this is based.
The 'rpt' part indicates this is a kernel done by the Raspberry Pi Team.
The `rpi-v8` part refers to the 64-bit kernel build for the Pi 4 and CM4, which use the ARMv8 architecture;
the `rpi-2712` part refers to the build for the Pi 5 and CM5, which use the BCM2712 SoC.

On the CM5, it is necessary to have at least kernel version 6.12 for PTP hardware timestamping to work.

At the time of writing, the current version of the kernel shipped by Raspberry Pi is 6.12.34;
unfortunately PTP Hardware Clock support is completely broken in this version.
You can install an apt preferences file that will prevent this from being used with the following command.

```
kver=6.12.34; outfile="/etc/apt/preferences.d/block-kernel-${kver}.pref"; \
for rpi in v8 2712; do \
  for pkg in image headers dtb; do \
    printf "Package: linux-%s-%s+rpt-rpi-%s\nPin: version 1:%s-*\nPin-Priority: -1\n\n" \
      "$pkg" "$kver" "$rpi" "$kver"; \
    printf "Package: linux-%s-rpi-%s\nPin: version 1:%s-*\nPin-Priority: -1\n\n" \
      "$pkg" "$rpi" "$kver"; \
  done; \
done | sudo tee "$outfile" >/dev/null
```

I have been using 6.12.20 for a while and I know that this works well.
If you are running 6.12.34 and want to switch to 6.12.20,
first install that using this command:

```
kver=6.12.20; pver="1:${kver}-1+rpt1"; \
sudo apt-get install \
  linux-image-${kver}+rpt-rpi-v8=${pver} linux-headers-${kver}+rpt-rpi-v8=${pver} \
  linux-image-${kver}+rpt-rpi-2712=${pver} linux-headers-${kver}+rpt-rpi-2712=${pver} \
  linux-headers-${kver}+rpt-common-rpi=${pver} linux-kbuild-${kver}+rpt=${pver}
```

If you have 6.12.34 already installed, you will need to force it to boot from the earlier version
by using this command:

```
kver=6.12.20; \
sudo cp /boot/vmlinuz-${kver}+rpt-rpi-v8    /boot/firmware/kernel8.img && \
sudo cp /boot/initrd.img-${kver}+rpt-rpi-v8 /boot/firmware/initramfs8 && \
sudo cp /boot/vmlinuz-${kver}+rpt-rpi-2712    /boot/firmware/kernel_2712.img && \
sudo cp /boot/initrd.img-${kver}+rpt-rpi-2712 /boot/firmware/initramfs_2712
```

Then reboot

```
sudo reboot
```

and get rid of the 6.12.34 package:

```
kver=6.12.34; sudo apt-get purge -y linux-image-${kver}+rpt-rpi-v8 linux-image-${kver}+rpt-rpi-2712
```

There is also an [rpi-update](https://github.com/raspberrypi/rpi-update/blob/master/README.md) command
that you can use to install a bleeding edge kernel.

## OS configuration

If you want to do this using SSH from your main machine, then

* run `raspi-config` to enable SSH (under Interfacing)
* find the current IP address using `ifconfig`

Update packages

```
sudo apt update
sudo apt upgrade
```

and reboot.

Run `raspi-config`:

* enable serial port (under Interface/Serial Port); answer
   * No to login shell accessible over serial
   * Yes to enable serial port hardware

For the CM4, but not the CM5, add the following at the end of `/boot/firmware/config.txt`.

```
# realtime clock
dtoverlay=i2c-rtc,pcf85063a,i2c_csi_dsi
# fan
dtoverlay=i2c-fan,emc2301,i2c_csi_dsi
# Make /dev/ttyAMA0 be connected to GPIO header pins 8 and 10
# This always disables Bluetooth
dtoverlay=disable-bt
```

Disable the system service that initialises the modem:
```
sudo systemctl disable hciuart
```

Set the timezone:

```
sudo dpkg-reconfigure tzdata
```

Use raspi-config to set
* wifi country (under System Options > Wireless LAN)
* hostname (under System Options)

Reboot.

## Verify OS setup

Check the RTC:

```
sudo hwclock --show
```

Check that the current date is correct:

```
date
```

Check support for the fan controller. Do

```
ls /sys/class/thermal
```

You should see:

```
cooling_device0  thermal_zone0
```
