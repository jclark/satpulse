---
title: GPS configuration
---

Although satpulsed can do GPS configuration automatically,
there are a couple of cases where you may still want to do GPS configuration separately.

## Supported GPS modules

SatPulse has a GPS configuration engine that currently supports
u-blox receivers (from the u-blox 6 platform through to the X20 platform)
and Unicore Nebulas IV receivers (UM980, UM981, UM982, UM960).
This configuration engine is shared by `satpulsed` and `satpulsetool gps`.
But satpulsed takes a conservative approach to GPS configuration and will not perform some kinds of configuration:
- it will not make any persistent changes to the GPS configuration;
- it will not reset the GPS or make changes that would cause a reset;
- in particular, it will not change the GNSS constellations that are enabled, nor change the set of signals that are enabled for each constellation;
- it will not change the GPS speed.

These kinds of configuration must be done using `satpulsetool gps`.

If the default speed of your GPS is below 38400, I recommend increasing it to at least 38400.
This provides sufficient bandwidth that satpulsed will enable messages that provide information about satellites and signals in view,
which will then be shown in the web interface.

I also recommend choosing the constellations you want to enable (GPS, GAL, BDS, GLO, QZSS).

The following command changes the speed and the enabled constellations, and makes the changes persistent.

```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --speed 38400 --gnss GPS,GAL,BDS  --save
```

Here
* `-d /dev/ttyAMA0` specifies the serial device
* `-s 9600` specifies the current speed
* `--speed 38400` specifies the new speed that the module should use
* `--gnss GPS,GAL,BDS` specifies the constellations that you want enabled (other constellations will be disabled); GPS, GAL and BDS refer to the US, European and Chinese constellations respectively; you can change according to your geopolitical preferences
* `--save` says to save the changes persistently.

You can also choose the frequency bands that should be used. For example, `--band L1,L5` will enable the L1 and L5 bands.
By default, it will enable all bands.

u-blox L5 modules need [special configuration](https://content.u-blox.com/sites/default/files/documents/GPS-L5-configuration_AppNote_UBX-21038688.pdf) to make use of the GPS L5 signal while it is still pre-operational.
If you use `--gnss`, satpulsetool will do this for you.

See the [satpulsetool-gps(1)]({%link man/satpulsetool-gps.1.md %}) man page for full details.

Note that you can, if you prefer, do all configuration using `satpulsetool gps`, and
then tell satpulsed not to perform any configuration.

## Unsupported GPS modules

If the factory default configuration of the module:
* emits NMEA RMC or ZDA messages, and
* generates a time pulse once a second (with the rising edge aligned to the start of the second),
then you should be able to use the module with satpulsed without any further configuration.

It is important to ensure that the module is not generating more information than
can be transmitted at the speed it is using.
If the module generates NMEA GSV messages for multiple constellations but uses a speed of 9600,
this can lead to delays in messages being received in satpulsed, which can lead to satpulsed
associating the incorrect second with a pulse.

### Interactively configuring the module

If the module can be configured with text commands, you can use a command like this to interactively type or copy/paste configuration commands for the module
and see responses.

```
rlwrap -- socat - /dev/ttyAMA0,raw,b9600,crnl
```

Install rlwrap and socat first.
This gives you a readline editing interface for typing commands,
and ensures commands are terminated with CRLF (which is what most GPS modules need).
It is also workable even when the GPS is generating output.

### Using manufacturer's Windows application

Most GPS manufacturers provide a Windows application for configuring their modules. For example, u-blox has u-center.

So one way to do the configuration is to connect the module to a Windows machine, do the necessary configuration there and then save it.
If your module has Dupont pins, then you can get an inexpensive USB-to-TTL converter with Dupont female connectors.

Many of these Windows applications can work over a TCP connection.
You can use satpulsed to proxy the serial connection to TCP.
This allows you to use the Windows application to perform configuration without touching the hardware.
For this purpose, satpulsed can be run with a [minimal configuration]({%link setup/satpulsed.md %}) that does no timing.

To make it proxy the serial connection to TCP port 2006, add:

```
[[proxy.tcp]]
listen = ":2006"
```

(there's nothing special about port 2006; use whatever you want).

### What to configure

* Enable at least the NMEA RMC message or the ZDA message
* Enable a PPS signal
	* avoid a pulse width that is extremely long (e.g. 50% duty cycle) or extremely short; 0.1s is the most common pulse width and works well
	* the pulse period must be 1 second (so it is a PPS signal)
	* the start of the second should be aligned with the rising edge of the pulse
	* the pulse should be emitted only when the GPS module has a lock
* To be able to view information about satellite positions and signals
    * increase the serial speed to at least 38400
    * enable NMEA GSV, GSA messages
	* output NMEA version 4.10 or 4.11 (this is needed for full multi-constellation support)
* With a timing receiver, either
    * enable survey mode, or
    * specify a fixed position
* Disable SBAS
