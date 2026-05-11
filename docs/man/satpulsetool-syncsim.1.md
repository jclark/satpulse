# NAME

satpulsetool-syncsim - simulate synchronizing a PHC with a GPS receiver

# SYNOPSIS

**satpulsetool** [*global options*] **syncsim** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-C**\|**\-\-show\-default\-config**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-stats** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-clock\-log** *path*] [**\-\-ts\-log** *path*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-g**\|**\-\-generate**]\
&nbsp;&nbsp;&nbsp;&nbsp;*config.toml*

# DESCRIPTION

The **satpulsetool** **syncsim** command runs a simulation of a PTP hardware clock (PHC) being synchronized to a GPS receiver.
It generates the events that the synchronization process would receive in real operation, and feeds them to the same code that **satpulsed(8)** uses to synchronize the PHC.
It is useful for tuning the synchronization parameters of **satpulsed** for a particular hardware combination, and for evaluating its behaviour under faults such as signal outages and phase excursions.

The simulation is configured by a file in TOML format.
The file can contain the following tables:

`sim`
: Overall simulation parameters such as duration.

`phc`
: Noise characteristics of the PHC oscillator.

`gps`
: Noise characteristics of the PPS signal from the GPS receiver.

`pulse`
: Timing of the delivery of the PPS signal from the GPS receiver to the PHC.

`msg`
: Timing of the delivery of time messages from the GPS receiver.

`sync`
: Parameters of the synchronization process; the same as the `sync` table in **satpulse.toml(5)**.

`fault`
: Faults to be injected during the simulation, such as signal outages and phase excursions.


The full set of keys in each table can be seen by using the **\-\-show\-default\-config** option,
or by looking at the JSON schema in the same `configs/syncsim/` directory,
which also has some example configurations.

The simulator advances simulated time as fast as it can be computed. A simulation covering a day can be run in a matter of seconds.

The same log messages that **satpulsed(8)** would write during a normal run are written to standard error during the simulation, with timestamps corresponding the simulated time.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **syncsim** command.

**\-C**, **\-\-show\-default\-config**
: Print the default configuration as TOML to standard output and exit.
The output includes comments derived from the configuration schema.

**\-\-stats** *seconds*
: Log a summary of tracking statistics every *seconds* of simulated time.
This is the equivalent of the `log.interval` key in **satpulse.toml(5)**.
The default is 0, which disables periodic logging.
Stats are logged at INFO level, so the global **\-v** option is also required for them to appear.

**\-\-clock\-log** *path*
: Write a log of per-second clock offsets to *path*.
The format is the same as the *clock log* written by **satpulsed(8)**.

**\-\-ts\-log** *path*
: Write a log of PHC timestamps to *path* in JSON Lines format.

**\-g**, **\-\-generate**
: Generate timestamps to standard output in JSON Lines format and exit.
This is equivalent to **\-\-ts\-log** but writes to standard output and skips simulation of the controller.
Cannot be combined with **\-\-stats**, **\-\-clock\-log** or **\-\-ts\-log**.

# EXAMPLES

Show the default configuration with all keys and comments:

    satpulsetool syncsim --show-default-config

Run a simulation, with periodic stats and the final summary on standard output:

    satpulsetool -v syncsim --stats 600 timehat-f9t.syncsim.toml

Run a simulation and capture all four output streams:

    satpulsetool -v syncsim --stats 600 \
      --clock-log timehat-f9t.clock.log \
      --ts-log timehat-f9t.ts.jsonl \
      timehat-f9t.syncsim.toml \
      >timehat-f9t.result.toml 2>timehat-f9t.syncsim.log

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**, **satpulse.toml(5)**
