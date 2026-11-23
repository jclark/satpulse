---
title: SatPulse Workbench in Wasm
date: 2026-11-23
---

I have released a [Wasm](https://webassembly.org/) variant of [SatPulse Workbench]({% link _posts/2026-08-24-introducing-satpulse-workbench.md %}),
which runs entirely in the browser.
It uses Web Serial to access the GPS receiver.
This means you can configure a GPS receiver without having to install anything.
Today's release supports high-level configuration
for u-blox, Unicore, Zhongke (CASIC) and Septentrio.

See [Zero-Install SatPulse Workbench]({% link workbench/zero-install.md %}) for more information,
or try it out at [workbench.satpulse.net](https://workbench.satpulse.net/).

This is built from the same Go code base as the rest of SatPulse.
The Wasm-specific parts are small:
one Go package with less than 500 lines (out of about 78k total).
So although this is new, I am hopeful it will work reliably.
