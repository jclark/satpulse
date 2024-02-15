# Configuring SatPulse

Before doing the configuration described in this document,
you should install SatPulse either from source or from a package (.deb or .rpm).

## Required information

The GPS receiver must have two connections to the computer on which SatPulse is running.

First, there will be a serial connection between the USB or serial port on the GPS receiver to a USB or serial port on the computer.
You need to know which device this is. On a Raspberry Pi CM4 it is typically `/dev/ttyAMA0`.
On a PC, it might be `/dev/ttyS0` or `/dev/ttyACM0` or `/dev/ttyUSB0`, with possibly a higher number than `0`.
You also need to know the speed the GPS receiver is using; 9600 is the most common speed, but some recent devices use 38400.

Second, there will be a PPS connection between the PPS output of the GPS receiver and the pin on the ethernet controller.
You need to know the the name of the ethernet interface. On a Raspberry Pi CM4 using Raspberry Pi OS, it is typically `eth0`.
On a PC, it is typically something like `enp1s0` or `enp2s0`.
You also need to known the index of the pin; this is usually 0, but on a PC, could also be 1 or 2.

If you put the wrong values for these, there will be an error in the logs.

## Quickstart

After installation, there will be a configuration file and a systemd service template.

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
pin = 0

[serial]
speed = 9600
```

In the above, `#` starts a comment. The `[phc]` and `[serial]` lines mark the start of a *table*; the square brackets
enclose the name of the table.
`interface = "enp1s0"` is a key/value pair within the `phc` table. Values can be strings in double quotes
(e.g. `"enp1s0"`), numbers (e.g. 9600) or booleans (e.g. `true`, `false`).

The `phc` table gives information about the PTP hardware clock that SatPulse will be working with.
The `interface` key specifies the name of the ethernet interface with which the PTP hardware clock is associated.
The `pin` key specified the index of the pin that the GPS's PPS output is attached to; the default for this is 0.

The `serial` table provides information about the serial connection between the GPS receiver and the computer.
The `speed` gives the speed of the connection in bits-per-second (baud).

Although the serial device name can be specified in the configuration file, the systemd service template expects it to be supplied as an argument.
The systemd service template name is `satpulse@.service` and the argument for the systemd service template is the serial device name without `/dev/`.
For example, if the serial device is `/dev/ttyS0`, then the instantiated service would be named `satpulse@ttyS0.service`.
Systemd commmands need to be given the instantiated service name (although typically the `.service` part can be left out).

After editing the configuration file, you can start the service with

```
sudo systemctl start systemd@ttyS0.service
```

replacing `ttyS0` with the right value for your setup. You can the check that it is working with

```
sudo systemctl status systemd@ttyS0.service
```

This will make the service run automatically after the system boots:

```
sudo systemctl enable systemd@ttyS0.service
```

Here are some other systemd command you may need. Stop the service:

```
sudo systemctl stop systemd@ttyS0.service
```

Restart the service (e.g. after editing the configuration file)

```
sudo systemctl stop systemd@ttyS0.service
```

Enable the service:

```
sudo systemctl enable systemd@ttyS0.service
```

Show the logs for the last 5 minutes:

```
sudo journalctl -u satpulse@ttyACM0 -S -5m
```











