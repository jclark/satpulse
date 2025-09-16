---
title: Setup ptp4l
---

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

## Running ptp4l directly

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

## Running as a service

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

## Connecting satpulsed to ptp4l

For correct operation of ptp4l as a PTP server, satpulsed needs to update ptp4l
with metadata about the PHC time.
Enable this by adding the following to `satpulse.toml`

```
[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"
```

The value of `ptp4l.udsAddress` needs to match the ptp4l `uds_address` option,
which defaults to `/var/run/ptp4l`.