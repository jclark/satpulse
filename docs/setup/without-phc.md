---
title: Using SatPulse without a PTP Hardware Clock (PHC)
---

The core function of SatPulse, which is to act as a source of time to a PTP server, requires a PHC.
However, SatPulse also has extensive GPS-related functionality,
which does not require a PHC.

## satpulsed

There are a number of situations where you may want to use satpulsed without a PHC:
- you do not have PHC hardware,
- you are running on a non-Linux OS, which does not provide PHC support, or
- you want to use the GPS-related functionality of satpulsed without synchronizing the PHC.

You can do this by leaving out the `phc` section from `satpulse.toml`,
or by commenting out or omitting the `interface` key in the `phc` section.

The functionality configured by following sections will work as normal without a PHC:

* `serial`
* `gps`
* `log`
* `http`
* `proxy.tcp`
* `proxy.socket`

The `ntp` section also works without a PHC.
The samples sent to the NTP daemon are based on the timing of the serial messages.
This is imprecise but is useful when the NTP daemon is reading PPS timestamps.

The functionality configured by the following sections will not have any effect when running without a PHC.

* `phc`
* `leapSecond`
* `ptp`

Here is an example of what a suitable `satpulse.toml` file might look like on macOS:

```
# Example config file for macOS
[serial]
# This can be overridden with the -d option on the command-line
device = "/dev/cu.usbserial-DBE128L5"
speed = 38400

[gps]
# Uncommenting the following line will enable configuration of the GPS
# (changes are not persistent across GPS reboots)
config = true
#vendor = "u-blox"

[log]
# Uncomment the following line to enable logging of packets from the GPS receiver (in /var/log/satpulse)
# packet = true

# Uncomment the following two lines to allow HTTP monitoring on port 2000
[[http]]
listen = ":2000"

# Uncomment this to allow a TCP connection to proxy the serial port
[[proxy.tcp]]
listen = ":2006"
#readOnly = true

# Uncomment this to allow a Unix domain socket to proxy the serial port
# This can be used with
#    satpulsetool gps --socket /var/run/satpulse.sock
[[proxy.socket]]
# /var/run needs root
path = "/tmp/satpulse.sock"
```

When running without a PHC, `satpulsed` does not need root privileges.
It does, of course, need access to the GPS serial device,
which typically any member of the `dialout` group has.

## satpulsetool

`satpulsetool gps` works fully without a PHC.

`satpulsetool sdp` does not work at all without a PHC.
