---
title: Precisely determining antenna position
---

The normal mode of operation of a GNSS receiver is to simultaneously solve for position, velocity and time using at least four satellites. GNSS receivers designed specifically for timing applications also offer a timing mode, where the position is fixed, the velocity is zero and the receiver solves only for time, which it can do using a single satellite. High-precision receivers designed for RTK such as the u-blox ZED-F9P also offer a timing mode.

In order to operate in timing mode, the position of the receiver must first be determined. Timing receivers typically offer a survey-in feature, where the receiver determines the position by averaging the position solutions over some user-configurable period. The SatPulse daemon will use this by default.

This is convenient, but there is a more precise way to determine the antenna position, which can lead to improved performance. This involves collecting raw observation data from the receiver for a number of hours and then submitting this data to an online Precise Point Positioning (PPP) service. The online service has access to additional data about the satellite orbits and clocks, which enable it do produce a much more precise position. Final data about the satellite orbits at a particular time only becomes available about 2 weeks after that time. So for the best possible results it is necessary to wait for 2 weeks after the data is collected before submitting it to the service. But good results can also be obtained using a more rapid service using data that becomes available after about 2 days.

Online PPP services typically expect the data to be in [RINEX](https://igs.org/wg/rinex/) format.
This document explains how to use SatPulse to collect the data, convert it to RINEX format.
It also explains how to use one particular PPP service, the CSRS-PPP service operated by Canadian Geodetic Survey of Natural Resources Canada. Although this service is operated by the Canadian Government, it works for data collected anywhere in the world.

This guide is written for u-blox receivers.
A timing or high-precision module (names have a T or P suffix, like LEA-M8T or ZED-F9P) is needed because only they have a timing mode and support raw output.

## Creating RINEX observation file for submission

Install rtklib
```
sudo apt install rtklib
```

Stop the satpulse service if it is running
```
sudo systemctl stop satpulse@ttyAMA0
```

Set speed to 115200 (assuming current speed is 9600) persistently
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --speed 115200 --save
```

CSRS-PPP supports GPS (L1 and L2), GLONASS (L1 and L2) and Galileo (L1 and L5).
So enable GPS, Galileo and GLONASS (with time pulse aligned to GPS)

```
satpulsetool gps -d /dev/ttyAMA0 -s 115200 --gnss GPS,GAL,GLO --time-gnss GPS
```

In `/etc/satpulse.toml`, change the speed accordingly

```
[serial]
device = "/dev/ttyAMA0"
speed = 115200
```

add a read-only TCP port that makes the UBX packets available

```
[[proxy.tcp]]
listen = ":2008"
protocol = "UBX"
```

also a socket that `satpulsetool gps` can use:

```
[[proxy.socket]]
path = "/var/run/satpulse.sock"
```

Start the satpulse service again

```
sudo systemctl start satpulse@ttyAMA0
```

Now enable raw output of observations using the socket

```
satpulsetool gps --socket /var/run/satpulse.sock --raw-out obs
```

(satpulsetool sets the permissions of /var/run/satpulse.sock to be in the dialout group, same as tty devices. So assuming the user is in the dialout group, this works without sudo.)

The next command needs to run for a bit, so it's a good idea to run it under tmux or screen.

Collect the UBX data, using str2str from rtklib

```
str2str -in tcpcli://localhost:2008 -out YYYYMMDD.ubx
```

Here 2008  matches the number specified for the listen property in the `[[proxy.tcp]]` section. Use an extension of `.ubx`. YYYYMMDD is the date of collecting the data.

Interrupt with Ctrl-C when you have enough data (4 hours).

Turn off raw output
```
satpulsetool gps --socket /var/run/satpulse.sock --raw-out none
```

Now convert to RINEX using `satpulsetool convobs` {% include new-in-03.html %}

```
satpulsetool convobs -r ubx -o YYYYMMDD.obs YYYYMMDD.ubx
```

See the [satpulsetool-convobs(1)]({%link man/satpulsetool-convobs.1.md %}) man page for full details.

Alternatively, you can convert using convbin from RTKLIB

```
convbin -od -os YYYYMMDD.ubx -o YYYYMMDD.obs
```

(Note that `convbin --help` is wrong in that `-od -os` options are not on by default).

Compress it:

```
gzip YYYYMMDD.obs
```

## Submitting the RINEX file to CSRS-PPP

Register for the CSRS-PPP service at https://webapp.csrs-scrs.nrcan-rncan.gc.ca/geod/tools-outils/ppp.php

You now have to wait for a period of time depending on how good you want the results to be.
* Ultra-rapid service gives results 90 minutes after the hour
* Rapid service gives results 17-18 hours after the end of the day
* Final service gives results 12-15 days after the end of the week

After waiting, log in to CSRS-PPP page. Select Static and ITRF under processing mode and upload the compressed RINEX file YYYYMMDD.obs.gz.

## Using the results

CSRS will email back the results as a zip file. Unzip the file. The .sum file contains a machine-readable summary of the results, including ECEF coordinates and 95% sigma. Use awk to print out lines to add to `/etc/satpulse.toml`

```
awk '$1 == "POS" && $2 ~ /^[XYZ]$/ { coord[++n] = $6; sigma[n] = $8 }
END { printf "fixedPosECEF=[%s,%s,%s]\nfixedPosAcc=%.4f\n", coord[1], coord[2], coord[3], sqrt(sigma[1]^2 + sigma[2]^2 + sigma[3]^2) }' YYYYMMDD.sum
```

This will output something like:

```
fixedPosECEF=[-1144567.4109,6091234.9865,1504567.9101]
fixedPosAcc=0.0180
```

You can then paste that into the `[gps]` section of `/etc/satpulse.toml`.
However, I suggest first adding 0.02 or 0.03 to fixedPosAcc to account for the potential discrepancy between WGS84 (used by GNSS) and ITRF (used by CSRS). See this [paper](https://navi.ion.org/content/72/2/navi.693) for more details.
Then restart the satpulse service.

It's worth noting that the position will change slightly over time, because of [tectonic plate](https://www.usgs.gov/media/images/tectonic-plates-earth) movement. You can estimate this by going to the [ITRF web site](https://itrf.ign.fr/). Use the map to find a station near to you. Then click on the station. Then do *Add point to selection*. The click on *Get ITRF coordinates of these points* and select *Full kinematic models* as the type of coordinates. The X, Y and Z velocity values give an estimate how the ECEF coordinates will change in a year. For me (in Bangkok, on the Eurasian plate), it's [-0.0245,-0.0059,-0.0102], which is a drift of about 2.7cm a year. Other tectonic plates, notably the Pacific plate, move much faster (7-11 cm/year). This level of precision probably isn't going to make a difference for a PTP timing application, but it could make a difference for RTK.
