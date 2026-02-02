# Verification: Minimum Elevation

Applies to: `min-elev-*`, `get-min-elev`

## When to Use

Use this verification strategy when testing tags that set the minimum satellite elevation angle for position fixes.

## Procedure

1. **Send the elevation change command**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG --packet-log cmd.jsonl
   ```

2. **Query configuration** if `get-min-elev` exists to verify the setting took effect

3. **Capture output** (5-10 seconds to see GSA and GSV sentences):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 10 --packet-log after.jsonl
   ```

4. **Cross-reference GSA and GSV** to verify satellites used in fix meet elevation threshold

## Observable Verification

### Extract Satellites Used in Fix (GSA)

GSA sentences contain the PRN/satellite IDs used in the current fix in fields 3-14:

```
$GNGSA,A,3,07,30,01,17,14,19,03,09,,,,,1.05,0.62,0.85,1*04
           ^^ ^^ ^^ ^^ ^^ ^^ ^^ ^^  <-- satellite IDs used in fix
```

Extract satellite IDs from GSA:
```bash
# Get satellite IDs from GSA (fields 3-14, excluding empty fields)
grep 'GSA.*ascii' after.jsonl | head -1 | \
  sed 's/.*ascii":"//;s/\\r\\n.*//' | \
  cut -d',' -f4-15 | tr ',' '\n' | grep -v '^$'
```

### Extract Satellite Elevations (GSV)

GSV sentences contain satellite info in groups of 4 fields: PRN, elevation, azimuth, SNR:

```
$GPGSV,4,1,13,7,76,177,46,30,66,306,42,1,52,59,23,17,43,288,45,1*5F
              ^ ^^       ^^ ^^       ^ ^^       ^^ ^^
            PRN elev   PRN elev   PRN elev   PRN elev
```

Extract PRN and elevation pairs:
```bash
# Parse GSV to get PRN,elevation pairs
grep 'GSV.*ascii' after.jsonl | sed 's/.*ascii":"//;s/\\r\\n.*//' | while read line; do
  # Remove checksum and split by comma
  fields=$(echo "$line" | sed 's/\*[0-9A-F]*$//' | tr ',' ' ')
  # Skip header fields (talker+GSV, total, seq, numSV) and process satellite groups
  set -- $fields
  shift 4  # skip first 4 fields
  while [ $# -ge 4 ]; do
    prn=$1; elev=$2
    if [ -n "$prn" ] && [ -n "$elev" ]; then
      echo "$prn $elev"
    fi
    shift 4
  done
done | sort -u
```

### Cross-Reference Check

For each satellite ID in GSA, look up its elevation in GSV and verify it meets the minimum:

```bash
# Example: verify all satellites in fix have elevation >= 20
MIN_ELEV=20

# Get satellites used in fix from GSA
grep 'GSA.*ascii' after.jsonl | head -1 | \
  sed 's/.*ascii":"//;s/\\r\\n.*//' | \
  cut -d',' -f4-15 | tr ',' '\n' | grep -v '^$' | sort -u > /tmp/fix_sats.txt

# Get all satellite elevations from GSV
grep 'GSV.*ascii' after.jsonl | sed 's/.*ascii":"//;s/\\r\\n.*//' | while read line; do
  fields=$(echo "$line" | sed 's/\*[0-9A-F]*$//' | tr ',' ' ')
  set -- $fields
  shift 4
  while [ $# -ge 4 ]; do
    prn=$1; elev=$2
    if [ -n "$prn" ] && [ -n "$elev" ]; then
      echo "$prn $elev"
    fi
    shift 4
  done
done | sort -u > /tmp/all_sats.txt

# Check each satellite in fix
while read sat; do
  elev=$(grep "^$sat " /tmp/all_sats.txt | awk '{print $2}')
  if [ -n "$elev" ] && [ "$elev" -lt "$MIN_ELEV" ]; then
    echo "FAIL: Satellite $sat has elevation $elev < $MIN_ELEV"
  fi
done < /tmp/fix_sats.txt
```

## Verification Comments

Use these comment formats:

```toml
# Verified ACK received on [MODEL]
# Verified get-min-elev returns expected value on [MODEL]
# Verified satellites in fix have elevation >= [N] degrees on [MODEL]
```

## Caveats

- Verification requires satellites at various elevations to be visible
- If all visible satellites are above the threshold anyway, the test is inconclusive
- Some receivers may take a few seconds to exclude low-elevation satellites after the setting change
- The minimum elevation setting affects position accuracy vs availability tradeoff
