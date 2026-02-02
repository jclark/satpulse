# Verification: NMEA Version

Applies to: `nmea-ver-3`, `nmea-ver-400`, `nmea-ver-410`, `nmea-ver-411`, `get-nmea-ver`

## When to Use

Use this verification strategy when testing tags that change the NMEA protocol version output by the receiver.

## Procedure

1. **Send the version change command**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG --packet-log cmd.jsonl
   ```

2. **Query configuration** if `get-nmea-ver` exists to verify the setting took effect

3. **Capture output** (3-5 seconds to see NMEA sentences):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 5 --packet-log after.jsonl
   ```

4. **Check observable differences** in talker IDs and GSV structure

## Observable Differences

### Talker IDs

Different NMEA versions use different talker IDs for BeiDou and QZSS:

| Version | BeiDou | QZSS |
|---------|--------|------|
| V3.01   | BD     | -    |
| V4.00   | BD     | -    |
| V4.10   | BD     | -    |
| V4.11   | GB     | GQ   |

Check talker IDs in the capture:
```bash
# Extract and count talker IDs
grep 'NMEA' after.jsonl | sed 's/.*ascii":"\$\([A-Z]*\).*/\1/' | cut -c1-2 | sort | uniq -c
```

**V3.01/V4.00/V4.10 expected:** `BD` prefix for BeiDou (e.g., `$BDGSV`)
**V4.11 expected:** `GB` prefix for BeiDou (e.g., `$GBGSV`), `GQ` prefix for QZSS (e.g., `$GQGSV`)

### GSV Signal ID Field

NMEA 4.10 and 4.11 add a signal ID field at the end of GSV sentences:

| Version | GSV Format | Example |
|---------|------------|---------|
| V3.01   | No signal ID | `$GPGSV,4,1,13,7,73,176,51,...,40*4D` |
| V4.00   | No signal ID | `$GPGSV,5,1,17,7,72,176,49,...,40*4A` |
| V4.10   | Has signal ID | `$GPGSV,5,1,17,7,72,176,48,...,25,1*6A` |
| V4.11   | Has signal ID | `$GPGSV,4,1,13,7,73,176,51,...,43,1*50` |

The signal ID appears before the checksum (e.g., `,1*6A` where `1` is the signal ID for L1).

Check for signal ID:
```bash
# V4.10/V4.11 GSV ends with signal ID before checksum: ,N*XX
grep 'GSV.*ascii' after.jsonl | head -1 | grep -E ',[0-9]\*[0-9A-F]{2}'
```

## Verification Comments

Use these comment formats:

```toml
# Verified ACK received on [MODEL]
# Verified get-nmea-ver returns expected value (XX) on [MODEL]
# Verified GB/GQ talker IDs appear after nmea-ver-411 on [MODEL]
# Verified BD talker ID appears after nmea-ver-3 on [MODEL]
# Verified GSV sentences include signal ID after nmea-ver-410 on [MODEL]
```

## Caveats

- QZSS satellites (GQ) are only visible in Asia-Pacific region
- BeiDou satellites (BD/GB) may not be visible in some regions
- Changes take effect immediately; no restart required
