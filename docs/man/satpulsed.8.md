# NAME

satpulsed - use a GPS receiver as an external clock source for PTP

# SYNOPSIS

**satpulsed** [**\-h**\|**\-\-help**] [**\-V**\|**\-\-version**] [**\-v**\|**\-\-verbose**] [**\-w**\|**\-\-wait**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-s**\|**\-\-systemd\-log**] [**\-f**\|**\-\-config\-file** *path*] [**\-d**\|**\-\-serial\-device** *device-path*]

# DESCRIPTION

**satpulsed** enables a GPS receiver to be used as a source of time for the **ptp4l** PTP daemon, which allows ptp4l to work as a server (grandmaster) synchronized to time distributed by GPS.
The PPS output of the GPS receiver must be connected to a Software Defined Pin (SDP) of an Ethernet controller with a PTP Hardware Clock (PHC) that supports external timestamping. There must also be a serial device connected to the GPS receiver.
**satpulsed** synchronizes the PHC by reading messages from the serial device and timestamps from the external timestamping channel of the PHC.
It also provides timing metadata to ptp4l using the PTP Management Protocol.

In addition, it
- can automatically configure a GPS receiver that uses the UBX protocol
- can work as a refclock for chronyd to enable a combined NTP/PTP time server
- can proxy the serial data to TCP (similarly to ser2net)
- provides a graphical web interface for monitoring the GPS receiver

**satpulsed** requires a configuration file **satpulse.toml(5)**.

The companion program **satpulsetool(1)** supports the use of **satpulsed**; in particular, it can be used to perform certain kinds of GPS configuration that are not done by **satpulsed**, such as configuration of baud rate and constellations/signals to be enabled.

**satpulsed** does not fork itself into the background. It should normally be run using **systemd(1)** as a `Type=simple` service. It needs root permissions so it can access the PHC.

# OPTIONS

**\-h**, **\-\-help**  
: Show help.

**\-V**, **\-\-version**
: Show version information.

**\-v**, **\-\-verbose**
: Log more information.

**\-w**, **\-\-wait**
: Wait for the network interface to be ready. This would typically be used in a systemd service file.

**\-s**, **\-\-systemd\-log**
: Log to standard output in a format suitable for **systemd**.

**\-f**, **\-\-config\-file** *path*
: Configuration file.

**\-d**, **\-\-serial\-device** *device-path*
: Serial device to use.

# ENVIRONMENT

**SATPULSE_CONFIG_FILE**
: Default configuration file path if **\-f** option is not specified.

# FILES

**/etc/satpulse.toml**
: Conventional location of configuration file.

# EXAMPLES

Start the daemon with the normal configuration file:

    satpulsed -f /etc/satpulse.toml

Run in verbose mode with systemd logging:

    satpulsed --systemd-log --wait -f /etc/satpulse.toml

# EXIT STATUS

**0**
: Successful termination or help/version requested.

**1**
: Runtime error.

**64** (EX_USAGE)
: Command line usage error.

**77** (EX_NOPERM)
: Permission denied.

**78** (EX_CONFIG)
: Configuration file error.

Normally, the systemd unit file for **satpulsed** should specify that the daemon should not be automatically restarted with exit codes 64, 77, and 78.

# SEE ALSO

**satpulse.toml(5)**, **satpulsetool(1)**, **satpulsetool-gps(1)**, **systemd(1)**, **ptp4l(8)**, **chronyd(8)**, **ser2net(8)**