# NAME

satpulsed - integrated GPS daemon

# SYNOPSIS

**satpulsed** [**\-h**\|**\-\-help**] [**\-V**\|**\-\-version**] [**\-v**\|**\-\-verbose**] [**\-w**\|**\-\-wait**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-s**\|**\-\-systemd\-log**] [**\-f**\|**\-\-config\-file** *path*] [**\-d**\|**\-\-serial\-device** *device-path*]

# DESCRIPTION

**satpulsed** is a daemon that connects to a GPS receiver over a serial port and can

- synchronize a PTP hardware clock and provide timing metadata to **ptp4l(8)**, allowing **ptp4l** to act as a PTP grandmaster;
- provide timing information to an NTP daemon, either from a synchronized PHC or from serial messages, allowing the NTP daemon to act as a stratum 1 NTP server;
- act as a Ntrip caster, serving RTCM corrections from the GPS receiver to Ntrip clients;
- act as an Ntrip server, pushing RTCM corrections from the GPS receiver to an Ntrip caster;
- act as an Ntrip client, pulling RTCM corrections from an Ntrip caster and feeding them to the GPS receiver;
- act as a TCP server, proxying packets to and from the GPS receiver;
- monitor the GPS receiver through logs, Prometheus metrics and a Web UI.

These functions are controlled by a required configuration file in TOML format **satpulse.toml(5)**.
For supported GPS receivers, **satpulsed** can also configure the receiver as needed for the selected functions.

The companion program **satpulsetool(1)** supports the use of **satpulsed**; in particular, it can be used to perform certain kinds of GPS configuration that are not done by **satpulsed**, such as configuration of baud rate and constellations/signals to be enabled.

**satpulsed** does not fork itself into the background. It should normally be run using **systemd(1)** as a `Type=simple` service.

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

**SATPULSE_VENDORS**
: The possible vendors of the connected GPS receiver, as a comma-separated list of vendor names (the same names accepted by the `[gps]` `vendor` key), or `all` for any vendor. It can be overridden by the `[gps]` `vendor` key.

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
