---
title: Using RTK
---

Real-Time Kinematic (RTK) positioning is a precision positioning technology, which uses two GNSS receivers: a base station with a precisely known position which provides correction data in the form of RTCM messages, and a rover, which can use this correction data to determine its position with centimeter-level accuracy.
The distance between the base and the rover is limited to about 10-20 kilometers.

The requirements for an RTK base station and a precision time server are similar.
GNSS receivers that perform well for RTK typically also provide good timing performance, and because RTK is a much larger market, RTK-capable receivers often offer better value for timing applications.
Both applications require a stationary antenna with a precisely known position.
SatPulse enables a single GNSS receiver to be used simultaneously for both purposes.
Running an RTK base and contributing to a public caster such as [RTK2go](http://rtk2go.com/) is a good way to make your GNSS receiver as useful as possible.
RTK is also useful for determining the precise antenna position needed for GNSS time mode.

This guide shows how to configure SatPulse as an RTK base station,
and also how you can use SatPulse as a rover to verify the correct operation of the base station.
This works best with u-blox receivers; with other receivers, you will need to use other tools for some of the configuration.
I developed this guide with a u-blox ZED-F9T as the base and a u-blox ZED-F9P as the rover.

## Base station

### Enabling RTCM messages

If you have a u-blox receiver that supports RTCM (e.g. ZED-F9P or ZED-F9T), SatPulse makes it easy to configure your receiver to output RTCM messages. If you have a non u-blox receiver, you will have to use another program to enable the appropriate messages.

Edit the `[gps]` table of `/etc/satpulse.toml` so it includes the following lines:

```
[gps]
config=true
rtcmOutput=true
```

This will configure a sensible set of messages:
* 1005 for the antenna reference position of the base station
* MSM4 messages for every major GNSS constellation that is enabled; if MSM4 is not supported (e.g. on a ZED-F9T with some firmware versions), MSM7 will be enabled instead. The MSM4 and MSM7 messages are, respectively:
   * GPS: 1074 and 1077
   * GLONASS: 1084 and 1087
   * Galileo: 1094 and 1097
   * BeiDou: 1124 and 1127
* 1230 (GLONASS code-phase bias) messages if GLONASS is enabled

These are all enabled at 1Hz.

You can alternatively use `satpulsetool gps --rtcm-out` for a bit more control. The `rtcmOutput=true` configuration in the TOML file is equivalent to `satpulsetool gps --rtcm-out auto`.

### Other receiver configuration

RTCM messages consume bandwidth, so you should enable a sufficiently high baud rate. Assuming the receiver is also being used for timing, it is important that the RTCM messages do not consume such a high percentage of the bandwidth that timing messages are significantly delayed. I therefore recommend using a high baud rate such as 115200. You can use `satpulsetool gps --speed` to configure this.

You should also think about the signals that should be enabled. Generally, more is better for RTK. You can specify the constellations that should be enabled using `satpulsetool gps --gnss`. For example, `satpulsetool gps --gnss GPS,GAL,BDS,QZSS` would enable GPS, Galileo, BeiDou and QZSS.

### Making RTCM messages available over TCP

You also need to configure satpulsed to proxy RTCM packets from the receiver over TCP. This is done by adding a section like this to `/etc/satpulse.toml`.

```
[[proxy.tcp]]
listen = ":2009"
protocol = "RTCM"
```

This says to make RTCM packets available on port 2009.

## Rover

### Using satpulsed

You can use satpulsed on the rover side also, for example to determine the fixed position of the antenna using an RTK base station.

First stop the satpulse service. Assuming your receiver is at `/dev/ttyACM0`

```
sudo systemctl stop satpulse@ttyACM0
```

### Receiver configuration for u-blox

For this experiment, we will turn off configuration by satpulsed and instead do configuration with satpulsetool.
First edit `/etc/satpulse.toml` to disable configuration by satpulsed
For example:

```
[gps]
config=false
```

Next use satpulsetool to undo any non-persistent configuration satpulsed did. Assuming your receiver had previously been using 9600 baud do:

```
satpulsetool gps -d /dev/ttyACM0 -s 9600 --reload
```

Then set the receiver to use the speed a suitably high speed, e.g. 115200

```
satpulsetool gps -d /dev/ttyACM0 -s 9600 --speed 115200
```

I also suggest enabling only RMC and GGA messages.

```
satpulsetool gps -d /dev/ttyACM0 -s 115200 --nmea-out RMC,GGA
```

### Daemon configuration

Now edit `/etc/satpulse.toml` to do the following

 * specify the serial speed that you have configured, e.g. 115200
 * make NMEA output available on a TCP port, e.g. 2007
 * give serial port access via a Unix domain socket

For example:

```
[serial]
device = "/dev/ttyACM0"
speed = 115200

[[proxy.tcp]]
listen = ":2007"
protocol = "NMEA"

[[proxy.socket]]
path = "/var/run/satpulse.sock"

```

### Verify configuration

Now you restart the satpulse service:

```
sudo systemctl start satpulse@ttyACM0
```

Now ensure you have netcat installed

```
which nc
```

If not, install openbsd-netcat:

```
sudo apt install openbsd-netcat
```

Now if you do

```
nc localhost 2007
```

you should see some NMEA sentences (including GGA and RMC).

The 6th comma-separated field of GGA sentences (where $xxGGA counts as field 0) should be 1 or 2 at this point.

### Supply RTCM corrections

For this we will need the socat package.

```
sudo apt install socat
```

First make sure you have enabled a high baud rate using `satpulsetool gps --speed 115200`.

Assuming your base is on the local network as `ptp.lan`, you can run the following command to forward the RTCM data from TCP port on the base to the rover:

```
socat UNIX-CONNECT:/var/run/satpulse.sock TCP:ptp.lan:2009
```

Here the port 2009 must match what you configured on the base. This command should just run silently until you interrupt it.

Now in another window, do

```
nc localhost 2007
```

If everything is working, then the 6th field of the GGA sentences should change to 5, indicating a floating RTK solution.
This should take less than 30 seconds assuming reasonably good conditions.
After a few minutes, the 6th field should change again to 4, indicating a fixed RTK solution.

### Extracting precise position

The GGA line now gives the precise position:
- field 2 is ddmm.mmmmm
- field 3 sign of latitude, N positive, S negative
- field 4 is the longitude ddmm.mmmmm
- field 5 is sign of longitude, E positive, W negative
- field 9 is altitude above mean sea level
- field 11 is geoid separation

To get LLH coordinates, you need to
- convert ddmm.mmmm degress/minutes into decimal degrees dd + mm.mmmmm/60
- add the altitude above mean sea level to geoid separation to get height above ellipsoid

You can then stick this into an online LLH to ECEF converter like [ConvertECEF](https://convertecef.com/) (select HAE in the altitude box).

You could then use this ECEF position as the fixed position for time mode.


## Using NTRIP

In the above example, we have used a direct, unsecured TCP connection between the base and rover.

In production, it is more common to use NTRIP. With NTRIP, there are three roles
- an server, which is connected to the receiver on the base
- a client, which is connected to the receiver on the rover
- a caster, which can relay data between multiple servers and clients

We can experiment with NTRIP by installing rtklib on the base and rover.

```
sudo apt install rtklib
```

On the base, we can run a simple NTRIP caster using the str2str program from rtklib

```
str2str -in tcpcli://localhost:2009 -out ntripc://jjc:xyzzy@:2101/bkk
```

Here
* 2009 is the TCP port on the base over which satpulsed is providing RTCM messages
* `ntripc://` says to run an NTRIP caster
* `jjc` is the username
* `xyzzy` is the password
* 2101 is the default port for NTRIP
* `bkk` is the mount point

To use NTRIP on the rover with satpulsed, you would need a writeable TCP connection
(which is not secure). Add this to `/etc/satpulse.toml`

```
[[proxy.tcp]]
listen = ":2006"
```

On the rover, we can then run a simple NTRIP client using the str2str program:

```
str2str -in ntrip://jjc:xyzzy@ptp.lan:2101/bkk -out tcpcli://localhost:2006
```

Here
* `ntrip://` says to run an NTRIP client
* `jjc` is the username to give to the caster
* `xyzzy` is the password to give to the caster
* `2101` is the port of the caster to connect to
* `ptp.lan` is the hostname of the caster
* `bkk` is the mount point of the caster to connect to
* 2006 is the port on localhost to which to send RTCM messages
