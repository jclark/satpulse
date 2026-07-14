# NAME

satpulsewb - serve SatPulse Workbench, a browser GUI for GPS receivers

# SYNOPSIS

**satpulsewb** [**\-h**\|**\-\-help**] [**\-L**\|**\-\-listen** *host:port*] [**\-T**\|**\-\-token**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-d**\|**\-\-serial\-device** *path* [**\-s**\|**\-\-device\-speed** *bps*]] [**\-\-vendor** *name*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log** *path*]

# DESCRIPTION

**satpulsewb** serves SatPulse Workbench, a web application for interactive GPS receiver configuration and monitoring.
It runs an HTTP server with an embedded single-page frontend, prints one URL per network interface, and serves a GUI session until stopped.
It is a commissioning tool run by the user, typically over SSH on the box with the receiver, not a daemon.

The Workbench offers device-independent receiver configuration that requires no knowledge of the receiver's protocol, along with live monitoring (position, time, satellites, signals), a packet inspector, sending configuration message files chosen from the message-file library (see ENVIRONMENT), and correction stream (Ntrip or TCP) forwarding.

With no options, **satpulsewb** binds all interfaces on its default port (15754), falling back to an OS-picked port if it is taken, and protects the session with a token generated for this run.
The printed URLs carry the token as a query parameter; the frontend stores it and strips it from the URL bar.
Anyone with a printed URL controls the receiver until **satpulsewb** exits.
Any number of windows can watch the session, but only one at a time holds the write seat and can change the receiver; opening the URL in a second window takes the seat, and the first window becomes a live read-only viewer with a "Use here" button to take it back.
On macOS and Windows, when run from a local desktop session, **satpulsewb** opens its loopback URL in the default browser.
It does not open a browser over SSH, with **\-\-listen**, or on Linux, where the launched browser's command line would expose the token to other users of the machine.

There is no TLS support.
On a network you do not trust, listen on loopback only and reach it through an SSH tunnel:

    remote$ satpulsewb -L localhost:15754
    local$ ssh -L 2050:localhost:15754 192.168.1.50

The tunnel's first port is the one to browse to locally, here *http://localhost:2050/*; any free port will do, and ending it with the remote host's last octet keeps concurrent tunnels apart.
The host and port after it are the address the remote host resolves, which is where **satpulsewb** is listening.
Since **\-\-listen** disables the token, the printed URL needs no token to open.
Both steps can be one command, with **\-t** so that Ctrl-C reaches **satpulsewb** and releases the receiver:

    local$ ssh -t -L 2050:localhost:15754 192.168.1.50 satpulsewb -L localhost:15754

Without **\-\-serial\-device**, the session starts disconnected and the receiver is chosen and connected from the GUI.
With it, **satpulsewb** connects at startup; a browser arriving later catches up on the current state.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help.

**\-L**, **\-\-listen** *host:port*
: Listen on the given address instead of all interfaces on the default port.
With an explicit port, a bind failure is an error; there is no fallback port, since the address may be the target of an SSH tunnel.
**\-\-listen** also disables the access token, since the typical use is a tunnel; serving without a token on a non-loopback address prints a warning.
Without **\-\-token**, **\-\-listen** trusts the local browser environment.

**\-T**, **\-\-token**
: Require the generated access token even with **\-\-listen**.
Without **\-\-listen** this is the default.

**\-d**, **\-\-serial\-device** *path*
: Serial device connected to a GPS receiver, to connect to at startup.

**\-s**, **\-\-device\-speed** *bps*
: Serial device baud rate.
If this option is omitted, the device's current speed is used.

**\-\-vendor** *name*
: Restrict probing and packet format detection to a receiver vendor.
This applies to every connection made in the session, whether at startup or from the GUI.
The value is case-insensitive.
Typical values are **u\-blox**, **Unicore**, **NovAtel**, **Bynav**, **SinoGNSS**, **Allystar**, **Techtotop**, and **Zhongke**.
If this option is omitted, the vendor is autodetected.

**\-\-packet\-log** *path*
: Log packets exchanged with the receiver to *path* in JSONL format.

**\-v**, **\-\-verbose**
: Increase logging verbosity.

**\-V**, **\-\-version**
: Show version information.

# ENVIRONMENT

**SATPULSE_GPSMSG_PATH**
: Colon-separated list of directories to search for message files, replacing the default search path.
A message file is identified as *vendor*/*file*.toml under a search directory; the first match along the path wins, so a file in an earlier directory shadows a same-named file in a later one.
Include entries in a message file resolve relative to the file itself, not along the search path, so a shadowing file must have its included files alongside it.

The default search path is the user's own library followed by the installed one.
The user's library is *satpulse/gpsmsg* under the platform's user configuration directory: *~/.config/satpulse/gpsmsg* on Linux (or under **$XDG_CONFIG_HOME** when set), and *~/Library/Application Support/satpulse/gpsmsg* on macOS.
The installed library is */usr/local/share/satpulse/gpsmsg* then */usr/share/satpulse/gpsmsg* on Linux, and *share/satpulse/gpsmsg* under the Homebrew prefix on macOS (*/opt/homebrew* on Apple silicon, */usr/local* on Intel).

# EXAMPLES

Serve on all interfaces with a generated token, connecting from the GUI:

    satpulsewb

Connect to a receiver at startup:

    satpulsewb -d /dev/ttyACM0

Loopback only, for use through an SSH tunnel:

    satpulsewb -L localhost:15754

Start it over SSH and tunnel to it in a single command, then browse to *http://localhost:2050/*:

    ssh -t -L 2050:localhost:15754 192.168.1.50 satpulsewb -L localhost:15754

# SEE ALSO

**satpulsetool-gps(1)**, **satpulsed(8)**
