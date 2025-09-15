---
title: satpulse service
---

The satpulse service needs a configuration file and is run using systemd.

## Configuration file

The configuration file will be at

- `/etc/satpulse.toml` if you installed from a package
- `/usr/local/etc/satpulse.toml` if you installed from source

The configuration file is in [TOML](https://toml.io/en/) format, which is inspired by the INI file format
and can be edited with a normal text editor (e.g. `nano`).

A minimal configuration file would look like this:

```
# Configuration file for satpulse
[phc]
interface = "enp1s0"

[serial]
speed = 9600

# following are optional
[gps]
config = true

[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"

[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

In the above, `#` starts a comment. The lines with square brackets mark the start of a *table*; the square brackets
enclose the name of the table. Following the start of each table are the key/value pairs in that table.
Values can be strings in double quotes
(e.g. `"enp1s0"`), numbers (e.g. `9600`) or booleans (e.g. `true`, `false`).

The above configuration file specifies that

* the ethernet interface with the pin that the GPS output pin is attached to is `enp1s0` (the pin index is defaulted to 0)
* the serial speed is 9600 baud (the serial device is usually specified by systemd commands)
* the GPS receiver should be configured to work optimally for timing; note that this won't make any persistent changes to the GPS
  receiver, so you can turn it off and on again to undo any changes
* ptp4l should be updated using the Unix domain socket at `/var/run/ptp4l`
* timing samples should be sent to chrony using the SOCK protocol through a socket at `/var/run/chrony.satpulse.sock`


## Using systemd with the satpulse service

The systemd service template name is `satpulse@.service` and the expected argument is the serial device name without `/dev/`.
For example, if the serial device is `/dev/ttyAMA0`, then the instantiated service would be named `satpulse@ttyAMA0.service`.
Systemd commands need to be given the instantiated service name (although typically the `.service` part can be left out).

After editing the configuration file, you can start the service with

```
sudo systemctl start satpulse@ttyAMA0.service
```

replacing `ttyAMA0` with the right value for your setup. You can the check that it is working with

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