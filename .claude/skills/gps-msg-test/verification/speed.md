# Verification: Baud Rate Changes

Applies to: `speed-*` tags

## When to Use

Use this verification strategy when testing baud rate change commands.

## Safety Rules

- **Do not test speeds below 38400** unless that is the receiver's default speed. Low speeds often have insufficient bandwidth for message output, causing configuration commands to fail.
- Be prepared to reconnect at the new speed after sending the command.

## Challenge: USB CDC Devices

`/dev/ttyACMx` devices (USB CDC) accept any baud rate setting because the USB driver ignores it. The actual communication is always at USB speed. This means:
- Reconnecting at the "new" baud rate doesn't prove anything
- The receiver may have changed its internal UART speed, but you can't tell via USB

## Verification for /dev/ttyUSBx (Real UART)

1. **Send the speed change command**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t speed-115200 --packet-log change.jsonl
   ```

2. **Reconnect at the new speed**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s 115200 --capture 3 --packet-log verify.jsonl
   ```

3. **Check for valid messages** in the capture. If the speed change worked, you'll see valid NMEA/binary messages. If it failed, you'll see garbage or no output.

## Verification for /dev/ttyACMx (USB CDC)

Since USB ignores baud rate, verify indirectly by examining message timing:

1. **Capture a burst of messages** at current speed:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log before.jsonl
   ```

2. **Send the speed change command**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t speed-38400 --packet-log change.jsonl
   ```

3. **Capture messages after change**:
   ```bash
   out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --capture 3 --packet-log after.jsonl
   ```

4. **Compare inter-message timing**:
   - Within a single navigation epoch, multiple messages are sent in a burst
   - At 38400 baud, gaps between messages in a burst are noticeably longer than at 115200
   - This isn't precise but can distinguish significantly different speeds

### Timing Analysis

Look at timestamps of consecutive messages in the same epoch:
```bash
# Extract timestamps from NMEA messages
grep '"tag":"NMEA"' after.jsonl | head -10 | jq -r '.t'
```

At 115200 baud, consecutive messages in a burst might be ~5ms apart.
At 38400 baud, they might be ~15ms apart.

## Recovery

If communication is lost after a baud rate change:
1. Try the new baud rate first
2. Try common baud rates: 9600, 38400, 115200, 460800
3. Power cycle the receiver (reverts to saved or default baud rate)
