# Verification: Reload Configuration

Applies to: `reload`

## Required Semantics (from satpulsetool-gps.1.md)

**--reload**: Reloads the configuration of the GPS receiver from its non-volatile memory. Any configuration settings that have not been saved will be lost. This can be used to undo any changes made by the satpulse daemon.

Key points:
- Reloads config from NVM
- Discards unsaved runtime changes
- Does NOT clear satellite data (position fix should be maintained)

## Why Reload is Safe to Test

Unlike `save` or `factory-reset`, `reload` does not modify persistent state. It only affects the running configuration by loading whatever was previously saved. This makes it safe to test without user consent.

## Verification Strategy

The test verifies that reload restores NVM config by:
1. Establishing baseline from NVM
2. Making a runtime change (enable/disable a message)
3. Verifying the change took effect (message appears/disappears)
4. Reloading from NVM
5. Verifying the runtime change was undone (message state matches baseline)

## Procedure

### 1. Reload to establish baseline

Start from known saved state:
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reload
```
Note: The message file should include `delay = 3` on the reload message to wait for reload to take effect.

### 2. Capture baseline

See what NMEA messages are being output:
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log baseline.jsonl
```

### 3. Check which messages are present

Look for presence/absence of specific message types:
```bash
grep 'RMC"' baseline.jsonl   # Is RMC present?
grep 'GGA"' baseline.jsonl   # Is GGA present?
grep 'ZDA"' baseline.jsonl   # Is ZDA present?
```

### 4. Change the running config

Toggle a message that is present in baseline:
```bash
# If RMC is present in baseline, disable it:
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t nmea-rmc-off
```

Or toggle a message that is absent:
```bash
# If ZDA is absent in baseline, enable it:
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t nmea-zda
```

### 5. Verify the change took effect

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log changed.jsonl
grep 'RMC"' changed.jsonl   # Should be absent if disabled
grep 'ZDA"' changed.jsonl   # Should be present if enabled
```

### 6. Reload to restore NVM config

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reload
```

### 7. Verify restoration

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log restored.jsonl
grep 'RMC"' restored.jsonl   # Should match baseline (present again if was present)
grep 'ZDA"' restored.jsonl   # Should match baseline (absent again if was absent)
```

### 8. Verify satellite data preserved

Check that RMC shows valid position (status field = 'A') immediately after reload:
```bash
grep 'RMC"' restored.jsonl | head -1 | jq -r '.ascii'
```
The RMC sentence should show status 'A' (valid) not 'V' (void), indicating position fix was maintained.

## Expected Results

| Capture | Message State | Position Valid |
|---------|---------------|----------------|
| baseline.jsonl | RMC present, ZDA absent (example) | Yes (A) |
| changed.jsonl | RMC absent, ZDA present (opposite) | Yes (A) |
| restored.jsonl | RMC present, ZDA absent (matches baseline) | Yes (A) |

This proves that:
1. Reload establishes a known baseline from saved config
2. Runtime changes modify the running config (message appears/disappears)
3. Reload restores the saved config, undoing runtime changes
4. Reload does NOT clear satellite data (position remains valid)
