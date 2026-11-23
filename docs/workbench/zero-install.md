---
title: Zero-Install SatPulse Workbench
toc: false
---

Zero-Install SatPulse Workbench is a variant of [SatPulse Workbench]({% link workbench/index.md %})
that runs entirely in the browser.
To use it, just visit [workbench.satpulse.net](https://workbench.satpulse.net)
with a suitable browser.
It is built from the same code as the `satpulsewb` program,
but compiled to WebAssembly (Wasm).
It uses Web Serial to access the GPS receiver, which must be attached to the same computer
as the browser.
The browser must support Web Serial: Google Chrome and Microsoft Edge have supported it for some time;
Firefox [added support in version 151](https://hacks.mozilla.org/2026/05/web-serial-support-in-firefox/).
Note that Apple Safari does not support Web Serial.
After your browser downloads the Wasm from workbench.satpulse.net, which is hosted on GitHub Pages,
it does not contact the site.
It runs entirely locally, except that, like the installed version, the map view fetches its tiles from OpenStreetMap.

To get started:

1. open the Device dropdown and choose "Add a device..."
2. this brings up a dialog that allows you to give the browser access to a serial device
3. choose the serial speed in the Speed dropdown
4. click Connect

The Zero-Install SatPulse Workbench does not have the Corrections tab that is present in the installed version.

It typically uses a version more recent than the last released version.
The exact commit can be seen in the
[satpulse-workbench-web](https://github.com/jclark/satpulse-workbench-web/blob/main/deploy.json) repository, which is used to build the Wasm from the main satpulse repository.
The Configuration tab supports configuration of GPS receivers from
[u-blox]({% link gps-module-support/u-blox.md %}), [Unicore]({% link gps-module-support/unicore.md %}), [Zhongke Microelectronics]({% link gps-module-support/zhongke.md %}) and [Septentrio]({% link gps-module-support/septentrio.md %}).
