---
title: Setup on macOS
---

## Connecting the GPS receiver

Computers running macOS generally lack RS232 ports or GPIO pins,
which means that the GPS receiver has to be connected using a [USB serial adapter]({% link hardware/usb-serial.md %}).
If you want to run a stratum 1 NTP server, with the GPS receiver supplying time to the NTP server,
you will need a USB serial adapter that has CTS/RTS pins.

The USB serial adapter needs to be wired up as follows:

| GPS pin | USB serial pin |
| --- | --- |
| VCC | VCC |
| GND | GND |
| TXD | RXD |
| RXD | TXD |
| PPS | CTS |

Most USB serial adapters are what is called full-speed,
which means they run at 12Mbps, which was the maximum speed supported by USB1.
I have found that some Macs do not handle full-speed devices as efficiently as they might,
and this significantly reduces the accuracy possible with a PPS signal.
My MacBook Air M3 has this problem, but my Mac Mini M4 doesn't.
The easiest workaround is to connect the USB serial adapter via a USB2 or USB3 hub;
this works because the hub performs the translation between full-speed transactions and high-speed USB2 transactions.
Alternatively you can use a high-speed USB serial adapter,
i.e. one using the FTDI FT232H chip,
such as the [Adafruit FT232H breakout](https://www.adafruit.com/product/2264).

## Install SatPulse

Install using the Homebrew tap `jclark/satpulse`. {% include new-in-03.html %}
While 0.3 is still a prerelease, you need to use the `satpulse-pre` formula:

```sh
brew tap jclark/satpulse
brew install jclark/satpulse/satpulse-pre
```

If you want the bleeding edge, you can instead install the `master` branch:

```sh
brew install --HEAD jclark/satpulse/satpulse
```

Alternatively, you can [install from source]({% link setup/satpulse-install.md %}#install-from-source).

On an Apple Silicon Mac, Homebrew installs files under `/opt/homebrew`;
on an Intel Mac, it uses `/usr/local` instead.
This page is written assuming the Apple Silicon layout.

The Homebrew tap installs the following files.

| File | Location |
|---|---|
| `satpulsed` | `/opt/homebrew/sbin/satpulsed` |
| `satpulsetool` | `/opt/homebrew/bin/satpulsetool` |
| `satpulsewb` | `/opt/homebrew/bin/satpulsewb` |
| `find-serial` | `/opt/homebrew/bin/find-serial` |
| config | `/opt/homebrew/etc/satpulse.toml` (not overwritten on upgrade) |
| service config | `/opt/homebrew/etc/find-serial.env` (not overwritten on upgrade) |
| man pages | `/opt/homebrew/share/man/man{1,5,8}/...` |
| gpsmsg tree | `/opt/homebrew/share/satpulse/gpsmsg/...` |
| logs | `/opt/homebrew/var/log/satpulse/...` |
| service wrapper | `/opt/homebrew/opt/<formula>/libexec/satpulse-service` |
| launchd plist | `/opt/homebrew/opt/<formula>/sh.brew.<formula>.plist` |

## Serial connection

See [Verify serial connection to GPS module]({% link setup/gps-serial.md %}) for
how to determine the serial device and speed used by your GPS receiver.
Note that in many cases, the device name used by macOS will depend on which USB port the GPS receiver is plugged into.

## Use SatPulse Workbench

Once you know the serial device and speed, I recommend using [SatPulse Workbench]({% link workbench/index.md %})
to verify that your receiver is tracking satellites and has acquired a lock.
Run `satpulsewb` with no arguments:

```
satpulsewb
```

If you are running locally, it will open the browser for you.
It will also print a URL that you can copy and paste into your browser.
You can then use the dropdown boxes to select the device name and speed.

If you have a receiver for which SatPulse has experimental high-level configuration support
(see [GPS module support]({% link gps-module-support/index.md %})),
you should specify a suitable `--vendor` option. For example:

```sh
satpulsewb --vendor zhongke
```

for an ATGM332D or ATGM336H by Zhongke Microelectronics.

## Configure and run satpulsed

The next step is to get `satpulsed` running.
You will need to [edit its configuration file]({% link setup/satpulsed.md %}#configuration-file),
which on macOS is at `/opt/homebrew/etc/satpulse.toml`.
At a minimum you will need to specify the serial speed.

On macOS `satpulsed` is usually run as a service using launchd.
The Homebrew installation of `satpulsed` is by default set up so that you do not need to specify the serial device;
this ensures the service will continue to work even if the GPS receiver gets plugged into a different USB port.
This works using a macOS-specific utility `find-serial`, which is included in the Homebrew tap.
The launchd `.plist` file calls `satpulsed` via a service wrapper.
The wrapper calls `find-serial` to discover the serial device,
which in turn calls `satpulsed` with an option specifying that device.
This overrides any device specified in `satpulse.toml`. 
This scheme works automatically provided there is only a single USB serial device connected.
You can change how the wrapper uses `find-serial` by editing `/opt/homebrew/etc/find-serial.env`.

If you are using a USB serial adapter with a serial number that gives you a stable device name,
then you can disable the use of `find-serial` for device discovery by setting

```
FIND_SERIAL_DISABLE=true
```

in that file, and instead specifying the device in `satpulse.toml`.

If you have multiple serial devices that can be distinguished by their USB VID and PID,
then you can make find-serial discover the right device by setting, for example:

```sh
FIND_SERIAL_OPTS="--vid 1546 --pid 01A9"
```

The service is managed with `brew services`.
Usually the daemon is started as a per-user LaunchAgent without using `sudo`.
Start the service with:

```sh
brew services run satpulse-pre
```

If you installed the latest code on the `master` branch, start the service with:

```sh
brew services run satpulse
```

If you use the `start` subcommand instead of `run`, then the service will be started automatically at login.
The service can be stopped using the `stop` subcommand.

The service writes the daemon's log output to files under `/opt/homebrew/var/log/satpulse/`.

See [Monitor satpulsed]({% link setup/monitor.md %}) for monitoring,
and [RTK setup]({% link setup/rtk.md %}) for positioning with RTK.

## Use with NTP

You can set up a stratum 1 time server by using `satpulsed` together with the [chrony](https://chrony-project.org/) NTP server.

The [NTP page]({% link setup/ntp.md %}) has general instructions for using `satpulsed` with an NTP server.
This section is a summary, with additional macOS specifics.
The only way of connecting a PPS signal on macOS is [via the serial port]({% link setup/ntp.md %}#pps-signal-connected-via-serial-port).
Note the `kernel` and `wait` methods discussed on that page are not available in macOS: only the `poll` method is.

You will need to download and install chrony.
I recommend using the version included with [ChronyControl](https://whatroute.net/chronycontrol.html),
which provides a nice GUI for monitoring chrony.

As mentioned above, the PPS pin on the GPS receiver needs to be connected to the CTS pin on the USB serial adapter.
You can use `satpulsetool serial` to check that the PPS signal is visible. For example,

```
satpulsetool serial -d /dev/cu.usbserial-BG03U08C -s 38400 -p cts -t 20
```

In the above command:

* `/dev/cu.usbserial-BG03U08C` specifies the device
* `-s 38400` says to use a speed of 38400
* `-p cts` says to use the CTS pin
* `-t 20` says to run for 20 seconds

This will print the time of each pulse received.
It may take a few seconds to acquire the PPS signal.
You can use the `-j` option to get rich information in JSONL format, for example:

```
{"device":"/dev/cu.usbserial-BG03U08C","t":"2026-09-24T07:29:44.004961468Z","uncertainty":[0.002006302,0.002008615],"pollWidths":[0.002004542,0.002009167]}
```

This is showing things not working well, with the USB serial adapter plugged directly into a MacBook Air M3.
Observe that each of the poll widths is ~2ms. They should mostly be less than 500µs.
So in this case, you should use a hub between the MacBook and the USB serial adapter.
Not all hubs work equally well.

After you have got PPS working with `satpulsetool serial`,
configure `satpulsed` to also read a PPS signal over the serial port.
In the `[serial]` table in `satpulse.toml`, add a line:

```
pps.pin = "cts"
```

The next stage is to configure a connection between `satpulsed` and chrony.
There are two approaches to this.
Homebrew is oriented towards running services as per-user LaunchAgents,
whereas chrony expects to run as root.

### Running satpulsed as a normal user

The first approach is to run `satpulsed` as a normal user and not root.
Usually chrony does not allow normal users to send time information to it.
But we can make this work by creating a directory with special permissions.

```
sudo install -d -o root -g wheel -m 0755 /var/db/satpulse
sudo chmod +a "user:$(id -un) allow write,file_inherit,only_inherit" /var/db/satpulse
```

The directory cannot be under `/var/run` because that is cleared on boot, so we use `/var/db` instead.
The `chmod` command sets an ACL so that any file created in the directory inherits permissions for you to write to it.

Add the following line to `/etc/chrony.d/chrony.conf`:

```
refclock SOCK /var/db/satpulse/chrony.sock refid CTS precision 1e-4
```

Then in `satpulse.toml`, in the `[ntp]` table, create a matching line

```
sock.path = "/var/db/satpulse/chrony.sock"
```

Then restart `satpulsed`:

```sh
brew services stop satpulse-pre
brew services run satpulse-pre
```

If you use `start` rather than `run`, then satpulsed will run automatically when you log in.

### Running satpulsed as root

The other approach is to run satpulsed as a LaunchDaemon.
This runs as root and has the big advantage that the service launches at boot rather than user login.
However, it does not work so smoothly with brew.
In this case we can use the normal location for the chrony socket.

Add the following line to `/etc/chrony.d/chrony.conf`:

```
refclock SOCK /var/run/chrony.satpulse.sock refid CTS precision 1e-4
```

Then in `satpulse.toml`, in the `[ntp]` table, create a matching line

```
sock.path = "/var/run/chrony.satpulse.sock"
```

With this approach, `brew services` must be run with `sudo`.
If it was running as a per-user LaunchAgent, then stop it first:

```sh
brew services stop satpulse-pre
```

Then start it again using `sudo`:

```sh
sudo brew services start satpulse-pre
```

When you do this brew will warn about changing ownership of various files installed by the tap.
It does this in an effort to keep things secure, but it is not thorough enough to be actually secure.
So in practice, the root installation is trusting the Homebrew user.

When you upgrade satpulse, you will need to stop satpulsed, and then change the ownership of the files back to yourself, before upgrading:

```sh
sudo chown -h "$(id -un)" \
  /opt/homebrew/opt/satpulse-pre \
  /opt/homebrew/var/homebrew/linked/satpulse-pre \
  /opt/homebrew/opt/satpulse-pre/bin \
  /opt/homebrew/opt/satpulse-pre/sbin \
  /opt/homebrew/opt/satpulse-pre/libexec \
  /opt/homebrew/opt/satpulse-pre/libexec/satpulse-service
```

`brew services` does not support `run` with `sudo`, so you must use `start`, which will both start the service and register it to start on boot.
Use the `stop` subcommand instead to stop and unregister the service.

You can avoid the need to change ownership of files on upgrade by using the `--sudo-service-user=root` option on every use of `sudo brew services` applying to the formula.

### Finishing NTP setup

Finally restart chrony by using the GUI or running the command

```
sudo launchctl kickstart -k system/org.chrony-project.chronyd

```

If everything is working, then chrony should switch over to using the `CTS` refclock.
Run

```sh
chronyc sources
```

Within a minute or two, the output should include a line like:

```
#* CTS                           0   4   377    19  -6245ns[-9762ns] +/-  100us
```

You may be able to get better accuracy by using "pre-warming".
For example, in `satpulse.toml` add:

```
[sample.serial.pps]
pollPreWarm = 50e-3
```

This uses additional CPU time (5% of one core in this case),
to reduce the latency caused by power management.
This is also supported in `satpulsetool serial` with the `--poll-pre-warm` option.

## find-serial utility

The find-serial utility, which is used by the service wrapper, can also be used directly.

With no arguments, it works similarly to `satpulsetool serial`, displaying the available USB serial devices.
It will print one line for each device, for example:

```
device=/dev/cu.usbmodem11301 vid=1546 pid=01A9 location=1130100 model="u-blox GNSS receiver" vendor="u-blox AG - www.u-blox.com"
```

If you specify a `--vid` or `--pid` option, it will print lines only for devices with a matching VID or PID.

With `--exec`, find-serial instead runs a command,
replacing a `{}` argument with the device path.
This means you do not have to look up the device name at all:

```
find-serial --exec -- satpulsetool gps -s 9600 -d {}
```

If you have more than one USB serial device, then use `--vid` or `--pid` to select the device. For example:

```
find-serial --vid 1546 --pid 01A9 --exec -- satpulsewb -s 9600 -d {}
```

With `--wait`, if there is no matching device, then find-serial will listen for notifications of new USB devices,
until there is a matching device,
instead of failing immediately.
