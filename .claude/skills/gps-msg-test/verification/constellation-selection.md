# Verification: Constellation Selection

Applies to: `gnss-gps`, `gnss-gal`, `gnss-glo`, `gnss-bds`, `gnss-all`

## When to Use

Use this verification strategy when testing tags that change which GNSS constellations the receiver uses.

## Prerequisites

GSV (Satellites in View) messages must be enabled. If not already enabled, enable them first using the appropriate `nmea-gsv` or similar tag.

## Procedure

1. **Enable GSV output** if not already enabled

2. **Send the constellation selection command**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG --packet-log cmd.jsonl
   ```

3. **Wait for the change to take effect** (some receivers need a few seconds)

4. **Capture output** (5-10 seconds to see multiple GSV cycles):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 10 --packet-log after.jsonl
   ```

5. **Check which GSV message types appear**

## GSV Message Prefixes

Each constellation has its own GSV prefix:
- `GPGSV` - GPS satellites
- `GLGSV` - GLONASS satellites
- `GAGSV` - Galileo satellites
- `GBGSV` - BeiDou satellites

## Checking Constellation Selection

```bash
# List all GSV message types received
grep '"msg":".*GSV"' after.jsonl | grep -o '"msg":"[^"]*"' | sort -u

# Count each type
grep -c '"msg":"GPGSV"' after.jsonl
grep -c '"msg":"GLGSV"' after.jsonl
grep -c '"msg":"GAGSV"' after.jsonl
grep -c '"msg":"GBGSV"' after.jsonl
```

## Expected Results

After selecting a constellation:
- **GPS only (`gnss-gps`):** Only `GPGSV` messages, no `GLGSV`/`GAGSV`/`GBGSV`
- **Galileo only (`gnss-gal`):** Only `GAGSV` messages
- **All constellations (`gnss-all`):** Mix of all GSV types (depending on what's visible)

Note: Some receivers may still report 0 satellites for disabled constellations rather than omitting the message entirely. Check the satellite count in the GSV sentence.

## Caveats

- The receiver needs a sky view to report satellites; indoor testing won't show satellites
- Some constellation changes require a receiver reset to take effect
- If using `get-gnss` query, verify the returned values match what was set
