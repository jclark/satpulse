---
title: BeagleY-AI as a PTP grandmaster
header:
  image: /assets/images/beagley-ai-gps-mini.jpg
  teaser: /assets/images/beagley-ai-gps-mini.jpg
---

Precision timing is a bit of a niche interest.
Precision network timing is a niche within that.
And low-cost precision network timing is a further niche within that.
So perhaps it is not surprising that the hardware options for low-cost precision network timing are limited.
But it is perhaps surprising just how limited they are.
If you want to build a low-cost PTP grandmaster that has proper hardware timestamping of a PPS input, the options have been
the Raspberry Pi CM4/CM5 or Intel NICs, and that's it.

Since one of the main goals of SatPulse is to make it easy and inexpensive to run a PTP grandmaster,
I have spent a lot of time over the last few years fruitlessly trying to identify additional hardware options.
Recently I have been using AI agents to do a more thorough search of available hardware to identify possible candidates.
Typically, this is not a feature that vendors advertise: so it is necessary to analyze Linux device trees and drivers,
reference manuals and schematics in order to determine
whether there is any possibility of a particular board working for this.
Then it's a matter of buying one and seeing whether it actually can be made to work.

I am very excited to share that this effort has finally yielded a new option: the [BeagleY-AI](https://www.beagleboard.org/boards/beagley-ai).

It turns out that very little was needed to make this work:
a new [device tree overlay](https://github.com/beagleboard/BeagleBoard-DeviceTrees/pull/122)
and a couple of fixes to SatPulse:
[one](https://github.com/jclark/satpulse/pull/506) to make it handle PHCs where the routing between pins and timestamping channels
is handled by the device tree rather than the PHC subsystem,
and [one](https://github.com/jclark/satpulse/commit/6f8f02dd2a9be5f46cb6cd331300b3d9f82a0b67) to make it work around a kernel bug in how PTP_EXTTS_REQUEST2 is implemented for the BeagleY-AI hardware.
The device tree overlay is now in the BeagleBoard kernels and the fixes have been included in SatPulse 0.3.
So this is now fully ready for use.
I have added documentation on the [hardware]({% link hardware/bb-build.md %}) and on the [OS setup]({% link setup/bb-debian.md %}).
You can also use it without SatPulse, specifically in chrony (use `refclock PHC /dev/ptp0:extpps:pin=-1 poll 0`).
Unfortunately, ts2phc in linuxptp is affected by the kernel bug,
but I submitted the tiny [patch](https://github.com/torvalds/linux/commit/e3aaeebbef56ceaa27001d1d5a0c1d73d648e6f4) needed to fix this,
and it is now in Linus's tree, so this will be fixed in 7.4.

The BeagleY-AI is an SBC that uses the TI [AM67A](https://www.ti.com/product/AM67A) SoC.
In terms of pure CPU power, it is a lot slower than the Raspberry Pi 5.
It uses quad Cortex-A53 cores at 1.4 GHz, which is the same as the Raspberry Pi 3B+.
So if CPU speed matters to you, this is not a good choice.
However, for a time server, it is plenty fast enough.
It has a bunch of AI stuff, which I know nothing about.
It is mechanically compatible with the Raspberry Pi 5,
which means it can use many Raspberry Pi 5 accessories, such as cases.

It is available at the moment from [Seeed Studio](https://www.seeedstudio.com/BeagleYr-AI-TI-AM67A.html) for $70.
It is usually carried by [Mouser](https://www.mouser.com/new/beagleboardorg/beagleboard-beagle-y-ai-computer/)
and [DigiKey](https://www.digikey.com/en/products/detail/beagleboard-by-seeed-studio/102991834/22532596),
but they are out of stock.

As well as PPS input, the PHC also supports PPS output (called periodic output by the kernel PHC subsystem).
Unfortunately, the physical pins that support this are on the CSI1/DSI0 FPC connector.
This means that you need a cable and breakout board to access them (as shown in the photo above):
specifically, a 22-pin FPC to Dupont breakout board and
a 22-pin 0.5mm pitch FFC cable.
In my testing, a type B FFC cable, with the conductor on each end on opposite sides of the cable,
made the numbers on the breakout board come out right.
It also needs another overlay
(the combined overlay `k3-am67a-beagley-ai-cpts-hw1tspush-gpio13-genf-csi1` is in the latest BeagleBoard kernel package),
and a non-trivial [kernel patch](https://lore.kernel.org/netdev/20261029135556.2508688-1-jjc@jclark.com/).

Now let's look at how the BeagleY-AI compares to the Raspberry Pi CM4/CM5
in terms of its support for precision network timing.
The most important hardware difference is that the PHC that does PPS hardware timestamping
is part of the Ethernet PHY in the Raspberry Pi CM4/CM5,
but is part of the Ethernet MAC in the BeagleY-AI.
The Raspberry Pi CM5 in fact has a second PHC in its MAC, but this does only packet timestamping.
(The Raspberry Pi 5 has the same MAC PHC as the Raspberry Pi CM5 but no PHY PHC,
which means it works well as a PTP client, but is of little use as a PTP server.)
Now the PHY is closer to the wire, so in theory a PHY can do packet timestamping more accurately than a MAC.
In practice it doesn't make any difference, as far as I can tell.
But having the PHC in the PHY has one enormous disadvantage, at least when implemented as it is on the Raspberry Pi CM4/CM5.
The disadvantage is that communication between the CPU and the PHY is much slower because it goes over the slow MDIO bus,
compared to the communication between the CPU and the MAC which, on the BeagleY-AI, goes over the SoC interconnect.
This makes synchronizing the time between the system clock and the PHC much slower and less accurate.
The system call that is used for synchronization is the PTP_SYS_OFFSET_EXTENDED ioctl.
A single call takes an astonishing 73 ms on a CM5 running the stock Raspberry Pi kernel,
compared to 85 µs on the BeagleY-AI, a factor of more than 800.
This translates into a big difference in accuracy.
On the BeagleY-AI, using PTP_SYS_OFFSET_EXTENDED
to synchronize the system clock from the PHC results in the system clock being about 36 ns slow,
whereas on a Raspberry Pi CM5 on a stock Raspberry Pi kernel, it ends up being 32 µs fast,
i.e. nearly 900 times worse.
CM5 on the Raspberry Pi kernel is the worst case:
things are better on the CM4 (because the MDIO bus is faster),
and are better on mainline kernels.
I have been working on a rather complex [patch](https://lore.kernel.org/netdev/20261018100257.566267-1-jjc@jclark.com/) that, if accepted, would dramatically improve this.
Until then, if you care about PHC-system clock synchronization (which is critical if you are running a combined PTP/NTP server),
the BeagleY-AI is clearly superior.

I want to mention one other point which I think is really important,
and that is the availability of documentation for the hardware.
The AM67A has very complete publically available documentation, including for all the timing hardware;
the TRM is about 2,500 pages.
For the Raspberry Pi CM4/CM5 the situation is dismal:
there is no public documentation on the timing-related hardware, in particular the BCM54210PE PHY.

I will have more to say both on the timing hardware in the BeagleY-AI and
on the PHC-system clock synchronization problem on the Raspberry Pi CM4/CM5 in future posts.
But hopefully there is enough in this post to explain why I am so excited about the BeagleY-AI.
