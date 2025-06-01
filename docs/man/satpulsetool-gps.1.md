# NAME

satpulsetool-gps - configure a GPS receiver

# SYNOPSIS

**satpulsetool** [*global options*] **gps** [*options*]

# DESCRIPTION

The **satpulsetool** **gps** command is used to configure a GPS receiver for use with satpulsed.

# OPTIONS

**-h**, **--help**  
: Show usage help for the `gps` command.

**-d**, **--serial-device** *path*  
: Path to the serial device to communicate with the GPS receiver.

**-s**, **--device-speed** *bps*  
: Set the speed of the host serial port (as specified by **-d**) in bits per second.

**--socket** *path*  
: Path to a Unix-domain socket to connect to the GPS receiver instead of a serial device.

**--speed** *bps*  
: Configure the GPS receiver's serial speed in bits per second.

**--packet-log** *path*  
: Log to *path* a description of the packets sent to and received from the GPS receiver. The log is in `.jsonl` (JSON lines) format.

**-g**, **--gnss** *list*  
: List of GNSS constellations that should be enabled. The list is comma-separated and valid constellation names are: `GPS`, `GAL` or `Galileo`, `BDS` or `BeiDou`, `GLO` or `GLONASS`, `QZSS`, `NAVIC`, `SBAS`.  
  
**-b**, **--band** *list*
: List of frequency bands that should be enabled for the specified GNSS constellations. The list is command-separated and valid band names are: `L1`, `L2`, `L5`, `E5`, `E6` or `L6`. `L1` covers 1559-1610 MHz, including the L1 C/A, L1C, E1 and B1C signals at 1575.42MHz, as well as the GLONASS L1 and BeiDou B1I signals. `L2` covers 1215-1252 MHz. `L5` covers only signals at 1176.45 MHz such as GPS L5, Galileo E5a, BeiDou B2a. `E5` covers both `L5` and other signals in the range 1164-1210 Mhz. `E6` and `L6` mean the same and cover 1260-1300 Mhz.

**--save**  
: Save the configuration changed by this command to GPS receiver's non-volatile memory. Exactly what is saved depends on the specific GPS receiver; satpulsetool will save the minimum possible to ensure that everything that was changed by this command is saved.

**--save-all**  
: Save the current running configuration of the GPS receiver to its non-volatile memory.

**--reload**
: Reloads the configuration of the GPS receiver from its non-volatile memory. Any configuration settings that have not been saved will be lost. This can be used to undo any changes made by the satpulse daemon.

**--reset**  
: Perform a reset that reloads the configuration of the GPS receiver from its non-volatile memory (as with the **-reload** option), and discards information about the last known position, current time, and satellite orbital data (both ephemeris and almanac).

**--factory-reset**
: Restore the non-volatile memory of the GPS receiver to its default settings, and the perform a reset as with the **--reset** option.

**--nmea**  
: Enable NMEA output from the GPS receiver.

**--binary**
: Enable binary messages from the GPS receiver instead of NMEA.

**--force-probe**  
: Force writing probe to serial device even when there is no output from the GPS receiver

**-p**, **--pps**  
: Configure the GPS receiver to enable PPS (pulse-per-second) output.

**--pvt-out** *flags*  
: Configure which Position, Velocity, and Time (PVT) messages to output. The *flags* parameter is a comma-separated list of:
  
  **pos**
  : Enable position messages (latitude, longitude, altitude by default)
  
  **vel**
  : Enable velocity messages (North, East, Down components by default)
  
  **time**
  : Enable time of navigation solution messages
  
  **tp**
  : Enable time pulse messages (time of the PPS signal)
  
  **leap**
  : Enable leap second information messages
  
  **tai**
  : Request time in TAI (or constant offset from TAI) rather than UTC
  
  **ecef**
  : Request position/velocity in Earth-Centered, Earth-Fixed coordinates
  
  **off**
  : Turn off PVT messages that are not explicitly enabled
  
  **daemon**
  : Enable messages needed by satpulsed (equivalent to `tp,tai,leap,off`)

**--raw-out** *flags*  
: Configure raw data message output. The *flags* parameter is a comma-separated list of:
  
  **obs**
  : Enable raw observation/measurement messages (for RINEX observation files)
  
  **nav**
  : Enable raw navigation data messages (e.g., GPS subframe data)
  
  **none**
  : Disable all raw data messages

**--rtcm-out** *flags*  
: Configure RTCM 3.x message output. The *flags* parameter is a comma-separated list of:
  
  **MSM4**
  : Enable MSM4 messages for all enabled GNSS constellations
  
  **MSM7**
  : Enable MSM7 messages for all enabled GNSS constellations
  
  **ARP**
  : Enable Antenna Reference Point messages (RTCM message type 1005)
  
  **none**
  : Disable all RTCM messages

**--nmea-out** *flags*  
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
  
  **none**
  : Disable all NMEA messages

**--disable-time-mode**  
: Disable time mode (cannot be used with `--survey`).

**--survey**  
: Instruct the GPS receiver to perform a position survey.

**--survey-time** *seconds*  
: Set duration of the position survey (default: 2000).

**--survey-acc** *meters*  
: Required survey accuracy in meters (default: 20.0). Minimum value is 0.001 (1 mm).


# EXAMPLES

Enable GPS and GALILEO on `/dev/ttyACM0` at 9600 baud:

    satpulsetool gps -d /dev/ttyACM0 -s 9600 -g GPS,GAL

Start a survey for 3000 seconds with 1.5m accuracy:

    satpulsetool gps --survey --survey-time 3000 --survey-acc 1.5 --serial-device /dev/ttyUSB0

Connect over a socket and reset the receiver:

    satpulsetool gps --socket /var/run/satpulse.sock --reset

Configure receiver for satpulsed with time pulse and leap second messages:

    satpulsetool gps -d /dev/ttyACM0 --pvt-out daemon

Enable RTCM MSM4 output for base station use:

    satpulsetool gps -d /dev/ttyUSB0 --rtcm-out MSM4,ARP --gnss GPS,GAL,BDS

Enable only NMEA RMC messages:

    satpulsetool gps -d /dev/ttyACM0 --nmea --nmea-out RMC

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**
