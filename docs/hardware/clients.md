---
title: PTP client hardware
---

For PTP to work well, clients need to have NICs with PTP hardware timestamping support.
This means that the ethernet MAC or PHY has hardware that timestamps PTP packets when they are received and transmitted.
This is a common feature of modern NICs.

When choosing a NIC for Linux, it is much better if there is a driver with the necessary PTP support included in the Linux kernel (called an *in-tree* driver), rather than provided separately by the manufacturer (called an *out-of-tree* driver).

It is also very desirable for PTP clients to support what the Linux kernel calls [cross-timestamping]({% link intro/timing.md %}#synchronizing-the-system-clock).
This is the ability to take simultaneous readings of the PHC and the system clock.
Cross-timestamping dramatically improves the accuracy with which the system clock can be synchronized to the PHC. What matters for most applications is the accuracy of the system clock.

## Intel

Intel NICs generally have PTP hardware timestamping with good in-tree Linux drivers.

There are two ways to get cross-timestamping support with an Intel NIC.

* For ethernet controllers on the PCIe bus, [PTM]({%link hardware/ptm.md %}) support is necessary. The inexpensive way to get this today (September 2025) is to buy an Intel motherboard with an i226-V, which supports 2.5Gbps.
* The Intel e1000e driver also supports cross-timestamping with a different mechanism. To take advantage of this, choose an Intel motherboard with an i219-V or i219-LM on board. The way this works is using a clock called the ART (Always Running Timer), which is integrated into the motherboard chipset. The ethernet controller can capture both its PHC and the ART at the same instant. The system clock is derived from the TSC (Time Stamp Counter), which is part of the CPU. But the kernel is able to maintain a precise mapping from ART time (t<sub>ART</sub>) to TSC time (t<sub>TSC</sub>), which allows it to convert the (t<sub>PHC</sub>, t<sub>ART</sub>) pair from the ethernet controller into the (t<sub>PHC</sub>, t<sub>TSC</sub>) pair needed for cross-timestamping.

## ARM

Some ARM SBCs support PTP hardware timestamping.
None support cross-timestamping.

### Raspberry Pi

The following Raspberry Pi models have PTP hardware timestamping:

- Raspberry Pi 5
- Raspberry Pi Compute Module 5 (CM5)
- Raspberry Pi Compute Module 4 (CM4)

The Raspberry Pi 5 and the CM5 have the same Ethernet MAC, provided by the RP1 chip, which provides MAC-level hardware timestamping using the `macb` driver.
The CM5 and CM4 both have the same Ethernet PHY, which provides PHY-level hardware timestamping using the `bcm-phy-ptp` driver.
(This means that on a CM5 eth0 has two PHCs, one MAC-level and one PHY-level.)

Only the PHY-level hardware timestamping has support for a PPS input pin,
which is needed to work as a PTP server.
The MAC PHC works better for a PTP client,
because the time of the system clock can be more accurately compared with the time of the MAC PHC than with the time of the PHY PHC,
which allows the system clock to be more accurately synchronized with the MAC PHC than the PHY PHC.
On a CM5, Raspberry Pi OS will by default use the PHY PHC for timestamping network packets.
Since kernel 6.18, it is possible to make it use the MAC PHC:

```sh
sudo ethtool --set-hwtimestamp-cfg eth0 index 1 qualifier precise
```

Note that the Raspberry Pi 4 has a slightly different PHY from the CM4 and does not support hardware timestamping at either the PHY or MAC level.

### Rockchip

The following Rockchip SoCs include a MAC that supports PTP hardware timestamping:

- RK3566
- RK3568
- RK3576
- RK3588 / RK3588S

The stmmac Linux driver supports this.
I have personally verified this on the Radxa Zero 3E, which uses the RK3566,
and the ODROID-M1, which uses the RK3568.
There are boot logs on the Internet confirming it for the
[RK3576](https://blog.csdn.net/wb4916/article/details/160342865),
[RK3588](https://www.spinics.net/lists/linux-watchdog/msg30663.html) and
[RK3588S](https://github.com/ryan4yin/nixos-rk3588/blob/main/Debug.md).

A board using one of these SoCs should work provided it has an RJ45 port that uses the SoC's MAC.
This will usually be the case for any Gigabit port, but 2.5G ports typically use the RTL8125,
which does not have support for PTP hardware timestamping in the mainline kernel.

In choosing a board, I would recommend choosing something that has Armbian standard support.
As of the time of writing (Q4 2026), the best choices seem to me to be:

- RK3566: [Radxa Zero 3E](https://radxa.com/products/zeros/zero3e/) (the [Radxa ROCK 3C](https://radxa.com/products/rock3/3c/) also looks interesting but has only Armbian community support)
- RK3568: [ODROID-M1](https://www.hardkernel.com/shop/odroid-m1-with-4gbyte-ram/)
- RK3576: [Radxa ROCK 4D](https://radxa.com/products/rock4/4d/)
- RK3588S: [Orange Pi 5](https://www.orangepi.org/html/hardWare/computerAndMicrocontrollers/details/Orange-Pi-5.html)

(I have not yet tested ROCK 3C, ROCK 4D or Orange Pi 5, but I have ordered them and so should be able to confirm in a few weeks.)
