# NAME

satpulsewb - serve SatPulse Workbench, a browser GUI for GPS receivers

# SYNOPSIS

**satpulsewb** [**\-h**\|**\-\-help**] [**\-L**\|**\-\-listen** *host:port*] [**\-T**\|**\-\-token**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-d**\|**\-\-serial\-device** *path* [**\-s**\|**\-\-device\-speed** *bps*] [**\-\-vendor** *name*]]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*]

# DESCRIPTION

**satpulsewb** serves SatPulse Workbench, a web application for interactive GPS receiver configuration and monitoring.
It runs an HTTP server with an embedded single-page frontend, prints one URL per network interface, and serves a GUI session until stopped.
It is a commissioning tool run by the user, typically over SSH on the box with the receiver, not a daemon.

The Workbench offers device-independent receiver configuration that requires no knowledge of the receiver's protocol, along with live monitoring (position, time, satellites, signals), a packet inspector, and correction stream (Ntrip or TCP) forwarding.

With no options, **satpulsewb** binds all interfaces on its default port (15754), falling back to an OS-picked port if it is taken, and protects the session with a token generated for this run.
The printed URLs carry the token as a query parameter; the frontend stores it and strips it from the URL bar.
Anyone with a printed URL controls the receiver until **satpulsewb** exits.

There is no TLS support.
On a network you do not trust, listen on loopback only and reach it through an SSH tunnel:

    remote$ satpulsewb -L localhost:15754
    local$ ssh -L 15754:localhost:15754 remotehost

Without **\-\-serial\-device**, the session starts disconnected and the receiver is chosen and connected from the GUI.
With it, **satpulsewb** connects at startup; a browser arriving later catches up on the current state.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help.

**\-L**, **\-\-listen** *host:port*
: Listen on the given address instead of all interfaces on the default port.
With an explicit port, a bind failure is an error; there is no fallback port, since the address may be the target of an SSH tunnel.
**\-\-listen** also disables the access token, since the typical use is a tunnel; serving without a token on a non-loopback address prints a warning.

**\-T**, **\-\-token**
: Require the generated access token even with **\-\-listen**.
Without **\-\-listen** this is the default.

**\-d**, **\-\-serial\-device** *path*
: Serial device connected to a GPS receiver, to connect to at startup.

**\-s**, **\-\-device\-speed** *bps*
: Serial device baud rate.
If this option is omitted, the device's current speed is used.

**\-\-vendor** *name*
: Restrict probing and packet format detection to a receiver vendor for the startup connection.
The value is case-insensitive.
Typical values are **u\-blox**, **Unicore**, **NovAtel**, **Bynav**, **SinoGNSS**, **Allystar**, **Techtotop**, and **Zhongke**.
If this option is omitted, the vendor is autodetected.

**\-\-packet\-log** *path*
: Log packets exchanged with the receiver to *path* in JSONL format.

**\-v**, **\-\-verbose**
: Increase logging verbosity.

**\-V**, **\-\-version**
: Show version information.

# EXAMPLES

Serve on all interfaces with a generated token, connecting from the GUI:

    satpulsewb

Connect to a receiver at startup:

    satpulsewb -d /dev/ttyACM0

Loopback only, for use through an SSH tunnel:

    satpulsewb -L localhost:15754

# SEE ALSO

**satpulsetool-gps(1)**, **satpulsed(8)**
