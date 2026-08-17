---
title: Installing SatPulse
---

Linux packages are available for:

* Debian-based distributions using apt-based package management: Debian, Raspberry Pi OS, Ubuntu
* Fedora-based distributions using rpm-based package management: Fedora, CentOS, RHEL

On macOS, SatPulse can be installed with Homebrew.

Alternatively, you can install from source.

## Install on Linux from a package

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

## Install on macOS with Homebrew

On macOS, SatPulse builds and installs from source via the Homebrew tap
[`jclark/satpulse`](https://github.com/jclark/homebrew-satpulse). {% include new-in-03.html %}
The macOS port is still new, so use the prerelease channel;
the tap's README gives the install commands,
and covers running satpulsed as a service and where files are installed.

## Install from source

On all platforms, the first steps are the same:

1. [Install Go](https://go.dev/doc/install)
2. Make sure you have `git` installed; if not, install according to OS:
   * Debian: `sudo apt install git`
   * Fedora: `sudo dnf install git`
   * macOS: `brew install git`
   * Windows: install [Git for Windows](https://git-scm.com/download/win)
3. Clone the satpulse repository: `git clone https://github.com/jclark/satpulse.git`
4. Change into the satpulse directory: `cd satpulse`

The build steps depend on the platform.

### Linux

Build with `make`, then install with `sudo make install`.

After this, you will have:

* the SatPulse daemon installed as `/usr/local/sbin/satpulsed`
* the configuration file for the daemon installed as `/usr/local/etc/satpulse.toml`
* the systemd service template unit file for the daemon installed as `/etc/systemd/system/satpulse@.service`
* the SatPulse command line tool installed as `/usr/local/bin/satpulsetool`
* the GPS message files installed under `/usr/local/share/satpulse/gpsmsg`, organized by vendor directory {% include new-in-03.html %}

### macOS and FreeBSD

Build using the `unix-build.sh` script, which puts the binaries under `out/`.

### Windows

Build using the `win-build.ps1` PowerShell script, which puts the binaries under `out\`.

satpulsed can run as a Windows service.
From an elevated PowerShell:

```
.\satpulsed.exe --register -f C:\satpulse\satpulse.toml --log-file C:\satpulse\satpulsed.log
```

This registers a `satpulsed` service that starts automatically at boot,
reading the given configuration file and writing the daemon log to the `--log-file` path.
The executable you ran is registered in place;
add `--copy 'C:\Program Files\SatPulse'` to first copy it to that directory and register the copy,
so that the running service does not lock the binary you build.
Start and stop the service with `sc start satpulsed` and `sc stop satpulsed`.
Service starts, stops, and any terminal error are recorded in the Windows Event Log under the source `satpulsed`.

Remove the service registration with:

```
.\satpulsed.exe --unregister
```

This stops the service if it is running and unregisters it.
It deletes no files: when the registered executable is not the one running,
its path is printed so you can remove it yourself.
It works without the configuration file,
so a broken or deleted configuration does not prevent unregistration.