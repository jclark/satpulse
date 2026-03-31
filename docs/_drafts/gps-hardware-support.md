---
Title: GPS hardware support in SatPulse 0.2
---
XXX this is not a good intro any more: we should highlight that we are going to have a whirlwind tour of the GNSS modules out there
For the last 8 months or so, I have been working on improving the GPS hardware support in SatPulse, but it has really accelerated in the last 3 months.

So what does GPS hardware support actually mean? All GPS receivers support NMEA which provides information about navigation solutions as they are computed by the receiver. GPS receivers also support RTCM, which is used both to consume corrections provided to the receiver and to provide corrections for use by other receivers. Apart from that, every GPS receiver supports one or more vendor-specific protocols. The messages used by these protocols can be divided into two groups. The first group performs a similar function to NMEA: they are messages periodically emitted by the receiver to provide information about the navigation solutions computed and other aspects of the ongoing operation of the receiver. The second group performs configuration: these include both requests that are input to the receiver to query and alter configuration, and messages that are output by the receiver as responses to these requests. Hardware support involves enabling SatPulse to make use of both kinds of message.

SatPulse supports vendor-specific periodic messages in the obvious way, by converting vendor-specific messages into a uniform abstraction. In the source code, this abstraction is defined by the `gpsprot` (GPS protocol) package. In 0.1, there were abstract messages that provide information about

- the current time in either UTC or TAI; also includes sawtooth error/quantization error and time accuracy
- the progress of a survey-in operation
- the next scheduled leap second
- satellite positions and signal strengths

In 0.2, there are in addition messages that provide information about

- the characteristics of a navigation solution; this includes
  - the technique used to compute the solution (e.g. ranging code or carrier phase)
  - dimensionality of the fix i.e. 3D, 2D or time only
  - what sensors beyond GNSS contributed to the solution (e.g. INS)
  - kinds of corrections used (e.g. SBAS, RTK, PPP)
  - accuracy (position, velocity)
  - DOP in all its flavours
  - age of differential corrections
  - GNSS constellations and bands used in computing the solution
- position, in ECEF or geodetic coordinates
- velocity, in ECEF or geodetic coordinates (i.e speed and course-over-ground, or NED)

These abstract messages can be serialized as JSON, and can be seen in the event log, which also includes information about pulse edges.
The messages also feed the observability subsystem, which exposes them as Prometheus metrics.

For configuration, there are now two kinds of configuration support as I described in more detail in my [earlier blog](https://satpulse.net/2026/01/29/improving-gps-configuration.html).

- high-level configuration, intent-based configuration, where you describe what you want, and SatPulse turns that into the appropriate messages for the specific receiver
- low-level configuration, which is based on message files

High-level configuration in 0.2 provides a similar set of features as in 0.1. There are a couple of new properties that can be set:

- minimum elevation angle for satellites to be used in the navigation solution
- RTCM base ID - the RTCM reference station ID of the RTCM messages that the receiver generates

I have added a few features to the message file implementation since the last blog:

- the message file implementation now understands the request/response patterns of various protocols, which allows it to identify which messages from the receiver are responses to messages sent
- there is a file inclusion capability, which is useful for allowing model-specific message files to share messages that work across multiple vendor models

The high/low-level configuration split leads naturally to two tiers of support. In both tiers, there is

- support for generating the full range of abstract messages from the vendor-specific messages
- support for the protocol in the message file implementation

The difference is that Tier 1 provides configuration support primarily via the high-level mechanism; message files are used to supplement this to support vendor-specific features that are not exposed via high-level configuration, whereas Tier 2 uses message files to provide configuration support.
For Tier 2, the provided message files use a common set of conventions for tags, which provides a degree of vendor-independence in the user interface.

SatPulse 0.1 had support only for u-blox. In 0.2, there is support for the following vendors

1. u-blox - tier 1
2. Unicore - tier 1
3. Quectel - tier 2
4. Zhongke/CASIC - tier 2
5. Allystar - tier 2
6. Techtotop/Taidou - tier 2
7. ByNav - tier 2
8. ComNav/SinoGNSS - tier 2

Apart from u-blox, these are all Chinese. There are other Western vendors, but they are all either much more expensive than u-blox (Septentrio, Trimble, NovAtel) or the chips are designed to be integrated into mobile phones and are not available as modules that you can buy separately (e.g. Broadcom). Septentrio has recently come down in price a bit and they also have an excellent reputation for timing, so I hope to add support for Septentrio in the future. I already have some support for NovAtel, because some Chinese vendors have adopted the NovAtel OEM 6/7 series protocol.

I have chosen which Chinese modules to support based on a number of factors:

- Are they at least dual-band? Given the low cost of dual-band receivers, there are relatively few cases where single-band modules remain interesting.
- Do they have any features that are particularly interesting for precision timing and/or positioning?
- How common and popular are they on Taobao?
- Are there any Western vendors that resell them?

Taobao recently started supporting direct shipping to my country of residence,
which makes using Taobao much more convenient.
I have found Taobao vendors much more knowledgeable about what they are selling than AliExpress vendors.
If you can buy from Taobao, I recommend it.

## u-blox

SatPulse supports all u-blox receivers starting from LEA-6T all the way through to ZED-X20P. I have tested it on all timing receivers:

- LEA-6T
- LEA-M8T
- LEA-M8F
- ZED-F9T (both L1/L2 and L1/L5 variants)
- NEO-F10T

and on both current high-precision receivers:

- ZED-F9P
- ZED-X20P

and on many standard precision receivers, including:

- NEO-M9N
- M10050-KB
- NEO-F10N (which is L1/L5)

In 0.2, the improvements in u-blox support are:

- support for richer information (navigation solution characteristics, position, velocity)
- support for additional configuration properties (minimum elevation and RTCM base id)
- support for ZED-X20P

I have done much more testing on u-blox receivers than on any other brand.

## Unicore

There is tier 1 support for the UM980 family, which consists of

- UM980, base model, which includes support for Galileo HAS (which is promised but not yet available for u-blox ZED-X20P)
- UM981, adds INS
- UM982, adds dual antennas
- UM960, lower end version, without PPP support

These use a protocol which is similar to that used by NovAtel OEM6/7 messages. Periodic data messages have dual binary/ASCII formats;
the packet formats are similar, but the binary packet format has different sync bytes.
Configuration messages are ASCII lines, similar but different from Novatel.
These modules also have some undocumented support for some periodic data messages that use the same packet format as NovAtel.
This includes the RANGECMPB raw message which can be used by RTKLIB to generate RINEX observation files.

In the West, these modules are distributed by [ArduSimple](https://www.ardusimple.com/product/simplertk3b-budget/), [gnss.store](https://gnss.store/collections/unicore-gnss-modules) and [SparkFun](https://www.sparkfun.com/sparkfun-triband-gnss-rtk-breakout-um980.html).

SparkFun have a [GitHub repo with the latest UM980 firmware](https://github.com/sparkfun/SparkFun_RTK_Torch/tree/main/UM980_Firmware).

Unicore have another range of modules UM6XX, which uses a completely different protocol, which is not yet supported.

## Quectel

Quectel have a bewildering array of GNSS modules. There are two families that I find interesting and that SatPulse supports:

- LG290P family - this is Quectel's high-end module which includes support for Galileo HAS in the most recent firmware
  - LG290P is the base model
  - LG580P is the dual antenna variant
  - LG680P is the LG290P in the 22x17mm u-blox ZED form factor
- LC29H family - this is a relatively inexpensive L1/L5 model based on the Airoha AG3335 chipset; there are several variants, of which the most interesting are:
  - LC29H(AA) is the best fit for base-station case; it can produce RTCM corrections but does not have the RTK engine that can consume them; it is supposed to get OSNMA support at some point
  - LC29H(DA) is the one that is suitable for use as an RTK rover

Quectel's vendor specific protocol uses the NMEA proprietary extension mechanism. The NMEA sentences start with `$PQTM` (The `P` is the NMEA proprietary extension mechanism, and `QTM` is Quectel's 3-letter identifier). The LG290P supports a richer set of PQTM messages than the LC29H.
The LC29H also supports some NMEA proprietary sentences defined by Airoha; these start with `$PAIR`.

In the West, SparkFun sell an [LG290P board](https://www.sparkfun.com/sparkfun-quadband-gnss-rtk-breakout-lg290p-qwiic.html).
They also have a [GitHub repo with the latest LG290P firmware](https://github.com/sparkfun/SparkFun_RTK_Postcard/tree/main/Firmware).

On Taobao, a good shop to buy from is [Mozi Technology/Mozihao](https://mzhtek.taobao.com/).

## Zhongke/CASIC

[Zhongke Microelectronics](https://icofchina.com/) is best known for the ATGM332D and ATGM336H series of modules.
The difference between these is the form-factor: ATGM332D uses the u-blox NEO form factor,
whereas the ATGM336H uses the MAX form factor.
The notable feature of these modules is that they are extraordinarily cheap.
The module (without a board) is about $1.25 on Taobao.

The vendor-specific protocol is CASIC. CAS stands for Chinese Academy of Sciences,
and Zhongke in Chinese also refers to the Chinese Academy of Sciences. The CASIC protocol is UBX-like.

There are actually literally dozens of different versions of these modules, and there are some very significant differences between them.

The most common version is ATGM332D-5N31. These module names follow a common pattern.
The 5 indicates what generation chip is used, in this case the AT6558.
N I think stands for navigation. The 3 says what constellations are supported:
1 means GPS, 2 means BeiDou, 4 means GLONASS, and these are added together to indicate the set of constellations supported. So 3 means GPS+BeiDou and 7 means GPS+BeiDou+GLONASS.
I haven't figured out how the final digit works.

There is a more recent and less common 6 series (e.g. the ATGM332D-6N74) which adds support for Galileo. This uses the AT6668 chip. More interestingly, there is also an F8 series (e.g. [ATGM332D-F8N76](https://www.icofchina.com/daohang/duopin/2531.html)), which is dual-band.
This uses the AT9880 chip.
It is not common yet but I expect it will become common:
like the rest of the ATGM332D series this is extraordinarily cheap, about $2.15 on Taobao;
it seems to be the cheapest dual-band module in the world.

The 5 series use a default baud rate of 9600, whereas the 6 and F8 series use 115200.
More significantly, although they all use the same packet format,
the 6 and F8 series support a different set of messages.
The 5 series uses NAV class messages, whereas the 6 and F8 series uses NAV2 class messages.

There is some English language documentation available for the version of CASIC that uses NAV messages;
there is nothing available on the NAV2 messages.
Fortunately, one of my Taobao sellers was able to supply me with a Chinese language protocol manual which describes this version of CASIC.
SatPulse has support for both the NAV messages and the NAV2 messages.

As well as the low-end ATGM332D/ATGM336H modules, Zhongke also make higher-end modules designed explicitly for timing.
I have the AT632-6T-30. This is similar to the 6 series of the ATGM332D, and is L1 only, but has a full set of timing features, which makes it interesting.
These use a TIM2 class of message. In particular it has a TIM2-TPX message including quantization error.
It is interesting to have a modern L1 timing receiver; u-blox have not done an L1 timing receiver since the LEA-M8T.
One noticeable difference is that the peak-to-peak amplitude of the sawtooth error is about 6ns on the AT632 compared to 21ns on the M8T,
presumably reflecting the higher clock speed of the more modern receiver.

Zhongke provide a GnssToolkit3 application for Windows for configuring their receivers.

## Allystar

I first came across [Allystar](https://www.allystar.com/en) because of their TAu1201 module, which has been one of the cheapest L1/L5 modules available.
The [StarRiver store](https://xinghewei.tmall.com/) on Taobao sell a variant of their SR1723 board with the TAU1201.
This costs about $7.50. With various fees, shipping and tax to Thailand, the all-in cost is about $10.50.
In the West, it will be a bit more.
The TAU1201 uses their Cynosure III chip. Their latest chip is Cynosure IV, which is available in the TAU951M-P200.
This is a more expensive module that also does RTK. 

Allystar provide a Windows app called [Satrack](https://docs.datagnss.com/rtk-board/files/Satrack_client_V1.31.007.zip) for working with Allystar modules.

Allystar's binary protocol is UBX-like. They haven't done a particularly thorough job of it.
For example, the message that provides information about satellite positions and signals provides
less information than is available via NMEA, and Satrack doesn't make use of it.


## Bynav

[Bynav](https://www.bynav.com/en/) makes M20/M10 series of modules based on their own Alice 22nm SoC.
They are strong in the automotive industry.
I have been mainly testing with the M20, but have also tried the M10.
The M21 adds an IMU on top of the M20; the M22 adds a slightly better grade of IMU.
The M20D adds dual antennas.
The M10 is a smaller-footprint, cheaper, lower-end version of the M20 (update rate of 10Hz vs 50Hz on the M20).
They seem pretty decent for RTK.

Bynav's vendor-specific protocol is NovAtel-style. Their binary and ASCII packet formats for periodic messages (called logs in NovAtel parlance)
are exactly the same as documented by NovAtel for the OEM6/7 range.
Bynav support some messages defined by NovAtel and add some messages of their own.
The configuration commands are NovAtel-style, but not compatible.

In the West, they are sold by [gnss.store](https://gnss.store/collections/bynav-gnss-modules).

## ComNav/SinoGNSS

This company brands itself as [ComNav Technology](https://www.comnavtech.com/) for Western audiences,
but uses [SinoGNSS](https://www.sinognss.com/) for Chinese audiences.
Their latest modules are the K9xx series.
Base module is [K901](https://www.comnavtech.com/product/oem/k901.html). [K902](https://www.comnavtech.com/product/oem/k902.html) adds an IMU. [K922](https://www.comnavtech.com/product/oem/k922.html) adds dual antennas.
They all have support for Galileo HAS.
These are widely available on Taobao.

The protocol documentation on the ComNav site is rather out of date.
My Taobao seller gave me the current Chinese-language protocol documentation,
which was from another company called [Qeetek](https://www.qinnav.com/product/module/), which also advertises these modules.
So I suspect these modules are actually made by Qeetek.

The situation with the protocol is very similar to Bynav.
Their binary and ASCII packet formats for periodic messages are exactly the same as NovAtel.
They support some messages defined by NovAtel and some of their own.
The configuration commands are NovAtel-style, but not compatible.
But the configuration system is not in my view well-designed.
One major deficiency is that it does not allow the current configuration to be queried.
Another problem is that responses to configuration commands are not designed to be machine readable:
they are not wrapped in a protocol packet that allows them to be distinguished from other traffic from the receiver.

Apart from the weakness in the configuration protocol, I found convergence of both Galileo HAS 
and its BeiDou equivalent (B2b-PPP) to be flaky.

## Techtotop/Taidou

I had not heard of [Techtotop](https://www.techtotop.com/enindex.aspx), known in Chinese as Taidou, until a month or so ago.
They make their own GNSS silicon, but they seem to be almost completely unknown outside China.

I find them interesting because they have relatively inexpensive modules specifically designed for timing.
The one I have is the T303-5D, which is an L1/L5 module. This isn't on their web site, but I believe it is just a smaller footprint
version of the [T302-5D](https://www.techtotop.com/detail.aspx?cid=1088).
I got this module from [this shop on Taobao](https://shop471758324.taobao.com/).

It uses a UBX-style protocol called SDBP, about which there appears to be exactly zero information available in English.
My Taobao seller was able to provide a Chinese language protocol manual with full details.
The quality of documentation and protocol design seem good to me:
it has everything I expect of a timing module, including quantization error reporting.

Techtotop also provide the [TDMonitor](https://www.techtotop.com/category.aspx?NodeID=49) application for Windows,
which is similar to u-center (although the UI is all Chinese).

## Conclusions

The GPS subsystem in SatPulse 0.1 was designed to be multi-protocol but only implemented support for u-blox.
In 0.2, broad multi-protocol support has been implemented.
SatPulse provides support for several modules which are not supported by any other open-source project
that I am aware of.
XXX say something about expansion of data model







