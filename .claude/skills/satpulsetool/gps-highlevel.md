# satpulsetool gps: high-level configuration

Device-independent options that satpulsetool translates into the right
commands for the receiver. Requires a receiver whose `--show-receiver` output
has a `Supports:` line (see `gps.md`). Full option reference:
satpulsetool-gps(1). All examples assume `-d DEV -s SPEED` or `-f FILE`.

Show current configuration:

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --show-config
```

Enable constellations and bands:

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 -g GPS,GAL -b L1,L2
```

Configure PPS (pulse width in seconds, 0 disables):

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --pps 0.5
```

Start a position survey (for timing mode):

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --survey --survey-time 3000 --survey-acc 1.5
```

Use a known fixed position:

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --fixed-pos-ecef -941709.7,5965766.5,1553280.3 --fixed-pos-acc 0.1
```

Switch the receiver from NMEA to its binary protocol (what satpulsed
wants), and choose which message groups it sends with `--pvt-out`,
`--sats-out`, `--raw-out`, and `--nmea-out` (values in satpulsetool-gps(1)):

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary
satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out pos,time,tp --sats-out sat
```

Configure RTCM output:

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --rtcm-out MSM4,ARP -g GPS,GAL,BDS
```

Change the receiver's serial speed (the host port follows):

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --speed 115200
```

`--json` writes the result as one JSON object with a `config` field for the
properties set or queried.

## Non-volatile memory and resets

Confirm with the user before any of these (see SKILL.md).

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --save
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
satpulsetool gps -d /dev/ttyACM0 -s 38400 --reset
satpulsetool gps -d /dev/ttyACM0 -s 38400 --factory-reset
```

`--save` combined with configuration changes saves what this command changed;
`--save-all` saves the whole running configuration. `--reload` restores the
saved configuration, discarding unsaved changes (including any made by
satpulsed). `--reset` also discards position, time, and orbital data.
`--factory-reset` restores non-volatile memory to defaults and then resets.
