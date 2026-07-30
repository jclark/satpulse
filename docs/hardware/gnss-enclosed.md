---
title: Enclosed GNSS receivers
redirect_from:
  - /hardware/gnss-receivers.html
evk_gallery:
  - url: /assets/images/evk-f10t-top.jpg
    image_path: /assets/images/evk-f10t-top.jpg
    alt: "u-blox EVK-F10T top view"
    title: "EVK-F10T top view"
  - url: /assets/images/evk-f10t-front.jpg
    image_path: /assets/images/evk-f10t-front.jpg
    alt: "u-blox EVK-F10T front view"
    title: "EVK-F10T front view"
  - url: /assets/images/evk-f10t-rear.jpg
    image_path: /assets/images/evk-f10t-rear.jpg
    alt: "u-blox EVK-F10T rear view"
    title: "EVK-F10T rear connectors"
---

This page contains recommendations for GNSS boards that have their own enclosure, separate from the host PC.
At the moment, this covers only receivers that are convenient for timing applications,
which means that they must provide a PPS output using an SMA or BNC connector,
with the signal between 0 and 3.3V.
These receivers also work well as an RTK base station.

It will also need to provide a serial connection, which can be either USB (USB C or micro USB), or RS-232 (typically DB-9 female).
When there is a serial USB connection, then that usually provides power also.
With an RS-232 connection, there will be a separate DC power input.

All the models listed here use u-blox GNSS modules.

![Various GNSS receivers](/assets/images/gnss-receiver-compare.jpg)

## BG7TBL TS-1

![BG7TBL TS-1 GNSS receiver](/assets/images/ts1-front.jpg)

This has

- u-blox LEA-M8T module
- RS-232 DB-9 female serial connector
- SMA female antenna connector
- SMA female PPS output operating at 0-3.3V

It is CNY398 direct from BG7TBL's Taobao shop. It is also available from AliExpress and eBay resellers (at about $80).

## gnss.store USB dongles

![gnss.store USB dongle](/assets/images/f9t-dongle.jpg)

[gnss.store](https://gnss.store/) sell USB-A dongles with SMA female antenna input (on the left) and SMA female PPS output (on the right).
They are available with a range of different u-blox modules:

* [ZED-F9T-00B](https://gnss.store/zed-f9t-timing-gnss-modules/108-elt0095.html), the original L1/L2 version
* [ZED-F9T-10B](https://gnss.store/zed-f9t-timing-gnss-modules/166-elt0147.html), the L1/L5 version
* [ZED-F9T-20B](https://gnss.store/zed-f9t-timing-gnss-modules/444-elt0333.html), latest one which supports L1/L2 or L1/L5
* [ZED-F9P](https://gnss.store/zed-f9p-gnss-modules/126-233-elt0110.html#/61-gnss_module-l1_l2_zed_f9p)
* [ZED-X20P](https://gnss.store/high-precision-rtk-gnss-modules/410-elt0422.html)

These use high-end u-blox modules, so are inevitably quite expensive: between EUR220 and EUR235.

The dongles are quite big and can obstruct adjacent ports. I usually use a short USB extension cable.

Overall, I find this a very convenient form factor.

## u-blox Evaluation Kits

{% include gallery id="evk_gallery" %}

u-blox make evaluation kits for most of their modules, which are available through distributors like DigiKey and Mouser.

Evaluation kits for some timing modules include an SMA connector for the time-pulse output,
and so meet the requirements for this page. Currently available are:

- EVK-F9T-20, which uses the ZED-F9T-20B module and a micro USB connector
- EVK-F10T-00, which uses the NEO-F10T module and has a USB C connector

The EVK-F9T-20 is considerably more expensive than the comparable gnss.store ZED-F9T-20B dongle,
but it does include a high-quality antenna. The only potential advantage of the EVK is that it exposes some additional pins:

- TP2 provides a second independent timepulse
- EXTINT supports the time mark feature

Time mark is a feature where the receiver reports (using UBX) the GNSS time that a pulse is input on a pin;
this could be useful for measuring PTP in conjunction with the LinuxPTP perout feature.

The EVK-M8-T and EVK-M8F do not have SMA time pulse out, so I am not including them.

