# NAME

satpulsetool-gps - configure a GPS receiver

# SYNOPSIS

**satpulsetool** [*global options*] **gps** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-d**\|**\-\-serial\-device** *path*] [**\-s**\|**\-\-device\-speed** *bps*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-f**\|**\-\-config\-file** *path*] [**\-\-socket** *path*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-show\-receiver**] [**\-c**\|**\-\-show\-config**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-g**\|**\-\-gnss** **GPS**\|**GAL**\|**BDS**\|**GLO**\|**QZSS**\|**NAVIC**\|**SBAS**,...]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-b**\|**\-\-band** **L1**\|**L2**\|**L5**\|**E5**\|**L6**,...] [**\-\-min\-elev** *degrees*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-p**\|**\-\-pps** *width*] [**\-\-ant\-cable\-delay** *nanos*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-time\-gnss** **GPS**\|**GAL**\|**BDS**\|**GLO**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-survey**] [**\-\-survey\-time** *seconds*] [**\-\-survey\-acc** *meters*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-fixed\-pos\-ecef** *X,Y,Z*] [**\-\-fixed\-pos\-acc** *meters*] [**\-\-mobile**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-nmea**] [**\-\-binary**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-pvt\-out** **pos**\|**vel**\|**time**\|**tp**\|**leap**\|**survey**\|**qual**\|**epoch**\|**tai**\|**ecef**\|**after**\|**daemon**\|**off**,...]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-sats\-out** **sat**\|**sig**\|**none**,...]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-raw\-out** **obs**\|**nav**\|**none**,...]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-rtcm\-out** **MSM4**\|**MSM7**\|**ARP**\|**auto**\|**none**,...] [**\-\-rtcm\-base\-id** *id*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-nmea\-out** **RMC**\|**GGA**\|**GSA**\|**GSV**\|**ZDA**\|**VTG**\|**GLL**\|**none**,...]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-speed** *bps*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-save**] [**\-\-save\-all**] [**\-\-reset**] [**\-\-reload**] [**\-\-factory\-reset**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-vendor** *name*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-m**\|**\-\-msg\-file** *path*] [**\-t**\|**\-\-tag** *list*] [**\-\-show\-tags**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*] [**\-\-capture** *seconds*]

# DESCRIPTION

The **satpulsetool** **gps** command is used to configure a GPS receiver for use with satpulsed.
It can also be used to capture packets from the receiver.
Two kinds of configuration are supported, high-level and low-level;
they cannot be performed simultaneously.

With high-level configuration, configuration changes are requested with device-independent semantics;
**satpulsetool** determines the best way to implement the request for the particular GPS receiver.
High-level configuration is currently supported on two families of GPS receiver:
u-blox receivers (from the u-blox 6 platform through to the X20 platform);
Unicore Nebulas IV receivers (UM980, UM981, UM982, UM960).

With low-level configuration, a *message file* is used.
A message file is a file in TOML format that defines a collection of named messages.
SatPulse provides a library of message files, which can be used to configure a wide variety of different GPS receivers.

If no options other than connection options and `--packet-log` are specified,
then it will detect the receiver and show information about it,
as if the `--show-receiver` option had been specified.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **gps** command.

## Connection options

The following options specify how to connect to the GPS receiver.

**\-d**, **\-\-serial\-device** *path*
: Path to the serial device to communicate with the GPS receiver.

**\-s**, **\-\-device\-speed** *bps*
: Set the speed of the host serial port (as specified by **\-d**) in bits per second.

**\-f**, **\-\-config\-file** *path*
: Read the serial device path and speed from a TOML configuration file.
The file format is compatible with **satpulse.toml(5)**, but only the `device` and `speed` keys in the `[serial]` table are used; all other keys are ignored.
This option cannot be combined with **\-d** or **\-s**.

**\-\-socket** *path*
: Path to a Unix-domain socket to connect to the GPS receiver instead of a serial device.
This is for use with the `proxy.sock` table array in the TOML config file for **satpulsed**.

## High-level configuration

The following options query the receiver.

**\-\-show\-receiver**
: Detect the GPS receiver and show information about it.

**\-c**, **\-\-show\-config**
: Show the current configuration of the GPS receiver.

The following options control which satellites and signals the receiver uses.

**\-g**, **\-\-gnss** *list*
: List of GNSS constellations that should be enabled. The *list* parameter is a comma-separated list of:

  **GPS**
  : Global Positioning System (United States)

  **GAL** or **Galileo**
  : Galileo (European Union)

  **BDS** or **BeiDou**
  : BeiDou Navigation Satellite System (China)

  **GLO** or **GLONASS**
  : Global Navigation Satellite System (Russia)

  **QZSS**
  : Quasi-Zenith Satellite System (Japan)

  **NAVIC**
  : Navigation with Indian Constellation (India)

  **SBAS**
  : Satellite-Based Augmentation Systems (various countries)

**\-b**, **\-\-band** *list*
: List of frequency bands that should be enabled for the specified GNSS constellations. The *list* parameter is a comma-separated list of:

  **L1**
  : 1559-1610 MHz band, including L1 C/A, L1C, E1, B1C signals at 1575.42 MHz, GLONASS L1, and BeiDou B1I

  **L2**
  : 1215-1252 MHz band

  **L5**
  : 1176.45 MHz signals including GPS L5, Galileo E5a, BeiDou B2a

  **E5b**
  : 1202.025-1207.14 MHz band including Galileo E5b, BeiDou B2I at 1207.14 and and GLONASS L3 at 1202.025

  **E5**
  : 1164-1210 MHz band, includes L5 and E5b

  **E6** or **L6**
  : 1260-1300 MHz band

**\-\-min\-elev** *degrees*
: Set the minimum elevation angle in degrees for satellites to be used. Satellites below this angle above the horizon are ignored.

The following options configure the time pulse (PPS) signal.

**\-p**, **\-\-pps** *width*
: Configure the GPS receiver to enable a pulse-per-second (PPS) signal with the specified pulse width in seconds. The *width* must be >= 0 and < 1.0. A width of 0 disables the PPS signal.

**\-\-ant\-cable\-delay** *nanos*
: Configure the antenna cable delay in nanoseconds. This setting compensates for the signal delay introduced by the antenna cable between the antenna and the GPS receiver.

**\-\-time\-gnss** *constellation*
: GNSS constellation used for timing purposes. The PPS signal is aligned to the system time of this GNSS. (The system times of different GNSSs can differ by tens of nanoseconds.) This uses the same constellation names as with **\-\-gnss** option, but only the major, global constellations are allowed (GPS, Galileo, BeiDou and GLONASS).

The following options control the time mode of the receiver. In time mode, the receiver assumes a fixed antenna position, which allows it to produce more accurate timing.

**\-\-survey**
: Perform a survey to determine the position of the antenna, and then run in a mode that assumes the position of the antenna does not change. The survey makes measurements for a period of time and then computes the position based on those measurements.

**\-\-survey\-time** *seconds*
: Set duration of the position survey (default: 2000).

**\-\-survey\-acc** *meters*
: Required survey accuracy in meters (default: 20.0). Minimum value is 0.001 (1 mm).

**\-\-fixed\-pos\-ecef** *X,Y,Z*
: Use the specified coordinates as the fixed position of the antenna, and then run in a mode that assumes the position of the antenna does not change. The coordinates are comma-separated X, Y, and Z coordinates in meters in the Earth-Centered, Earth-Fixed coordinate system.

**\-\-fixed\-pos\-acc** *meters*
: Set the accuracy of the fixed position in meters (default: 20.0). This value should reflect the actual uncertainty in the fixed position coordinates. Minimum value is 0.001 (1 mm).

**\-\-mobile**
: Run in a normal mode, where the position of the antenna may change. This undoes the effect of **\-\-survey** or **\-\-fixed-pos-ecef**.

The following options control which messages the receiver outputs.

**\-\-nmea**
: Enable NMEA output from the GPS receiver.

**\-\-binary**
: Enable binary messages from the GPS receiver instead of NMEA.

**\-\-pvt\-out** *flags*
: Configure which Position, Velocity, and Time (PVT) messages to output. The *flags* parameter is a comma-separated list of:

  **pos**
  : Enable position messages (latitude, longitude, altitude by default)

  **vel**
  : Enable velocity messages (North, East, Down components by default)

  **time**
  : Enable time of navigation solution messages

  **tp**
  : Enable time pulse messages (time of the PPS pulse)

  **leap**
  : Enable leap second information messages

  **survey**
  : Enable survey-in progress messages

  **qual**
  : Enable solution quality messages

  **epoch**
  : Enable messages to mark the end of a navigation epoch

  **tai**
  : Prefer time in TAI (or constant offset from TAI) rather than UTC

  **ecef**
  : Prefer position and velocity in ECEF coordinates rather than latitude and longitude

  **after**
  : If the time pulse message enabled by **tp** is emitted before the time pulse, then emit a time message also

  **ptp**
  : Enable the messages needed by satpulsed when it is being used as a source of time for PTP (equivalent to `tp,after,tai,leap,survey,qual,epoch,off`)

  **ntp**
  : Enable messages needed by satpulsed when it is being used as a source of time for NTP, without a PHC (equivalent to `time,leap,survey,qual,epoch,off`)

  **off**
  : Turn off PVT messages that are not explicitly enabled

**\-\-sats\-out** *flags*
: Configure messages providing information about satellites. The *flags* parameter is a comma-separated list of:

  **sat**
  : Enable output of information about each individual space vehicle (SV), including azimuth and elevation; this does not include information about each of the signals from an SV.

  **sig**
  : Enable output of information about each signal received from each SV.

  **none**
  : Disable all messages providing information about satellites.

**\-\-raw\-out** *flags*
: Configure raw data message output. The *flags* parameter is a comma-separated list of:

  **obs**
  : Enable raw observation/measurement messages (for RINEX observation files)

  **nav**
  : Enable raw navigation data messages (e.g., GPS subframe data)

  **none**
  : Disable all raw data messages

**\-\-rtcm\-out** *flags*
: Configure RTCM 3.x message output. The *flags* parameter is a comma-separated list of:

  **MSM4**
  : Enable MSM4 messages for all enabled GNSS constellations

  **MSM7**
  : Enable MSM7 messages for all enabled GNSS constellations

  **ARP**
  : Enable Antenna Reference Point messages (RTCM message type 1005)

  **auto**
  : Enable RTCM support intelligently. Enables ARP and either MSM4 or MSM7 messages. It will prefer MSM4 over MSM7 if both are available. Add the MSM7 flag to prefer MSM7.

  **none**
  : Disable all RTCM messages

**\-\-rtcm\-base\-id** *id*
: Set the RTCM reference station ID. The *id* must be an integer between 0 and 4095.

**\-\-nmea\-out** *flags*
: Configure NMEA message output. The *flags* parameter is a comma-separated list of:

  **RMC**
  : Enable RMC (Recommended Minimum) messages

  **GGA**
  : Enable GGA (Global Positioning System Fix Data) messages

  **GSA**
  : Enable GSA (DOP and Active Satellites) messages

  **GSV**
  : Enable GSV (Satellites in View) messages

  **ZDA**
  : Enable ZDA (Time and Date) messages

  **VTG**
  : Enable VTG (Vector Track Made Good) messages

  **GLL**
  : Enable GLL (Geographic Position - Latitude/Longitude) messages

  **none**
  : Disable all NMEA messages

The following option configures the receiver's serial port speed.

**\-\-speed** *bps*
: Configure the GPS receiver's serial speed in bits per second.

The following options control use of the receiver's non-volatile memory.

**\-\-save**
: Save the configuration changed by this command to GPS receiver's non-volatile memory. Exactly what is saved depends on the specific GPS receiver; satpulsetool will save the minimum possible to ensure that everything that was changed by this command is saved.

**\-\-save\-all**
: Save the current running configuration of the GPS receiver to its non-volatile memory.

**\-\-reload**
: Reloads the configuration of the GPS receiver from its non-volatile memory. Any configuration settings that have not been saved will be lost. This can be used to undo any changes made by the satpulse daemon.

**\-\-reset**
: Perform a reset that reloads the configuration of the GPS receiver from its non-volatile memory (as with the **\-reload** option), and discards information about the last known position, current time, and satellite orbital data (both ephemeris and almanac).

**\-\-factory\-reset**
: Restore the non-volatile memory of the GPS receiver to its default settings, and the perform a reset as with the **\-\-reset** option.

The following option restricts which configuration protocols are probed. It also restricts which packet formats are recognized.

**\-\-vendor** *name*
: GPS receiver vendor name.
  The following values are supported: `u-blox`, `Unicore`, `Allystar`, `Bynav`, `NovAtel`, `Quectel`, `SinoGNSS`, `Techtotop`, `Zhongke`, `other`.
  In addition, the following values are allowed and currently treated as equivalent to `other`: `Furuno`, `MediaTek`, `Septentrio`, `SkyTraq`, `Trimble`.
  Values are case-insensitive.
  If not specified, no restrictions are applied.

## Low-level configuration

In a TOML message file, messages are identified with tags.
A single tag may identify a single message or a sequence of messages.
A message file can also include messages with an empty or missing tag.

While sending messages, **satpulsetool** will attempt to determine which messages output by the receiver
are in response to the messages that were sent and will display those messages.
It will continue to listen for those messages for a period of 2 seconds after the last message was sent;
this duration can be changed with the **\-\-capture** option.

**\-m**, **\-\-msg\-file** *path*
: Specifies the TOML message file to use. This option is required to perform low-level configuration.
A *path* of **-** can be used to read from stdin.
If neither **\-t** nor **\-\-show\-tags** is specified, then messages with no tag are sent.

**\-t**, **\-\-tag** *list*
: Comma-separated list of tags selecting which messages to send from the message file.
Messages are sent in the order the tags are listed.

**\-\-show\-tags**
: Print all tags in the message file with their descriptions to stdout, validate tag constraints, and exit.

## Packet capture

The following options control packet capture.

**\-\-packet\-log** *path*
: Log to *path* a description of the packets sent to and received from the GPS receiver. The log is in `.jsonl` (JSON lines) format.

**\-\-capture** *seconds*
: Continue capturing packets for the specified number of seconds after configuration is complete.
Use `0` to capture indefinitely until interrupted.
With low-level configuration, this also controls how long to wait for responses after the last
message was sent.
Requires **\-\-packet\-log** or **\-\-msg\-file**.

# EXAMPLES

Show receiver information using device and speed from the satpulse configuration file:

    satpulsetool gps -f /etc/satpulse.toml

Enable GPS and GALILEO on `/dev/ttyACM0` at 9600 baud:

    satpulsetool gps -d /dev/ttyACM0 -s 9600 -g GPS,GAL

Start a survey for 3000 seconds with 1.5m accuracy:

    satpulsetool gps --survey --survey-time 3000 --survey-acc 1.5 -d /dev/ttyUSB0 -s 38400

Connect over a socket and reset the receiver:

    satpulsetool gps --socket /var/run/satpulse.sock --reset

Enable RTCM MSM4 output for base station use:

    satpulsetool gps -d /dev/ttyUSB0 -s 9600 -rtcm-out MSM4,ARP --gnss GPS,GAL,BDS

Enable only NMEA RMC messages:

    satpulsetool gps -d /dev/ttyACM0 -s 19200 -nmea --nmea-out RMC

Passively capture packets for 30 seconds without probing:

    satpulsetool gps -d /dev/ttyUSB0 -s 9600 --packet-log capture.jsonl --capture 30

Probe the receiver, then capture packets for 10 seconds:

    satpulsetool gps -d /dev/ttyACM0 -s 9600 --show-receiver --packet-log capture.jsonl --capture 10

Show the current GPS receiver configuration:

    satpulsetool gps -f serial.toml --show-config

where `serial.toml` contains:

    [serial]
    device = "/dev/ttyUSB0"
    speed = 9600

Send the messages tagged `ppp-has` from `um980.toml` (which enable Galileo HAS on a UM980):

    satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m um980.toml -t ppp-has

Send an ad-hoc command from stdin using a here document:

    satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
    [[line]]
    text = "CONFIG PPP ENABLE E6-HAS"
    TOML

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**
