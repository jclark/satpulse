---
title: Installing SatPulse
---

Packages are available for:

* Debian-based Linux distributions using apt-based package management: Debian, Raspberry Pi OS, Ubuntu
* Fedora-based Linux distributions using rpm-based package management: Fedora, CentOS, RHEL

Alternatively, you can install from source.

## Install from a package

Go to the [Releases](https://github.com/jclark/satpulse/releases) page, then under Assets, select the package with the appropriate extension:

| Distro | Intel/AMD | 64-bit ARM | 32-bit ARM |
| --- | --- | --- | --- |
| Debian-based (includes Raspberry Pi OS, Ubuntu) | `_amd64.deb` | `_arm64.deb` | `_armhf.deb` |
| Fedora-based | `.x86_64.rpm` | `.aarch64.rpm` | none |

The `_armhf.deb` package is for 32-bit ARM systems, such as the 32-bit version of Raspberry Pi OS. {% include new-in-03.html %}
Its binaries are built for ARMv6,
so they work on every Raspberry Pi model, including the Pi Zero.

The `.deb` file can be installed using e.g.

```
sudo dpkg -i satpulse_20250310_arm64.deb
```

The `.rpm` file can be installed using e.g.

```
sudo rpm -i satpulse-20250310.x86_64.rpm
```

With rpm, use `-U` instead of `-i` if you are upgrading from an earlier version.

## Install from source

1. [Install Go](https://go.dev/doc/install)
2. Make sure you have `git` installed
   * On Debian: `sudo apt install git`
   * On Fedora: `sudo dnf install git`
3. Clone the satpulse repository: `git clone https://github.com/jclark/satpulse.git`
4. Change into the satpulse directory: `cd satpulse`
5. Build it: `make`
6. Install it: `sudo make install`

After this, you will have:

* the SatPulse daemon installed as `/usr/local/sbin/satpulsed`
* the configuration file for the daemon installed as `/usr/local/etc/satpulse.toml`
* the systemd service template unit file for the daemon installed as `/etc/systemd/system/satpulse@.service`
* the SatPulse command line tool installed as `/usr/local/bin/satpulsetool`

On BSD (macOS or FreeBSD), build using the `bsd-build.sh` script, and copy the binaries into place manually.