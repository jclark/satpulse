---
title: Supported GPS vendor-defined protocols
toc: false
---

Although SatPulse can support any module using the vendor-independent NMEA and RTCM protocols,
support for protocols defined by the module vendor allows it to perform configuration and provide richer information than is available in NMEA.

SatPulse supports modules from the vendors (ordered approximately by maturity and extent of support):

- [u-blox]({% link gps-module-support/u-blox.md %})
- [Unicore]({% link gps-module-support/unicore.md %})
- [Zhongke/CASIC]({% link gps-module-support/zhongke.md %})
- [Septentrio]({% link gps-module-support/septentrio.md %})
- [Allystar]({% link gps-module-support/allystar.md %})
- [Quectel]({% link gps-module-support/quectel.md %})
- [Techtotop/Taidou]({% link gps-module-support/techtotop.md %})
- [ByNav]({% link gps-module-support/bynav.md %})
- [SinoGNSS/ComNav]({% link gps-module-support/sinognss.md %})
- [NovAtel]({% link gps-module-support/novatel.md %})

Supporting a protocol involves:
- recognizing the protocol's packet formats; protocols differ in whether they use a single packet format for both periodic data and configuration
- decoding the packet wire formats
- for periodic data, mapping the decoded packets into the device-independent data model that drives time-synchronization, observability and timing
- providing protocol-specific message types for conveniently describing protocol messages in message files
- handling request/response correlation when sending messages defined in message files
- providing message files to perform configuration
- supporting high-level configuration
- supporting conversion of raw observation messages into RINEX
- validating the implementation with real hardware
