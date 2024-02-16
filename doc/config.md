# Configuring SatPulse

Before doing the configuration described in this document,
you should install SatPulse either from source or from a package (`.deb` or `.rpm` file).

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

Installation will install a configuration file and a systemd service template.
You will need to edit the configuration file and use systemd commands with the name of the service template.

### Configuration file

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
* the serial speed is 9600 baud
* the GPS receiver should be configured to work optimally for timing; note that this won't make any persistent changes to the GPS
  receiver, so you can turn it off and on again to undo any changes
* ptp4l should be updated using the Unix domain socket at `/var/run/ptp4l`
* timing samples should be sent to chrony using the SOCK protocol through a socket at `/var/run/chrony.satpulse.sock`


### Systemd commands

The systemd service template name is `satpulse@.service` and the expected argument is the serial device name without `/dev/`.
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

### Install and configure ptp4l as a PTP server

Install the linuxptp package:
   * On Debian: `sudo apt install linuxptp`
   * On Fedora: `sudo dnf install linuxptp`

On Debian, the system supplied ptp4l service is not ideal, in particular it won't work for the Raspberry Pi CM4,
So you should install a replacement `ptp4l.service` file as `/etc/systemd/system/ptp4l.service`.
The replacement is in

*  `configs/ptp4l.service` in the source, 
*  `/usr/share/doc/satpulse/ptp4l.service when the .deb package has been installed

Next you will need to edit the ptp4l config file.
   * On Debian: the file is `/etc/linuxptp/ptp4l.conf`
   * On Fedora: the file is `/etc/ptp4l.conf`

You can start with this:

```
[global]
# We don't want ptp4l and satpulse to both adjust the PHC, so only run as a master.
masterOnly 1

# Uncomment this for rPI CM4
# tx_timestamp_timeout 100

# The presence of this section makes ptp4l run on this interface.
[eth0]
```

The network interface in the last line should match the interface in `satpulse.toml`.

Then start and enable the ptp4l service:

```
sudo systemctl enable --now ptp4l
```

### chrony

Add this to your chrony.conf file:

```
refclock SOCK /var/run/chrony.satpulse.sock poll 2 filter 4 refid GNSS
```

The socket path here `/var/run/chrony.satpulse.sock` needs to match that specified by the `sock.path` key in the `ntp` table.

Then restart chrony.

## Configuration file details

### TOML syntax

Note that
* Order of tables is not significant.
* Order of key/value pairs within a table is not significant.
* Case is significant.
* Numbers can use exponenential format e.g. `21.3e6`

As well as strings, numbers and booleans, values can be

* arrays e.g. `[1,2,3]` is an array of three numbers
* dates e.g. `2024-02-23` is 23rd February 2024 (note no double quotes here)

As well as tables, the configuration file can contain *table arrays*. With table arrays, the name
is enclosed in double square brackets, and each name can occur multiple times.
See `http` for an example.

### Schema

There is a JSON schema for the configuration file, which is installed in

* `/usr/share/doc/satpulse/config-schema.json` when installed from a package and
* `/usr/local/share/doc/satpulse/config-schema.json` when installed from source

With Visual Studio Code, the [Even Better TOML](https://marketplace.visualstudio.com/items?itemName=tamasfe.even-better-toml)
extension supports schema-sensitive editing. The first line of the TOML file can have a line like this:

```
#:schema /usr/share/doc/satpulse/config-schema.json
```

to tell the extension which schema to use.

### `phc` table

The `phc` table is about the PTP hardware clock. It can have following keys:

* `interface` - a string giving the name of an ethernet interface with the pin that the GPS's PPS output is attached to
* `pin` - an integer giving the index of the pin; the default is 0 (which is the only pin on the Raspberry Pi CM4)
* `channel` - an integer giving the timestamping channel to use; the default is 0; the only case in which you need
  to change the default is if you have multiple pins in use; in this case, each pin must specify a distinct channel

Example

```
[phc]
interface = "enp4s0"
pin = 1
channel = 0
```

### `serial` table

The `serial` table provides information about the serial connection between the GPS receiver and the computer.
It can have the following keys:

* `speed` - an integer giving the speed of the connection in bits-per-second (baud)
* `device` - a string giving the path of the serial device name; when SatPulse is run via systemd, the
  device will usually be specified in systemd commands, which will override any value specified here


### `gps` table

The `gps` table is about how to configure the GPS receiver. It can have the following keys:

* `config` - a boolean saying whether to perform configuration of the GPS receiver; `true` means to perform configuration;
  currently this works only with GPS receivers that support the UBX protocol (like those from u-blox)
  this won't make any persistent changes to the GPS receiver, which means you can turn the receiver off and on again to undo any changes made by SatPulse;
  if you use `false` here, then all the other keys in the table will be ignored
  and it is your responsibility to configure the GPS receiver appropriately
* `gnss` - a string giving the GNSS system to which the time pulse should be aligned; the GNSS specified here must be already be enabled on the receiver
  (SatPulse will not change the enabled GNSS systems since that is a rather disruptive operation); possible values are
   * `"GPS"` for the GNSS system operated by the USA
   * `"GAL"`, `"Galileo"`for the GNSS system operated by the EU
   * `"BDS"`, `"BeiDou"` for the GNSS system operated by China
   * `"GLO"`, `"GLONASS"` for the GNSS system operated by Russia
* `timeMode` - a boolean saying whether to enable time mode on the GPS receiver; GPS receivers designed for timing typically offer a mode
in which they assume the the antenna has a known fixed position; they can compute the time using the signal from a single satellite;
the position can be established by having the GPS spend some time determing the position itself (called a survey); or by explicitly
specifying the position; the default is `true`
* `surveyTime` - a boolean giving the time in seconds to perform a survey to establish the position of the GPS receiver antenna;
   SatPulse will only do a survey when `timeMode` is true and no fixed position has been set; the default is 2000
* `fixedPosECEF` - an array of three numbers giving the ECEF coordinates in meters of the GPS receiver's antenna receiver; if SatPulse initiaties a survey,
  then it will log the position determined by the survey when the survey finishes
* `fixedPosAcc` - a number giving the accuracy in meters of the `fixedPosECEF` coordinates; SatPulse will log the accuracy along with the position when
  a survey finishes
* `antennaCableLength` - a number giving the length in meters of the antenna cable; this is used to set the antenna cable delay in conjunction with the
  `antennaCableVF` key; the default is to not change the GPS receiver's configuration of the antenna cable delay
* `antennaCableVF`- a number giving the velocity factor of the antenna cable; the default is 0.66, which is appropriate for RG-58 cable
* `antennaCableDelay` - a number giving the delay in nanoseconds resulting from the propagation of the GPS signal through the antenna cable;
  the default is not to change the GPS receiver's configuration of the antenna cable delay; if both the `antennaCableLength` and the `antennaCableDelay`
  are specified, then the sum of the delays will be used


Example

```
[gps]
config = true
gnss = "GAL"
```


### `leapSecond` table

The `leapSecond` table gives information about the most recently announced leap second. It has the following keys:

* `date`: a date specifying when the leap second occurs in form YYYY-MM-DD; the month and day should be `06-30` or `12-31`
* `before`: the difference between TAI and UTC time before the leap second
* `after`: the difference between TAI and UTC time after the leap second; with a positive leap second, `after` will be one more than `before`


The default is:

```
[leapSecond]
date=2016-12-31
after=37
before=36
```

### `ptp` table

SatPulse can update a PTP server (called *grandmaster* in traditional PTP terminology) with metadata about the time.
Currently the only supported server is `ptp4l`. Updating `ptp4l` is enabled by specifying the `ptp4l.udsAddress`.

The `ptp` table controls this. It can have the following keys:

* `ptp4l.udsAddress` - a string giving the path of the Unix domain socket used by ptp4l for PTP management. By default
   ptp4l uses `/var/run/ptp4l`, but it can be changed with the ptp4l `uds_address` option. This key must be supplied
   to enable SatPulse to update ptp4l.
* `domain` - the PTP domain number; this defaults to 0
* `majorSdoId` - the PTP majorSdoId; this defaults to 0; in earlier versions of the PTP standard this is called `transportSpecific`
* `minorSdoId` - the PTP minorSdoId; this defaults to 0

Example

```
[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"
```

### `ntp` table

The `ntp` table controls how SatPulse sends information to an NTP daemon. Currently only chrony is supported.
It can have the following keys:

* `sock.path` - a string 

Example

```
[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

### `log` table

The `log` table controls how SatPulse should log information.

XXX

### `http` table array

The `http` table array specifies the port on which to enable an HTTP server for monitoring. It has a single key:

* `listen` - a string specifying an address that the HTTP server should listen on; the address is the form *host*`:`*port*,
where the *host* is a host name or IP address, and port is the port number; *host* can be omitted, which means to listen on all
IP addresses

This example would run an HTTP server on port 2006 on all IP addresses:

```
[[http]]
listen = ":2006"
```

This would listen on two specific addresses:

```
[[http]]
listen = "192.168.1.1:2006"

[[http]]
listen = "192.168.2.1:2006"
```

### `proxy.tcp` and `proxy.sock` table arrays

XXX

