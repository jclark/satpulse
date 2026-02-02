# Verification: Reset

Applies to: `reset`

## Required Semantics (from satpulsetool-gps.1.md)

**--reset**: Perform a reset that reloads the configuration of the GPS receiver from its non-volatile memory (as with the **--reload** option), and discards information about the last known position, current time, and satellite orbital data (both ephemeris and almanac).

Key points:
- Reloads config from NVM (like reload)
- ALSO clears satellite data (position, time, ephemeris, almanac)
- This is more extreme than reload - receiver must reacquire satellites

## Two Aspects to Verify

Reset has two distinct effects that must both be verified:

1. **NVM reload aspect**: Config is restored from NVM (same as reload)
2. **Cold start aspect**: Satellite data is cleared (receiver loses fix)

## Procedure

### Part 1: Verify NVM Reload Aspect

#### 1.1 Establish baseline with reload first

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reload
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log baseline.jsonl
```

Check which messages are present and verify valid position:
```bash
grep 'RMC"' baseline.jsonl | head -1 | jq -r '.ascii'
```
RMC should show status 'A' (valid fix).

#### 1.2 Make a runtime config change

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t nmea-rmc-off
```

#### 1.3 Verify change took effect

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log changed.jsonl
grep 'RMC"' changed.jsonl   # Should be absent
```

#### 1.4 Send reset command and capture

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reset --capture 5 --packet-log reset.jsonl
```

#### 1.5 Verify config restored from NVM

```bash
grep 'RMC"' reset.jsonl   # Should be present again (restored from NVM)
```

### Part 2: Verify Cold Start Aspect

After sending the reset command, verify that satellite data was cleared by checking the captured data.

#### 2.1 Check RMC status

```bash
grep 'RMC"' reset.jsonl | head -5 | jq -r '.ascii'
```

**Expected**: RMC should show status 'V' (void/invalid) immediately after reset:
- Field 2 contains 'V' (void) instead of 'A' (active)
- Position fields (lat/lon) are empty
- Example: `$GNRMC,,V,,,,,,,,0.0,E,N,V*5C`

#### 2.2 Check GSV messages

```bash
grep 'GSV"' reset.jsonl | head -5 | jq -r '.ascii'
```

**Expected**: GSV messages should show satellites WITHOUT elevation/azimuth data. This indicates ephemeris was cleared. Two possible patterns:

**(a) No satellites visible initially:**
```
$GPGSV,1,1,00*79
$GLGSV,1,1,00*65
```

**(b) Satellites visible but with unknown elevation/azimuth:**
GSV format is: `$xxGSV,total,num,sats,[PRN,elev,az,SNR],...`

After cold start, the PRN,elev,az,SNR groups will have empty elev/az fields:
```
$GPGSV,1,1,03,7,,,46,6,,,45,30,,,47,1*65
              ^ ^    ^ ^      ^ ^
              empty  empty    empty elevation/azimuth
```

Compare to normal GSV with known orbits:
```
$GPGSV,2,1,07,07,75,031,47,30,45,123,48,06,23,287,45,19,18,045,40*7A
              ^^,^^^    ^^,^^^    ^^,^^^    ^^,^^^
              elev,az   elev,az   elev,az   elev,az present
```

## Expected Results

| Check | Expected |
|-------|----------|
| Config restored | RMC present after reset (was disabled before) |
| RMC status after reset | 'V' (void) immediately |
| RMC position after reset | Empty lat/lon fields |
| GSV after reset | Satellites without elevation/azimuth, OR no satellites |

## Failure Indicators

If any of these occur, the reset command is not working correctly:

- RMC shows 'A' (valid) immediately after reset
- RMC shows valid lat/lon immediately after reset
- GSV shows satellites WITH elevation/azimuth data immediately after reset

## Difference from Reload

| Aspect | Reload | Reset |
|--------|--------|-------|
| Config from NVM | Yes | Yes |
| Clear satellite data | No | Yes |
| RMC status after | 'A' (valid) immediately | 'V' (void) initially |
| GSV after | Full elev/az data | Missing elev/az data |
| Time to recover | Instant | Several seconds to minutes |
