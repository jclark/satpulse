---
title: GPS receiver configuration
---

This page explains how to configure your GPS receiver using the `satpulsetool gps` command. For complete reference documentation, see the [satpulsetool-gps(1)]({% link man/satpulsetool-gps.1.md %}) man page.

## When to configure the GPS

Most modern GPS receivers work well for timing applications with their default settings. However, you may want to configure your GPS to:

- Optimise for stationary timing use
- Select specific GNSS constellations
- Adjust the PPS pulse width
- Save power by disabling unnecessary outputs
- Perform a survey-in for precise position determination

## Quick configuration for SatPulse

For most users, the simplest approach is to let SatPulse configure the GPS automatically by enabling `config = true` in the `[gps]` section of satpulse.toml. This will configure the GPS optimally for timing use.

If you prefer to configure manually or need specific settings, use satpulsetool gps as described below.

## Basic usage

First, identify your serial device and speed (see [Serial connection setup]({% link setup/gps-serial.md %})).

Show the current GPS configuration:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 -c
```

## Common configuration scenarios

### Configure for timing daemon use

Enable the messages needed by satpulsed:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --pvt-out daemon
```

This enables time pulse messages, leap second information, and other data required by the daemon.

### Survey-in for stationary receivers

If your antenna is stationary, you can improve accuracy by performing a survey-in:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --survey --survey-time 3600 --survey-acc 2.0
```

This performs a 1-hour survey with 2-metre accuracy requirement. The GPS will determine its precise position and then use this fixed position for all subsequent timing calculations.

### Select GNSS constellations

Enable specific satellite systems:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --gnss GPS,GAL,BDS
```

Common constellations:
- **GPS** - US Global Positioning System (most compatible)
- **GAL** - European Galileo (good accuracy)
- **BDS** - Chinese BeiDou (good in Asia-Pacific)
- **GLO** - Russian GLONASS (global coverage)

Using multiple constellations improves availability but may slightly reduce timing precision due to inter-system biases.

### Configure PPS output

Set the pulse width (in seconds):
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --pps 0.1
```

Common values:
- 0.1 (100ms) - Default for most receivers
- 0.01 (10ms) - Shorter pulse to reduce power
- 0.0001 (100µs) - Very short pulse for fast sampling

### Save configuration

Save your changes to the GPS receiver's non-volatile memory:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --save
```

Without saving, changes are lost when the GPS powers off.

## Advanced configuration

### Fixed position mode

If you know your antenna's precise coordinates (e.g., from a professional survey):
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --fixed-pos-ecef 4033638.3,752670.5,4900818.0 --fixed-pos-acc 0.5
```

This uses Earth-Centred Earth-Fixed (ECEF) coordinates in metres.

### Change serial speed

Configure the GPS to use a different baud rate:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --speed 115200 --save
```

After this, you'll need to use `-s 115200` for future connections.

### Enable specific frequency bands

For dual-frequency receivers:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --band L1,L5
```

### Antenna cable delay compensation

Compensate for signal delay in long antenna cables:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --ant-cable-delay 50
```

The delay is specified in nanoseconds (approximately 5ns per metre of cable).

## Troubleshooting

### Reset to defaults

If configuration causes problems:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --factory-reset
```

This restores all factory defaults.

### Reload saved configuration

Undo unsaved changes:
```
satpulsetool gps -d /dev/ttyAMA0 -s 9600 --reload
```

### Connection via SatPulse proxy

If SatPulse is running with a proxy socket configured:
```
satpulsetool gps --socket /var/run/satpulse.sock -c
```

## Receiver-specific notes

Different GPS receivers have varying capabilities:

- **u-blox** - Full support for all features
- **Quectel** - Most features supported
- **SkyTraq** - Basic configuration only
- **Generic NMEA** - Limited to standard NMEA commands

Check your receiver's documentation for specific capabilities.

## See also

- [satpulsetool-gps(1)]({% link man/satpulsetool-gps.1.md %}) - Complete command reference
- [Serial connection setup]({% link setup/gnss-serial.md %}) - Identifying serial devices
- [SatPulse configuration guide]({% link setup/satpulse-config.md %}) - Configuring satpulse.toml