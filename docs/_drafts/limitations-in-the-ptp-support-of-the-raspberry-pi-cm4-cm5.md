---
title: Limitations in the PTP support of the Raspberry Pi CM4/CM5
---

Ways in which PTP support on the PHY is less than ideal.

## PHC operations go over the slow MDIO bus

1. Poor sync with system clock
   - [mdio-timestamp v3 series](https://lore.kernel.org/all/20261105054839.2248046-1-jjc@jclark.com/)
   - [raspberrypi/linux#7673](https://github.com/raspberrypi/linux/pull/7673) (downstream form; 218 us bracket on CM5 with rpi-6.18.54)
   - [BeagleY-AI post](https://satpulse.net/2026/11/02/beagley-ai-as-a-ptp-grandmaster.html) (CM5 system clock 32 us fast on stock Pi kernel)
   - [Lasse Johnsen's 2022 RFC](https://lists.openwall.net/netdev/2022/04/20/89) (system clock only within 1-2 us of the PHC)
   - [rpi-cm4-ptp-guide#52](https://github.com/jclark/rpi-cm4-ptp-guide/issues/52) (CM5 phc2sys delay about 222 us, spikes of about 19 us)

2. Slow reading time of PHC
   - [mdio-timestamp v3 series](https://lore.kernel.org/all/20261105054839.2248046-1-jjc@jclark.com/)
   - [BeagleY-AI post](https://satpulse.net/2026/11/02/beagley-ai-as-a-ptp-grandmaster.html) (73 ms per PTP_SYS_OFFSET_EXTENDED on CM5, stock Pi kernel)
   - [satpulse#295](https://github.com/jclark/satpulse/issues/295) ("Reading the PHC on the CM5 is extremely slow")
   - [bcm_ptp_framesync_ts](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L217): framesync write, then up to 10 MDIO polls of INTR_STATUS

3. TX timestamp reading is slow, so tx_timestamp_timeout must be raised: [bcm_ptp_do_aux_work](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L503), [Jeff Geerling's CM4 PTP post](https://www.jeffgeerling.com/blog/2022/ptp-and-ieee-1588-hardware-timestamping-on-raspberry-pi-cm4/) (uses 200)

4. PHC steps (ADJ_SETOFFSET) invole a separate read and a write so will end up wildly off (4.8 ms off on CM5) 

## Packet timestamping


5. PHY does not timestamp PTP v2.1 messages (linuxptp 4.x default): [raspberrypi/linux#7515](https://github.com/raspberrypi/linux/issues/7515), [raspberrypi/linux#5947](https://github.com/raspberrypi/linux/issues/5947), [linuxptp-users](https://www.mail-archive.com/linuxptp-users@lists.sourceforge.net/msg03316.html), [rpi-cm4-ptp-guide#23](https://github.com/jclark/rpi-cm4-ptp-guide/issues/23)

6. packet ingress/egress latency uncalibrated
   - [rpi-cm4-ptp-guide#39](https://github.com/jclark/rpi-cm4-ptp-guide/issues/39) (CM5 PPS constant offset of about 170 ns while ptp4l shows about zero; i210 about 20 ns)
   - my own measurements are not published (about 280 ns RX-TX asymmetry, all on receive)

7. Can only timestamp PTP packets: means NTP packet timestamping needs NTP-over-PTP

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

14. PHY has same ID as the PHY in the Raspberry Pi 5 but is different
   - [mainline matches the model ID only](https://elixir.bootlin.com/linux/v7.3/source/drivers/net/phy/bcm-phy-ptp.c#L926)
   - [raspberrypi/linux#5104](https://github.com/raspberrypi/linux/issues/5104) and the [downstream fix](https://github.com/raspberrypi/linux/commit/13d915df82494bee901b8d4e9045242e39861761), which tells them apart by MDIO address (its message: "the two PHYs return the same ID when queried")
   - [Andrew Lunn, 2022](https://lists.openwall.net/netdev/2022/04/22/145): "Ah, they should not do that."
   - [LKML, Oct 2025](https://lkml.iu.edu/2510.3/04838.html): on mainline the Pi 5 PHY binds to the BCM54210E driver

15. On CM5 multiple packet timestampers on the same interface. causes general confusion (this is good and bad)

16. Silkscreen on CM5 IO board is wrong silkscreen says pin 9, SYNC_OUT is pin 6: [forum t=380887](https://forums.raspberrypi.com/viewtopic.php?t=380887)
   - fixed in revision 2 (marked R2 below the board graphic), from June 2025: [PCN 25](https://pip.raspberrypi.com/categories/1168-pcn/documents/RP-008021-PC-3-PCN%2025_%20Raspberry%20Pi%20CM5%20IO%20board%20revision%202.pdf) ("Fixes silkscreen errors for the USB_OTG_ID pin and Sync_Out pin")


## Kernel problems

17. Delay in ptp clock becoming available after interface comes up
   - [satpulse#7](https://github.com/jclark/satpulse/issues/7) (IFF_UP and carrier needed, even though /dev/ptp0 exists)
   - [rpi-cm4-ptp-guide#4](https://github.com/jclark/rpi-cm4-ptp-guide/issues/4) (ts2phc "interface eth0 does not have a PHC" at boot)
   - [raspberrypi/linux#5881](https://github.com/raspberrypi/linux/issues/5881) (Pi 5, so the MAC, not the PHY)

18. Disabling EXTTS can deadlock: [satpulse#493](https://github.com/jclark/satpulse/issues/493) (the "Related observation" section)

19. P2P timestamping does not work [rpi-cm4-ptp-guide#34](https://github.com/jclark/rpi-cm4-ptp-guide/issues/34) (CM4), [rpi-cm4-ptp-guide#49](https://github.com/jclark/rpi-cm4-ptp-guide/issues/49) (CM5)