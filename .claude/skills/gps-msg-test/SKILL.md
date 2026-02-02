---
name: gps-msg-test
description: Test GPS messages from a TOML file using satpulsetool
allowed-tools: Read, Bash, Glob, Grep, Edit
---

# GPS Message Test Skill

Test GPS configuration messages and report verification results. This skill runs as an agent and returns a verification report.

## Required Information

Extract the following from the user's request or conversation context:

1. **Message file** - TOML message file path (e.g., `configs/gpsmsg/allystar.toml`)
2. **Serial device** - Device path (e.g., `/dev/ttyUSB0`)
3. **Baud rate** - Speed (e.g., `115200`)
4. **Receiver model** - Model name for verification comments (e.g., `TAU1201`)
5. **Tag(s) to test** - Which tag(s) to test (e.g., `pps`, `nmea-rmc`)
6. **Add comments** (optional) - Whether to add verification comments to the TOML file (default: yes)

If any required information is missing, ask the user before proceeding.

## Setup

Ensure the tool is built:
```bash
make
```

Determine architecture and set ARCH variable:
```bash
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
```

The satpulsetool binary is at: `out/$ARCH/satpulsetool`

## CRITICAL: Workflow for Each Tag

**You MUST follow this workflow for EACH tag tested:**

1. **Test** - Send the command and verify response
2. **Add comment** - IMMEDIATELY add verification comment to the TOML file
3. **Report** - Return results to user

**DO NOT** test multiple tags and then add comments later. Add the comment right after each successful test.

## Verification Levels

### Level 1: Command Accepted

Check satpulsetool output for acknowledgment:
- **NMEA protocols:** Look for `OK` in response (e.g., `$PQTMCFGPPS,OK*xx`)
- **Binary protocols:** Look for ACK packet in output
- **Line protocols:** No error message, or explicit OK response

### Level 2: Configuration Persisted

After setting a value, query it back and confirm it matches.

1. Check if a corresponding `get-*` tag exists:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE --show-tags
   ```

2. If `get-$TAG` exists (e.g., `get-pps` for `pps`), query and verify:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t get-$TAG --packet-log get.jsonl
   ```

3. Parse the response and compare to expected values:
   - **NMEA:** Parse the comma-separated fields directly from the ASCII response
   - **Binary protocols:** Use the message definition to parse the response:
     1. Look up the original set command in the message file to find `payload.types`
     2. Binary packets have fixed-length headers and trailing checksums - the payload is between them
     3. Use the `payload.types` field to decode the payload (e.g., `U4I4U1` means: unsigned 4-byte, signed 4-byte, unsigned 1-byte)
     4. All multi-byte values use little-endian encoding
     5. Type codes: `U1`/`U2`/`U4` = unsigned 1/2/4 bytes, `I1`/`I2`/`I4` = signed, `X1`/`X2`/`X4` = hex/bitfield
     6. Compare decoded values against `payload.values` from the set command

### Level 3: Observable Effect

The configuration change produces a measurable effect. See verification strategy references below for details on each message type.

## Verification Procedure

For each tag, follow these steps IN ORDER:

1. **Send the command** and check for ACK:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG --packet-log test.jsonl
   ```

2. **Query configuration** if a get command exists (Level 2)

3. **Verify observable effect** using the appropriate strategy (Level 3)

4. **IMMEDIATELY add verification comments** to the TOML file (unless --no-comment specified)
   - Do this RIGHT AFTER testing each tag
   - Do NOT batch up multiple tests before adding comments

5. **Report results** to user before moving to next tag

## Verification Strategy References

Choose the appropriate strategy based on the tag being tested:

| Tag Pattern | Strategy | Reference |
|-------------|----------|-----------|
| `nmea-ver-*` | NMEA version | [verification/nmea-version.md](verification/nmea-version.md) |
| `nmea-*`, `binary-*`, `rtcm-*` | Message enable/disable | [verification/message-enable-disable.md](verification/message-enable-disable.md) |
| `gnss-*` | Constellation selection | [verification/constellation-selection.md](verification/constellation-selection.md) |
| `pps`, `pps-off` | PPS configuration | [verification/pps.md](verification/pps.md) |
| `reload` | Reload configuration | [verification/reload.md](verification/reload.md) |
| `save` | Save configuration | [verification/save.md](verification/save.md) |
| `reset` | Reset/reboot | [verification/reset.md](verification/reset.md) |
| `factory-reset` | Factory reset | [verification/factory-reset.md](verification/factory-reset.md) |
| `survey` | Survey-in | [verification/survey.md](verification/survey.md) |
| `speed-*` | Baud rate changes | [verification/speed.md](verification/speed.md) |

## Reading Packet Logs

Packet logs are JSONL with one JSON object per line. Key fields:
- `t` - timestamp (ISO 8601)
- `tag` - protocol type: `NMEA`, `ASBIN`, `UBX`, `CASBIN`, `RTCM`, etc.
- `msg` - specific message type: `GPGGA`, `GPRMC`, `MON-VER`, etc.
- `out` - direction: `true` = sent to receiver, `false` = received from receiver
- `ascii` - for NMEA: the raw sentence
- `bin` - for binary: hex-encoded payload

## Adding Verification Comments

After testing, add verification comments to the TOML file (unless `--no-comment` was specified). Find the `[[messages]]` section for the tested tag and add comments directly above it.

### Comment Format

Use one line per verification performed, always including the receiver model:

**Successful verification:**
```toml
# Verified ACK received on TAU1201
# Verified RMC sentences appear in capture after enable on TAU1201
[[messages]]
tags = ["nmea-rmc"]
```

**Query verification (Level 2):**
```toml
# Verified ACK received on TAU1201
# Verified get-pps returns expected values on TAU1201
[[messages]]
tags = ["pps"]
```

**Observable effect verification (Level 3):**
```toml
# Verified ACK received on TAU1201
# Verified GPRMC messages appear in capture after enable on TAU1201
[[messages]]
tags = ["nmea-rmc"]
```

**Feature not supported:**
```toml
# NAK received on TAU1201 - survey mode not supported on this receiver
[[messages]]
tags = ["survey"]
```

**Verification requires manual steps:**
```toml
# Verified ACK received on TAU1201
# Note: PPS output effect requires manual verification with oscilloscope
[[messages]]
tags = ["pps"]
```

### Common Verification Phrases

Choose phrases based on what was verified:

| Verification Type | Comment Format |
|------------------|----------------|
| ACK received | `Verified ACK received on [MODEL]` |
| Message appears | `Verified [MSG] messages appear in capture after enable on [MODEL]` |
| Message stops | `Verified [MSG] messages no longer appear after disable on [MODEL]` |
| Query matches | `Verified get-[TAG] returns expected values on [MODEL]` |
| Config persists | `Verified save persists configuration across reload on [MODEL]` |
| Config restores | `Verified reload restores saved configuration on [MODEL]` |
| Reset works | `Verified reset causes receiver to restart on [MODEL]` |
| Survey starts | `Verified survey starts and status shows in progress on [MODEL]` |
| Constellation | `Verified GSV shows only [CONST] satellites after [TAG] on [MODEL]` |
| Baud change (UART) | `Verified communication works at new baud rate on [MODEL]` |
| Baud change (USB) | `ACK received but baud rate change cannot be verified on USB CDC device` |
| Not supported | `NAK received on [MODEL] - [feature] not supported on this receiver` |
| Manual needed | `Note: [feature] requires manual verification with [equipment]` |

## Report Format

Return a verification report:

```
Receiver: [MODEL]

Verification comments for TOML:
# Verified ACK received on [MODEL]
# Verified [specific verification] on [MODEL]

[Any notes about manual verification needed]
```

## Safety

**Ask user before testing:**
- `save` - modifies persistent state
- `factory-reset` - loses all configuration
- `speed-*` below 38400 - may cause bandwidth issues

## Troubleshooting

### No response from receiver
1. Verify device exists: `ls -la $DEV`
2. Check permissions: user should be in `dialout` group
3. Try different baud rates
4. Check if another process has the device open: `fuser $DEV`

### Command sent but NAK received
- Check message format matches protocol spec
- Some commands require specific preconditions
- Review packet log for error details

### Observable effect not seen
- Some changes require save + reset to take effect
- Capture duration may be too short
- Check if prerequisite configuration is needed
