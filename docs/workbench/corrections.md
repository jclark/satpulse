---
title: Corrections
toc: false
corrections_gallery:
  - url: /assets/images/wb-allystar-corrections.png
    image_path: /assets/images/wb-allystar-corrections.png
    alt: "Corrections tab connected to an Ntrip caster, listing the RTCM messages received"
    title: "RTCM corrections from an Ntrip caster"
---

The Corrections tab allows you to pull corrections from either an Ntrip caster or a TCP server
and feed them to the receiver.
The messages, which may be in RTCM or SPARTN format, are decoded on the way through,
and the table shows what came out, summarized by message type.

{% include gallery id="corrections_gallery" %}

Points to note:

* Tick "Send position as NMEA" when you are using an RTK service
  that does not have an endpoint specific to your location.
  The service uses the position you send to decide which corrections to give you.
  Nothing will connect until the receiver has a fix, since there is no position to send until then.
* MSM messages are counted per epoch.
  A message split over several packets counts once,
  and the Splits column says how many extra packets it took.
