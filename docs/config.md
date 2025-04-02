---
title: SatPulse configuration file
---
## Location

The SatPulse configuration file will be at

- `/etc/satpulse.toml` if you installed from a package
- `/usr/local/etc/satpulse.toml` if you installed from source

## Syntax

The configuration file is in [TOML](https://toml.io/en/) format, which is inspired by the INI file format
and can be edited with a normal text editor (e.g. `nano`).

Comments begin with a `#` and continue to the end of the line.
The configuration file consists of tables and table arrays:
lines enclosed in single square brackets start tables;
lines enclosed in double square brackets start table arrays.
The name of the table or table array occurs within the square brackets.
With a table, there is only one instance of the table.
With a table array, there is one instance for each double square bracket line.

After the starting line, a table or table array consists of key/value pairs
of the form *key* `=` *value*. A value can be 

* a string in double quotes e.g. `"eth0"`
* a number e.g. `2`, `9600`, `1.73e5`
* a boolean e.g. `true`, `false`
* a date e.g. `2024-02-23` is 23rd February 2024 (note no double quotes here)
* an array e.g. `[1,2,3]` is an array of three numbers

Note that
* Order of tables is not significant.
* Order of key/value pairs within a table is not significant.
* Case is significant.

Example

```
# Configuration file for satpulse
[phc]
interface = "enp1s0"
pin  = 0

[serial]
speed = 9600

[gps]
config = true

[[http]]
listen = "192.168.1.1:2006"

[[http]]
listen = "192.168.1.2:2006"
```

## Schema

There is a JSON schema for the configuration file, which is installed in

* `/usr/share/doc/satpulse/config-schema.json` when installed from a package and
* `/usr/local/share/doc/satpulse/config-schema.json` when installed from source

With Visual Studio Code, the [Even Better TOML](https://marketplace.visualstudio.com/items?itemName=tamasfe.even-better-toml)
extension supports schema-sensitive editing. The first line of the TOML file can have a line like this:

```
#:schema /usr/share/doc/satpulse/config-schema.json
```

to tell the extension which schema to use.

## `phc` table

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

## `serial` table

The `serial` table relates to the serial connection between the GPS receiver and the computer.
It can have the following keys:

* `speed` - an integer giving the speed of the connection in bits-per-second (baud)
* `device` - a string giving the path of the serial device name; when SatPulse is run via systemd, the
  device will usually be specified in systemd commands, which will override any value specified here


## `gps` table

The `gps` table relates to configuration of the GPS receiver. It can have the following keys:

* `config` - a boolean saying whether to perform configuration of the GPS receiver; `true` means to perform configuration;
  currently this works only with GPS receivers that support the UBX protocol (like those from u-blox)
  this won't make any persistent changes to the GPS receiver, which means you can turn the receiver off and on again to undo any changes made by SatPulse;
  if you use `false` here, then all the other keys in the table other than `pulseWidth` will be ignored
  and it is your responsibility to configure the GPS receiver appropriately
* `pulseWidth` - a number giving the time pulse width in seconds that this GPS receiver has been configured with; this is not relevant
   unless the ethernet controller is one that timestamps both edges (e.g. Intel i210); it is also not relevant if SatPulse
   is performing configuration of the GPS receiver (in which case the pulse width will be set to a known value); if this is not
   specified, then SatPulse will attempt to retrieve the configured time pulse width using the UBX protocol
* `gnss` - a string giving the GNSS system to which the time pulse should be aligned; the GNSS specified here must be already be enabled on the receiver
  (SatPulse will not change the enabled GNSS systems since that is a rather disruptive operation); possible values are
   * `"GPS"` for the GNSS system operated by the USA
   * `"GAL"`, `"Galileo"`for the GNSS system operated by the EU
   * `"BDS"`, `"BeiDou"` for the GNSS system operated by China
   * `"GLO"`, `"GLONASS"` for the GNSS system operated by Russia
* `stationary` - a boolean saying whether the GPS receiver is stationary; if this is true, the receiver will be configured to assume a stationary
   position; on a timing receiver, time mode will be enabled, which is a mode in which a fixed position is established for the receiver,
   and thereafter the time is computed using  a single satellite;
   the position can be established by having the GPS spend some time determing the position itself (called a survey)
   or by explicitly specifying the position (using `fixedPosECEF`); the default is `true`
* `surveyTime` - a boolean giving the time in seconds to perform a survey to establish the position of the GPS receiver antenna;
   SatPulse will only do a survey when `stationary` is true and no fixed position has been set; the default is 2000
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
fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
```

## `leapSecond` table

The `leapSecond` table relates to leap seconds. It has the following keys:

* `date` - a date giving the date of the most recently announced the leap second in the form YYYY-MM-DD; the month and day should be `06-30` or `12-31`
* `before` - an integer giving the difference in seconds between TAI and UTC time before the leap second
* `after`- an integer giving the difference in seconds between TAI and UTC time after the leap second; with a positive leap second, `after` will be one more than `before`


The default is:

```
[leapSecond]
date=2016-12-31
after=37
before=36
```

## `ptp` table

SatPulse can update a PTP server (called *grandmaster* in traditional PTP terminology) with metadata about the time.
Currently the only supported server is `ptp4l`. Updating `ptp4l` is enabled by specifying the `ptp4l.udsAddress`.

The `ptp` table controls this. It can have the following keys:

* `ptp4l.udsAddress` - a string giving the path of the Unix domain socket used by ptp4l for PTP management. By default
   ptp4l uses `/var/run/ptp4l`, but it can be changed with the ptp4l `uds_address` option. This key must be supplied
   to enable SatPulse to update ptp4l.
* `domain` - the PTP domain number; this defaults to 0
* `majorSdoId` - the PTP majorSdoId; this defaults to 0; in earlier versions of the PTP standard this is called `transportSpecific`
* `minorSdoId` - the PTP minorSdoId; this defaults to 0
* `clockAccuracy` - the accuracy in nanoseconds of the PTP grandmaster instance when synchronized to the GPS receiver; SatPulse will consider the PHC to be *in sync* when it has succeeded in adjusting the PHC so that the absolute values of the offsets between pulses from the GPS receiver and the PHC are consistently less than this; when SatPulse considers the PHC in sync, it will set the the clockClass attribute of the PTP grandmaster instance to 6, meaning that it is synchronized to a primary reference; when out of sync, it will set the clockCass to 52, meaning that it is degraded; when the clock class is 6, it will set the clockClass attribute to be the value specified by this key, rounded up to a value supported by PTP (e.g. 10, 25, 100, 250, 1000); the default is 150, which should be easily achievable with any GPS and which will result in a PTP clockAccuracy of 0x22, meaning accurate to within 250ns (thus allowing 150ns of error from the offset and 100ns from other source)

Example

```
[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"
```

## `ntp` table

The `ntp` table is about how SatPulse sends information to an NTP daemon. Currently only chrony is supported.
It can have the following keys:

* `sock.path` - a string 

Example

```
[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

## `log` table

The `log` table is about how SatPulse should log information. SatPulse does two kinds of logging: it logs through systemd,
and it can also write its own applicaton-specific log files.

The following keys relate to logging through systemd:

* `verbose` - a boolean saying whether to log verbosely; default is false
* `interval` - an integer giving the time in seconds over which a log message should summarize the synchronization status;
  the default is 30; the status is computed once per second, and a value of 1 will log that status directly; a value
  of 0 will not log the synchronization status

The following keys relate to its own log

* `clock` - a boolean saying whether to create a *clock* log; the clock log is in text format and records offsets between the PHC and the GPS;
   it is intended to be convenient for statistical analysis
* `event` a boolean saying whether to create an  *event* log; the event log is on JSON Lines format and
  records the events input into the synchronization process (pulses and GPS messages)
* `dir` - a string giving the directory in which to write log files; this defaults to `/var/log/satpulse`

## `http` table array

The `http` table array controls the HTTP monitoring interface. It has a single key:

* `listen` - a string specifying an address that the HTTP server should listen on; the address is the form *host*`:`*port*,
where the *host* is a host name or IP address, and port is the port number; *host* can be omitted, which means to listen on all
IP addresses

This example would run an HTTP server on port 2006 on all IP addresses:

```
[[http]]
listen = ":2006"
```

This example would listen on two specific IP addresses:

```
[[http]]
listen = "192.168.1.1:2006"

[[http]]
listen = "192.168.2.1:2006"
```

## `proxy.tcp` and `proxy.sock` table arrays

The `proxy.tcp` table array provides network access to the GPS receiver over TCP/IP.
This can be used with, for example, [u-center](https://www.u-blox.com/en/product/u-center)
or [Lady Heather](http://www.ke5fx.com/heather/readme.htm) (with the `/ip` option) to monitor your GPS.
**SatPulse does not perform any access control:
using `proxy.tcp` without `readOnly=true` allows anyone on the local network to write to your GPS receiver.**

It has the following key:

* `listen` - a string specifying an address that the TCP server should listen on; the address is the form *host*`:`*port*,
where the *host* is a host name or IP address, and port is the port number; *host* can be omitted, which means to listen on all
IP addresses; this key is required in each `[[proxy.tcp]]` table array element

The `proxy.sock` table array provides access to the GPS receiver over a Unix domain socket.


It has the following key:

* `path` - a string specifying the path of the socket; this key is required in each `[[proxy.sock]]` table array element;
  this key is required in each `[[proxy.sock]]` table array element
* `group` - the group id for the socket path; by default this is `dialout` (this is the usual group for the tty devices,
   so a user that can access tty devices will be able to access the socket); when the socket is not readOnly, then the mode
   will be 0660, so only root and members of this group will be able to write to the socket

In addition, `proxy.tcp` and `proxy.sock` can both have the following keys:

* `readOnly` - a boolean saying whether access to the GPS receiver should be read-only; this means that serial packets will be forwarded from the GPS receiver to the network, but the network will not be able to send packets to the GPS receiver
* `nmeaOnly` - a boolean saying whether this should only proxy NMEA packets
* `writeLockTimeout` - a number giving the time in seconds that a writer to the GPS receiver should have exclusive write access; if client writes to the GPS receiver (which is allowed only when readOnly is false), then no other client will be able to write to the GPS receiver for this period of time; the default is 2 seconds
