# Logobs Package Design

## Overview
Implementation of point 2 from staging in time-sync.md: "Factor out logging from mon into logobs package"

The logobs package provides observability through logging, extracting clock and statistics logging functionality from the mon package into dedicated observers that implement the `obs.Observer` interface.

## Design Rationale

### Key Design Decisions

1. **circbuf.Buffer**: Rename `window` type to `circbuf.Buffer`
   - The window type is essentially a circular buffer used for maintaining a sliding window of samples
   - Avoids "util" anti-pattern (packages with "util" in the name tend to become dumping grounds)
   - Clear, reusable name following Go conventions
   - Will be shared between mon, logobs, and future phcsync packages

2. **ReopenLog on Observer interface**: Add to support log rotation
   - Required for SIGHUP signal handling to rotate logs
   - ClockLogObserver needs to reopen its log file on signal
   - Dispatcher will call observer.ReopenLog() instead of mon.ReopenLog()
   - All observers implement it (as no-op if they don't write files)

3. **Separate observer types**: StatsLogObserver and ClockLogObserver as distinct types
   - Follows existing pattern established by promobs and sseobs packages
   - Clean separation of concerns (statistics vs raw data logging)
   - Easier to test independently
   - Can be enabled/disabled independently based on configuration

## Architecture

### Package Structure
```
internal/
├── circbuf/          # New: Circular buffer implementation
│   ├── buffer.go     # Generic Buffer[T] type (moved from mon/window.go)
│   └── buffer_test.go
├── logobs/           # New: Logging observers
│   ├── statslog.go   # StatsLogObserver - periodic statistics summaries
│   ├── statslog_test.go
│   ├── clocklog.go   # ClockLogObserver - per-sample clock data
│   └── clocklog_test.go
├── mon/              # Modified: Core monitoring without logging
│   ├── monitor.go    # Remove logging code
│   └── sample.go     # Update to use circbuf.Buffer
└── obs/              # Modified: Observer interface
    └── observer.go   # Add ReopenLog() method
```

### Component Responsibilities

**StatsLogObserver**:
- Accumulates statistics over a configurable interval
- Logs summaries via slog: mean/RMS offset, frequency stats, sample counts
- No file I/O (uses structured logging)

**ClockLogObserver**:
- Writes per-sample data to a file
- Format: timestamp, offset, frequency, outlier flag, era, sync state
- Handles log rotation via ReopenLog()
- Manages file lifecycle (open, write, close)

**circbuf.Buffer**:
- Generic circular buffer implementation
- Used by mon package for outlier detection (MAD algorithm)
- Will be used by future phcsync package

## Implementation Plan

### Part A: StatsLogObserver Implementation

**Phase 1**: Create circbuf package and update mon
- Move window type from mon to circbuf as Buffer[T]
- Update mon package to import and use circbuf.Buffer
- Ensure all tests pass

**Phase 2**: Create StatsLogObserver
- Extract statistics accumulation from mon/monitor.go
- Implement obs.Observer interface
- Add periodic logging based on interval

**Phase 3**: Remove stats from mon package
- Remove all statistics-related code from monitor.go
- Keep sampler.Sample() calls for observability

**Phase 4**: Integrate StatsLogObserver
- Update daemon to create StatsLogObserver when LogInterval > 0
- Include in combined observer passed to monitor

### Part B: ClockLogObserver Implementation

**Phase 5**: Update Observer interface
- Add ReopenLog() method to Observer interface
- Update all existing observers to implement it

**Phase 6**: Create ClockLogObserver
- Extract clock logging from mon/monitor.go
- Implement full Observer interface including ReopenLog()
- Handle file operations and log format

**Phase 7**: Remove clock logging from mon
- Remove all file logging code from monitor
- Remove ReopenLog() from Monitor

**Phase 8**: Update dispatcher
- Change from calling mon.ReopenLog() to obs.ReopenLog()

**Phase 9**: Integrate ClockLogObserver
- Update daemon to create ClockLogObserver when path configured
- Include in combined observer

## Testing Strategy

1. **Unit tests**: Each new component has comprehensive tests
2. **Integration tests**: Verify observers work correctly when combined
3. **Regression tests**: Ensure log formats remain compatible
4. **Performance tests**: Verify no performance degradation

## Migration Impact

- No changes to configuration format
- Log file formats remain identical
- Transparent to end users
- Prepares codebase for future phcsync implementation

---

## Implementation Progress

### Current Status
- **Started**: 2025-10-11
- **Current Phase**: Part B completed (Phases 5-9)
- **Completed**: 2025-10-11
- **Status**: ✅ COMPLETE - All phases of logobs package implementation finished

### Phase Checklist

#### Part A: StatsLogObserver
- [x] Phase 1: Create circbuf package and update mon to use it
  - [x] Create `internal/circbuf/buffer.go`
  - [x] Create `internal/circbuf/buffer_test.go`
  - [x] Update `internal/mon/sample.go` to use circbuf.Buffer
  - [x] Delete `internal/mon/window.go`
  - [x] Delete `internal/mon/window_test.go`
  - [x] Run tests to verify circbuf works correctly

- [x] Phase 2: Create StatsLogObserver in logobs package
  - [x] Create `internal/logobs/statslog.go`
  - [x] Create `internal/logobs/statslog_test.go`
  - [x] Move accumulation types and methods from mon

- [x] Phase 3: Update mon package to remove stats logging
  - [x] Remove stats struct and methods from monitor.go
  - [x] Remove accumulation types
  - [x] Remove stats-related code from recordSample()

- [x] Phase 4: Integrate StatsLogObserver into daemon
  - [x] Add StatsLogObserver to combineObservers()
  - [x] Update combineObservers() signature
  - [x] Test integration

#### Part B: ClockLogObserver
- [x] Phase 5: Update Observer interface with ReopenLog
  - [x] Update obs/observer.go interface
  - [x] Update existing observers (sseobs, promobs)

- [x] Phase 6: Create ClockLogObserver in logobs package
  - [x] Create `internal/logobs/clocklog.go`
  - [x] Create `internal/logobs/clocklog_test.go`

- [x] Phase 7: Update mon package to remove clock logging
  - [x] Remove file logging code
  - [x] Remove ReopenLog() method

- [x] Phase 8: Update dispatcher
  - [x] Change to use observer.ReopenLog()

- [x] Phase 9: Integrate ClockLogObserver into daemon
  - [x] Update combineObservers() to create ClockLogObserver
  - [x] Add ptime import and pass leap second
  - [x] Test integration - all tests pass

### Notes
<!-- Add implementation notes and issues here -->