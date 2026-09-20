# NAME

satpulsetool-pollsim - simulate the serial PPS polling loop

# SYNOPSIS

**satpulsetool** [*global options*] **pollsim** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-C**\|**\-\-show\-default\-config**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-edge\-log** *path*]\
&nbsp;&nbsp;&nbsp;&nbsp;*config.toml*

# DESCRIPTION

The **satpulsetool** **pollsim** command runs the polling loop that **satpulsed(8)** uses to detect PPS edges on a serial modem-control input (the `poll` method of the `[serial]` table's `pps` settings in **satpulse.toml(5)**) against a simulated pulse and host, in virtual time.
The loop itself is the same code; the simulator supplies its clock, its timer and its view of the pin.
It is useful for judging how the loop behaves when the host stalls the polling thread, when its timers overshoot, when its state queries slow down while the machine idles, and when the pulse disappears, without waiting for those things to happen on hardware.

The simulation is configured by a file in TOML format.
The file can contain the following tables:

`sim`
: Duration and random seed.

`pulse`
: The pulse as seen on the pin: its width and the jitter of its leading edge.

`host`
: How long a state query takes and how it varies, how timer sleeps are truncated and overshoot, and how long reading the clock takes.

`poll`
: The loop's `pollPreWarm` setting, its minimum spacing between queries, and the uncertainty limit beyond which **satpulsed** does not forward an edge to the NTP server.

`fault`
: Periods with no pulse, single stalls of the polling thread at given times, and bursts of random stalls.

The full set of keys in each table can be seen by using the **\-\-show\-default\-config** option.
The `configs/pollsim/` directory has example configurations.

A stall of the polling thread lands wherever the loop is at the time: inside a state query, between two queries, or asleep, in which case only the part of the stall after the scheduled wakeup delays it.

At the end of the run the command prints statistics as TOML key/value lines: the number of pulses and of candidate edges, how many edges were forwarded, rejected, or too coarse to forward, how many pulses were missed, how many forwarded edges were in error by more than the uncertainty limit, the median, 90th percentile and maximum error of forwarded edges against the true edge on the pin, the longest gap between forwarded edges and the number of gaps over 4 s, the numbers of acquisitions, losses, tracking misses and rejections, and the number of state queries and the fraction of a core they cost.

The same log messages that **satpulsed(8)** would write are written to standard error during the simulation, with timestamps corresponding to the simulated time; the track status lines appear with the global **\-v** option.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **pollsim** command.

**\-C**, **\-\-show\-default\-config**
: Print the default configuration as TOML to standard output and exit.
The output includes a comment on every key.

**\-\-edge\-log** *path*
: Write every candidate edge to *path* in JSON Lines format, with its simulated time `t`, its error `err` against the true edge, its `uncertainty`, whether it was `rejected`, and whether it was `forwarded`.

# EXAMPLES

Run the scenario of a burst of host stalls on a Mac with a USB serial adapter:

    satpulsetool -v pollsim configs/pollsim/outage-20260920.pollsim.toml

# SEE ALSO

**satpulsetool(1)**, **satpulsed(8)**, **satpulse.toml(5)**
