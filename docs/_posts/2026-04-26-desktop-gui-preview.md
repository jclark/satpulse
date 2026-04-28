---
title: Desktop GUI preview
---

I have been working on a desktop GUI for SatPulse. Here are a couple of demo videos.

{% include video id="lBE5Ls4ly0M" provider="youtube" %}

{% include video id="aDrLsjJSugU" provider="youtube" %}

This is a native app running on macOS. It uses the same
[GPS library]({% link _posts/2026-04-11-design-of-satpulse-compared-with-gpsd.md %})
that `satpulsed` and `satpulsetool` use.
The code is on the `desktop-gui` branch of the [repo](https://github.com/jclark/satpulse).

It uses a tabbed layout, with tabs for:

* monitoring - this is conceptually quite similar to what the Web monitoring app does, but has a richer set of components
* log-level monitoring - this is similar to what you get with the packet log in `satpulsetool` or with the `--packet-log` option in `satpulsed`; but instead of giving a stream of packets, it groups packets by protocol and message ID; it also allows decoding of packets similar to what is provided by `satpulsetool decode`
* high-level configuration - this allows you to configure the GPS receiver in a high-level, device-independent way, without needing any knowledge of the receiver protocol, similar to `satpulsetool gps`
* low-level configuration - this uses message files to provide a lower-level device-dependent way, similar to `-m` and `-t` options of `satpulsetool gps`
* corrections - this allows you to pull RTCM corrections from an NTRIP caster or TCP server and feed them to the receiver, with a live view of the corrections stream; I plan to provide this functionality in a future release of `satpulsed` using a `[stream.pull]` section in the configuration file

This is built using [Wails](https://wails.io/), which supports Linux and Windows, as well as macOS.
But my initial testing has focused on macOS, partly because that's what I use day-to-day for desktop work,
and partly because there's a gap on macOS, since GNSS vendors provide their proprietary apps only for Windows.

This is still very much in the preview phase: some aspects of the UI are rough, particularly the monitoring tab.
With Wails, the UI is written using standard Web technologies (HTML, CSS and JavaScript),
which run in the platform's native webview component.
The desktop GUI and the `satpulsed` Web monitoring app are both written using Preact and Tailwind CSS;
this should make it possible in the future for the monitoring tab to share Preact components with satpulsed.

Why bother with writing a GUI at all? I developed the device-independent configuration approach to give the best possible
user experience for `satpulsed`. But it turned out to be a lot more engineering effort than I anticipated.
And to be honest, from the point of view of `satpulsed`, it doesn't really earn its keep:
the added benefit to users is too small to justify the effort that went into it.
I added `satpulsetool` to help unlock the value of device-independent configuration to a wider audience.
But `satpulsetool` can't fully do this.
Most users only have one GNSS receiver, so the value to users of this configuration model
isn't so much that it's device-independent,
but that it avoids requiring the user to know anything about the receiver's native protocol;
and the kind of user that benefits most from this is a non-technical user, who will want a GUI not a CLI.
