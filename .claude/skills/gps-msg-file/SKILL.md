---
name: gps-msg-file
description: Create GPS message TOML files for configuring GPS receivers via satpulsetool
allowed-tools: Read, Glob, Grep, Bash, Write, Edit, Task, Skill
---

# GPS Message File Creation Skill

Create a complete TOML message file for a GPS receiver by using the gps-msg-add skill for each functional group.

## Required Information

Extract the following from the user's request or conversation context:

1. **Protocol documentation** - PDF, markdown, or text file describing the receiver's command protocol
2. **Output file** - TOML file path to create (e.g., `configs/gpsmsg/receiver.toml`)
3. **Serial device** (optional) - For testing (e.g., `/dev/ttyUSB0`)
4. **Baud rate** (optional) - For testing (e.g., `115200`)

If any required information is missing, ask the user before proceeding.

## Workflow

### Step 1: Create the initial file

Create the message file with a header:
```toml
#:schema ./gpsmsg-schema.json
# [Receiver model] configuration messages
# [Manufacturer] [Model] GNSS receiver
# Default baud rate: [speed]
```

### Step 2: Add message groups

Use `/gps-msg-add` to add each functional group, one at a time. If a device is provided, `/gps-msg-add` will invoke `/gps-msg-test` to verify each group and add verification comments.

Groups are ordered by testability - start with groups where effects are easy to observe and verify automatically.

## Message Groups (Ordered by Testability)

### 1. Version/Probe (`get-version`)
Query firmware version - useful as a connectivity test. Do this first to verify basic communication works.

### 2. NMEA Output Control (individual message tags)
NMEA message output control with individual tags for each sentence type:
- `nmea-gga`, `nmea-gga-off` - Fix data (position, quality, satellites)
- `nmea-gsa`, `nmea-gsa-off` - Active satellites and DOP
- `nmea-gsv`, `nmea-gsv-off` - Satellites in view
- `nmea-rmc`, `nmea-rmc-off` - Recommended minimum (time, position, velocity)
- `nmea-zda`, `nmea-zda-off` - Time and date
- `nmea-daemon` - Convenience tag enabling RMC, GGA, GSV, GSA (messages used by satpulse daemon)

Easy to verify by capturing before/after. Individual tags allow granular control.

### 3. Reload (`reload`)
Reload configuration from NVM - restores saved settings. Safe to test (doesn't change persistent state) and useful for verifying other commands work. Should include `delay = 3` to wait for reload to complete.

### 4. Fix Rate (`fix-rate-*`, `get-fix-rate`)
Fix rate (navigation solutions per second) configuration:
- `get-fix-rate` - Query current fix rate
- `fix-rate-1` through `fix-rate-20` (1, 2, 5, 10, 20 Hz)

Add if the receiver supports configuring the fix interval. Message output rate is often specified as a multiple of the fix rate (one message every N fixes), so the fix rate determines the actual message rate.

**Skip if:** Receiver has no configurable fix rate

### 5. Binary Output Control (individual message tags)
Binary message output control with individual tags:
- Protocol-specific message names (e.g., `asbin-navtime`, `asbin-timeutc`, `asbin-svinfo`, `asbin-svin`, `ubx-navpvt`)
- Each message gets its own enable/disable tag pair

### 6. NMEA Version (`nmea-ver-*`, `get-nmea-ver`)
NMEA protocol version configuration:
- `get-nmea-ver` - Query current version
- `nmea-ver-3` - Set to NMEA 3.01
- `nmea-ver-400` - Set to NMEA 4.00
- `nmea-ver-410` - Set to NMEA 4.10 (adds signal ID to GSV)
- `nmea-ver-411` - Set to NMEA 4.11 (changes talker IDs: GB for BeiDou, GQ for QZSS)

Verifiable by observing talker ID changes (BD vs GB) and GSV signal ID field.

### 7. Elevation Mask (`min-elev-*`, `get-min-elev`)
Minimum satellite elevation configuration:
- `get-min-elev` - Query current elevation mask
- `min-elev-0` through `min-elev-45` in 5-degree increments

Verifiable by cross-referencing GSA (satellites used) with GSV (satellite elevations).

### 8. Constellation Selection (`gnss-*`, `get-gnss`)
GNSS constellation configuration with per-constellation and combination tags:
- `get-gnss` - Query current constellation settings
- `gnss-gps` - Enable GPS only (with all available bands)
- `gnss-gal` - Enable Galileo only
- `gnss-glo` - Enable GLONASS only
- `gnss-bds` - Enable BeiDou only
- `gnss-gps-gal` - Enable GPS and Galileo
- `gnss-all` - Enable all constellations

Verifiable via GSV message prefixes (GPGSV, GAGSV, GLGSV, GBGSV).

### 9. PPS Configuration (`pps`, `pps-off`, `get-pps`)
PPS configuration:
- `get-pps` - Query current PPS configuration
- `pps` - Enable with sensible defaults (100us pulse width, rising edge, only when locked)
- `pps-off` - Disable PPS output

Observable effect requires oscilloscope or PHC with external timestamp input.

### 10. Reset Commands
Restart commands with different levels of data clearing:
- `hot-start` - Keeps ephemeris data (fastest restart)
- `warm-start` - Clears ephemeris, keeps almanac
- `cold-start` - Clears all satellite data
- `reset` - Reload config from NVM AND clear satellite data (combines reload + cold-start semantics)

Verifiable by checking RMC status (V=void after reset) and GSV (missing elevation/azimuth data after cold start).

### 11. Survey-in (`survey`, `survey-off`, `get-survey`)
Survey-in configuration for base station positioning:
- `get-survey` - Query current survey configuration
- `survey` - Start survey with sensible defaults (2000 seconds, 20m accuracy)
- `survey-off` - Stop survey / return to mobile mode (sets parameters to 0)

If the protocol provides a periodic survey status message, add:
- `asbin-svin` or similar - Enable survey status message at 1Hz

**Skip if:** Receiver doesn't support survey-in mode

### 12. Fixed Position (`fixed-pos-example`, `fixed-pos-off`, `get-fixed-pos`)
Fixed ECEF position for base station:
- `get-fixed-pos` - Query current fixed position
- `fixed-pos-example` - Set fixed ECEF position (example coordinates - replace with yours)
- `fixed-pos-off` - Clear fixed position

**Skip if:** Receiver doesn't support fixed position mode

### 13. RTCM Output (`rtcm-arp`, `rtcm-msm4`, `rtcm-msm7`, `rtcm-off`)
RTCM message output for base station:
- `rtcm-arp` - Enable ARP message (1005)
- `rtcm-msm4` - Enable MSM4 messages for all constellations (1074/1084/1094/1124)
- `rtcm-msm7` - Enable MSM7 messages for all constellations (1077/1087/1097/1127)
- `rtcm-off` - Disable all RTCM messages

**Skip if:** Receiver doesn't support RTCM output

### 14. Port Configuration (`get-uart0`, `get-uart1`, `speed-*`)
Serial port configuration:
- `get-uart0`, `get-uart1` - Query port configuration
- `speed-9600`, `speed-19200`, `speed-38400`, `speed-57600`, `speed-115200`, `speed-230400`, `speed-460800`

Verifiable on real UARTs (/dev/ttyUSBx) by reconnecting at new speed.

**Notes:**
- Some protocols require specifying which port to configure
- Test speeds at or above 38400 unless that is below the receiver's default
- Place after survey-in since baud rate changes can lose communication

### 15. Save (`save`) - requires user consent
Save current configuration to NVM. Only test if user consents - modifies persistent state.

### 16. Factory Reset (`factory-reset`) - requires user consent
Reset receiver to factory defaults AND clear satellite data. Only test if user consents - loses all configuration.

Verifiable by checking that config reverts to defaults and RMC shows void status.

## Using gps-msg-add

For each group, use the gps-msg-add skill, providing:
- The protocol documentation file
- The target message file
- A description of the message group (e.g., "NMEA output control - individual RMC, GGA, GSV messages")
- Serial device and baud rate (if testing)

The gps-msg-add skill will:
1. Extract relevant message definitions from the protocol doc
2. Generate TOML entries with correct tags
3. If device is provided, use gps-msg-test for verification

## Reference

### Existing message files
Look at existing files in `configs/gpsmsg/` for patterns:
- `allystar.toml` - Allystar binary protocol (comprehensive example with verification comments)
- `lg290p.toml` - Quectel (NMEA proprietary sentences)
- `atgm332d-f8.toml` - ZKW CASIC binary protocol

### Tag naming conventions

See [configs/gpsmsg/tags.md](../../../configs/gpsmsg/tags.md) for the complete tag reference.

Key rules:
- Tags use lowercase with hyphens
- Enable/disable pairs use `-off` suffix
- Query commands use `get-` prefix
- Parametric commands include the value: `min-elev-15`, `speed-115200`
