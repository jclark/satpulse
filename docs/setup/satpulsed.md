---
title: Configure and run satpulsed
redirect_from:
  - /setup/without-phc.html
---

The main program in SatPulse is `satpulsed`, which is a daemon that
manages a GPS receiver: it configures the receiver, logs and distributes its packets,
provides monitoring over HTTP, and can act as a source of time.

Usually, satpulsed is run by the system's service manager (systemd on Linux or launchd on macOS).
It can also be run from the command line.
See the [satpulsed]({%link man/satpulsed.8.md%}) for details.
In either case a configuration is necessary to run satpulsed.
satpulsed needs access to the GPS serial device;
it does not need root privileges unless it is synchronizing a PHC.
satpulsed writes a log to stdout;
see [Running as a service](#running-as-a-service) for how to access this when running under a service manager.

## Configuration file

The service expects the configuration file to be at: 

- on Linux, `/etc/satpulse.toml` if you installed from a package,
  or `/usr/local/etc/satpulse.toml` if you installed from source
- on macOS, `/opt/homebrew/etc/satpulse.toml`

satpulsed reads the configuration file when it starts,
so restart it after changing the file.

The configuration file is in [TOML](https://toml.io/en/) format, which is inspired by the INI file format
and can be edited with a normal text editor (e.g. `nano`).

A minimal configuration file looks like this:

```
# Configuration file for satpulse
[serial]
speed = 9600
```

In the above, `#` starts a comment. The lines with square brackets mark the start of a *table*; the square brackets
enclose the name of the table. Following the start of each table are the key/value pairs in that table.
Values can be strings in double quotes
(e.g. `"/dev/ttyAMA0"`), numbers (e.g. `9600`) or booleans (e.g. `true`, `false`).

This minimal configuration makes satpulsed read packets from the receiver,
but not do much with them.
Further tables enable further functionality:
`[gps]` for receiver configuration (see below),
`[log]` for packet logging,
`[[http]]` for [monitoring]({% link setup/monitor.md %}),
`[[proxy.tcp]]` and `[[proxy.socket]]` for proxying the serial connection, and
`[ntp]` and `[phc]` for use as a time source
(see [Basic use with NTP]({% link setup/ntp.md %}) and [Precision timing with a PHC]({% link setup/phc.md %})).

In SatPulse 0.2 and older, the default configuration file installed by the package
has an `interface` key in the `[phc]` table;
comment it out unless you are [synchronizing a PHC]({% link setup/phc.md %}).

The `[serial]` table must specify the speed that the GPS is using,
and can also specify the serial device.
Usually the serial device is supplied by the service manager,
but it can also be added to the `[serial]` table,
which is useful for when satpulsed is run directly from the command-line, for example:

```
[serial]
device = "/dev/ttyAMA0"
speed = 9600
```

The configuration file is described in full in its man page [satpulse.toml(5)]({%link man/satpulse.toml.5.md %}).

## GPS configuration

Many GPS receivers have a factory default configuration that emits NMEA messages,
and satpulsed is workable with that:
by default, satpulsed does not change the receiver's configuration.

If you have a receiver for which SatPulse supports high-level configuration --
currently u-blox receivers (from the u-blox 6 platform through to the X20 platform)
and Unicore Nebulas IV receivers (UM980, UM981, UM982, UM960) --
then satpulsed can do additional configuration of the receiver.
The easiest approach is to add the following to `satpulse.toml`:

```
[gps]
config = true
vendor = "u-blox"
#vendor = "unicore"
```

`config = true` gives satpulsed permission to change the configuration of the receiver
(the default for `config` is false).
At startup, satpulsed then configures the receiver taking into account the other sections
of the configuration file, so that the receiver supports what the rest of the configuration asks for;
for example, with an `[ntp]` table it enables a time pulse and messages reporting UTC time.
It is a good idea to also specify `vendor`:
this restricts packet-format recognition and configuration probing to that vendor;
without it, all supported packet formats and configuration protocols are tried.
There are many other options that can be specified in the `[gps]` table to control
how satpulsed does configuration,
which are described in [satpulse.toml(5)]({%link man/satpulse.toml.5.md %}).
Note that the configuration changes made by satpulsed are never persistent;
you can always get rid of any changes done by satpulsed by power cycling the GPS.

With an unsupported receiver, you will need to configure it yourself,
as described in [GPS configuration]({%link setup/gps-config.md %}#unsupported-gps-modules).

There are some kinds of changes that satpulsed will not do:
- it will not change the serial speed of the module;
- it will not reset the module;
- it will not make changes to the constellations and signals used by the GPS, since this typically needs a reset to be effective.

These changes can be performed using the `gps` subcommand of `satpulsetool`,
documented in [satpulsetool-gps(1)]({%link man/satpulsetool-gps.1.md %}).

Note that satpulsed will not configure the GPS receiver to output messages about satellite positions and signals,
unless the serial speed is at least 38400.

See [GPS configuration]({%link setup/gps-config.md %}) for more information about using `satpulsetool gps`. 

## Running as a service

### Linux

On Linux, satpulsed usually runs as a service using systemd.
The service template name is `satpulse@.service` and the expected argument is the serial device name without `/dev/`.
For example, if the serial device is `/dev/ttyAMA0`, then the instantiated service would be named `satpulse@ttyAMA0.service`.
Systemd commands need to be given the instantiated service name (although typically the `.service` part can be left out).

Start the service with

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

Restart the service:

```
sudo systemctl restart satpulse@ttyAMA0.service
```

The service logs to the systemd journal.
Use `journalctl` to view the logs:

```
sudo journalctl -u satpulse@ttyAMA0
```

Show logs for the last 5 minutes:

```
sudo journalctl -u satpulse@ttyAMA0 -S -5m
```

Follow the logs in real-time:

```
sudo journalctl -u satpulse@ttyAMA0 -f
```

Show only warnings and errors:

```
sudo journalctl -u satpulse@ttyAMA0 -p 0..4
```

### macOS

On macOS, the service is managed with `brew services`,
as described in the [Homebrew tap](https://github.com/jclark/homebrew-satpulse). {% include new-in-03.html %}
The service writes the daemon's log output to files under `/opt/homebrew/var/log/satpulse/`.
