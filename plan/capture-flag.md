# Add --capture flag to satpulsetool gps

GitHub issue: #197

## Overview

Add a `--capture` argument to `satpulsetool gps` that captures packets for a specified duration after configuration ends. This is useful for debugging and collecting packet logs.

Usage: `satpulsetool gps --packet-log foo.jsonl --capture 5s`

## Requirements

- `--capture` takes a duration (e.g., `5s`, `1m`)
- `--capture` requires `--packet-log` (error if used without it)
- Configuration output prints immediately before capture begins
- Capture can be interrupted with Ctrl+C

## Files to modify

### internal/gpscmd/gpsflags.go

1. Add field to `flagVars` struct:
   ```go
   captureDuration time.Duration
   ```

2. Add flag parsing (after line 136):
   ```go
   flags.DurationVar(&vars.captureDuration, "capture", 0, "capture packets for `duration` after config")
   ```

3. Add validation (after line 156):
   ```go
   if vars.captureDuration > 0 && vars.packetLogPath == "" {
       return nil, usage, fmt.Errorf("--capture requires --packet-log")
   }
   ```

4. Update `summary` constant to include `[--capture duration]`

### internal/gpscmd/gpscmd.go

1. Update `run()` signature to accept `captureDuration time.Duration`

2. Update call site in `Cmd()` (line 44) to pass `v.captureDuration`

3. Add `keepReading()` helper function:
   ```go
   func keepReading(ctx context.Context, lg *slog.Logger, pCh <-chan scan.Packet, dur time.Duration) {
       lg.Info("capturing packets", "duration", dur)
       timer := time.NewTimer(dur)
       defer timer.Stop()
       for {
           select {
           case <-ctx.Done():
               lg.Debug("capture interrupted")
               return
           case <-timer.C:
               lg.Debug("capture complete")
               return
           case _, ok := <-pCh:
               if !ok {
                   return
               }
           }
       }
   }
   ```

4. Call `keepReading()` between line 143 (after printing config) and line 148 (before `conn.Stop()`):
   ```go
   if captureDuration > 0 {
       keepReading(ctx, lg, pCh, captureDuration)
   }
   ```

## Code flow

```
Cmd()
  └─ run()
       ├─ Setup packet logging (pktLog)
       ├─ Start scanner goroutine (writes to pCh, logs via pktLog)
       ├─ gpscfg.Configure() - config phase
       ├─ Print config results
       ├─ keepReading() - NEW: drain pCh for duration  <-- capture happens here
       ├─ conn.Stop() - stop scanner
       ├─ Drain remaining packets
       ├─ wg.Wait()
       └─ writeTestLogTail() if test mode
```

The scanner goroutine handles actual packet logging via `pLog.LogInput()`. The `keepReading()` function just keeps the channel drained so the scanner can continue running.

### docs/man/satpulsetool-gps.1.md

1. Add to SYNOPSIS (after `--packet-log`):
   ```
   [**\-\-capture** *duration*]\
   ```

2. Add option description (after `--packet-log` description):
   ```
   **\-\-capture** *duration*
   : After configuration completes, continue capturing packets for the specified duration (e.g., `5s`, `1m`). Requires **\-\-packet\-log**.
   ```

3. Add example:
   ```
   Capture packets for 10 seconds after configuring:

       satpulsetool gps -d /dev/ttyACM0 -s 9600 --packet-log capture.jsonl --capture 10s
   ```

## Estimate

~30 lines across 3 files.
