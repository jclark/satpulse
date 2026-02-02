# Verification: Factory Reset

Applies to: `factory-reset`

## Required Semantics (from satpulsetool-gps.1.md)

**--factory-reset**: Restore the non-volatile memory of the GPS receiver to its default settings, and then perform a reset as with the **--reset** option.

Key points:
- Clears NVM to factory defaults
- THEN performs a reset (which clears satellite data)
- This is the most extreme operation - loses all saved config AND satellite data

## CAUTION

Factory reset **modifies persistent state** (clears NVM). Only test with user permission. The receiver will lose all saved configuration.

## Two Aspects to Verify

Factory reset has two distinct effects that must both be verified:

1. **NVM clearing aspect**: NVM is cleared to factory defaults (saved config is lost)
2. **Cold start aspect**: Satellite data is cleared (receiver loses fix)

## Procedure

### Part 1: Verify Cold Start Aspect

This is easier to verify and should be checked first.

#### 1.1 Send factory-reset and capture immediately

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t factory-reset --capture 5 --packet-log factory.jsonl
```

#### 1.2 Check RMC status

```bash
grep 'RMC"' factory.jsonl | head -5 | jq -r '.ascii'
```

**Expected**: RMC should show status 'V' (void/invalid) immediately after factory-reset:
- Field 2 contains 'V' (void) instead of 'A' (active)
- Position fields (lat/lon) are empty
- Example: `$GNRMC,,V,,,,,,,,0.0,E,N,V*5C`

#### 1.3 Check GSV messages

```bash
grep 'GSV"' factory.jsonl | head -5 | jq -r '.ascii'
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

### Part 2: Verify NVM Clearing Aspect

This requires comparing behavior before and after factory-reset.

#### 2.1 Establish baseline with reload

Before factory-reset, reload and capture the saved configuration:
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reload
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log baseline.jsonl
```

Note which NMEA messages are present. The user's saved config likely differs from factory defaults.

#### 2.2 Send factory-reset

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t factory-reset --capture 5 --packet-log factory.jsonl
```

#### 2.3 Verify config changed to factory defaults

```bash
grep 'NMEA"' factory.jsonl | jq -r '.msg' | sort -u
```

Compare which messages are present vs baseline. Factory defaults typically include:
- GGA, GSA, GSV, RMC, VTG (common defaults)
- No binary messages

If the user had customized message output (e.g., disabled GSV, enabled binary), factory reset should revert to defaults.

#### 2.4 Verify NVM was cleared (not just runtime)

Reload and check if it matches factory defaults (not the previous saved config):
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reload
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log after-reload.jsonl
```

The messages in after-reload.jsonl should match factory.jsonl (factory defaults), NOT baseline.jsonl (user's previous saved config).

## Expected Results

| Check | Expected |
|-------|----------|
| RMC status after factory-reset | 'V' (void) immediately |
| RMC position after factory-reset | Empty lat/lon fields |
| GSV after factory-reset | Satellites without elevation/azimuth, OR no satellites |
| Config after factory-reset | Factory defaults (may differ from baseline) |
| Config after reload | Still factory defaults (NVM was cleared) |

## Failure Indicators

**Cold start aspect failures:**
- RMC shows 'A' (valid) immediately after factory-reset
- RMC shows valid lat/lon immediately after factory-reset
- GSV shows satellites WITH elevation/azimuth data immediately after factory-reset

**NVM clearing aspect failures:**
- Config after factory-reset matches user's saved config (not factory defaults)
- Reload after factory-reset restores user's old saved config

## Difference from Reset

| Aspect | Reset | Factory Reset |
|--------|-------|---------------|
| Clears NVM | No | Yes |
| Config after | Restored from NVM | Factory defaults |
| Clear satellite data | Yes | Yes |
| Persistent effect | None | Loses saved config |
