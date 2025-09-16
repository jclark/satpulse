---
title: Configure and run satpulsed
---

The main program in SatPulse is `satpulsed`, which is a daemon that
synchronizes the PTP Hardware Clock from GPS.

satpulsed usually runs as a service using systemd.
It can also be run from the command line.
See the [satpulsed]({%link man/satpulsed.8.md%}) for details.
In either case a configuration is necessary to run satpulsed.

## Configuration file

The service expects the configuration file to be at: 

- `/etc/satpulse.toml` if you installed from a package
- `/usr/local/etc/satpulse.toml` if you installed from source

The configuration file is in [TOML](https://toml.io/en/) format, which is inspired by the INI file format
and can be edited with a normal text editor (e.g. `nano`).

A minimal configuration file looks like this:

```
# Configuration file for satpulse
[phc]
interface = "enp1s0"

[serial]
speed = 9600
```

In the above, `#` starts a comment. The lines with square brackets mark the start of a *table*; the square brackets
enclose the name of the table. Following the start of each table are the key/value pairs in that table.
Values can be strings in double quotes
(e.g. `"enp1s0"`), numbers (e.g. `9600`) or booleans (e.g. `true`, `false`).

The `[phc]` table specifies information about the PTP hardware clock.
As a minimum it needs to specify the ethernet interface with the pin that the GPS output pin is attached to.
In this example it is `enp1s0`.
If the pin is not 0, then it also needs to specify the pin index.

```
[phc]
interface = "enp1s0"
pin = 1
```

The `[serial]` table must specify the speed that the GPS is using,
and can also specify the serial device.
Usually the serial device is specified as part of the systemd service name in systemd commands,
but it can also be added to the `[serial]` table,
which is useful for when satpulsed is run directly from the command-line, for example:

```
[serial]
device = "/dev/ttyAMA0"
speed = 9600
```

The configuration file is described in full in its man page [satpulse.toml(5)]({%link man/satpulse.toml.5.md %}).

## GPS configuration

Often satpulsed will be able to work with a GPS module without any changes to its factory default configuration.
Specifically, a configuration that
* emits NMEA RMC or ZDA messages, and
* generates a time pulse once a second (with the rising edge aligned to the start of the second) 
is sufficient for satpulsed to perform time synchronization.

However, improved accuracy and richer monitoring features can be achieved with additional configuration.

How to configure a GPS module depends on whether SatPulse has support for configuring that module.
Currently SatPulse has support for configuring a wide range of u-blox modules and chips from 6th generation through 10th generation.
With an unsupported GPS module, you will need to configure it yourself,
as described in [GPS configuration]({%link setup/gps-config.md %}#unsupported-gps-modules).

With a supported GPS module, the easiest approach is to add the following to `satpulse.toml`:

```
[gps]
config = true
```

This gives satpulsed permission to change the configuration of the GPS module
(the default for `config` is false).
It will set things up to take full advantage of the capabilities of the module.
There are many other options that can be specified in the `[gps]` table to control
how satpulsed does configuration,
which are described in [satpulse.toml(5)]({%link man/satpulse.toml.5.md %}).
Note that the configuration changes made by satpulsed are never persistent;
you can always get rid of any changes done by satpulsed by power cycling the GPS.

There are some kinds of changes that satpulsed will not do:
- it will not change the serial speed of the module;
- it will not reset the module;
- it will not make changes to the constellations and signals used by the GPS, since this typically needs a reset to be effective.

These changes can be performed using the `gps` subcommand of `satpulsetool`,
documented in [satpulsetool-gps(1)]({%link man/satpulsetool-gps.1.md %}).

Note that satpulsed will not configure the GPS receiver to output messages about satellite positions and signals,
unless the serial speed is at least 38400.

See [GPS configuration]({%link setup/gps-config.md %}) for more information about using `satpulsetool gps`. 

## Running satpulsed as a service

The systemd service template name is `satpulse@.service` and the expected argument is the serial device name without `/dev/`.
For example, if the serial device is `/dev/ttyAMA0`, then the instantiated service would be named `satpulse@ttyAMA0.service`.
Systemd commands need to be given the instantiated service name (although typically the `.service` part can be left out).

After editing the configuration file, you can start the service with

```
sudo systemctl start satpulse@ttyAMA0.service
```

replacing `ttyAMA0` with the right value for your setup. You can then check that it is working with

```
sudo systemctl status satpulse@ttyAMA0.service
```

This will make the service run automatically after the system boots:

```
sudo systemctl enable satpulse@ttyAMA0.service
```

Here are some other systemd command you may need. Stop the service:

```
sudo systemctl stop satpulse@ttyAMA0.service
```

Restart the service (e.g. after editing the configuration file)

```
sudo systemctl restart satpulse@ttyAMA0.service
```

Enable the service:

```
sudo systemctl enable satpulse@ttyAMA0.service
```

Show the logs for the last 5 minutes:

```
sudo journalctl -u satpulse@ttyAMA0 -S -5m
```