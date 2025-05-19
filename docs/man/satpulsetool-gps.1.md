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

**--socket** *path*  
: Path to a Unix-domain socket to connect to the GPS receiver instead of a serial device.

**-s**, **--device-speed** *bps*  
: Local serial port speed in bits per second (must be non-zero).

**--speed** *bps*  
: Set receiver's serial speed in bits per second (must be non-zero).

**--log-packets** *path*  
: Log raw GPS packets to the specified path.

**--flash**  
: Save the configuration changes to receiver's flash memory.

**--reset**  
: Reset the GPS receiver.

**--nmea**  
: Enable NMEA output from the GPS receiver.

**--force**  
: Force writing configuration to serial device even if no GPS is detected.

**-p**, **--pps**  
: Configure the GPS receiver to enable PPS (pulse-per-second) output.

**-g**, **--gnss** *list*  
: Enable a list of GNSS constellations (comma-separated). Valid values are: `GPS`, `GAL`, `BDS`, `GLO`, `QZSS`, `NAVIC`, `SBAS`.  
  The first GNSS listed must be a major constellation.

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