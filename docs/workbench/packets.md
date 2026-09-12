---
title: Packets
toc: false
packets_gallery:
  - url: /assets/images/wb-f9p-packets.png
    image_path: /assets/images/wb-f9p-packets.png
    alt: "Packets tab with a ZED-F9P, decoding a UBX-NAV-PVT message"
    title: "UBX and RTCM packets from a ZED-F9P"
  - url: /assets/images/wb-casic-packets.png
    image_path: /assets/images/wb-casic-packets.png
    alt: "Packets tab with a Zhongke AT6558D, decoding a CASIC NAV-PV message"
    title: "CASIC packets from a Zhongke AT6558D"
---

The Packets tab provides a low-level packet view.
It does not show a linear stream of packets.
Instead each row is a message type, with a count of how many have been seen.
A receiver typically emits a batch of messages once a second,
and the table shows you the messages of each type within one batch.

{% include gallery id="packets_gallery" %}

Points to note:

* The row shows the first message of the current batch.
  The chevron opens up the rest of the messages of that type in the same batch.
* A message type that has stopped arriving goes grey rather than disappearing.
* Clicking on a row decodes the message and shows it as JSON in the pane below.
* Snapshot shows the whole of the last batch in time order, across all message types.
* Messages that Workbench sends to the receiver appear too, marked Tx.
