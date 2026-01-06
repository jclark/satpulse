# Plan: Deprecate pulseWidth Configuration (Issue #185)

## Summary
Remove pulseWidth from data flow; rely on reset mode auto-detection.

## Changes

### 1. Deprecation warning (internal/daemon/config.go)
In `readConfig()` after line 131:
```go
if !math.IsNaN(cfg.GPS.PulseWidth) {
    slog.Warn("gps.pulseWidth is deprecated and will be ignored; pulse width is now auto-detected")
}
```

### 2. Simplify gps.go (internal/daemon/gps.go)
- Remove `gpsTimePulseFlags` type/constants, use bool
- Change `target()`: remove pulseWidth return, take `timePulseEnabled bool`
- Remove `pulseWidth()` method
- Remove `PropIDTimePulseWidth` from `target.Get`

### 3. Simplify daemon.go (internal/daemon/daemon.go)
- Replace `gpsTimePulseFlags` with bool `timePulseEnabled := clk != nil`
- Change `createConfigTarget()` to return `(*gpsprot.ConfigTarget, error)`
- Remove `gcfg.ConfigProps.GetTimePulseWidth()` usage
- Remove `pulseWidth` from `daemon.NewDispatcher()` signature
- Pass `clk.DriverFlags.Edges()` to `phcsync.NewController()`

### 4. Simplify phcsync (internal/phcsync/)

**controller.go:**
- `NewController()`: replace `pt PulseType` with `edgesPerPulse int`
- Remove `pulseWidthSpec` field
- Remove line 351 (`c.pt.PulseWidth = c.pulseWidthSpec`)
- Pass `c.pt.EdgesPerPulse` to `newResetSampleGenerator`

**reset.go:**
- `newResetSampleGenerator()`: replace `pt PulseType` with `edgesPerPulse int`
- Initialize `pt` internally: `pt: PulseType{EdgesPerPulse: edgesPerPulse}`

### 5. Clean up gpsevent.NewDispatcher (internal/gpsevent/dispatcher.go)
- Remove `pulseWidth` and `phcFlags` parameters (unused)

### 6. Update documentation (docs/man/satpulse.toml.5.md)
- Remove `pulseWidth` entry (lines 90-93)
- Update line 88: remove pulseWidth from exception list

### 7. Update config schema (configs/config-schema.json)
- Remove `pulseWidth` property (lines 29-33)

### 8. Update systest (systest/config.yml)
Add to "Remove old options" task:
```yaml
- { section: "gps", option: "pulseWidth" }
```

## Files Modified

| File | Changes |
|------|---------|
| internal/daemon/config.go | Add deprecation warning |
| internal/daemon/gps.go | Remove pulseWidth, gpsTimePulseFlags → bool |
| internal/daemon/daemon.go | Remove pulseWidth, tpFlags → bool |
| internal/phcsync/controller.go | NewController takes edgesPerPulse int |
| internal/phcsync/reset.go | newResetSampleGenerator takes edgesPerPulse int |
| internal/gpsevent/dispatcher.go | Remove unused parameters |
| docs/man/satpulse.toml.5.md | Remove pulseWidth docs |
| configs/config-schema.json | Remove pulseWidth from schema |
| systest/config.yml | Remove pulseWidth from old configs |
