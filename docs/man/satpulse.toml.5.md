# NAME

satpulse.toml - satpulsed configuration file

# DESCRIPTION

The SatPulse configuration file will be conventionally stored at

- `/etc/satpulse.toml` if you installed from a package
- `/usr/local/etc/satpulse.toml` if you installed from source

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

There is a JSON schema for the configuration file, which is installed in

* `/usr/share/doc/satpulse/config-schema.json` when installed from a package and
* `/usr/local/share/doc/satpulse/config-schema.json` when installed from source

With Visual Studio Code, the [Even Better TOML](https://marketplace.visualstudio.com/items?itemName=tamasfe.even-better-toml)
extension supports schema-sensitive editing. The first line of the TOML file can have a line like this:

```
#:schema /usr/share/doc/satpulse/config-schema.json
```

to tell the extension which schema to use.

# TABLES

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
  if you use `false` here, then all the other keys in the table other than `vendor` will be ignored
  and it is your responsibility to configure the GPS receiver appropriately
* `vendor` - a string giving the manufacturer of the GPS module; this restricts which packet formats the scanner recognizes and which configuration protocols are probed; it is also used to interpret non-standard numbering of space vehicles in NMEA GSV and GSA sentences; the following values are supported: `"u-blox"`, `"Unicore"`, `"Allystar"`, `"Bynav"`, `"NovAtel"`, `"Quectel"`, `"SinoGNSS"`, `"Techtotop"`, `"Zhongke"`, `"other"`; in addition, the following values are allowed and currently treated as equivalent to `"other"`: `"Furuno"`, `"MediaTek"`, `"Septentrio"`, `"SkyTraq"`, `"Trimble"`; values are case-insensitive; if `vendor` is not specified, no restrictions are applied
* `timeGNSS` - a string giving the GNSS system to which the time pulse should be aligned; the GNSS specified here must be already be enabled on the receiver
  (SatPulse will not change the enabled GNSS systems since that is a rather disruptive operation); possible values are
   * `"GPS"` for the GNSS system operated by the USA
   * `"GAL"`, `"Galileo"`for the GNSS system operated by the EU
   * `"BDS"`, `"BeiDou"` for the GNSS system operated by China
   * `"GLO"`, `"GLONASS"` for the GNSS system operated by Russia
* `mobile` - a boolean saying whether the GPS receiver is mobile; if this is false, the receiver will be configured to assume a stationary
   position; on a timing receiver, time mode will be enabled, which is a mode in which a fixed position is established for the receiver,
   and thereafter the time is computed using a single satellite;
   the position can be established by having the GPS spend some time determining the position itself (called a survey)
   or by explicitly specifying the position (using `fixedPosECEF` or `fixedPosLLH`); the default is `false`
* `surveyTime` - a number giving the time in seconds to perform a survey to establish the position of the GPS receiver antenna;
   SatPulse will only do a survey when `mobile` is false and no fixed position has been set; the default is 2000
* `surveyAcc` - a number giving the accuracy in meters that the survey must achieve; the survey will continue until this accuracy
   is reached and the survey time has elapsed; the default is 20
* `resurvey` - a boolean saying whether to force a new survey even if a survey has already been completed; the default is `false`
* `fixedPosECEF` - an array of three numbers giving the ECEF coordinates in meters of the GPS receiver's antenna; this must not be specified with `fixedPosLLH`; if SatPulse initiates a survey,
  then it will log the position determined by the survey when the survey finishes
* `fixedPosLLH` - an array of three numbers giving latitude in degrees, longitude in degrees, and WGS84 ellipsoid height in meters of the GPS receiver's antenna; this must not be specified with `fixedPosECEF`
* `fixedPosAcc` - a number giving the accuracy in meters of the fixed position coordinates; SatPulse will log the accuracy along with the position when a survey finishes
* `minElevation` - a number giving the minimum elevation in degrees for satellites to be used by the GPS receiver; the default is to not change the GPS receiver's configuration
* `antennaCableLength` - a number giving the length in meters of the antenna cable; this is used to set the antenna cable delay in conjunction with the
  `antennaCableVF` key; the default is to not change the GPS receiver's configuration of the antenna cable delay
* `antennaCableVF`- a number giving the velocity factor of the antenna cable; the default is 0.66, which is appropriate for RG-58 cable
* `antennaCableDelay` - a number giving the delay in nanoseconds resulting from the propagation of the GPS signal through the antenna cable;
  the default is not to change the GPS receiver's configuration of the antenna cable delay; if both the `antennaCableLength` and the `antennaCableDelay`
  are specified, then the sum of the delays will be used
* `rtcmOutput` - a boolean saying whether the GPS receiver should be configured to output RTCM messages; if true, then the receiver will be configured to generate MSM messages for each enabled constellation; it will also generate ARP (1005) messages, and if GLONASS is enabled, GLONASS code phase bias (1230) messages; if false, then RTCM messages will be turned off; if the key is not specified, then the configuration of RTCM messages will not be changed
* `rtcmPreferMSM7` - a boolean saying whether MSM7 messages should be preferred over MSM4 messages; when `rtcmOutput` is true, the receiver will be configured to generate MSM7 or MSM4 messages; if the receiver does not support the preferred kind of MSM message, then the other kind will be generated; the default is true when any Ntrip mountpoint or RTCM stream push has MSM7-to-MSM4 conversion enabled, and false otherwise
* `rtcmBaseID` - an integer giving the RTCM reference station ID (DF003) to use in RTCM messages generated by the GPS receiver; the value must be between 0 and 4095; the default is to not change the GPS receiver's configuration
* `satellitesOutput` - a boolean saying whether the GPS receiver should be configured to output information about the satellites in view; this information can be used by the HTTP monitoring interface; the default is to configure output based on whether the HTTP interface is enabled, but not to perform configuration unless the serial speed is at least 38400 (this is because at low serials speeds the satellites output can cause significant delays in receiving the timing messages that SatPulse relies upon)

Example

```
[gps]
config = true
fixedPosLLH = [51.5007, -0.1246, 11.0]
fixedPosAcc = 0.05
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
* `domainNumber` - the PTP domain number; this defaults to 0
* `majorSdoId` - the PTP majorSdoId; this defaults to 0; in earlier versions of the PTP standard this is called `transportSpecific`
* `minorSdoId` - the PTP minorSdoId; this defaults to 0
* `clockAccuracy` - the accuracy in nanoseconds of the PTP grandmaster instance when synchronized to the GPS receiver.
   This will be rounded up to a value allowed by the PTP clockAccuracy enumeration (e.g. 10, 25, 100, 250, 1000).
   The value before being rounded up is used to choose buckets for Prometheus observability.
   The default is 150, which will be rounded up to 250, corresponding to the 0x22 clockAccuracy enumerated constant.
* `offsetScaledLogVariance` - the offsetScaledLogVariance of the PTP grandmaster instance when synchronized to the GPS receiver.
   This is a measure of the clock stability.
   It is an integer in the range 0x0 to 0xFFFF, which provides an efficient, PTP-specific encoding of the Allan deviation.
   The default is 0xFFFF, which means unknown.
   It is usually more convenient to specify this using the `allanDeviation` key,
   but some PTP profiles require specific integer values.
* `allanDeviation` - the Allan Deviation at a tau of 1 second.
   If specified, this is used to compute `offsetScaledLogVariance`.

Example

```
[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"
```

## `ntp` table

The `ntp` table controls how SatPulse sends samples to an NTP daemon.
It supports two protocols: the refclock SOCK protocol defined by chrony, and the SHM protocol defined by NTP.

If the `[phc]` table is not present, then the samples will be based on the timing of the serial messages.
This is imprecise but is useful when the NTP daemon has a separate source of PPS samples, which do not include time-of-day information.
The samples from SatPulse can be used to complete the PPS samples.

The following key enables use of the refclock SOCK protocol:

* `sock.path` - a string giving the path of the Unix domain socket that is used to communicate with chrony using the SOCK protocol;
  the NTP daemon must be configured with a matching path; with chrony this is specified in the `refclock SOCK` line

Example:

```
[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

The following keys are used with the SHM protocol:

* `shm.segment` - an integer from 0 to 255 giving the NTP SHM segment index;
  this key enables use of the SHM protocol
* `shm.precision` - an optional integer from -128 to 127 overriding the SHM precision field, in log2 seconds;
  the default is for SatPulse to choose a precision based on how the sample was generated

The segment is created with mode 0600 when the index is 0 or 1, and otherwise with mode 0666,
so indices 0 and 1 are only suitable when satpulsed and the NTP daemon run as the same user.

Example:

```
[ntp]
shm.segment = 2
```

## `log` table

The `log` table is about how SatPulse should log information. SatPulse does two kinds of logging: it logs through systemd,
and it can also write its own application-specific log files.

The following keys relate to logging through systemd:

* `verbose` - a boolean saying whether to log verbosely; default is false
* `interval` - an integer giving the time in seconds over which a log message should summarize the synchronization status;
  the default is 30; the status is computed once per second, and a value of 1 will log that status directly; a value
  of 0 will not log the synchronization status

The following keys relate to its own log files:

* `clock` - a boolean saying whether to create a *clock* log; the clock log is in text format and records offsets between the PHC and the GPS;
   it is intended to be convenient for statistical analysis
* `packet` - a boolean saying whether to create a *packet* log; the packet log is in JSON Lines format and
  records packets received by and sent to the GPS receiver
* `event` - a boolean saying whether to create an *event* log; the event log is in JSON Lines format and
  records the events input into the synchronization process (pulses and GPS messages)
* `track` - a boolean saying whether to create a *track* log; the track log is in JSON Lines format and
  records one trackpoint per navigation epoch
* `dir` - a string giving the directory in which to write log files; this defaults to `/var/log/satpulse`

## `http` table array

The `http` table array controls the HTTP monitoring interface. It has the following keys:

* `listen` - a string specifying an address that the HTTP server should listen on; the address is the form *host*`:`*port*,
where the *host* is a host name or IP address, and port is the port number; *host* can be omitted, which means to listen on all
IP addresses
* `gui` - a boolean saying whether to enable the web GUI at `/` and Server-Sent Events at `/sse`; default is `true`
* `metrics` - a boolean saying whether to enable Prometheus metrics at `/metrics`; default is `true`
* `position` - a boolean saying whether to enable the current GPS position at `/position` as JSON; default is `true`

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

Note that the HTTP monitoring interface can provide a graphical view of the available satellites, but the GPS receiver needs to output the necessary information.
If the HTTP monitoring interface is enabled and GPS configuration is enabled, then the GPS receiver will be automatically configured to output this information.
However, this will be done only if the serial speed is at least 38400.
This can be overridden using the `satellitesOutput` key in the `gps` table.
But if you want the graphical view, it is recommended to increase the serial speed to at least 38400.

## `ntrip` table

The `ntrip` table controls the Ntrip caster. The caster serves RTCM correction data from the GPS receiver to Ntrip clients.
It starts only when at least one `[[ntrip.mountpoint]]` table is configured.
The same table also provides source table defaults for RTCM streams pushed to a remote caster using `[[stream.push]]`.
A mountpoint with just a name is enough for a working caster:

```
[[ntrip.mountpoint]]
name = "BKK"
```

The `ntrip` table can have the following keys:

* `listen` - a string giving the TCP listen address; the default is `":2101"`
* `country` - a string giving the country code reported in the source table; the default is `"XXX"`
* `network` - a string giving the network name reported in the source table; the default is `"Misc"`
* `lat`, `lon` - numbers giving the latitude and longitude reported in the source table; they must be specified together; the default is from the GPS receiver's fixed position, if any, otherwise 0
* `generator` - a string giving the generator reported in the source table; the default is derived from the receiver or SatPulse version
* `formatDetails` - a string giving the RTCM message list reported in the source table; when MSM7 to MSM4 conversion is being done, this should describe the messages before conversion; the conversion will also substitute MSM7 message numbers with MSM4 message numbers; the default is derived from the enabled GNSS systems
* `bitrate` - an integer giving the default bitrate reported in the source table for mountpoints and pushed RTCM streams; the default is 0

Each `[[ntrip.mountpoint]]` table has the following keys:

* `name` - a string giving the mountpoint name; this is required and must be a single URL path component
* `description` - a string giving the mountpoint description reported in the source table; the default is the mountpoint name
* `bitrate` - an integer giving the bitrate reported in the source table for this mountpoint; the default is `ntrip.bitrate`
* `auth` - a table saying that clients must authenticate to use this mountpoint; the default is not to authenticate
* `msm7to4` - a boolean saying whether MSM7 RTCM packets should be converted to MSM4 before being sent to clients; non-MSM7 packets are forwarded unchanged; the default is false

The `auth` table has the following keys:

* `anyUser` - a boolean saying whether any user defined in the `[[user]]` table may use this mountpoint
* `users` - an array of strings giving the names of users that may use this mountpoint

At most one of `auth.anyUser` and `auth.users` can be specified; if neither is, then no user can use the mountpoint.

Example

```
[[user]]
name = "rover"
password = "secret"

[ntrip]
listen = ":80"
country = "THA"

[[ntrip.mountpoint]]
name = "BKK"
description = "Bangkok"
auth.users = ["rover"]
```

## `user` table array

The `user` table array defines users that can be referenced by other tables.
It has the following keys:

* `name` - a string giving the user name; this key is required
* `password` - a string giving the password

Defining a user does not by itself enable authentication for any service.

## `proxy.tcp` and `proxy.socket` table arrays

The `proxy.tcp` table array provides network access to the GPS receiver over TCP/IP.
This can be used with, for example, [u-center](https://www.u-blox.com/en/product/u-center)
or [Lady Heather](http://www.ke5fx.com/heather/readme.htm) (with the `/ip` option) to monitor your GPS.
**SatPulse does not perform any access control:
using `proxy.tcp` without `readOnly=true` allows anyone on the local network to write to your GPS receiver.**

It has the following key:

* `listen` - a string specifying an address that the TCP server should listen on; the address is the form *host*`:`*port*,
where the *host* is a host name or IP address, and port is the port number; *host* can be omitted, which means to listen on all
IP addresses; this key is required in each `[[proxy.tcp]]` table array element

The `proxy.socket` table array provides access to the GPS receiver over a Unix domain socket.


It has the following key:

* `path` - a string specifying the path of the socket; this key is required in each `[[proxy.socket]]` table array element
* `group` - the group id for the socket path; by default this is `dialout` (this is the usual group for the tty devices,
   so a user that can access tty devices will be able to access the socket); when the socket is not readOnly, then the mode
   will be 0660, so only root and members of this group will be able to write to the socket

In addition, `proxy.tcp` and `proxy.socket` can both have the following keys:

* `protocol` - a string saying that only packets with this protocol are to be forwarded; recognized values are `"RTCM"`, `"NMEA"`, `"UBX"`, `"CASBIN"`, `"ASBIN"`, `"SDBP"`, `"UNCB"`, `"UNCA"`, `"NOVB"` and `"NOVA"`
* `readOnly` - a boolean saying whether access to the GPS receiver should be read-only; this means that serial packets will be forwarded from the GPS receiver to the network, but the network will not be able to send packets to the GPS receiver; this defaults to true if `protocol` is specified and false otherwise
* `writeLockTimeout` - a number giving the time in seconds that a writer to the GPS receiver should have exclusive write access; if client writes to the GPS receiver (which is allowed only when readOnly is false), then no other client will be able to write to the GPS receiver for this period of time; the default is 2 seconds

## `stream.pull` table

The `stream.pull` table configures the network endpoint to be used as the source of correction data for the GPS receiver.
The data is scanned as RTCM and sent to the receiver over the configured serial connection.
Two kinds of endpoint are supported: Ntrip and TCP.

With an Ntrip endpoint, SatPulse acts as an Ntrip client, receiving from an Ntrip caster.
The following keys may be specified:

* `ntrip.address` - a string giving the address of the Ntrip caster, in the form *host* or *host*`:`*port*; when the port is omitted, the default is 2101; this key is required
* `ntrip.mountpoint` - a string giving the caster mountpoint to use; this key is required
* `ntrip.username` - a string giving the user name for Ntrip basic authentication
* `ntrip.password` - a string giving the password for Ntrip basic authentication
* `ntrip.nmeaSend` - a boolean saying whether SatPulse should send the receiver's position to the caster as NMEA GGA; this is needed by Virtual Reference Station casters before they will stream corrections; when true, SatPulse uploads the receiver's current position after connecting and re-uploads it periodically (see `ntrip.nmeaSendInterval`); the default is false
* `ntrip.nmeaSendInterval` - a number giving the interval in seconds between GGA uploads when `ntrip.nmeaSend` is true; a value of 0 means upload only once per connection; the default is 5

Example

```
[stream.pull]
ntrip.address = "caster.example.com"
ntrip.mountpoint = "RTCM"
ntrip.username = "rover"
ntrip.password = "secret"
```

With a TCP endpoint, SatPulse acts as a TCP client, receiving from a TCP server.
The following key must be specified:

* `tcp.address` - a string giving the TCP address to connect to, in the form *host*`:`*port*

Example using TCP

```
[stream.pull]
tcp.address = "192.168.1.42:2006"
```

SatPulse could be configured to act as the TCP server on 192.168.1.42 using:

```
[[proxy.tcp]]
listen = ":2006"
protocol = "RTCM"
```

## `stream.push` table array

The `stream.push` table array allows streams of packets from the GPS receiver to be sent to network endpoints.
Each `[[stream.push]]` table specifies one endpoint.
Two kinds of endpoint are supported: Ntrip and UDP.

The `[[stream.push]]` table can have the following key:

* `protocol` - a string giving the packet protocol to forward; recognized values are `"RTCM"`, `"NMEA"`, `"UBX"`, `"CASBIN"`, `"ASBIN"`, `"SDBP"`, `"UNCB"`, `"UNCA"`, `"NOVB"` and `"NOVA"`; the receiver must output packets using this protocol

With an Ntrip endpoint, SatPulse acts as an Ntrip server, sending to an Ntrip caster.
The following keys may be specified:

* `ntrip.address` - a string giving the address of the remote Ntrip caster, in the form *host* or *host*`:`*port*; when the port is omitted, the default is 2101; this key is required
* `ntrip.mountpoint` - a string giving the mountpoint to push to on the remote caster; this key is required and must be a single URL path component
* `ntrip.password` - a string giving the password for uploading to the remote caster; this key is required
* `ntrip.description` - a string giving the stream description sent to the remote caster; the default is the mountpoint name
* `ntrip.bitrate` - an integer giving the bitrate sent to the remote caster; the default is the `bitrate` key in the top-level `ntrip` table
* `ntrip.msm7to4` - a boolean saying whether MSM7 RTCM packets should be converted to MSM4 before being pushed; non-MSM7 packets are forwarded unchanged; this can be used only when `protocol` is `RTCM`; the default is false

For Ntrip endpoints, `protocol` defaults to `"RTCM"`.
When the protocol is `RTCM`, the source table information sent to the remote caster uses the fields from the `ntrip` table together with the `ntrip.description`, `ntrip.bitrate`, and `ntrip.msm7to4` keys from the push entry.

Example using Ntrip

```
[ntrip]
country = "THA"

[[stream.push]]
ntrip.address = "caster.example.com"
ntrip.mountpoint = "BKK"
ntrip.password = "secret"
ntrip.description = "Bangkok"
```

With a UDP endpoint, SatPulse acts as a UDP client, sending packets to a UDP address.
The following key must be specified:

* `udp.address` - a string giving the UDP destination, in the form *host*`:`*port*

Example using UDP

```
[[stream.push]]
udp.address = "127.0.0.1:10110"
```

## `sync` table

The `sync` table specifies parameters that control the process of synchronizing the PHC with the time from the GPS receiver. The table has subtables `reset`, `converge` and `track`, which configure parameters for reset, converging and tracking synchronization modes. There is also a `share` subtable for parameters shared between modes.
SatPulse starts in reset mode. In this mode, it observes timestamps and GPS messages for a short period and then generates a single sample, which is usually used to step the PHC. It then transitions to converging mode, where the PHC frequency is adjusted by a PI servo to bring the phase of the PHC into alignment with the GPS time. It then transitions to tracking mode, where it adjusts the frequency to maintain alignment. If a problem is encountered in converging or tracking mode, it will transition back to reset mode.

There are over 30 parameters. They are tightly coupled to the implementation and may change between releases.
The defaults have been chosen to work well in most situations.
Changes should be made with caution, as a poor choice of parameters may make SatPulse work unreliably or not at all.
The full set of keys is documented in the configuration file schema.
The keys you are most likely to want to change are:

* `track.kp`, `track.ki` - numbers specifying the proportional and integral constants used for the tracking mode servo; the defaults are 0.8 and 0.3
* `track.ignoreSawtoothCorrection` - a boolean specifying whether to ignore the sawtooth correction provided by the GPS receiver; the default is false; this should be set to true if the PPS signal from the GPS receiver passes through a disciplining process before reaching the PHC; this happens for example when the PHC is connected to a GPSDO or with the Intel E810 time synchronization network adapters.
* `share.detectInvalidLeap` - a boolean saying whether to try to detect invalid backwards leaps in the GPS messages; the default is true; these leaps happen in a very specific combination of circumstances (the GPS firmware was released before the last leap second, SatPulse has not configured the receiver and the receiver does a cold start); if these circumstances do not apply, this can be set to false.
* `reset.expectedDelay` - the expected delay in seconds between the pulse and the corresponding GPS message; when there are multiple messages in the same navigation epoch, the delay is measured from the pulse to the first message in the epoch, rather than to the message which carries the current time; the default is 0.1
* `reset.pulseWindow` - the number of pulses to use in generating the first sample; the default is 5; a larger value more thoroughly checks the timestamps and messages being generated, at the cost of increased start-up time

# EXAMPLES

```
[phc]
interface = "enp1s0"
pin = 0

[serial]
speed = 9600

[gps]
config = true

[[http]]
listen = ":2000"
```

# FILES

`/usr/share/doc/satpulse/config-schema.json`
: JSON schema for the configuration file, installed by the package.
  When installed from source, it will be in `/usr/local/share/doc/satpulse/`.

# SEE ALSO

**satpulsed(8)**
