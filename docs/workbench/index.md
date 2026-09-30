---
title: Getting started with SatPulse Workbench
---

SatPulse Workbench provides a graphical interface for GNSS receiver configuration and monitoring. {% include new-in-03.html %}
It is an interactive tool for exploring and experimenting with a GNSS receiver.
It is web-based, which means that you use it through a web browser.
Concretely, it is a command-line program `satpulsewb` that runs on the computer the GNSS receiver is attached to and acts as a web server.
It supports Linux, macOS and Windows.
The web-based approach means that you can run `satpulsewb` on a headless SBC such as a Raspberry Pi,
and then interact with it from a browser running on a Mac or PC.
But it works equally well if you have the receiver attached locally.
See [satpulsewb(1)]({% link man/satpulsewb.1.md %}) for details and command-line options.

First, [install SatPulse]({% link setup/satpulse-install.md %}) on the computer the receiver is attached to.
You also need to know the serial device name and speed (baud rate):
the [serial connection]({% link setup/gps-serial.md %}) page explains how to find these with `satpulsetool serial`,
and how to deal with serial device permissions.
Workbench can discover serial devices, but does not detect their speed.

## Running satpulsewb

No configuration is required.
You can run `satpulsewb` with no arguments:

```
satpulsewb
```

It will print URLs that you can copy and paste into your browser.
Each URL includes a unique, generated token to provide a modest level of security.
Anybody with the URL can control the receiver while this instance of `satpulsewb` is running.
If you are running locally, it will also open the browser for you.

For a headless machine, log in with SSH and run `satpulsewb` there.
It prints a URL for each network interface;
open the one with the machine's address on your network in a browser on your laptop or desktop.
Keep `satpulsewb` running while you use Workbench.
Ctrl-C stops it.

## Firewalls and SSH

By default, `satpulsewb` listens on all network interfaces on TCP port 15754 (a mnemonic based on the GPS L1 frequency).
If you cannot open Workbench from another computer, check the firewall on the machine running `satpulsewb`:
it needs to allow incoming TCP connections on that port from your network.
If port 15754 is already in use, `satpulsewb` chooses another port,
so check the port in the printed URL.

If you are running a firewall, then using an SSH tunnel may be more convenient.
If you can log in with SSH, this lets you reach Workbench without opening another port in the firewall.
It also encrypts the connection, which is useful on a network you do not trust.
For example, run this on your laptop or desktop:

```
ssh -t -L 2050:localhost:2051 192.168.1.50 satpulsewb --listen localhost:2051
```

Replace `192.168.1.50` with the address of the machine the receiver is attached to;
use `username@192.168.1.50` if your username there is different.
This starts `satpulsewb` on that machine and forwards port 2050 on your computer to port 2051 on that machine.
Open `http://localhost:2050/` in your browser.
2050 in this example is the local port, which can be any free port on the machine running the browser.
2051 in this example is the remote port, which can be any free port on the machine running `satpulsewb`.
The SSH tunnel connects these two ports. 
No URL token is used in this case, since SSH provides security.
Keep the SSH session running while you use Workbench.
The `-t` option for ssh lets Ctrl-C stop `satpulsewb` and release the receiver.

## Connecting to the receiver

At the top of the Workbench window, select the serial device from the Device dropdown,
or type its name, then select the Speed and click Connect.
The device is on the computer running `satpulsewb`, which may be different from the computer running the browser.
If satpulsed or another program is using the serial device, stop it first.

You can also give the device and speed on the command line:

```
satpulsewb -d /dev/ttyACM0 -s 38400
```

With both options specified, Workbench connects to the receiver at startup.
Replace `/dev/ttyACM0` and `38400` with your device name and speed.

For vendors whose high-level configuration support is experimental,
you need to specify the `--vendor` option to enable it.
For example, for a Zhongke Microelectronics receiver using CASIC:

```
satpulsewb --vendor zhongke
```

See [GPS module support]({% link gps-module-support/index.md %}) for whether high-level configuration
is supported for your receiver and whether this support is experimental.


## Using the interface

SatPulse Workbench has three main areas of functionality: monitoring, configuration and corrections.
The interface is tab-based.

* The [Monitor]({% link workbench/monitor.md %}) tab provides high-level monitoring,
  including a map view and a sky view.
* The [Packets]({% link workbench/packets.md %}) tab provides low-level monitoring of packets flowing to and from the receiver.
* The [Corrections]({% link workbench/corrections.md %}) tab allows you to pull corrections from either an Ntrip caster or a TCP server
  and feed them to the receiver.
* The [Configuration]({% link workbench/configuration.md %}) tab exposes a high-level configuration model,
  which is independent of any vendor protocol: you describe your intended configuration in GNSS terms.
  This tab will be disabled if SatPulse does not support high-level configuration for the connected receiver.
* The [Message file]({% link workbench/message-file.md %}) tab provides a low-level configuration model,
  where you select messages to send from a library of message files, organized by vendor.

Only one browser window at a time can modify the GNSS receiver.
If you open a new browser window, then the previous window is put into a read-only mode.

The log at the bottom of the window is there on every tab.
It can be resized by dragging its top edge, and filtered by level, by component and by a search string.

The detected receiver is shown at the top left of the window.
Receiver detection requires high-level configuration support.
If receiver detection fails, it will show e.g. `Unknown (SBF)` where `SBF` is the detected packet format.
