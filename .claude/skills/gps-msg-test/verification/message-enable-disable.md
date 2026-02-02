# Verification: Message Enable/Disable

Applies to: NMEA output control, binary output control, RTCM output control

## When to Use

Use this verification strategy when testing tags that enable or disable periodic message output from the receiver.

## Procedure

1. **Capture before** (3 seconds):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log before.jsonl
   ```

2. **Check if message is already in desired state**:
   - For enable commands: if message already appears in before capture, first disable it using the corresponding `-off` tag
   - For disable commands: if message doesn't appear in before capture, first enable it using the corresponding enable tag
   - This ensures the test demonstrates an actual state change

3. **If state change needed, set opposite state first**:
   ```bash
   # For testing enable: first disable
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG-off --packet-log setup.jsonl
   # Then capture again to confirm disabled state
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log before.jsonl
   ```

4. **Send the enable/disable command**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG --packet-log cmd.jsonl
   ```

5. **Capture after** (3 seconds):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log after.jsonl
   ```

6. **Compare message counts** between before and after captures to verify state change.

## Reading Packet Logs

Packet logs are JSONL with one JSON object per line. Key fields:
- `t` - timestamp (ISO 8601)
- `tag` - protocol type: `NMEA`, `ASBIN`, `UBX`, `CASBIN`, `RTCM`, etc.
- `msg` - specific message type: `GPGGA`, `GPRMC`, `MON-VER`, etc.
- `out` - direction: `true` = sent to receiver, `false` = received from receiver
- `ascii` - for NMEA: the raw sentence
- `bin` - for binary: hex-encoded payload

## Counting Messages

Count received messages of a specific type:

```bash
# Count GPRMC messages received
grep -c '"msg":"GPRMC"' after.jsonl

# Count all NMEA messages received
grep '"tag":"NMEA"' after.jsonl | grep -c '"out":false'

# Count specific binary message type
grep -c '"msg":"NAV-PVT"' after.jsonl
```

## Expected Results

For a 3-second capture at 1Hz update rate:
- **Enabled message:** 2-4 instances (depends on timing of capture start)
- **Disabled message:** 0 instances

## Common NMEA Message Types

- `GPRMC` / `GNRMC` - Recommended Minimum (time, position, velocity)
- `GPGGA` / `GNGGA` - Fix data (position, quality, satellites)
- `GPGSV` / `GLGSV` / `GAGSV` / `GBGSV` - Satellites in view (per constellation)
- `GPZDA` / `GNZDA` - Time and date
- `GPVTG` / `GNVTG` - Course over ground

Prefix meanings:
- `GP` - GPS
- `GL` - GLONASS
- `GA` - Galileo
- `GB` - BeiDou
- `GN` - Multiple constellations
- `BD` - BeiDou (used by some receivers like Allystar TAU1201)
