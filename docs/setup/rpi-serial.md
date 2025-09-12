---
title: Raspberry Pi serial specifics
---

This page contains RPi-specific UART information referenced from the serial connection setup page.

## UART configuration on Fedora

<!-- From rpi-cm4-ptp-guide/fedora.md lines 293-340 -->
### UART

When using an GPS inside the IO board case, we will want to connect the GPS to one of the UARTs
provided by the CM4.

With Fedora, it is not convenient to use the UART that is connected to the TXD and RXD pins
on the 40-pin header (pins 8 and 10). This is because U-Boot by default enables the serial console
on that UART, which makes it try to interpret output from the GPS as keyboard input to U-boot.
(Although in theory we could [make U-Boot not do this](https://fedoraproject.org/wiki/Architectures/ARM/Raspberry_Pi/HATs#Deactivate_Serial_Console_entirely), keyboard input to U-Boot appears
not to be working at the moment, so this would require a custom version of U-Boot.)

Fortunately, the hardware provides other UARTs (numbers 2 through 5) that we can enable on other pins.
UART number *n* will appear as `/dev/ttyAMA`*n*. UART 2 is used for other functions (eeprom reading and poe fan).
So a sensible choice is UART 3. This can be enabled by adding the following to the bottom of `/boot/efi/config.txt`.

```
# UART configuration
# Make UART3 TXD, RXD available on pins 7, 29 (GPIOs 4, 5) respectively
# Device will be /dev/ttyAMA3
dtoverlay=uart3
```

You can use UART 4 instead with the following:
```
# Make UART4 TXD, RXD available on pins 24, 21 (GPIOs 8, 9) respectively
# Device will be /dev/ttyAMA4
dtoverlay=uart4
```

This means an internal GPS needs to be wired up differently when using Fedora. Here's an example assuming we are
using the `uart3` overlay.

![image](https://github.com/jclark/rpi-cm4-ptp-guide/assets/499966/cdef0aab-8628-43f4-baa5-4bc705612529)

The wiring is as follows

| Color | GPS pin | Jumpers | Pin # | Pin function |
| --- | --- | --- | --- | --- |
| yellow | PPS | J2 | 9 | SYNC_OUT |
| white | RX | HAT | 7 | GPIO 4, TXD3 |
| green | TX | HAT | 29 | GPIO 5, RXD3 |
| black | GND | HAT | 6 | Ground |
| red | VCC | HAT | 4 | 5V power |

The GPS in the photo is a Quescan SR1612Z1, which costs about $10; it uses the ZongKhe Micro [AT6558](https://www.icofchina.com/d/file/xiazai/2016-12-05/b1be6f481cdf9d773b963ab30a2d11d8.pdf) chipset; the default speed is 38400. For other options, see the [GPS hardware](gps-hw.md) page.