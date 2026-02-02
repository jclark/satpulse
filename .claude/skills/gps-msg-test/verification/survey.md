# Verification: Survey-in

Applies to: `survey`, `get-survey`, `survey-status`

## When to Use

Use this verification strategy when testing survey-in commands. Survey-in is used for base station positioning and takes a long time to complete (minutes to hours).

## Verification Strategy

Full survey completion takes too long to wait for during testing. Instead, verify that the survey started.

### Using Survey Status Messages

If the protocol provides a periodic survey status message, this is the best verification method:

1. **Enable the survey status message** (if not already enabled):
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t survey-status --packet-log enable.jsonl
   ```

2. **Start the survey**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t survey --packet-log survey.jsonl
   ```

3. **Capture output for a few seconds**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 5 --packet-log status.jsonl
   ```

4. **Check for survey status messages** indicating survey is in progress. The exact message type depends on the protocol.

### Using Query Command

If a `get-survey` tag exists:

```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t survey --packet-log survey.jsonl
# Wait a moment
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t get-survey --packet-log status.jsonl
```

Check the response for fields indicating survey is active.

## Caveats

- Survey requires a sky view; indoor testing won't complete
- Survey completion time depends on accuracy requirements
- Some receivers require base station mode to be set first
