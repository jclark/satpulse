---
name: gps-msg-add
description: Add a group of related GPS configuration messages to a TOML message file
allowed-tools: Read, Glob, Grep, Bash, Write, Edit, Task, Skill
---

# GPS Message Add Skill

Add a group of related GPS configuration messages to a TOML message file.

## Required Information

Extract the following from the user's request or conversation context:

1. **Protocol documentation** - PDF, markdown, or text file describing the receiver's command protocol
2. **Target message file** - TOML file path (e.g., `configs/gpsmsg/receiver.toml`)
3. **What to add** - Description of messages to add, ranging from general ("PPS configuration") to specific ("set PPS pulse width to 100us, rising edge, only when locked")
4. **Serial device** (optional) - For testing (e.g., `/dev/ttyUSB0`)
5. **Baud rate** (optional) - For testing (e.g., `115200`)

If any required information is missing, ask the user before proceeding.

## Workflow

### Step 1: Extract Relevant Message Definitions

Spawn a subagent (Task tool with general-purpose agent) to:
- Read the protocol documentation
- Find messages related to the requested functionality
- Extract the complete message specification including:
  - Message format (NMEA, binary class/id, line command)
  - Field definitions, types, and valid values
  - Default values and examples
- Return a compact summary of just the relevant message definitions

The subagent prompt should be specific about what functionality is needed. For example:
> "Read the protocol documentation at [path] and extract all message definitions related to PPS configuration. For each message, provide: message name/ID, format type (NMEA/binary/line), all fields with their types and valid values, and any examples."

### Step 2: Design Message Functionality

Based on the extracted definitions and the user's description, determine:
- What specific messages to create
- What each message should accomplish
- What parameter values to use

If the user provided a detailed description (e.g., "100us pulse, rising edge"), use those values directly.
If the user provided a general description (e.g., "PPS configuration"), design a useful set of messages:
- Query current configuration (`get-pps`)
- Enable with sensible defaults (`pps`)
- Disable (`pps-off`)

Reference existing message files in `configs/gpsmsg/` for patterns, especially `allystar.toml` which has comprehensive examples with verification comments.

### Step 3: Choose Tag Names

Follow the tag naming conventions in [configs/gpsmsg/tags.md](../../../configs/gpsmsg/tags.md).

Key rules:
- Tags use lowercase with hyphens
- Enable/disable pairs use `-off` suffix
- Query commands use `get-` prefix
- Parametric commands include the value: `min-elev-15`, `speed-115200`

Group related messages under the same tag when they should be sent together. For example, `rtcm-msm7` enables MSM7 messages for all constellations (1077, 1087, 1097, 1127). Use `rtcm-arp` separately to enable 1005 ARP.

### Step 4: Generate TOML Entries

Generate the message entries in the correct format. See `configs/gpsmsg/gpsmsg-schema.json` for the schema.

Message types:
- `[[line]]` - Plain text commands (CR/LF terminated)
- `[[nmea]]` - NMEA-style commands (auto-adds `$` prefix and checksum)
- `[[binary]]` - Raw hex bytes
- `[[ubx]]` - u-blox UBX binary protocol
- `[[casbin]]` - CASIC binary protocol (ZKW/Zhongkewei)
- `[[asbin]]` - Allystar binary protocol

Each message can have:
- `text` or `hex` - the message content
- `tag` - grouping tag for `-t` selection
- `description` - human-readable description (only needed on first message with each tag)
- `delay` - seconds to wait after sending (useful for reload/reset commands)
- For binary protocols: `class`, `id`, `payload.types`, `payload.values`

Add a comment above each logical group explaining what it does.

### Step 5: Test the Messages (if device provided)

If serial device and speed were provided, use the gps-msg-test skill to test each new tag. Provide:
- The target message file
- The serial device
- The baud rate
- The receiver model (ask user if not already known)
- The tag to test

The test skill will:
1. Send the command and check for ACK/OK
2. Query configuration if a corresponding `get-*` tag exists
3. Verify observable effects where possible
4. Add verification comments to the TOML file

## Example: NMEA Message Control

For individual NMEA message control (e.g., RMC):

```toml
# NMEA RMC Message Control

# Verified ACK received on TAU1201
# Verified RMC messages (GNRMC/BDRMC) appear at 1Hz after enable on TAU1201
[[asbin]]
tag = "nmea-rmc"
description = "Enable NMEA RMC message at 1Hz"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF0, 0x05, 1]

# Verified ACK received on TAU1201
# Verified BDRMC messages stop after disable on TAU1201
[[asbin]]
tag = "nmea-rmc-off"
description = "Disable NMEA RMC message"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF0, 0x05, 0]
```

## Example: Constellation Selection

Per-constellation tags with all bands enabled:

```toml
# GNSS Constellation Selection (CFG-NAVSAT 0x06 0x0C)

# GPS all bands (L1 + L1C + L5 + L2C)
# Verified ACK received on TAU1201
# Verified GSV shows only GPS satellites after gnss-gps on TAU1201
[[asbin]]
tag = "gnss-gps"
description = "Enable GPS all bands (L1/L1C/L5/L2C)"
class = 0x06
id = 0x0C
payload.types = "U4"
payload.values = [0x00000701]

# Galileo all bands (E1 + E5A + E5B + E6)
# Verified ACK received on TAU1201
# Verified GSV shows only Galileo satellites after gnss-gal on TAU1201
[[asbin]]
tag = "gnss-gal"
description = "Enable Galileo all bands (E1/E5A/E5B/E6)"
class = 0x06
id = 0x0C
payload.types = "U4"
payload.values = [0x00700010]
```

## Example: Reset Commands

Distinct commands for different reset levels:

```toml
# Restart Commands (CFG-SIMPLERST 0x06 0x40)

# Hot start (fastest, keeps ephemeris)
[[asbin]]
tag = "hot-start"
description = "Hot start - keeps ephemeris data"
class = 0x06
id = 0x40
payload.types = "U1"
payload.values = [3]

# Warm start (clears ephemeris, keeps almanac)
[[asbin]]
tag = "warm-start"
description = "Warm start - clears ephemeris"
class = 0x06
id = 0x40
payload.types = "U1"
payload.values = [2]

# Cold start (clears all satellite data)
[[asbin]]
tag = "cold-start"
description = "Cold start - clears all satellite data"
class = 0x06
id = 0x40
payload.types = "U1"
payload.values = [1]
```

## Example: Reset (reload + cold-start)

The `reset` tag combines reload from NVM with clearing satellite data:

```toml
# Reset: reload config from NVM + cold start (clears satellite data)
# Verified config restored from NVM and GSV shows no el/az after reset on TAU1201
# First load config from NVM
[[asbin]]
tag = "reset"
description = "Reset - reload config and clear satellite data"
class = 0x06
id = 0x09
payload.types = "U4U4"
payload.values = [1, 0x07]

# Then cold start (mode 1 clears satellite data; mode 0 does not)
[[asbin]]
tag = "reset"
class = 0x06
id = 0x40
payload.types = "U1"
payload.values = [1]
```

## Example: Factory Reset

Factory reset clears NVM then resets:

```toml
# Factory reset
# Verified receiver restarts after factory-reset on TAU1201
# Verified GSV shows satellites without elevation/azimuth after factory-reset on TAU1201
# Verified RMC shows 'V' (void) status after factory-reset on TAU1201
[[asbin]]
tag = "factory-reset"
description = "Factory reset - clears all settings"
class = 0x06
id = 0x09
payload.types = "U4U4"
payload.values = [2, 0xFFFFFFFF]

# Then reset to apply
[[asbin]]
tag = "factory-reset"
class = 0x06
id = 0x40
payload.types = "U1"
payload.values = [0]
```

## Example: Fixed Position

Use ECEF coordinates (example from allystar.toml):

```toml
# Fixed Position Mode (CFG-FIXEDECEF 0x06 0x14)

# Poll current fixed ECEF position
# Verified CFG-FIXEDECEF response received on TAU1201
[[asbin]]
tag = "get-fixed-pos"
description = "Query current fixed ECEF position"
class = 0x06
id = 0x14

# Set fixed ECEF position (coordinates in cm as S4)
# X=-1144698.0455m, Y=6090335.4099m, Z=1504171.3914m
# Verified ACK received on TAU1201
# Verified get-fixed-pos returns expected values on TAU1201
[[asbin]]
tag = "fixed-pos-example"
description = "Set fixed ECEF position (example coordinates - replace with yours)"
class = 0x06
id = 0x14
payload.types = "I4I4I4"
payload.values = [-114469805, 609033541, 150417139]

# Clear fixed position
# Verified ACK received on TAU951M-P200
# Verified get-fixed-pos returns zeros after fixed-pos-off on TAU951M-P200
[[asbin]]
tag = "fixed-pos-off"
description = "Clear fixed position"
class = 0x06
id = 0x14
payload.types = "I4I4I4"
payload.values = [0, 0, 0]
```

## Example: RTCM Output

ARP (1005) is separate from MSM messages:

```toml
# Enable RTCM ARP (1005) message
# Verified ACK received on TAU951M-P200
[[asbin]]
tag = "rtcm-arp"
description = "Enable RTCM ARP message (1005)"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF8, 0x05, 1]

# Enable RTCM MSM7 messages for all constellations
# Rate 1 = output every position fix
# Verified ACK received for all 4 commands on TAU951M-P200
# Verified RTCM 1077, 1087, 1097, 1127 messages appear in capture after enable on TAU951M-P200
# 1077 GPS MSM7
[[asbin]]
tag = "rtcm-msm7"
description = "Enable RTCM MSM7 messages for all constellations"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF8, 0x4D, 1]

# 1087 GLONASS MSM7
[[asbin]]
tag = "rtcm-msm7"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF8, 0x57, 1]

# 1097 Galileo MSM7
[[asbin]]
tag = "rtcm-msm7"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF8, 0x61, 1]

# 1127 BeiDou MSM7
[[asbin]]
tag = "rtcm-msm7"
class = 0x06
id = 0x01
payload.types = "U1U1U1"
payload.values = [0xF8, 0x7F, 1]
```

## Safety

**Ask for explicit permission before testing commands that:**
- Change baud rate (can lose communication)
- Save to NVM/flash (persistent changes)
- Factory reset (loses all configuration)

## Notes

- Always read the target message file first to understand existing patterns
- Match the style of existing entries (comment style, ordering, etc.)
- Test one message at a time before adding complex sequences
- For binary protocols, use a subagent to carefully calculate payload bytes
- Reference `configs/gpsmsg/allystar.toml` for comprehensive examples with verification comments
