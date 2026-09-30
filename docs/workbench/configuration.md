---
title: Configuration
toc: false
configuration_gallery:
  - url: /assets/images/wb-f9p-config-signals.png
    image_path: /assets/images/wb-f9p-config-signals.png
    alt: "Configuration tab with the Satellites and signals section open"
    title: "Satellites and signals"
  - url: /assets/images/wb-f9p-config-timepulse.png
    image_path: /assets/images/wb-f9p-config-timepulse.png
    alt: "Configuration tab with the Time pulse section open"
    title: "Time pulse"
  - url: /assets/images/wb-f9p-config-timemode.png
    image_path: /assets/images/wb-f9p-config-timemode.png
    alt: "Configuration tab with the Time mode section open, in fixed position mode"
    title: "Time mode"
  - url: /assets/images/wb-f9p-config-messages.png
    image_path: /assets/images/wb-f9p-config-messages.png
    alt: "Configuration tab with the Messages section open"
    title: "Messages"
  - url: /assets/images/wb-casic-config-signals.png
    image_path: /assets/images/wb-casic-config-signals.png
    alt: "Configuration tab with a Zhongke AT6558D, showing constellations the receiver does not support greyed out"
    title: "Satellites and signals for a Zhongke AT6558D"
  - url: /assets/images/wb-casic-config-messages.png
    image_path: /assets/images/wb-casic-config-messages.png
    alt: "Configuration tab with a Zhongke AT6558D, Messages section open with the RTCM subsection greyed out"
    title: "Messages for a Zhongke AT6558D"
---

The Configuration tab exposes a high-level configuration model,
which is independent of any vendor protocol.
You edit the configuration as a form, and Apply sends the whole thing to the receiver as one request.
There are sections for the satellites and signals, the time pulse, the time mode,
the message output, the serial speed, and saving and resetting.

{% include gallery id="configuration_gallery" %}

Points to note:

* Opening the tab reads the configuration out of the receiver,
  so the form starts from the configured state of the receiver.
* Message output is configured differently from other properties.
  The form does not tell you what messages are currently enabled.
  Messages are changed in groups.
  When you change a group, messages in the group are enabled and disabled
  to match what you have specified for the group.
* Things the connected receiver cannot do are greyed.
* The status line at the bottom names the sections with changes waiting to be applied.
  Discard goes back to the values read from the receiver.
* Minimum, NTP and PTP fill in the message groups for you.
  NTP and PTP set up what satpulsed needs for those two modes,
  so they are the quick way to get a receiver ready for the daemon.
* ECEF coordinates for a fixed position are checked against the Earth's surface.
