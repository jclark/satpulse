---
title: Installing SatPulse
---

<!-- From quickstart.md lines 3-13 -->
This assumes a Linux distribution that uses systemd. Instructions differ slightly between:

* Debian-based distributions using apt-based package management: Debian, Raspberry Pi OS, Ubuntu
* Fedora-based distributions using rpm-based package management: Fedora, CentOS, RHEL

Installation will install a configuration file and a systemd service.
You will need to edit the configuration file and then use systemd commands to start and enable the service.

After that you should get ptp4l and chrony working.

## Installation

<!-- From quickstart.md lines 14-18 -->
You can either install from a package (`.deb` or `.rpm` file) or from source.

### Install from a package

<!-- From quickstart.md lines 19-38 -->
Go to the [Releases](https://github.com/jclark/satpulse/releases) page, then under Assets, select the  package with the appropriate extension:

| Distro | Intel/AMD | ARM |
| --- | --- | --- |
| Debian-based (includes Raspberry Pi OS, Ubuntu) | `_amd64.deb` | `_arm64.deb` |
| Fedora-based | `.x86_64.rpm` | `.aarch64.rpm` |

The `.deb` file can be installed using e.g.

```
sudo dpkg -i satpulse_20250310_arm64.deb
```

The `.rpm` file can be installed using e.g.

```
sudo rpm -i satpulse-20250310.x86_64.rpm
```

Use `-U` instead of `-i` if you are upgrading from an earlier version.

### Install from source

<!-- From quickstart.md lines 41-58 -->
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
* the systemd service template unit file for the daemon installed as `/etc/systemd/system/satpulse@.service`
* the SatPulse command line tool installed as `/usr/local/sbin/satpulsetool`