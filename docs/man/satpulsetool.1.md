# NAME

satpulsetool - command-line tool to support satpulsed

# SYNOPSIS

**satpulsetool** [*global options*] *command* [*command options*]

# DESCRIPTION

**satpulsetool** is a command-line tool that supports **satpulsed**.

# COMMANDS

The *command* must be one of the following.

**gps**
: Configure and control the GPS receiver.

**sdp**
: Manage Software Defined Pins (SDPs) of PTP Hardware Clocks (PHCs)

**syncsim**
: Simulate synchronizing a PHC with a GPS receiver

**convobs**
: Convert GNSS observation data

**decode**
: Decode a GPS packet from hex or ASCII data.

**annotate**
: Add fields to a JSONL packet log showing decoded packets.

**pack**
: Convert a JSONL packet log to a packet byte stream.

**scan**
: Convert a packet byte stream to a JSONL packet log.

**replay**
: Replay a JSONL packet log, generating JSONL events, similar to an event log.

**ntrip**
: Ntrip client.

**pmc**
: PTP management client

# OPTIONS

These options must be specified before the *command*.

**\-h**, **\-\-help**  
: Show usage help.

**\-V**, **\-\-version**
: Show the version and exit.

**\-v**
: Be verbose. Multiple **\-v** options will increase verbosity.

# ENVIRONMENT

**SATPULSE_VENDORS**
: Declares the receiver vendors that may be attached to this machine, as a comma-separated list of vendor names (as accepted by **\-\-vendor**), or `all` for any vendor. It provides the default for the **\-\-vendor** option of the commands that take one (**gps**, **scan**): a single declared vendor behaves as if it were passed to **\-\-vendor**, while several form the candidate set for packet-format recognition and configuration-protocol probing. **\-\-vendor** overrides it. Unset means no declaration.

# EXAMPLES

Show help for satpulsetool:

    satpulsetool --help

Show help for satpulsetool gps command:

    satpulsetool gps --help

# SEE ALSO

**satpulsed(8)**, **satpulsetool-gps(1)**, **satpulsetool-pack(1)**, **satpulsetool-scan(1)**, **satpulsetool-sdp(1)**, **satpulsetool-syncsim(1)**, **satpulsetool-convobs(1)**
