---
title: GNSS modules
---

The capabilities of a GNSS receiver are mostly determined by the module it contains.
This page describes GNSS modules organized by vendor.
For background information on the module capabilities mentioned here,
see the [GNSS basics]({% link intro/gnss-basics.md %}), [Precision timing]({% link intro/timing.md %}) and [Precision positioning]({% link intro/positioning.md %}) pages.

## u-blox

u-blox is a Swiss company, which makes both modules and chips.
It makes some of its chips available to other vendors to make modules.
SatPulse has [support for u-blox modules]({% link gps-module-support/u-blox.md %}).
Its proprietary protocol is UBX and is used by all of its products.
However, the details of the UBX messages used varies greatly between products.

u-blox provides a Windows application called u-center for configuring their products.
It can work over a TCP/IP connection and so can be used with SatPulse.

### Product categories

u-blox has three main product categories

* standard precision: firmware starts with `SPG`; these are basic products without high-end features
* high precision: firmware starts with `HPG`; these offer high-end features including RTK and raw data
* timing: firmware starts with `TIM`; these are specifically designed for timing and include timing mode and raw data

There is also a frequency and time synchronization category (firmware starts with `FTS`), but there has only ever been one product in that category (the LEA-M8F).

u-blox appears to have a policy of not making firmware upgrades for timing products available on their web site; you can only get them if you are a commercial partner with an NDA in place. This is a major disadvantage of timing products as compared to high precision products.

### Platforms

#### M8

M8 is the earliest platform that I think it is still worth considering. It is L1-only.

For standard precision, M10 platform is available very cheaply and that is a better choice than M8.

But for a timing mode product, the LEA-M8T is still a solid choice if you get it at a suitably cheap price.
As well as timing mode, it supports raw data.

The LEA-M8F is a unique product and there's nothing else like it. It is sort of a GPSDO and has unique performance characteristics.
However, it is now getting quite old.

#### F9

F9 is u-blox's first dual-frequency platform. There are two products: ZED-F9T, which is the timing mode product,
and ZED-F9P, which is the high-precision product. 

The first products were L1/L2-only (e.g. ZED-F9T-00). More recently, L1/L5 variants have become available (e.g. ZED-F9T-10).
In 2025, they also released a ZED-F9T variant (ZED-F9T-20) that can do either L1/L2 or L1/L5.
(Note that this is not technically triple-band, because it cannot do L1, L2 and L5 simultaneously.)

The latest ZED-F9P firmware supports OSNMA.

The ZED-F9T has two timing features that are not provided by the ZED-F9P: it has a proprietary differential timing mode
(rather like RTK but for timing), and it provides quantization error reporting. (Early firmware for ZED-F9P also did quantization
error reporting, but that was removed in later firmware versions.)

But so far I have not found that ZED-F9T provides better timing performance in SatPulse than the ZED-F9P (although this
could be due to limitations in my test equipment).

F9 and subsequent platforms use a completely different (and better) approach to configuration within the framework of the UBX protocol.

#### M10

M10 is u-blox's latest SPG platform. It is L1-only.

The M10050-KB chip is available to OEMs, and several Chinese OEMs make very inexpensive modules using this chip.
This is probably the best value choice today for an inexpensive L1 module.

It supports all constellations, but no high-end features.

#### F10

F10 is a dual-band L1/L5 platform.

There are both standard precision products (MAX-F10S, NEO-F10N) and a timing product (NEO-F10T).

The standard precision F10 is u-blox's cheapest dual-band product and I have got good results from it. I would recommend it.

In my testing so far, I have not found the timing mode product (NEO-F10T) provides any better performance,
and it has the major disadvantage of no firmware updates being available. So I wouldn't recommend it at this point.
The NEO-F10T is a cheaper alternative to the ZED-F9T: it is not an upgrade.

#### X20
 
X20 is u-blox's first all-band platform i.e. L1/L2/L5/L6.

So far they have released the high precision product, the ZED-X20P.
Judging by the Interface Manual that they have released, the 2.00 firmware is still lacking important features,
notably satellite-broadcast PPP.
So personally I am waiting till this matures a bit: I expect in due course it will be a good product.

#### F20

F20 is u-blox's triple band platform L1/L2/L5 (so like X20 but without satellite-broadcast PPP).

## Unicore

Unicore is a Chinese company, which makes both modules and chips.
SatPulse has [support for Unicore modules]({% link gps-module-support/unicore.md %}).

Unicore provide a Windows application called UPrecise for configuring their modules.
It can work over a TCP/IP connection and so can be used with SatPulse.

Their current high precision products are the NebulasIV range, based on their UC9810 chip. It includes

* UM980 - base model
* UM981 - adds an inertial sensor
* UM982 - adds dual antennas
* UM960 - lower end model

These are all-band receivers and primarily oriented at the RTK market.
One notable feature of UM980 is that the latest firmware supports all three satellite-broadcast PPP services.
They are resold by companies in Europe and the USA.
SatPulse supports these, including high-level configuration. These use a similar protocol to NovAtel receivers.

The NebulasIV range also includes a timing receiver, the UT986, but it uses a different protocol and is not widely available.

There is a separate range of standard precision products, including the UM620,
which also use a different protocol.

## Allystar

Allystar is a Chinese company, which makes both modules and chips.
SatPulse has [support for Allystar modules]({% link gps-module-support/allystar.md %}).

Allystar provides a Windows application called Satrack for configuring their modules.
It can work over a TCP/IP connection and so can be used with SatPulse.

Allystar make the TAU1201 dual-band L1/L5 GNSS module,
which is one of the least expensive dual-band modules available.
The module itself is under $10 in China.
I've tested it and found its performance to be excellent.

They also make some modules with higher-end features, but I have not yet evaluated their performance.

Allystar have their own binary protocol, which is similar in style to UBX.

## Quectel

Quectel is Chinese company which makes modules, but not chips.
SatPulse has [support for Quectel modules]({% link gps-module-support/quectel.md %}).

Quectel provides a Windows application called QGNSS for configuring their modules.
Unfortunately it does not support working over a TCP/IP connection.

Their flagship product is the LG290P, which is an all-band receiver.

They use a protocol based on NMEA, using proprietary sentences.
