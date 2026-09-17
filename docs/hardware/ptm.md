---
title: PTM hardware
toc: false
---

See [Synchronizing the system clock]({% link intro/timing.md %}#synchronizing-the-system-clock) for what PTM and cross timestamping are and why they are useful.

You can tell whether a particular NIC supports cross timestamping
by using the `phc_ctl` command from the `linuxptp` package with just the interface name as the argument. The last line of the output will say
`has cross timestamping support` if cross timestamping support is working for that NIC.
The e1000e driver supports cross timestamping for some on-motherboard NICs,
such as the Intel I219-V, in a device-specific way that does not depend on PTM.
Apart from that, support for cross timestamping is currently based on PTM.

You can check whether the PCIe subsystem has PTM support by searching for `Precision Time Measurement` in the output of `sudo lspci -vv`.
For PTM to work, it needs to be supported by the NIC device and the root complex (host bridge) and any PCI bridges
between them. You can use `lspci -t` to identify the relevant bridges.
The CPU also needs support for the Always Running Timer feature;
this corresponds to the `art` flag in `/proc/cpuinfo`.

The driver for the NIC also needs to have cross timestamping support: [searching](https://elixir.bootlin.com/linux/latest/A/ident/getcrosststamp) the Linux kernel for `getcrosststamp` will find drivers which might have support for cross timestamping

The only widely available NICs with support for PTM are the Intel i225/i226, which use the `igc` driver. However, the `igc` driver [disables](https://patchwork.kernel.org/project/netdevbpf/patch/20211228182421.340354-2-anthony.l.nguyen@intel.com/)
support for PTM on the i225-V, which means that PTM works on the i225-LM, i226-V and i226-LM controllers. The i225-V is much more common than the more expensive i225-LM. The i225-T1 PCIe card uses the i225-LM controller.

It is not straightforward to predict whether a motherboard has the necessary support. I have tried a variety of systems, and the rule that covers the systems I've tried is that if the motherboard uses an 11th gen or newer chipset, it is likely to work.  For motherboards with separate CPUs, I found the B660 and Z690 worked, but the Z490 didn't. (The Z490 had some PTM support but the bridge for the x16 slot didn't: I was using that slot since it was a ITX system.) For mini PCs with soldered on CPUs, I found that Jasper Lake (N5095/N6000) and Alder Lake-N (N100) work, but Gemini Lake (J4125) doesn't.

The Open Compute Project maintains a [PTM Readiness](https://www.opencompute.org/wiki/PTM_Readiness) page with list of various hardware products and their status with respect to PTM support.