---
title: Setting up Debian on BeagleY-AI
---

These instructions are for Debian 13, rather than the TI SDK (which I have not tried).

## OS installation

The installation process is to use a USB SD card adapter on another machine to write an image on an SD card,
and then insert the card into the BeagleY-AI.
I recommend using [bb-imager](https://www.beagleboard.org/bb-imager), BeagleBoard's imager.
This downloads the image for you.
I used it on macOS.
Using bb-imager allows you to do some configuration before writing the image.
Under Software, choose the "BeagleY-AI Debian 13 v7.3.x-k3 IoT" image (this is the one with no desktop).
Note the wifi is 2.4GHz only.

## OS configuration

Update your system:

```
sudo apt update
sudo apt upgrade
```

Check that you have the necessary overlay for PPS input:

```
ls -l /boot/firmware/overlays/k3-am67a-beagley-ai-cpts-hw1tspush-gpio13.dtbo
```

Edit `/boot/firmware/extlinux/extlinux.conf`.
Uncomment the `fdtoverlays` line in the default label `microSD (default)`,
and change it so that it looks like this:

```
label microSD (default)
    kernel /Image.gz
    append console=ttyS2,115200n8 root=/dev/mmcblk1p2 ro rootfstype=ext4 fsck.repair=yes rootwait net.ifnames=0
    fdtdir /
    fdt /ti/k3-am67a-beagley-ai.dtb
    fdtoverlays /overlays/k3-am67a-beagley-ai-uart-ttyama0.dtbo /overlays/k3-am67a-beagley-ai-cpts-hw1tspush-gpio13.dtbo
    #initrd /initrd.img
```

This uses two overlays:

* `k3-am67a-beagley-ai-uart-ttyama0` puts ttyAMA0 TX/RX on pins 8 and 10
* `k3-am67a-beagley-ai-cpts-hw1tspush-gpio13` makes pin 33 be a PPS input connected to the PHC

After this, reboot.

## Verify OS setup

```
ls /proc/device-tree/chosen/overlays/
```

should show the overlays specified in `fdtoverlays`.

```
ls -l /dev/ttyAMA0
```

should show a link to `ttyS3`.
