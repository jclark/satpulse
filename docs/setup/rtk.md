---
title: RTK setup
---

The [Precision positioning]({% link intro/positioning.md %}#real-time-kinematic) page explains the concepts behind RTK.
This page shows how to configure SatPulse for the base side and the rover side.

## RTK base

Setting up an RTK base requires:

- configuring the receiver, and
- configuring a way to get RTCM messages from the base to the rover.

satpulsed can deliver the RTCM messages to the rover in three ways:
as an Ntrip caster, as an Ntrip server pushing to a remote caster, or as a plain TCP server.

### Receiver configuration

The receiver needs some configuration to act as a base.
Two things are mandatory: a stationary position, either surveyed or explicitly specified, and RTCM output.

If the receiver supports high-level configuration (see [GPS module support]({% link gps-module-support/index.md %})),
both can be done entirely with the `[gps]` table of `/etc/satpulse.toml`:

```
[gps]
config = true
rtcmOutput = true
```

`config = true` is a normal part of any satpulsed setup;
the RTK-specific addition is `rtcmOutput = true`,
which enables a sensible set of RTCM messages (ARP and MSM).
Leave `mobile` out (or set it to false) and the receiver will assume a stationary position,
performing a survey to establish it.
This is enough for a working base.
If high-level configuration is not available for your receiver,
you need to use low-level configuration with a message file, described below;
message files also allow configuration of details that high-level configuration cannot express.

The `[gps]` table allows some additional configuration.
The most important is an explicit fixed position, specified with `fixedPosECEF` or `fixedPosLLH`:
the rover's position will only be as accurate as the base's position,
so for a permanent base it is worth [determining the position precisely]({% link howtos/precise-position.md %})
rather than relying on a survey.
The `surveyTime` and `surveyAcc` keys control the survey.
The RTCM reference station ID can be configured with `rtcmBaseID`.

Two important settings are potentially disruptive and so need to be made with `satpulsetool gps`.
RTCM messages consume bandwidth, so you should enable a sufficiently high baud rate;
if the receiver is also being used for timing, it is important that the RTCM messages do not delay the timing messages.
I recommend a high baud rate such as 115200, which you can configure with `satpulsetool gps --speed`.
Generally, more constellations are better for RTK; you can enable them with `satpulsetool gps --gnss`.

Low-level configuration is done by using 
`satpulsetool gps -m` to send messages selected by tag from a TOML message file.
The message files shipped with SatPulse in `/usr/share/satpulse/gpsmsg` use standard tag names.
The ones relevant to a base are:

* `survey` - start a survey
* `fixed-pos-example` - set an explicit fixed position (edit in your own coordinates)
* `rtcm-arp`, `rtcm-msm4`, `rtcm-msm7` - enable RTCM output
* `mode-base` - put the receiver into base mode, on receivers that need it

Use `--show-tags` to see what a particular file provides.
For example, to enable RTCM output on a Quectel LG290P:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m /usr/share/satpulse/gpsmsg/quectel/lg290p.toml -t rtcm-arp,rtcm-msm4
```

### Ntrip caster

satpulsed includes a native Ntrip caster. {% include new-in-03.html %}
Unlike a normal Ntrip caster, this gets corrections direct from the receiver, rather than from an Ntrip server.
The minimum configuration needed for a caster is to specify a mountpoint name.

```
[[ntrip.mountpoint]]
name = "BKK"
```

The caster listens on the default Ntrip port 2101.
It constructs a source table with sensible defaults;
the one field worth setting explicitly is the country code:

```
[ntrip]
country = "THA"
```

The full set of keys available in the `[ntrip]` table are documented in [satpulse.toml(5)]({% link man/satpulse.toml.5.md %}).

By default no authentication is required.
To require it, define one or more users and specify that the mountpoint should use authentication:

```
[[user]]
name = "rover"
password = "secret"

[[ntrip.mountpoint]]
name = "BKK"
auth.anyUser = true
```

The mountpoint can alternatively be restricted to a specific set of users:

```
[[ntrip.mountpoint]]
name = "BKK"
auth.users = ["rover"]
```

Note that this means that the passwords are in plain text in the configuration file,
which is not a good practice if a high level of security is required.
You might want to set the permissions of `satpulse.toml` to restrict who can read it.

For RTK itself, MSM4 messages are usually sufficient, though I have found that sometimes MSM7 is needed.
MSM4 messages are smaller, but MSM7 messages are more precise and can be [converted to RINEX]({% link howtos/precise-position.md %}) for use with PPP.
The `msm7to4` key lets you have both:
configure the receiver to emit MSM7, and serve a second mountpoint on which MSM7 packets are converted to MSM4:

```
[[ntrip.mountpoint]]
name = "BKK"

[[ntrip.mountpoint]]
name = "BKK4"
msm7to4 = true
```

When any mountpoint has `msm7to4` enabled, `rtcmOutput` prefers MSM7 automatically.

You can check that the caster is working from another machine using satpulsetool's Ntrip client:

```
satpulsetool ntrip base.lan BKK
```

This writes the packets it receives as a JSONL packet log, so you can see the RTCM message types the base is producing.
Add `--user rover:secret` if the mountpoint requires authentication.

### Ntrip server

satpulsed can also act as an Ntrip server, where it pushes corrections to a separate Ntrip caster. {% include new-in-03.html %}
This can either be an Ntrip caster you operate yourself (such as the [BKG Ntrip caster]({% link intro/other-software.md %}#bkg-ntrip-caster)),
or a public caster network such as [RTK2go](http://rtk2go.com/). 

```
[[stream.push]]
ntrip.address = "rtk2go.com"
ntrip.mountpoint = "BKK"
ntrip.password = "secret"
```

MSM7-to-MSM4 conversion is available here too, as `ntrip.msm7to4`.

The source table pushed to the server is controlled by the `[ntrip]` table as with the Ntrip caster. 

### TCP server

The simplest arrangement, with no Ntrip involved, is to make the RTCM packets available on a TCP port:

```
[[proxy.tcp]]
listen = ":2009"
protocol = "RTCM"
```

There is no access control or encryption, so this is suitable only on a trusted network.

## RTK rover

SatPulse is designed for hardware RTK, where the RTK position solution is computed by the rover's receiver.
On the rover side, the job of satpulsed is to feed corrections to the receiver and monitor the solution status.

The `[stream.pull]` table configures a network endpoint as the source of correction data;
the data is scanned as RTCM and sent to the receiver over the configured serial connection. {% include new-in-03.html %}

### Ntrip client

With an Ntrip endpoint, satpulsed acts as an Ntrip client, receiving from an Ntrip caster:

```
[stream.pull]
ntrip.address = "base.lan"
ntrip.mountpoint = "BKK"
ntrip.username = "rover"
ntrip.password = "secret"
```

The username and password keys are needed only if the mountpoint requires authentication.

Many large-scale Ntrip services do not expose a separate mountpoint for each base station;
rather they expect the client to send its location as an NMEA GGA sentence;
they then use that location to either select a specific base station or synthesize RTCM corrections from other representations.
This kind of Ntrip caster is called a VRS (Virtual Reference Station).
An example is u-blox PointPerfect's Ntrip service.
To enable this, specify `ntrip.nmeaSend = true`.

```
[stream.pull]
ntrip.address = "caster.example.com"
ntrip.mountpoint = "AUTO"
ntrip.username = "rover"
ntrip.password = "secret"
ntrip.nmeaSend = true
```

This makes satpulsed send the receiver's current position after connecting and periodically thereafter.
The default is to send every 5 seconds;
you can change this with `ntrip.nmeaSendInterval`, which gives the interval in seconds;
0 means upload once per connection.

You can check whether a caster is working by using `satpulsetool ntrip`.
It connects to a mountpoint
and writes the packets it receives to standard output as a JSONL packet log.
The `--nmea-send-pos` and `--nmea-send-interval` options have a similar effect to the corresponding configuration keys:

```
satpulsetool ntrip --user rover:secret --nmea-send-pos 13.7563,100.5018 caster.example.com AUTO
```

### TCP client

With a TCP endpoint, satpulsed acts as a TCP client, receiving from a TCP server:

```
[stream.pull]
tcp.address = "base.lan:2009"
```

The address here matches the `proxy.tcp` configuration on the base shown above.

### Verifying the solution

To see whether the rover has an RTK solution, use the web GUI of satpulsed's [HTTP monitoring interface]({% link setup/monitor.md %}#http-monitoring-interface).
Enable it on the rover by adding an `[[http]]` table:

```
[[http]]
listen = ":2000"
```

The GPS status on the dashboard shows the fix type and which corrections are applied.
Without corrections, the fix type is normally `code`.
Once corrections are flowing, the fix type should change to `carrierFloat` in less than 30 seconds.
If it achieves an RTK fixed solution, which may take several minutes,
the fix type will change to `carrierFixed`.
RTK fixes, particularly `carrierFixed`, need a good sky view:
a sky view that is sufficient for a `code` fix may well be inadequate for an RTK fix.

The dashboard also has an RTCM card, which shows what corrections are flowing to the receiver.
This is fed by satpulsed parsing the correction messages it pulls.

With a u-blox receiver, it is also useful to enable the UBX-RXM-COR message,
which the receiver emits when it processes a correction message.
The RTCM card can then show which messages the receiver is actually making use of.
satpulsed understands this message but does not enable it,
so it needs low-level configuration with a [message file]({% link man/satpulsetool-gps.1.md %});
the message files shipped with SatPulse include a tag for it.
Assuming the receiver is connected by USB:

```
satpulsetool gps -d /dev/ttyACM0 -s 115200 -m /usr/share/satpulse/gpsmsg/u-blox/gen9.toml -t ubx-rxm-cor --port usb --save
```

The `--save` makes the setting persist across a receiver power cycle.
