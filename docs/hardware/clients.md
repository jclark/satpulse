---
title: PTP client hardware
---

For PTP to work well, clients need to have NICs with PTP hardware timestamping support. This is a common feature of modern NICs. The PTP features also need to be supported by the driver. Intel NICs generally have PTP hardware timestamping with Linux driver support.

It is also very desirable for PTP clients to support what the Linux kernel calls cross-timestamping.
This is the ability to take simultaneous readings of the PHC and the system clock.
Cross-timestamping dramatically improves the accuracy with which the system clock can be synchronized to the PHC. What matters for most applications is the accuracy of the system clock.

There are two ways to support cross-timestamping.

* For ethernet controllers on the PCIe bus, [PTM]({%link hardware/ptm.md %}) support is necessary. The inexpensive way to get this today (September 2025) is to buy an Intel motherboard with an i226-V, which supports 2.5Gbps.
* The Intel e1000e driver also supports cross-timestamping with a different mechanism. To take advantage of this, choose an Intel motherboard with an i219-V or i219-LM on board. The way this works is using a clock called the ART (Always Running Timer), which is integrated into the motherboard chipset. The ethernet controller can capture both its PHC and the ART at the same instant. The system clock is derived from the TSC (Time Stamp Counter), which is part of the CPU. But the kernel is able to maintain a precise mapping from ART time (t<sub>ART</sub>) to TSC time (t<sub>TSC</sub>), which allows it to convert the (t<sub>PHC</sub>, t<sub>ART</sub>) pair from the ethernet controller into the (t<sub>PHC</sub>, t<sub>TSC</sub>) pair needed for cross-timestamping.
