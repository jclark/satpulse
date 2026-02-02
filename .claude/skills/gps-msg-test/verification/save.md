# Verification: Save Configuration

Applies to: `save`

## When to Use

Use this verification strategy when testing the `save` command that persists configuration to NVM.

## Safety Warning

**This test modifies persistent state.** Only run with user consent.

## Procedure

1. **Make a configuration change** (e.g., disable an NMEA message):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t nmea-rmc-off --packet-log change.jsonl
   ```

2. **Verify the change took effect**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log after-change.jsonl
   grep -c '"msg":"GPRMC"' after-change.jsonl  # Should be 0
   ```

3. **Run save**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t save --packet-log save.jsonl
   ```

4. **Make a different change** (re-enable the message):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t nmea-rmc --packet-log reenable.jsonl
   ```

5. **Verify the message is now enabled**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log after-reenable.jsonl
   grep -c '"msg":"GPRMC"' after-reenable.jsonl  # Should be > 0
   ```

6. **Run reload** to restore from NVM:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t reload --packet-log reload.jsonl
   ```

7. **Verify reload restored the saved state** (message disabled again):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log after-reload.jsonl
   grep -c '"msg":"GPRMC"' after-reload.jsonl  # Should be 0
   ```

8. **Restore original setting** (important!):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t nmea-rmc --packet-log restore.jsonl
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t save --packet-log save2.jsonl
   ```

## Expected Results

- After step 2: message disabled
- After step 5: message enabled (unsaved change)
- After step 7: message disabled again (reload restored saved state)

This proves that save actually persisted the configuration.

## Caveats

- Always restore the original setting after testing
- Some receivers have limited NVM write cycles; avoid excessive save testing
