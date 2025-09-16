---
title: GPS configuration
---

Often satpulsed will be able to work with a GPS module without any changes to its factory default configuration.
Specifically, a configuration that
* emits NMEA RMC or ZDA messages, and
* generates a time pulse once a second (with the rising edge aligned to the start of the second) 
is sufficient for satpulsed to perform time synchronization.

However, improved accuracy and richer monitoring features can be achieved with additional configuration.

How to configure a GPS module depends on whether SatPulse has support for configuring that module.
Currently SatPulse has support for configuring a wide range of u-blox modules and chips from 6th generation through 10th generation.

## Supported GPS modules

With a supported GPS module, the easiest approach is to enable configuration in satpulse.toml.
```
[gps]
configure=true
```

This will make satpulsed configure the GPS on the fly each time it starts up.
It will set things up to take full advantage of the capabilities of the module.
This can be customized by specifying additional options in the `[gps]` section.
See [satpulse.toml(5)]({%link man/satpulse.toml.5.md %}) man page for full details.

Note that the configuration changes made by satpulsed are never persistent.

However, satpulsed is conservative in the configuration changes that are enabled by `configure=true`.
- it will never make persistent changes; you can also get rid of any changes done by satpulsed by power cycling the GPS.
- it will not change the serial speed of the module
- it will not reset the module
- it will not make changes to the constellations and signals used by the GPS, since this typically needs a reset to be effective

These kinds of changes are instead done using the `gps` subcommand of `satpulsetool`.
See the [satpulsetool-gps(1)]({%link man/satpulsetool-gps.1.md %}) man page for full details.

If the default speed of your GPS is below 38400, I recommend increasing it to at least 38400.
This provides sufficient bandwidth that satpulsed will enable messages that provide information about satellites and signals in view,
which will then be shown in the web interface.

I also recommend choosing the constellations you want to enable (GPS, GAL, BDS, GLO, QZSS).

For example:

```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --speed 38400 --gnss GPS,GAL,BDS  --save
```

Here
* `-d /dev/ttyAMA0` specifies the serial device
* `-s 9600` specifies the current speed
* `-s 38400` specifies the new speed that the module should use
* `--gnss GPS,GAL,BDS` specifies the constellations that you want enabled (other constellations will be disabled); GPS, GAL and BDS refer to the US, European and Chinese constellations respectively; you can change according to your geopolitical preferences
* `--save` says to save the changes persistently.

You can also choose the frequency bands that should be used. For example, `--band L1,L5` will enable the L1 and L5 bands.
By default, it will enable all bands.

u-blox L5 modules need [special configuration](https://content.u-blox.com/sites/default/files/documents/GPS-L5-configuration_AppNote_UBX-21038688.pdf) to make use of the GPS L5 signal while it is still pre-operational.
If you use `--gnss`, satpulsetool will do this for you.

## Unsupported GPS modules

You will need to configure the module yourself.

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
You can use satpulsed in GPS-only mode to proxy the serial connection to TCP.
This allows you to use the Windows application to perform configuration without touching the hardware.
To run in GPS-only mode, comment out the `interface` line in the `[phc]` section

```
[phc]
#interface = "enp1s0"
```

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
* With a timing receiver, either
    * enable survey mode, or
    * specify a fixed position
* Disable SBAS
