---
title: Precision timing with a PHC
redirect_from:
  - /setup/chrony.html
  - /setup/ptp4l.html
---

This page describes how to use SatPulse for precision timing,
using a network interface whose PTP Hardware Clock (PHC) can timestamp a PPS signal in hardware.
satpulsed synchronizes the PHC from the GPS;
this supports running a PTP server with ptp4l,
and gives chrony a much more accurate source than the software PPS timestamping
used in [Basic use with NTP]({% link setup/ntp.md %}).
This applies to Linux only, and needs unusual, specialist hardware:
see the [hardware section]({% link hardware/index.md %}) for suitable hardware,
in particular the Raspberry Pi CM4/CM5 builds and Intel NIC builds.

The stages are:

1. verify that the PPS signal is reaching the PHC;
2. configure satpulsed to synchronize the PHC;
3. set up chrony;
4. if you want to provide a PTP server, set up ptp4l.

## Verify PPS input on the PHC

SatPulse needs to know the name of the ethernet interface
with the pin that the PPS output of the GPS module is connected to. 

It also needs to know the number of the pin.
These pins are often called Software Defined Pins (SDPs).
They are numbered starting from 0 and also have a name.
SatPulse defaults to using pin 0.
So if there is only one pin, then there is no need to specify anything.

The ethernet controller will have a PTP Hardware Clock (PHC) device associated with it,
which has an index which is a non-negative integer.
If the index is N, then the device path is /dev/ptpN.
In Linux, the network interface has an associated PHC and
operations related to PPS input apply to the PHC rather than directly to the network interface.

satpulsetool provides a subcommand [sdp]({%link man/satpulsetool-sdp.1.md%}) which is useful for identifying the interface.

* `satpulsetool sdp` with no arguments shows information about each interface that has one or more SDPs;
* `satpulsetool sdp -i --pin 0 eth0` checks for input on the specified interface and pin for 2 seconds by default; it prints out the timestamps it received as a number of seconds since the beginning of 1970; `--pin 0` is the default.

Make sure the interface is in an UP state;
`satpulsetool sdp` shows the status of each interface.
If it's not up, then bring it up with, for example:

```
sudo ip link set enp1s0 up
```

### CM4/5

On the Raspberry Pi CM4 and CM5 with Raspberry Pi OS, the network interface is `eth0` and there's only one pin,
which is named SYNC_OUT.

On a CM5, `satpulsetool sdp` should show something like this:

```
Interface: eth0
  Status: up, carrier
  Driver: macb
  PCI: 1f00100000.ethernet
  PTP clock device: /dev/ptp0
  Pins: SYNC_OUT
  External timestamp channels: 1
  Periodic output channels: 1
```

If it doesn't show anything, then your kernel is not working.

Doing

```
sudo satpulsetool sdp -i eth0
```

should print one or two numbers, like this

```
9623.787299552
9624.787296512
```

This means it has received two timestamps, one second apart, so everything is working as expected.

### Intel

With Intel ethernet controllers, the name of interface can vary.
With the i210/i225/i226, there are four pins called SDP0, SDP1, SDP2 and SDP3.

On my Intel box, which has two i225 cards in it, `satpulsetool sdp` shows this:

```
Interface: enp4s0
  Status: up, no-carrier
  Driver: igc
  PCI: 0000:04:00.0
  Vendor: Intel Corporation
  Device: Ethernet Controller I225-LM
  Revision: 03
  PTP clock device: /dev/ptp0
  Pins: SDP0, SDP1, SDP2, SDP3
  External timestamp channels: 2
  Periodic output channels: 2
Interface: enp5s0
  Status: down
  Driver: igc
  PCI: 0000:05:00.0
  Vendor: Intel Corporation
  Device: Ethernet Controller I225-LM
  Revision: 03
  PTP clock device: /dev/ptp1
  Pins: SDP0, SDP1, SDP2, SDP3
  External timestamp channels: 2
  Periodic output channels: 2
```

The one I want is the one that is up, which is `enp4s0`.
The GPS is attached to pin 1, so doing:

```
sudo satpulsetool sdp -i --pin 1 enp4s0
```

shows:

```
1757770748.482694937
1757770748.582695570
1757770749.482701362
1757770749.582701989
```

The i210 and i225/i226 NICs have the quirk that they timestamp both the rising and falling edges of every pulse.
This means that a PPS signal will result in two timestamps per second.
Most GPS receivers default to a pulse width of 0.1s
which means that you should see consecutive timestamps separated alternately by roughly 0.1s and 0.9s
(i.e. 100,000,000 and 900,000,000 nanoseconds).
By looking at the timestamps you can determine what the pulse width is.

If I try without `--pin 1`, it would report no timestamps received.

## Synchronize the PHC

satpulsed synchronizes the PHC when the `[phc]` table in `satpulse.toml` has a non-empty `interface` key:

```
[phc]
interface = "enp1s0"
```

The `interface` key specifies the ethernet interface with the pin that the GPS PPS output is attached to.
In this example it is `enp1s0`.
If the pin is not 0, then it also needs to specify the pin index:

```
[phc]
interface = "enp1s0"
pin = 1
```

## Set up chrony

Chrony serves three purposes here:

* it synchronizes the system clock, using samples that satpulsed derives from the PHC;
* it runs an NTP client, which provides an important check that the PHC time is correct; and
* it runs an NTP server, if desired.

Make sure chrony is installed. The package is called `chrony` on both Fedora and Debian.

Add this to `satpulse.toml`:

```
[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

This will make satpulsed send timing samples to chrony using its SOCK protocol through a socket at `/var/run/chrony.satpulse.sock`.

Add this line to your chrony configuration:

```
refclock SOCK /var/run/chrony.satpulse.sock poll 2 filter 4 refid GNSS
```

On Fedora, just add it to `/etc/chrony.conf`.
On Debian, I suggest making it a separate file `/etc/chrony/conf.d/satpulse.conf`.

This adds a refclock called `GNSS` to chrony that will read samples from  `/var/run/chrony.satpulse.sock` using the SOCK protocol.
The socket path in the chrony configuration needs to match the socket path in the satpulse configuration.

Then restart chrony. The service is named `chrony` on Debian, and `chronyd` on Fedora.

Verify that it is working:

```
chronyc sources
```

The output should include the `GNSS` reference clock.

## Set up ptp4l

The main program in linuxptp is ptp4l, which is a daemon that implements the PTP protocol.
It works as a server and a client: setup for both is very similar.

Install the linuxptp package:
   * On Debian: `sudo apt install linuxptp`
   * On Fedora: `sudo dnf install linuxptp`

Debian Bookworm uses linuxptp version 3. Debian Trixie and Fedora use version 4.
There are a few differences between these versions.
* Version 3 uses PTP version 2.0 whereas version 4 uses PTP version 2.1.
   This causes problems for some hardware that does not implement version 2.0 correctly,
   including the Raspberry Pi CM4/CM5.
   To workaround this, ptp4l needs an option of `ptp_minor_version 0`.
* Traditionally, PTP has used the terminology grandmaster and slave.
  The options in linuxptp 3 use this terminology.
  But PTP is switching to use server and client, and linuxptp has adopted this terminology.
  This has resulted in renaming options.

The CM4/CM5 drivers take longer than usual to report the transmitted timestamp after a PTP message has been sent.
This requires increasing the value of the `tx_timestamp_timeout` option from its default value of 10.
A value of 100 seems to be sufficient.

### Running ptp4l directly

Before setting up the ptp4l service and configuration files, you can verify the operation of ptp4l by running it directly from the command line
without any configuration file.
Run ptp4l on one machine as a client and on another machine as a server.

Configuration options that are usually in the configuration file can be specified on the command line using `--`.

The command to run ptp4l directly on the command line on eth0 looks like this:

```
sudo ptp4l -i eth0 -m -q
```

The `-m` and `-q` command line options make it log to stdout rather than the system logger.
You can also add `-l 7` to change the logging level to include debugging information.

The following options also need to be added depending on the situation:

* on the client machine, add `--clientOnly 1` for linuxptp 4, and `--slaveOnly 1` for linuxptp 3
* on the server machine, add `--serverOnly 1` for linuxptp 4, and `--masterOnly 1` for linuxptp 3
* for linuxptp 4, if the server or client is a CM4/CM5, add `--ptp_minor_version 0`
* if the machine is a CM4 or CM5, add `--tx_timestamp_timeout 100`

### Running as a service

On Debian, the system supplied ptp4l service is not ideal, in particular it won't work for the Raspberry Pi CM4,
so you should install a replacement `ptp4l.service` file as `/etc/systemd/system/ptp4l.service`.
The replacement is in

*  `configs/ptp4l.service` in the source, 
*  `/usr/share/doc/satpulse/ptp4l.service` when the .deb package has been installed

Next you will need to create a ptp4l config file.
   * On Debian: the file is `/etc/linuxptp/ptp4l.conf`
   * On Fedora: the file is `/etc/ptp4l.conf`

Start with this and edit according to the comments:

```
[global]
# When running as a server (on the machine with SatPulse), we do not want both satpulsed and ptp4l adjusting the PHC.
# Uncomment the next line with linuxptp 4
#serverOnly 1
# Uncomment the next line with linuxptp 3
#masterOnly 1

# Uncomment the next line with the Raspberry Pi CM4 or CM5
#tx_timestamp_timeout 100

# Uncomment the next line with linuxptp 4 for interoperability with Raspberry Pi CM4/CM5
#ptp_minor_version 0

# The presence of this section makes ptp4l run on this interface.
# Change eth0 to be whatever interface you want to run ptp4l on.
[eth0]
```

The network interface in the last line should match the interface in `satpulse.toml`.

There are many, many other options that can be changed. These options are commonly changed to conform to a particular PTP profile.
There are a number of example configuration files for various PTP profiles in `/usr/share/doc/linuxptp/configs`.

There are reports of P2P mode (`delay_mechanism P2P`) not working on the CM4/CM5.

Finally, start and enable the ptp4l service:

```
sudo systemctl enable --now ptp4l
```

### Connecting satpulsed to ptp4l

For correct operation of ptp4l as a PTP server, satpulsed needs to update ptp4l
with metadata about the PHC time.
Enable this by adding the following to `satpulse.toml`

```
[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"
```

The value of `ptp4l.udsAddress` needs to match the ptp4l `uds_address` option,
which defaults to `/var/run/ptp4l`.
