# NAME

satpulsetool-scan - convert a packet byte stream to a JSONL packet log

# SYNOPSIS

**satpulsetool** [*global options*] **scan** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-vendor** *name*] *file*\|**\-**

# DESCRIPTION

The **satpulsetool** **scan** command reads a GPS packet byte stream and writes a JSONL packet log to standard output.
It is the inverse of **satpulsetool** **pack**.

The input is split into packets using the same scanner used when reading from GPS receivers.
Recognized packets include timestamp, `tag`, `msg`, and either `bin` or `ascii` packet data.
Unrecognized bytes are also written as packet log entries, but without `tag` or `msg`.

Use **\-** as *file* to read from standard input.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **scan** command.

**\-\-vendor** *name*
: Restrict packet formats to those used by a receiver vendor.
The value is case-insensitive.
Typical values are **u\-blox**, **Unicore**, **NovAtel**, **Bynav**, **SinoGNSS**, **Allystar**, **Techtotop**, and **Zhongke**.
If this option is omitted, the **SATPULSE_VENDORS** environment variable supplies the default (see ENVIRONMENT), and if that too is unset, all supported packet formats are recognized.

# ENVIRONMENT

**SATPULSE_VENDORS**
: Declares the receiver vendors that may be attached to this machine, as a comma-separated list of vendor names (as accepted by **\-\-vendor**), or `all` for any vendor. It provides the default for **\-\-vendor**, which overrides it. Unset means no restriction.

# EXAMPLES

Convert a raw capture to a packet log:

    satpulsetool scan capture.bin > packets.jsonl

Read a raw capture from standard input:

    satpulsetool scan - < capture.bin > packets.jsonl

Scan with packet formats restricted for a u-blox receiver:

    satpulsetool scan --vendor u-blox capture.bin > packets.jsonl

Decode packet payloads after scanning:

    satpulsetool scan capture.bin | satpulsetool annotate - > decoded.jsonl

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-pack(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
