---
title: PTP client hardware
---

For PTP to work well, clients need to have NICs with PTP hardware timestamping support.
This means that the ethernet MAC or PHY has hardware that timestamps PTP packets when they are received and transmitted.
This is a common feature of modern NICs.

When choosing a NIC for Linux, it is much better if there is a driver with the necessary PTP support included in the Linux kernel (called an *in-tree* driver), rather than provided separately by the manufacturer (called an *out-of-tree* driver).

It is also very desirable for PTP clients to support what the Linux kernel calls cross-timestamping.
This is the ability to take simultaneous readings of the PHC and the system clock.
Cross-timestamping dramatically improves the accuracy with which the system clock can be synchronized to the PHC. What matters for most applications is the accuracy of the system clock.

## Intel

Intel NICs generally have PTP hardware timestamping with good in-tree Linux drivers.

There are two ways to get cross-timestamping support with an Intel NIC.

* For ethernet controllers on the PCIe bus, [PTM]({%link hardware/ptm.md %}) support is necessary. The inexpensive way to get this today (September 2025) is to buy an Intel motherboard with an i226-V, which supports 2.5Gbps.
* The Intel e1000e driver also supports cross-timestamping with a different mechanism. To take advantage of this, choose an Intel motherboard with an i219-V or i219-LM on board. The way this works is using a clock called the ART (Always Running Timer), which is integrated into the motherboard chipset. The ethernet controller can capture both its PHC and the ART at the same instant. The system clock is derived from the TSC (Time Stamp Counter), which is part of the CPU. But the kernel is able to maintain a precise mapping from ART time (t<sub>ART</sub>) to TSC time (t<sub>TSC</sub>), which allows it to convert the (t<sub>PHC</sub>, t<sub>ART</sub>) pair from the ethernet controller into the (t<sub>PHC</sub>, t<sub>TSC</sub>) pair needed for cross-timestamping.

## Raspberry Pi

The following Raspberry Pi models have PTP hardware timestamping:

- Raspberry Pi 5
- Raspberry Pi Compute Module 5 (CM5)
- Raspberry Pi Compute Module 4 (CM4)

The Raspberry Pi 5 and the CM5 have the same Ethernet MAC, provided by the RP1 chip, which provides MAC-level hardware timestamping using the `macb` driver.
The CM5 and CM4 both have the same Ethernet PHY, which provides PHY-level hardware timestamping using the `bcm-phy-ptp` driver.
(This means that on a CM5 eth0 has two PHCs, one MAC-level and one PHY-level.)

The PHY-level hardware timestamping is necessary to support the PPS input pin needed to work as a PTP server,
but the MAC-level hardware timestamping is sufficient to work as a PTP client.

Note that the Raspberry Pi 4 has a slightly different PHY from the CM4 and does not support hardware timestamping at either the PHY or MAC level.

No Raspberry Pi models support cross-timestamping.
