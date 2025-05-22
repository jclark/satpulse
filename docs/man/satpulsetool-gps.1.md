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

**--reset**  
: Perform a reset that discards configuration that has not been saved to non-volatile memory of the GPS receiver, and discards information about the last known position, current time, and satellite orbital data (both ephemeris and almanac).

**--factory-reset**
: Restore the non-volatile memory of the GPS receiver to its default settings, and the perform a reset as with the **--reset** option.

**--nmea**  
: Enable NMEA output from the GPS receiver.

**--force-probe**  
: Force writing probe to serial device even when there is no output from the GPS receiver

**-p**, **--pps**  
: Configure the GPS receiver to enable PPS (pulse-per-second) output.


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

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**