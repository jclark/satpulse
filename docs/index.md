---
layout: home
title: "SatPulse"
home_gallery:
  - url: /assets/images/wb-samui-monitor.png
    image_path: /assets/images/wb-samui-monitor.png
    alt: "SatPulse Workbench Monitor tab showing a code 3D position solution"
    title: "SatPulse Workbench"
  - url: /assets/images/grafana-phc-valtellina.png
    image_path: /assets/images/grafana-phc-valtellina.png
    alt: "Grafana panel of PHC offset statistics over 24 hours from satpulsed's Prometheus metrics"
    title: "PHC offset over 24 hours: Raspberry Pi CM5 with a Techtotop T303-5D"
---
The goal of the SatPulse project is to provide a suite of open-source software
for making use of a GPS receiver connected to a computer.
It has an emphasis on precision timing
and has especially deep support for the Raspberry Pi, from the Pi Zero to the Pi 5.
It supports a [wide range of GPS receivers]({% link gps-module-support/index.md %}).

The SatPulse implementation is under intensive development.
A [0.3 prerelease](https://github.com/jclark/satpulse/releases) is available,
which brings a large set of [new features]({% link recent-changes.md %}).
The rest of the website marks these with a {% include new-in-03.html %} marker.
It now supports Linux, macOS and Windows.
The [blog]({% link blog.md %}) also describes recent changes in SatPulse.

The [Setup guide]({% link setup/index.md %}) describes how to get started with SatPulse.

With 0.3, SatPulse supports four main use cases:

* timing
* monitoring and evaluation
* GPS receiver configuration
* precision positioning using RTK

{% include gallery id="home_gallery" %}

It supports these use cases through three programs:

* satpulsed, a program that runs as a service: it transfers time from a GPS receiver to an NTP or PTP time server (this does not yet work on Windows);
  it supports monitoring through a Web dashboard and Prometheus metrics;
  it can work as an RTK base station;
* satpulsewb, [SatPulse Workbench]({% link workbench/index.md %}): a web-based, graphical interface for GNSS receiver configuration and monitoring; it includes an Ntrip client allowing use of RTK positioning
* satpulsetool, a suite of command-line tools: most important is the gps subcommand that does GPS configuration; other subcommands are intended to support the use of satpulsed and satpulsewb

SatPulse has a distinctive approach to GPS configuration: it supports high-level configuration, which allows the intended configuration to be expressed in GNSS terms, independently of any vendor-specific configuration protocol.
This is complemented by support for low-level configuration using vendor-specific configuration messages.
The three programs all use a shared configuration engine.

Timing is the most mature part of SatPulse.
A typical NTP stratum-1 server, running on, for example, a Raspberry Pi, connects the PPS (pulse-per-second) output of a GPS receiver to a GPIO or a serial port pin.
SatPulse can take advantage of hardware designed for PTP (Precision Time Protocol).
The key difference is that the PPS output of the GPS receiver is connected to a PPS input pin *on the ethernet controller*.
This is supported only on Linux.
The Raspberry Pi CM4 and CM5 have this capability when used with a suitable IO board.
For more details, see the [introduction to precision network timing]({% link intro/timing.md %}).
Without this special hardware, SatPulse can still supply timing information to an NTP server.
SatPulse can supply time-of-day information, leaving the NTP server to access the PPS device.
Alternatively, SatPulse can make use of a PPS signal over the serial connection, including with USB-serial adapters. {% include new-in-03.html %}
This means that timing functionality is now available on macOS.

The development of SatPulse started in 2022, before AI coding agents were a thing.
As AI tooling has become more capable, the project has increasingly adopted it.
This has not only allowed the project to increase its ambition beyond precision timing,
but has also enabled improved quality through AI review and testing infrastructure developed with AI assistance. 

