---
title: Limitations in the PTP support of the Raspberry Pi CM4/CM5
---

The Raspberry Pi CM4 and CM5 have a SYNC_OUT pin, which is brought out onto the official IO boards.
This is connected to the BCM54210PE PHY, which has support for PTP.
In particular, the PHY allows a PPS signal to be timestamped in hardware with respect to a clock on the PHY.
This is supported by Linux.
In Linux terms, the clock is called a PTP hardware clock, and the timestamping capability is called
external timestamping.
This is an extremely unusual capability,
and it enables a PPS signal to be timestamped with an accuracy of the order of 10 ns.
The normal method of timestamping a PPS signal on a Raspberry Pi is to connect the PPS signal to a GPIO pin,
and then use the kernel PPS subsystem to timestamp it;
the delay between the occurrence of the physical PPS edge and the kernel timestamping it
is of the order of 10 us.

This feature became available in 2022 and was one of my main motivations for starting work on SatPulse.
Originally I worked on developing [scripts and documentation](link to Rpi-cm4) to make use of this using the existing
tools in linuxptp. But I found that there were a number of limitations that made this awkward,
but that could be cleanly worked around by software that was designed with these limitations in mind.

Over the last four years of working on SatPulse, I have learned a lot more about the PHY.
More recently, with the help of AI coding assistants, I have been able to understand the hardware
and kernel support much better, to the point where I have been able to submit kernel patches.

So I thought it would be useful to catalogue the various limitations I have found,
together with notes on how they can be worked around.
By limitation, I mean a way in which it is less than ideal.
I should say that I have yet to come across hardware that is ideal in every respect,
and indeed such a thing is probably impossible to create.
For those interested in precision timing,
this capability of the PHY is amazing and, with awareness of the limitations, can be used very effectively.

## PHC operations go over the slow MDIO bus

External timestamping can be implemented either in the Ethernet PHY or in the Ethernet MAC.
In the case of the CM4/CM5, it is implemented in the PHY.
The kernel communicates with the PHY over an MDIO bus.
This is a slow bus intended for occasional configuration of the PHY for things like link speed.
It is separate from the high-speed RGMII interface used for data.
With the PTP support on the PHY, it is also used for operations related to the PHC.
The basic problem here is that the MDIO bus is really slow
and so not well-suited for something which is timing sensitive.

Let me give you an idea of just how slow it is.
On the CM5, it runs at 2.08 MHz. Communication on the MDIO bus is done in frames
of 64 bits. So each frame takes about 31 us.
The PHC registers are in an expanded range which requires two frames to access.
Writing a PHC register takes a minimum of about 62 us, just from MDIO bus overhead.
Reading needs an additional frame so takes about 92 us.
On the CM4, the bus runs at 10 MHz, and MDIO operations are correspondingly faster.
When a PHC is in an Ethernet MAC, the time to read or write a register is of the order of 1 us.

One interesting direction would be to try to run the MDIO bus on the CM5 at a higher speed.
Currently the speed is set by the macb driver in a generic way so as not to exceed
the maximum speed of 2.5 MHz specified by IEEE 802.3.
I have done some initial experiments which indicate that a higher speed is likely to work;
since the PHY is the same part on the CM4 as on the CM5,
the PHY end should not have a problem with the higher speed.

The use of a slow MDIO bus shows up in a number of ways.

1. Reading the PHC is very slow: using Raspberry Pi kernels, it is 335 us on the CM4 and 2.272 ms on the CM5;
   on a mainline kernel the CM5 is 861 us.
   The Raspberry Pi kernel is worse on the CM5 because of a [commit](https://github.com/raspberrypi/linux/commit/707efde3a98c),
   which increases the polling interval for MDIO completion.

2. Synchronization between the system clock and the PHC is inaccurate as well as slow.
   Synchronization uses the PTP_SYS_OFFSET_EXTENDED ioctl, which brackets each PHC read with a system clock read.
   The ioctl allows you to specify the number of samples (i.e. PHC reads);
   the maximum is PTP_MAX_SAMPLES, which is 25, and several applications default to this.
   Applications then typically choose the PHC read with the smallest bracket, and
   use its midpoint as the estimate of the system clock time corresponding to the PHC time.
   The size of the bracket shows up in chrony's estimate of the accuracy.
   In the worst case, a CM5 running a Raspberry Pi kernel,
   a single PTP_SYS_OFFSET_EXTENDED ioctl with PTP_MAX_SAMPLES takes about 68 ms.
   One mitigation is to specify a smaller number of samples: 25 gains no accuracy and costs substantial time.
   (SatPulse uses 6.)
   The bigger problem is that there is a systematic bias in the midpoint estimate,
   which is approximately equal to the transmission time of one MDIO frame.
   The implication is that by default if you run a combined PTP/NTP server on a CM5
   with the Raspberry Pi kernel, you will be serving time that is wrong by about 31 us.
   the workaround is to specify `offset -31e-6` in the chrony refclock line
   (or `offset -7e-6` for a CM4).
   I have also submitted a [patch series](https://lore.kernel.org/all/20261105054839.2248046-1-jjc@jclark.com/)
   to fix this properly, but it is currently stalled; I haven't given up yet, but it is uncertain
   whether it will make it into mainline.
   (The analysis in the patch series also shows why the bias is one MDIO frame.)
   In the meantime, you can try out an earlier version from the [raspberrypi/linux](https://github.com/raspberrypi/linux/pull/7673) PR; this kernel can be installed using the command `sudo rpi-update pulls/7673`.

3. Stepping the PHC clock (using ADJ_SETOFFSET) is implemented as a separate read and write of the clock.
   The effect is that stepping the clock works very inaccurately:
   in the worst case, the clock will be about 4.7 ms behind the specified time.
   In SatPulse, I work around this by doing the step twice: I use the first step to estimate the error in the step,
   and then do a second step to correct it.

4. The MDIO bus is also used to read the timestamps of transmitted packets, so this is similarly slow.
   You can work around this in `ptp4l` by specifying an option of `tx_timestamp_timeout 200`.


## Packet timestamping

5. The PHY cannot timestamp PTP 2.1 messages. LinuxPTP 4.x sends PTP 2.1 messages by default;
   the workaround is to specify `ptp_minor_version 0` in the ptp4l configuration.

6. The driver does not compensate for packet ingress/egress latency.
   You can see this by connecting a CM4/CM5 back to back to a NIC with properly calibrated timestamping,
   such as an Intel i210 or i226: the path delay will be about 140 ns instead of close to 0.
   Left uncorrected, this will result in a PTP client of a CM4/CM5 grandmaster being ahead by about 140 ns.
   The fix is to add `ingressLatency 280` to the ptp4l options.
   The 280 figure is accurate to only 10 ns or so.

7. The PHY can only timestamp PTP packets. This is a problem when trying to use chrony's hardware timestamping,
   which by default needs to be able to timestamp NTP packets. The workaround is to use the NTP over PTP
   feature (now standardised in RFC 10030). See the documentation for the `ptpport` directive in chrony.conf
   for how to enable this.

## External timestamping

8. Reading time from PHC interferes with external timestamping
   - [raspberrypi/linux#5231](https://github.com/raspberrypi/linux/issues/5231) (Lasse Johnsen's comments there: it is the hardware; Timebeat blocks PHC access around each pulse)

9. PHC goes haywire when you unplug the cable
   - [rpi-cm4-ptp-guide#3](https://github.com/jclark/rpi-cm4-ptp-guide/issues/3) (stale timestamp delivered 4 times a second with no carrier)
   - [satpulse#9](https://github.com/jclark/satpulse/issues/9), [satpulse#159](https://github.com/jclark/satpulse/issues/159)
   - [satpulse PR #63](https://github.com/jclark/satpulse/pull/63) (handle loss of carrier)

10. External timestamp does not raise an interrupt, so the driver has to poll, which delays delivery by up to 0.25s
   - [schedule_delayed_work(..., HZ / 4)](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L659)
   - [7bfe91efd525](https://github.com/torvalds/linux/commit/7bfe91efd525a4608ccf4f78f9ec1dba748c5b37) ("uses a timer to poll for timestamps")
   - CM4: PHY interrupt not wired ([device tree](https://github.com/raspberrypi/linux/blob/rpi-6.18.y/arch/arm/boot/dts/broadcom/bcm2711-rpi-cm4.dts#L216): "No PHY interrupt")
   - CM5: PHY interrupt wired ([device tree](https://github.com/raspberrypi/linux/blob/rpi-6.18.y/arch/arm64/boot/dts/broadcom/bcm2712-rpi-cm5.dtsi#L165), RP1 GPIO 37), but an external timestamp does not raise it, even with the FSYNC bit unmasked in the 1588 INTR_MASK register (my test)
   - [satpulse#264](https://github.com/jclark/satpulse/issues/264)
   - [PHC synchronization architecture post](https://satpulse.net/2026/05/05/evolving-a-new-phc-synchronization-architecture-for-satpulse-0.2.html)



## Periodic output

11. External timestamping/periodoc output share the same pin
   - [n_pins = 1](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L737), named [SYNC_OUT](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L938) even as an input
   - [7bfe91efd525](https://github.com/torvalds/linux/commit/7bfe91efd525a4608ccf4f78f9ec1dba748c5b37): "Both functions use the SYNC_OUT/SYNC_IN1 pin, so cannot run simultaneously"
   - [Jeff Geerling's CM4 PTP post](https://www.jeffgeerling.com/blog/2022/ptp-and-ieee-1588-hardware-timestamping-on-raspberry-pi-cm4/)

12. Periodic output pulse is very narrow
   - [max 511 x 8 ns](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L161), also the [default](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L603)
   - [rpi-cm4-ptp-guide#35](https://github.com/jclark/rpi-cm4-ptp-guide/issues/35)

## Other hardware limitations

13. No public hardware documentation
   - [jclark/bcm54210pe-ptp-re](https://github.com/jclark/bcm54210pe-ptp-re) (my reverse-engineered description of the PTP hardware, from the Linux driver, Broadcom's OpenBCM SDK and measurements on CM4 and CM5)

14. PHY has same ID as the PHY in the Raspberry Pi 5 but is different
   - [mainline matches the model ID only](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L926)
   - [raspberrypi/linux#5104](https://github.com/raspberrypi/linux/issues/5104) and the [downstream fix](https://github.com/raspberrypi/linux/commit/13d915df82494bee901b8d4e9045242e39861761), which tells them apart by MDIO address (its message: "the two PHYs return the same ID when queried")
   - [Andrew Lunn, 2022](https://lists.openwall.net/netdev/2022/04/22/145): "Ah, they should not do that."
   - [LKML, Oct 2025](https://lkml.iu.edu/2510.3/04838.html): on mainline the Pi 5 PHY binds to the BCM54210E driver

15. On CM5 multiple packet timestampers on the same interface. causes general confusion (this is good and bad)

16. Silkscreen on CM5 IO board is wrong silkscreen says pin 9, SYNC_OUT is pin 6: [forum t=380887](https://forums.raspberrypi.com/viewtopic.php?t=380887)
   - fixed in revision 2 (marked R2 below the board graphic), from June 2025: [PCN 25](https://pip.raspberrypi.com/categories/1168-pcn/documents/RP-008021-PC-3-PCN%2025_%20Raspberry%20Pi%20CM5%20IO%20board%20revision%202.pdf) ("Fixes silkscreen errors for the USB_OTG_ID pin and Sync_Out pin")


## Kernel problems

17. The PTP clock is not attached to its network interface until the network interface is up.
    With ts2phc this can lead to an error on boot: interface eth0 does not have a PHC.
    In SatPulse, I work around this by using netlink to wait for the interface to be administratively up
    i.e. IFF_UP flag set.
    This is because MAC drivers attach the PHY only when the kernel opens the interface.

18. Disabling external timestamping can occasionally deadlock.
    This leaves the driver in an unusable state and requires a reboot.
    I am working on a [patch](https://github.com/jclark/linux/commits/bcm-phy-ptp-deadlock/series) for this.

19. Timestamping in P2P mode does not work.
    I submitted a [patch](https://lore.kernel.org/netdev/cover.1795772837.git.jjc@jclark.com/) to fix this.
    In the meantime, the workaround is to stick to the default E2E mode.

20. PTP over UDPv6 does not work, in either E2E or P2P mode:
    when the PHY writes a timestamp into an outgoing message, it sets the UDP checksum to zero.