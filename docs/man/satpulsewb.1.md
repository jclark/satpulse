# NAME

satpulsewb - serve SatPulse Workbench, a browser GUI for GPS receivers

# SYNOPSIS

**satpulsewb** [**\-h**\|**\-\-help**] [**\-V**\|**\-\-version**] [**\-v**\|**\-\-verbose**] [**\-L**\|**\-\-listen** *host:port*] [**\-t**\|**\-\-token**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-n**\|**\-\-no\-open\-browser**] [**\-\-tui**] [**\-d**\|**\-\-serial\-device** *path*] [**\-s**\|**\-\-device\-speed** *bps*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-vendor** *name*] [**\-\-packet\-log** *path*]

# DESCRIPTION

**satpulsewb** serves SatPulse Workbench, a web application for interactive GPS receiver configuration and monitoring.
It prints one URL per network interface and serves the Workbench while it is running; Ctrl-C stops it.

The Workbench offers device-independent receiver configuration, along with live monitoring (position, time, satellites, signals), a packet inspector, sending configuration message files chosen from the message-file library (see ENVIRONMENT), and correction stream (Ntrip or TCP) forwarding.
With **\-\-tui**, **satpulsewb** instead runs a terminal UI covering the same ground in the invoking terminal, with no web server and no browser.

With no options, **satpulsewb** binds all interfaces on its default port (15754), falling back to an OS-picked port if it is taken, and protects the session with a token generated for this run.
The printed URLs carry the token as a query parameter; the frontend strips it from the URL bar, so the URL to copy is the printed one.
A printed URL grants control of the receiver until **satpulsewb** exits.
Any number of windows can watch the session, but only one at a time holds the write seat and can change the receiver; opening the URL in a second window takes the seat, and the first window becomes a live read-only viewer with a "Use here" button to take it back.

When run from a local desktop session, **satpulsewb** opens its loopback URL in the default browser.
It never opens a browser over SSH or with **\-\-listen**.

On a network you do not trust, listen on loopback only and reach it through an SSH tunnel:

    remote$ satpulsewb -L localhost:15754
    local$ ssh -L 2050:localhost:15754 192.168.1.50

The tunnel's first port is the one to browse to locally, here *http://localhost:2050/*; any free port will do, and ending it with the remote host's last octet keeps concurrent tunnels apart.
The host and port after it are the address the remote host resolves, which is where **satpulsewb** is listening.
Since **\-\-listen** disables the token, the printed URL needs no token to open.
Both steps can be one command, with **\-t** so that Ctrl-C reaches **satpulsewb** and releases the receiver:

    local$ ssh -t -L 2050:localhost:15754 192.168.1.50 satpulsewb -L localhost:15754

The serial-device and device-speed options independently initialize the corresponding controls in the connection bar.
When both are specified, **satpulsewb** connects at startup; otherwise the session starts disconnected and the remaining connection settings are chosen in the GUI.
A browser arriving later shows the current session state.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help.

**\-V**, **\-\-version**
: Show version information.

**\-v**, **\-\-verbose**
: Log more information.

**\-L**, **\-\-listen** *host:port*
: Listen on the given address instead of all interfaces on the default port.
With an explicit port there is no fallback: a bind failure is an error, since the address may be the target of an SSH tunnel.
**\-\-listen** also disables the access token, since the typical use is a tunnel.
Without **\-\-token**, requests with a non-loopback Host are refused and a non-loopback bind prints a warning; **\-\-token** allows remote browser access.

**\-t**, **\-\-token**
: Require the generated access token even with **\-\-listen**.
Without **\-\-listen** this is the default.

**\-n**, **\-\-no\-open\-browser**
: Do not open a browser at startup.

**\-\-tui**
: Run a terminal UI in the invoking terminal instead of serving the web application.
No HTTP server is started and no browser is opened.
Cannot be combined with **\-\-listen** or **\-\-token**.

**\-d**, **\-\-serial\-device** *path*
: Prefill for the device control in the connection bar.
When both **\-d** and **\-s** are given, **satpulsewb** connects at startup with these values.

**\-s**, **\-\-device\-speed** *bps*
: Prefill for the speed control in the connection bar.
When both **\-d** and **\-s** are given, **satpulsewb** connects at startup with these values.

**\-\-vendor** *name*
: Restrict probing and packet format detection to a receiver vendor.
This applies to every connection made in the session, whether at startup or from the GUI.
The value is case-insensitive.
Typical values are **u\-blox**, **Unicore**, **NovAtel**, **Bynav**, **SinoGNSS**, **Allystar**, **Techtotop**, and **Zhongke** (or **CASIC**).
If this option is omitted, the **SATPULSE_VENDORS** environment variable applies (see ENVIRONMENT), and if that too is unset, the vendor is autodetected.

**\-\-packet\-log** *path*
: Log packets exchanged with the receiver to *path* in JSONL format.

# ENVIRONMENT

**SATPULSE_GPSMSG_PATH**
: Colon-separated list of directories to search for message files ahead of the built-in library.
A message file is identified as *vendor*/*file*.toml under a search directory; the first match along the path wins.
Include entries in a message file resolve relative to the file itself, not along the search path.
When **SATPULSE_GPSMSG_PATH** is unset, only the built-in library is used.

**SATPULSE_VENDORS**
: The possible vendors of the connected GPS receiver, as a comma-separated list of vendor names (as accepted by **\-\-vendor**), or `all` for any vendor. It can be overridden by **\-\-vendor**.

# EXAMPLES

Serve on all interfaces with a generated token, connecting from the GUI:

    satpulsewb

Connect to a receiver at startup:

    satpulsewb -d /dev/ttyACM0 -s 38400

Loopback only, for use through an SSH tunnel:

    satpulsewb -L localhost:15754

Start it over SSH and tunnel to it in a single command, then browse to *http://localhost:2050/*:

    ssh -t -L 2050:localhost:15754 192.168.1.50 satpulsewb -L localhost:15754

# SEE ALSO

**satpulsetool-gps(1)**, **satpulsed(8)**
