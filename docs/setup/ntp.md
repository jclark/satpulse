---
title: Basic use with NTP
---

This page describes how to use SatPulse to build an NTP server using general-purpose hardware.
This works on both Linux and macOS.
If your machine has a network interface with a PTP Hardware Clock (PHC) that can timestamp a PPS signal,
use the approach in [Precision timing with a PHC]({% link setup/phc.md %}) instead,
which covers NTP service as well as PTP.

This page shows the configuration for two NTP daemons: chrony and ntpd-rs.
It assumes the baseline setup is complete.

There are two different approaches depending on how your PPS signal is wired up:

* the PPS signal is connected via the same serial port that is used for messages;
* the PPS signal is connected to a GPIO pin.

## PPS signal connected via serial port

This approach uses a modem control line to carry a PPS signal. {% include new-in-03.html %}
An RS-232 style serial interface supports modem control signals in addition to TXD and RXD signals.
These signals have a direction.
The ones relevant to PPS are ones that go from the DCE (the GPS receiver in our case) to the DTE (the host computer). These are
DCD, DSR, CTS, RI. On an RS232 DB9 connector, these use pins 1, 6, 8, 9 respectively.
Note that modem control lines are the same pin on the DTE and DCE.

USB-serial adapters vary in which signals they support: some support none at all; some support CTS; some also support DCD and DSR.
In particular, USB-serial adapters that use the CDC-ACM driver (and so appear as /dev/ttyACM*N*) do not support using CTS for a PPS signal (it can only be used for flow control);
CH343 is one example.
However, with a suitable USB-serial adapter this approach can work for a computer that has a USB port but no serial port and no GPIO pins.

With this approach, satpulsed reads both the PPS signal and messages over the same port.
It uses the messages to figure out which second a PPS pulse corresponds to and then generates samples that are sent to the NTP daemon.
These samples have sufficient information for the NTP daemon to synchronize the system clock without any additional input.

You can use `satpulsetool serial` to check that the PPS signal is visible. For example,

```
satpulsetool serial -d /dev/ttyUSB0 -s 38400 -p cts -t 20
```

will print pulses detected on the CTS line for 20 seconds.

satpulsed supports three methods of using the operating system to determine the time of a PPS pulse edge:

* `kernel` means the kernel timestamps the time of a modem control line status change;
* `wait` means the kernel notifies the application of a modem control line status change;
* `poll` means that satpulsed continually asks for the current modem control line status.

Which methods are available depends on the operating system and the driver.
Generally, the `poll` method is always available if any of the three methods are available.
The `kernel` and `wait` methods are not available on macOS.
The `kernel` method is available on Linux only when the pin is DCD.
The `wait` method is not supported by some USB serial drivers on Linux (e.g. CP2102).

The `kernel` method is superior to the `wait` method when it is available.
The `poll` method is usually considered inferior to the `kernel` and `wait` methods,
but satpulsed uses a sophisticated predictive/adaptive algorithm which makes it competitive with the `kernel` method.

For the `poll` method, use a pulse width of at least a few milliseconds.
When satpulsed does configuration, it will use a width of 100ms, which works fine;
this is also the default pulse width for most modern GPS receivers.

You can use the `-m` option with `satpulsetool serial` to specify which method to use.
You can use `-j` to get the output in JSONL format, which includes more information when polling is used.
Using `satpulsetool -v serial` will show more information about what is going on.

## PPS signal connected via GPIO

This approach is Linux-only and is typically used on an SBC, like the Raspberry Pi,
that exposes GPIO pins.

With this approach, the work is divided between satpulsed and the NTP daemon.
satpulsed sends timing samples to the NTP daemon;
these are based on the timing of the serial messages from the GPS receiver,
so on their own they are imprecise: worse than you would typically get from NTP over a network.
The NTP daemon also reads the PPS signal from the GPS receiver, using the kernel PPS subsystem.
The PPS samples accurately mark the start of each second but do not identify which second it is.
The NTP daemon combines the two sources:
the PPS signal determines when a second starts,
and the samples from satpulsed determine which second it is.
The accuracy you can expect is in the microsecond range,
because the time of each PPS edge is measured in software by the kernel.


When using a Raspberry Pi, the PPS signal is connected to a GPIO pin.
GPS HATs typically wire the PPS signal to pin 12 (GPIO 18).
With Raspberry Pi OS, you can configure that pin as a PPS pin by adding the following at the bottom of `/boot/firmware/config.txt`:

```
dtoverlay=pps-gpio,gpiopin=18
```

Reboot after this.

To verify that the PPS signal is working, install pps-tools:

```
sudo apt install pps-tools
```

Then do:

```
sudo ppstest /dev/pps0
```

It should show PPS events once per second:

```
trying PPS source "/dev/pps0"
found PPS source "/dev/pps0"
ok, found 1 source(s), now start fetching data...
source 0 - assert 1775392263.000000336, sequence: 170178 - clear  0.000000000, sequence: 0
source 0 - assert 1775392264.000000215, sequence: 170179 - clear  0.000000000, sequence: 0
```

Exit with Ctrl-C.

## Configure satpulsed

Edit `/etc/satpulse.toml` to look like this:

```
[serial]
device = "/dev/ttyAMA0"
# fix to match your serial device speed
speed = 9600
# fix to match the pin you are using for PPS: "cts", "dcd", "dsr", "ri"
# comment out if you are using the GPIO approach
pps.pin = "cts"

[gps]
config = true

# Enable HTTP monitoring on port 2000
[[http]]
listen = ":2000"

[ntp]
# Use this for chrony
sock.path = "/var/run/chrony.satpulse.sock"
# Use this for ntpd-rs
# sock.path = "/run/ntpd-rs/satpulse.ttyAMA0.sock"
```

Uncomment the `sock.path` line for whichever of chrony or ntpd-rs you use.

The `sock.path` key in the `[ntp]` table makes satpulsed send timing samples to the NTP daemon using chrony's SOCK protocol.
The socket path in the NTP daemon configuration must match that specified here.
With `config = true` in the `[gps]` table and an `[ntp]` table but no `[phc]` table,
satpulsed will configure the GPS receiver appropriately for this mode of operation
(e.g. enable the time pulse, enable time mode, enable messages reporting UTC time).

You can point a browser at port 2000 and you should get a page showing information
from the GPS receiver.

You can use the `[sample.serial.pps]` table to tune how satpulsed generates samples with the serial approach.
For example:

```
[sample.serial.pps]
method = "poll"
```

will force use of the poll method.

The accuracy of measuring pulses is significantly affected by how the computer does power management.
On Linux, you can use `maxWakeupLatency` to trade off power consumption for better accuracy.

```
[sample.serial.pps]
maxWakeupLatency = 0
```

will tell Linux not to allow the CPU to sleep between pulses.
A value of about 10e-6 (i.e. 10 microseconds) will produce a compromise.
If this value is not set, then `satpulsed` will not make any change to power management.
You can experiment with this value using the `--max-wakeup-latency` option of `satpulsetool serial`;
root is required for this option.

See [satpulse.toml(5)]({% link man/satpulse.toml.5.md %}) for the full set of configuration keys.

## Set up chrony

Make sure chrony is installed. The package is called `chrony` on both Fedora and Debian.

For the serial approach a single refclock line needs to be added to the chrony configuration.
With the GPIO approach two refclock lines are needed.
On Debian, these can be conveniently added to a new file `/etc/chrony/conf.d/satpulse.conf`;
on Fedora, add them to `/etc/chrony.conf` instead.

For the serial approach, use the following:

```
refclock SOCK /var/run/chrony.satpulse.sock refid GPS
```

For the GPIO approach, use the following:

```
refclock PPS /dev/pps0 poll 2 lock GPS refid PPS
refclock SOCK /var/run/chrony.satpulse.sock offset 0.1 delay 0.2 refid GPS noselect
```

The `refclock PPS` line makes chrony use the kernel PPS API to read PPS samples from `/dev/pps0`.
These are accurate but lack time-of-day information.
The `refclock SOCK` line makes chrony read samples generated by satpulsed.
With the GPIO approach, these are inaccurate but include time-of-day.
The `offset 0.1` corrects for serial messages coming 0.1 second after the top of the second.
The `lock GPS` option makes chrony use the samples from satpulsed to complete the PPS samples;
the `noselect` option tells chrony not to use the satpulsed samples as an independent time source.

Then restart chrony. The service is named `chrony` on Debian, and `chronyd` on Fedora.

Verify that it is working:

```
chronyc sources
```

The output should include the added reference clocks.

Chrony can also use samples from other NTP servers to complete the PPS samples,
so if you want to check that the GPIO approach is really working,
you should at least temporarily comment out the `pool` line from `/etc/chrony/chrony.conf`.

To provide NTP service to other machines, add an `allow` directive such as this:

```
allow 192.168.1.0/24
```

## Set up ntpd-rs

Ubuntu has [announced](https://discourse.ubuntu.com/t/ntpd-rs-its-about-time/79154) plans to adopt
[ntpd-rs](https://github.com/pendulum-project/ntpd-rs) as its default NTP server, replacing chrony,
primarily because of memory safety.
Since SatPulse is written in Go, the combination of ntpd-rs and SatPulse provides a fully memory-safe timing stack.
Like SatPulse, ntpd-rs uses TOML for its configuration files and supports Prometheus,
so the combination makes for a pleasantly harmonious configuration and observability experience.

### Serial approach

Install ntpd-rs

```
sudo apt install ntpd-rs
```

Put the following in `/etc/ntpd-rs/ntp.toml`:

```
[observability]
observation-path = "/var/run/ntpd-rs/observe"
# uncomment for more information in logs
# log-level = "debug"

[[source]]
mode = "sock"
path = "/run/ntpd-rs/satpulse.ttyAMA0.sock"
precision = 1e-4

[synchronization]
minimum-agreeing-sources = 1

[[server]]
listen = "0.0.0.0:123"
```

### GPIO approach

The GPIO approach needs at least version 1.7.2,
which you can download from the [Releases page](https://github.com/pendulum-project/ntpd-rs/releases).

Next, you will need to remove your existing NTP daemon (e.g. systemd-timesyncd or chrony):

```
sudo apt remove systemd-timesyncd chrony
```

Then install ntpd-rs

```
sudo dpkg -i ./ntpd-rs_1.7.2-1_arm64.deb
```

Now you need to set things up so that ntpd-rs can access `/dev/pps0`.
Create a file `/etc/udev/rules.d/99-ntpd-rs-pps.rules` containing the line

```
KERNEL=="pps0", GROUP="ntpd-rs", MODE="0640"
```

This ensures that when `/dev/pps0` is created at boot time it will have a group and permissions
that enables ntpd-rs to access it. To make this take effect without rebooting, do

```
sudo udevadm control --reload
sudo udevadm trigger /dev/pps0
```

Now we need to configure ntpd-rs.
Put the following in `/etc/ntpd-rs/ntp.toml`:

```
[observability]
observation-path = "/var/run/ntpd-rs/observe"
# uncomment for more information in logs
# log-level = "debug"

[[source]]
mode = "pps"
path = "/dev/pps0"
precision = 1e-7
accuracy = 1e-6

[[source]]
mode = "sock"
precision = 1e-2
path = "/run/ntpd-rs/satpulse.ttyAMA0.sock"
accuracy = 0.2

[synchronization]
minimum-agreeing-sources = 1

[[server]]
listen = "0.0.0.0:123"
```

ntpd-rs distinguishes the accuracy of a source from its precision.
Accuracy means how close measurements are to true time.
Precision means how much measurements vary from each other.
The serial timing measurements from satpulsed are quite precise
but are extremely inaccurate.
Specifying a low accuracy for samples from satpulsed
ensures that ntpd-rs uses the PPS samples as the primary source,
and uses the satpulsed samples just to complete the PPS samples.
But note that if you specify an accuracy of 0.25 or worse,
you need to increase `maximum-source-uncertainty` from its default of 0.25.

### Monitoring

Now you can start ntpd-rs using

```
sudo systemctl start ntpd-rs
```

You can verify it's working by using

```
ntp-ctl status
```
