---
title: PTP server setup
---

## Install and configure ptp4l as a PTP server

<!-- From quickstart.md lines 169-206 -->
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

# Uncomment this for rPI CM4 or CM5
# tx_timestamp_timeout 100

# The presence of this section makes ptp4l run on this interface.
[eth0]
```

The network interface in the last line should match the interface in `satpulse.toml`.

Then start and enable the ptp4l service:

```
sudo systemctl enable --now ptp4l
```