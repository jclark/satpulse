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

**pmc**
: PTP management client

# OPTIONS

These options must be specified before the *command*.

**-h**, **--help**  
: Show usage help.

**-V**, **--version**
: Show the version and exit.

**-v**
: Be verbose. Multiple **-v** options will increase verbosity.

# EXAMPLES

Show the satpulsetool version:

    satpulsetool --version

# SEE ALSO

**satpulsed(8)**, **satpulsetool-gps(1)**
