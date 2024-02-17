# Getting started with SatPulse

This assumes a Linux distribution that uses systemd. Instructions differ slightly between:

* Debian-based distributions using apt-based package management: Debian, Raspberry Pi OS, Ubuntu
* Fedora-based distributions using rpm-based package management: Fedora, CentOS, RHEL

Installation will install a configuration file and a systemd service.
You will need to edit the configuration file and then use systemd commands to start and enable the service.

After that you should get ptp4l and chrony working.

## Installation

You can either install from source or from a package (`.deb` or `.rpm` file).

### Install from source

1. [Install Go](https://go.dev/doc/install)
2. Make sure you have `git` installed
   * On Debian: `sudo apt install git`
   * On Fedora: `sudo dnf install git`
3. Clone the satpulse repository: `git clone https://github.com/jclark/satpulse.git`
4. Change into the satpulse directory: `cd satpulse`
5. Build it: `make`
6. Install it: `sudo make install`

After this, you will have

* the SatPulse daemon installed `/usr/local/sbin/satpulsed`
* the configuration file for the daemon installed as `/usr/local/etc/satpulse.toml`
* the systemd unit file for the daemon installed as `/etc/systemd/system/satpulse@.service`
* the SatPulse command line tool installed as `/usr/local/sbin/satpulsetool`

### Install from a package

XXX

## Required information

The GPS receiver must have two connections to the computer on which SatPulse is running.

First, there will be a serial connection between the USB or serial port on the GPS receiver to a USB or serial port on the computer.
You need to know which device this is. On a Raspberry Pi CM4 it is typically `/dev/ttyAMA0`.
On a PC, it might be `/dev/ttyS0` or `/dev/ttyACM0` or `/dev/ttyUSB0`, with possibly a higher number than `0`.
You also need to know the speed the GPS receiver is using; 9600 is the most common speed, but some recent devices use 38400.

Second, there will be a PPS connection between the PPS output of the GPS receiver and the pin on the ethernet controller.
You need to know the the name of the ethernet interface. On a Raspberry Pi CM4 using Raspberry Pi OS, it is typically `eth0`.
On a PC, it is typically something like `enp1s0` or `enp2s0`.
You also need to known the index of the pin; this is usually 0, but on a PC, could also be 1 or 2.

If you put the wrong values for these, there will be an error in the logs.

## Configuration file

The configuration file will be at

- `/etc/satpulse.toml` if you installed from a package
- `/usr/local/etc/satpulse.toml` if you installed from source

The configuration file is in [TOML](https://toml.io/en/) format, which is inspired by the INI file format
and can be edited with a normal text editor (e.g. `nano`).

A minimal configuration file would look like this:

```
# Configuration file for satpulse
[phc]
interface = "enp1s0"

[serial]
speed = 9600

# following are optional
[gps]
config = true

[ptp]
ptp4l.udsAddress = "/var/run/ptp4l"

[ntp]
sock.path = "/var/run/chrony.satpulse.sock"
```

In the above, `#` starts a comment. The lines with square brackets mark the start of a *table*; the square brackets
enclose the name of the table. Following the start of each table are the key/value pairs in that table.
Values can be strings in double quotes
(e.g. `"enp1s0"`), numbers (e.g. `9600`) or booleans (e.g. `true`, `false`).

The above configuration file specifies that

* the ethernet interface with the pin that the GPS output pin is attached to is `enp1s0` (the pin index is defaulted to 0)
* the serial speed is 9600 baud (the serial device is usually specified by systemd commands)
* the GPS receiver should be configured to work optimally for timing; note that this won't make any persistent changes to the GPS
  receiver, so you can turn it off and on again to undo any changes
* ptp4l should be updated using the Unix domain socket at `/var/run/ptp4l`
* timing samples should be sent to chrony using the SOCK protocol through a socket at `/var/run/chrony.satpulse.sock`


## Systemd commands

The systemd service template name is `satpulse@.service` and the expected argument is the serial device name without `/dev/`.
For example, if the serial device is `/dev/ttyS0`, then the instantiated service would be named `satpulse@ttyS0.service`.
Systemd commmands need to be given the instantiated service name (although typically the `.service` part can be left out).

After editing the configuration file, you can start the service with

```
sudo systemctl start systemd@ttyS0.service
```

replacing `ttyS0` with the right value for your setup. You can the check that it is working with

```
sudo systemctl status systemd@ttyS0.service
```

This will make the service run automatically after the system boots:

```
sudo systemctl enable systemd@ttyS0.service
```

Here are some other systemd command you may need. Stop the service:

```
sudo systemctl stop systemd@ttyS0.service
```

Restart the service (e.g. after editing the configuration file)

```
sudo systemctl stop systemd@ttyS0.service
```

Enable the service:

```
sudo systemctl enable systemd@ttyS0.service
```

Show the logs for the last 5 minutes:

```
sudo journalctl -u satpulse@ttyACM0 -S -5m
```

## Install and configure ptp4l as a PTP server

Install the linuxptp package:
   * On Debian: `sudo apt install linuxptp`
   * On Fedora: `sudo dnf install linuxptp`

On Debian, the system supplied ptp4l service is not ideal, in particular it won't work for the Raspberry Pi CM4,
So you should install a replacement `ptp4l.service` file as `/etc/systemd/system/ptp4l.service`.
The replacement is in

*  `configs/ptp4l.service` in the source, 
*  `/usr/share/doc/satpulse/ptp4l.service` when the .deb package has been installed

Next you will need to edit the ptp4l config file.
   * On Debian: the file is `/etc/linuxptp/ptp4l.conf`
   * On Fedora: the file is `/etc/ptp4l.conf`

You can start with this:

```
[global]
# We don't want ptp4l and satpulse to both adjust the PHC, so only run as a master.
masterOnly 1

# Uncomment this for rPI CM4
# tx_timestamp_timeout 100

# The presence of this section makes ptp4l run on this interface.
[eth0]
```

The network interface in the last line should match the interface in `satpulse.toml`.

Then start and enable the ptp4l service:

```
sudo systemctl enable --now ptp4l
```

## chrony

Make sure chrony is installed. The package is called `chrony` on both Fedora and Debian.

Add this line to your chrony configuration:

```
refclock SOCK /var/run/chrony.satpulse.sock poll 2 filter 4 refid GNSS
```

On Fedora, just add it to `/etc/chrony.conf`.
On Debian, I suggest making it a separate file `/etc/chrony.d/conf.d/satpulse.conf`.
The socket path here `/var/run/chrony.satpulse.sock` needs to match that specified by the `sock.path` key in the `ntp` table.

Then restart chrony. The service is named `chrony` on Debian, and `chronyd` on Fedora.
