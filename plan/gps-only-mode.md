# Implementation Plan for Issue #76: GPS-only Mode Support

## Overview
Implement GPS-only operation mode for `satpulsed` by detecting when PHC interface is empty and bypassing PTP-related functionality while maintaining the same dispatcher architecture. This enables cross-platform support, allowing satpulsed to run on macOS and FreeBSD for the first time.

## PHC Configuration Changes (`internal/daemon/config.go`)

Modify `PHCConfig.OpenClock()` method to return `nil, nil` when `cfg.Interface` is empty:

```go
func (cfg PHCConfig) OpenClock(ctx context.Context, lg *slog.Logger) (*ts.Clock, error) {
    if cfg.Interface == "" {
        return nil, nil  // GPS-only mode
    }
    return ts.OpenClock(ctx, lg, cfg.Interface, cfg.PinDesc(), cfg.Wait)
}
```

## Main Daemon Changes (`internal/daemon/daemon.go`)

### Handle Nil Clock in run() Function
- After `clk, err := cfg.PHC.OpenClock(ctx, lg)` (line 81), check `if clk == nil`
- Skip timestamp worker startup (lines 239-242): `if clk != nil { tsCh, err := ts.StartWorker(...) }`
- Pass `nil` for tsCh to dispatcher when no clock available
- Update logging to indicate GPS-only vs PTP mode

### GPS Configuration Strategy
- Add `gpsTimePulseEnable` flag to `cfg.GPS.target()` method signature  
- When `gpsTimePulseEnable` flag absent: don't call `target.Props.SetPPS()`
- Pass time pulse enabled flag to `gpsevent.SetMsgOptions()` for appropriate message configuration
- Also add `gpsTimePulseGetWidth` flag for the existing PHC driver dependency
- Return `pulseWidth` from target() method to consolidate validation

**PTP mode**: Set `gpsTimePulseEnable` flag always, plus `gpsTimePulseGetWidth` when `phcFlags.Edges() != 1`
**GPS-only mode**: Don't set any flags - different GPS configuration and messages

**Message Configuration**:
- Modify `gpsevent.SetMsgOptions()` to accept `timePulseEnabled` parameter  
- Configure different PVT messages based on this parameter:
  - When `timePulseEnabled=true` (PTP mode): `TimePulsePVTMsgFlags` (TimePulse, TimePulseAfter, TAI, LeapSecond, Survey)
  - When `timePulseEnabled=false` (GPS-only mode): `NoTimePulsePVTMsgFlags` (Pos, Time, LeapSecond, Survey messages)
- Binary mode (NMEA disabled) already happens in `SetMsgOptions()`

## Dispatcher Creation

### daemon.NewDispatcher() Changes (line 276)
When `clk == nil`:
- Skip `servo.New(clk, lg)` creation (line 277)
- Skip `mon.NewMonitor()` creation (lines 282-290)
- Pass `nil` monitor to `gpsevent.NewDispatcher()`

### gpsevent.NewDispatcher() Changes
When monitor parameter is `nil`:
- Skip `combine.NewCombiner()` creation (line 60)
- Set `cb: nil` in Dispatcher struct
- Set `mon: nil` in Dispatcher struct

## Dispatcher.Run Changes (`internal/gpsevent/dispatcher.go`)

### Key Insight: tsCh Will Be Nil
In GPS-only mode, `tsCh` passed to `d.Run(tsCh, pCh)` will be `nil`, so:
- Event loop `for tsCh != nil || pktCh != nil` continues normally
- `case e, ok := <-tsCh:` branch never executes
- No timestamp processing occurs
- Only GPS packet processing via `pktCh` happens

### Required Nil Checks
Add nil checks for remaining monitor/combiner usage:
- **Line 93**: `defer d.mon.Close()` → `if d.mon != nil { defer d.mon.Close() }`
- **Line 149**: `d.mon.Tick(t)` → `if d.mon != nil { d.mon.Tick(t) }`
- **Line 154**: `d.mon.ReopenLog()` → `if d.mon != nil { d.mon.ReopenLog() }`
- **Line 335**: `d.mon.SetLeapSecond()` in LeapSecond() method
- **Line 248**: `d.cb.TimeMsg()` in Time() method (when GPS time messages received)

## Configuration Schema Changes (`configs/config-schema.json`)

- Remove `"required": ["phc"]` from root object (line 161)
- Remove `"required": ["interface"]` from phc section (line 68)
- PHC section becomes entirely optional

## Example Configuration Updates (`configs/satpulse.toml`)

- Add comments explaining GPS-only mode
- Show that `[phc]` section can be omitted entirely  
- Document that empty interface triggers GPS-only mode

## Implementation Steps

1. **Initial Refactor**: Decouple GPS configuration from PHC driver flags
   - Add `TimePulseEnable` and `TimePulseGetWidth` flags to `internal/daemon/gps.go`
   - Modify `GPS.target()` method signature to accept these flags
   - Update `gpsevent.SetMsgOptions()` to accept time pulse enabled parameter
   - Update call site in `daemon.go` line 179 to pass flags based on PHC driver state

2. **GPS-only Mode Implementation**: Add nil handling throughout
   - Implement all the changes listed above

## Files Modified

1. `internal/daemon/gps.go` - Add time pulse flags, update target() method
2. `internal/daemon/config.go` - Update OpenClock method  
3. `internal/daemon/daemon.go` - Handle nil clock, skip timestamp worker, use time pulse flags
4. `internal/gpsevent/dispatcher.go` - Add nil checks for monitor/combiner
5. `configs/config-schema.json` - Remove PHC requirements
6. `configs/satpulse.toml` - Add GPS-only mode documentation
7. `bsd-build.sh` - Add `./cmd/satpulsed` to targets for cross-platform builds

## GPS-only Mode Capabilities

- ✅ GPS monitoring via web interface
- ✅ TCP/socket proxying for GPS access
- ✅ GPS configuration capabilities
- ✅ GPS packet logging
- ✅ **Cross-platform support** (Linux, macOS, FreeBSD)
- ❌ No PTP timing synchronization
- ❌ No PHC servo control
- ❌ No reading of timestamps

## Cross-Platform Support

**Key Insight**: GPS-only mode enables satpulsed to work on macOS and FreeBSD for the first time!

**Current Platform Status**:
- **Linux**: Full PTP + GPS functionality (existing)
- **macOS**: Compiles but PHC fails → GPS-only mode will work
- **FreeBSD**: Build failure (term speed type casting) → GPS-only mode will work once fixed

**Cross-Platform Components**:
- ✅ GPS processing (UBX, NMEA, RTCM) - platform independent
- ✅ Serial communication - BSD support exists
- ✅ Web interface - platform independent  
- ✅ Configuration/logging - platform independent
- ❌ PHC operations - Linux only (expected limitation)

## Risk Mitigation Strategy

**⚠️ CRITICAL CONSTRAINT: GPS-only mode is nice-to-have functionality. Existing PTP mode is core functionality that MUST NOT be broken.**

### Safety-First Implementation Rules

1. **Zero changes to existing PTP code paths** - when `clk != nil`, behavior must be identical
2. **Only additive changes** - add `if clk == nil` branches, never modify PTP logic
3. **Defensive nil checks only** - add `if d.mon != nil` guards without changing the guarded code
4. **Preserve all function signatures** - no breaking changes to existing interfaces

### Implementation Pattern
```go
if clk == nil {
    // GPS-only mode: new code paths
    tsCh = nil
    // skip PTP components
} else {
    // Existing PTP code: UNCHANGED
    tsCh, err = ts.StartWorker(ctx, clk, lg)
    // existing logic continues exactly as before
}
```

### Nil Check Pattern
```go
if d.mon != nil {
    d.mon.Tick(t)  // existing call completely unchanged
}
```

## Testing Strategy

1. Test with empty PHC interface string
2. Test with omitted `[phc]` section entirely
3. Test SIGINT shutdown in GPS-only mode
4. Test SIGHUP log rotation with nil monitor
5. Verify web interface works in GPS-only mode
6. Verify TCP proxy functionality
7. Test GPS-only mode on macOS and FreeBSD platforms