# Prometheus Metrics Implementation Plan

## Overview

This plan describes implementing Prometheus metrics support for SatPulse (GitHub issue [#78](https://github.com/jclark/satpulse/issues/78)) by creating a harmonized observability interface that unifies SSE events, Prometheus metrics, and potentially logging.

The implementation is split into two stages:
- **Stage 1**: Observer interface refactoring (no new functionality, pure refactoring)
- **Stage 2**: Prometheus metrics implementation using the new architecture

## Design Goals

1. Create a `Sampler` interface in the `internal/mon` package for clock synchronization samples
2. Create a unified `Observer` interface in the `internal/obs` package that extends `Sampler` and `gpsprot.MsgHandler`
3. Move SSE-specific code out of core packages into dedicated observability implementations
4. Support multiple observability backends (SSE, Prometheus) through a fan-out pattern
5. Enable/disable UI and metrics endpoints via configuration

## Architecture

### Core Interfaces

#### `internal/mon/sampler.go`
```go
package mon

// SampleData contains all information about a synchronization sample
type SampleData struct {
    Kind      SampleKind    // sampleOK, sampleMissing, sampleOutlier - determines validity of other fields
    Ref       ptime.Time    // GPS reference time (different from system time)
    Offset    time.Duration // PHC/GPS offset (valid for sampleOK and sampleOutlier, 0 for sampleMissing)
    Freq      float64       // current frequency adjustment in PPB (always valid)
    SyncState SyncState     // current synchronization state (always valid)
    Era       ptime.Era     // for clock step tracking and logging (always valid)
}

// Sampler handles clock synchronization samples of all types
type Sampler interface {
    // Sample reports a clock synchronization sample of any kind
    Sample(data SampleData)
}
```

#### `internal/obs/observer.go`
```go
package obs

import (
    "github.com/jclark/satpulse/internal/gpsprot"
    "github.com/jclark/satpulse/internal/mon"
)

// Observer provides unified observability interface
type Observer interface {
    mon.Sampler
    gpsprot.MsgHandler
    
    // Release cleans up resources (e.g., closes SSE channels)
    Release()
}

// MultiObserver fans out calls to multiple observers
type MultiObserver struct {
    observers []Observer
}

func NewMultiObserver(observers ...Observer) *MultiObserver

```

### SSE Implementation

#### `internal/sseobs/sse.go`
```go
package sseobs

// SSEObserver implements obs.Observer for Server-Sent Events
type SSEObserver struct {
    sseCh    chan sse.Event
    lg       *slog.Logger
    lastTime ptime.Time
}

func New(lg *slog.Logger) *SSEObserver {
    sseCh := make(chan sse.Event, sseChannelSize)
    return &SSEObserver{
        sseCh: sseCh,
        lg:    lg,
    }
}

// Channel returns the SSE channel for the daemon to create broadcast from
func (s *SSEObserver) Channel() <-chan sse.Event {
    return s.sseCh
}

// Release closes the SSE channel
func (s *SSEObserver) Release() {
    close(s.sseCh)
}

// Implement all Observer methods, moving SSE generation code from:
// - internal/gpsevent/dispatcher.go (Time, Survey, Satellites events)
// - internal/mon/monitor.go (phc sample events)
//
// Sample() implementation should:
// - Reproduce EXACTLY the same SSE events as the current implementation
// - Use data.Kind for outlier flag in SampleEvent
// - Extract stepCount/stepCountChanging from data.Era  
// - Use data.Offset directly (already in nanoseconds as time.Duration)
// - Maintain identical event structure and timing to preserve web UI compatibility
```

### Prometheus Implementation

#### `internal/promobs/prometheus.go`
```go
package promobs

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusObserver implements obs.Observer for Prometheus metrics
type PrometheusObserver struct {
    reg *prometheus.Registry
    
    // Metrics will be added incrementally (see Prometheus Metrics Specification)
}

func New() *PrometheusObserver {
    // Create custom registry (no global registry)
    // Initialize with basic metrics first
}

// Handler returns the HTTP handler for /metrics endpoint
func (p *PrometheusObserver) Handler() http.Handler {
    return promhttp.HandlerFor(p.reg, promhttp.HandlerOpts{})
}

// Sample() implementation should:
// - Always update sync state gauge from data.SyncState
// - Always update frequency gauge from data.Freq  
// - For data.Kind == sampleOK: update offset histogram and rolling statistics
// - For data.Kind == sampleOutlier: increment outlier counter only (don't include in stats)
// - For data.Kind == sampleMissing: increment missing samples counter only
// - Track clock steps from data.Era changes
// - Maintain sliding window for rolling statistics (30-second window suggested)
```

### Daemon Integration

#### Configuration Changes

Update `docs/config.md` and `configs/config-schema.json` to add per-endpoint configuration for HTTP endpoints.

#### HTTPConfig (implemented in `internal/daemon/http.go`)
```go
type HTTPConfig struct {
    Listen  string `toml:"listen"`
    PProf   bool   `toml:"pprof"`
    GUI     *bool  `toml:"gui"`     // Defaults to true, uses gui() method
    Metrics *bool  `toml:"metrics"` // Defaults to true, uses metrics() method
}
```

#### `internal/daemon/daemon.go` Changes

1. Observer creation pattern:
```go
// In main daemon function - create promObs early
promObs := newPrometheusObserver(cfg)

// Pass promObs to NewDispatcher where it combines with sseObs
d, err := NewDispatcher(..., promObs, ...)

// Pass promObs to startHTTP for /metrics endpoint
err = startHTTP(..., promObs)
```

Helper functions in `daemon.go`:
```go
// newPrometheusObserver creates Prometheus observer if any HTTP endpoint needs metrics
func newPrometheusObserver(cfg *Config) *promobs.PrometheusObserver

// newSSEObserver creates SSE observer if any HTTP endpoint needs GUI  
func newSSEObserver(cfg *Config, sseCh chan<- sse.Event, ls ptime.LeapSecond, lg *slog.Logger) obs.Observer

// combineObservers combines individual observers into appropriate single observer
func combineObservers(promObs *promobs.PrometheusObserver, sseObs obs.Observer) obs.Observer
```

2. NewDispatcher function signature updated:
```go
func NewDispatcher(..., promObs *promobs.PrometheusObserver, ...) (*gpsevent.Dispatcher, error) {
    // Create sseObs internally 
    sseObs := newSSEObserver(cfg, sseCh, ls, lg)
    obs := combineObservers(promObs, sseObs)
    // Use obs for monitoring
}
```

3. HTTP server setup updated in `internal/daemon/http.go`:
```go
// Function signature includes promObs parameter
func startHTTP(..., promObs *promobs.PrometheusObserver) error

// In endpoint configuration loop:
for i, listener := range listeners {
    mux := http.NewServeMux()
    
    if cfg[i].PProf {
        registerPprofHandlers(mux)
    }
    
    if cfg[i].gui() {
        // Register GUI routes
    }
    
    if cfg[i].metrics() {
        mux.Handle("/metrics", promObs.Handler())
    }
    
    // Validation: at least one of pprof, gui, metrics must be enabled
}
```

## Critical Implementation Constraints

⚠️ **GOROUTINE AND CHANNEL LIFECYCLE PRESERVATION** ⚠️

The Observer refactoring MUST preserve the exact same goroutine creation order and channel closure sequence as the current implementation. This is a tricky area where even small changes can introduce race conditions or deadlocks.

**Requirements:**
- NO changes to goroutine creation order or timing
- NO changes to channel closure order or timing  
- Current `close(sseCh)` becomes `observer.Release()` at the exact same point
- Broadcast goroutine lifecycle remains identical
- HTTP server goroutine lifecycle remains identical

**Current behavior to preserve:**
- `close(sseCh)` in current code → `observer.Release()` in new code
- Same shutdown sequence and timing
- Same goroutine coordination patterns

This constraint ensures the refactoring is truly behavior-preserving and minimizes concurrency risks.

## Implementation Stages

### Stage 1: Observer Interface Refactoring

**Goal**: Refactoring to introduce the Observer interface with no Prometheus functionality. Adds gui config variable but preserves existing SSE behavior.

**Stage 1 Steps**:

1. **Create Sampler interface** in `internal/mon/sampler.go`
2. **Create obs package** with unified Observer interface and MultiObserver
3. **Implement SSE observer** in `internal/sseobs/`:
   - Move SSE event creation from `gpsevent/dispatcher.go`
   - Move sample event creation from `mon/monitor.go`
   - Preserve exact same SSE events and behavior
4. **Update gpsevent.Dispatcher**:
   - Replace `sseCh` with `Observer` interface
   - Remove SSE-specific code
   - Call observer methods instead
5. **Update mon.Monitor**:
   - Replace `sseCh` with `Sampler` interface
   - Remove SSE-specific code
   - Update `Monitor.Sample()` to call `sampler.Sample()` with populated `SampleData`
   - Update `Monitor.Tick()` to call `sampler.Sample()` with `Kind: sampleMissing` for missing samples
6. **Update configuration**:
   - Add `gui` boolean field to HTTPConfig in schema
   - Update docs/config.md to document the new field
   - Add validation: error if both pprof and gui are false for an endpoint
7. **Update daemon package**:
   - Implement `createSSEObserver` function only
   - Create SSE observer when any HTTP endpoint has gui=true
   - Pass observer to dispatcher and monitor
8. **Update HTTP implementation**:
   - Modify `startHTTP` to only register `/sse` and `/` routes when `cfg[i].GUI` is true
   - Keep existing pprof logic unchanged
9. **Test**: Verify SSE functionality works identically to before refactoring

### Stage 2: Prometheus Implementation

**Goal**: Add Prometheus metrics using the new Observer architecture, with configuration options.

**Stage 2 Steps**:

1. **Implement basic Prometheus observer** in `internal/promobs/`:
   - Start with simple metrics: sync_state and clock_offset_nanoseconds
   - Use custom registry (no global state)
   - Get basic infrastructure working
2. **Update daemon package**:
   - Add Metrics configuration support (GUI already done in Stage 1)
   - Create appropriate observers based on config (SSE, Prometheus, or both)
   - Update HTTP server routing to support /metrics endpoint
3. **Update configuration**:
   - Add metrics boolean field to HTTPConfig in schema (gui already done in Stage 1)
   - Update docs/config.md to document the new metrics field
4. **Expand Prometheus metrics incrementally**:
   - First: Add remaining Sample-based metrics (frequency, histogram, stats)
   - Second: Add GPS message-based metrics (satellites, survey)
   - Third: Add system metrics (start time, last PPS)
5. **Test**: Verify both SSE and Prometheus metrics work correctly

## Implementation Progress

### Stage 1: Observer Interface Refactoring ✅ COMPLETED

- ✅ Created Sampler interface in `internal/mon/sampler.go`
- ✅ Made MultiHandler type public and added Handlers() iterator in `internal/gpsprot/msg.go`
- ✅ Created MultiSampler in `internal/mon/sampler.go`
- ✅ Created Observer interface and MultiObserver in `internal/obs/observer.go`
- ✅ Implemented SSE observer in `internal/sseobs/sse.go`
- ✅ Updated gpsevent.Dispatcher to use Observer interface
- ✅ Updated mon.Monitor to use Sampler interface
- ✅ Added GUI configuration support with HTTPConfig.gui() method
- ✅ Updated daemon package to use observer architecture
- ✅ Updated HTTP implementation to conditionally register GUI routes
- ✅ Code compiles and tests pass

### Stage 2: Prometheus Implementation 🚧 IN PROGRESS

**Completed:**
- ✅ Created PrometheusObserver in `internal/promobs/prometheus.go`
- ✅ Added Prometheus dependencies to go.mod  
- ✅ Implemented basic metrics infrastructure with 2 core metrics:
  - `satpulse_in_sync` gauge - Synchronization status 
  - `satpulse_clock_offset_abs_nanoseconds` histogram - Offset distribution
- ✅ Added `metrics` configuration field to HTTPConfig with `metrics()` method
- ✅ Updated JSON schema for metrics configuration
- ✅ Modified daemon to create PrometheusObserver and pass to both NewDispatcher and startHTTP
- ✅ Updated startHTTP to register `/metrics` endpoint when enabled
- ✅ Added validation requiring at least one of pprof, gui, or metrics per endpoint
- ✅ Tested: `/metrics` endpoint accessible and returning real synchronization data
- ✅ Clock synchronization metrics (with a few changes)

**Still to do:**
- ⏳ Other metrics as specified in the Prometheus Metrics Specification section

## Prometheus Metrics Specification

Based on GitHub issue [#78](https://github.com/jclark/satpulse/issues/78) requirements and additional ideas from issue [#112](https://github.com/jclark/satpulse/issues/112):

### Clock Synchronization Metrics (from Sampler Interface)

- `satpulse_in_sync` (gauge) - Synchronization status (1 = in sync, 0 = out of sync)
- `satpulse_clock_offset_abs_nanoseconds` (histogram) - Histogram of absolute offset values for good samples
- `satpulse_clock_offset_nanoseconds` (gauge) - Current signed offset between PHC and GPS time in nanoseconds (no abs)
- `satpulse_clock_frequency_ppb` (gauge) - Current frequency adjustment in parts per billion
- `satpulse_offset_abs_sum_nanoseconds_total` (counter) - Sum of absolute values of offsets from good samples (for mean calculation)
- `satpulse_offset_sum_squares_nanoseconds_total` (counter) - Sum of squares of offsets from good samples (for stddev calculation)
- `satpulse_samples_missing_total` (counter) - Total missing samples
- `satpulse_samples_outlier_total` (counter) - Total outlier samples
- `satpulse_samples_good_total` (counter) - Total good samples (in sync and not outlier)
- `satpulse_sync_losses_total` (counter) - Total number of times sync was lost after being established
- `satpulse_time_to_sync_seconds` (gauge) - Time in seconds from startup to first sync acquisition

Note: The offset and frequency gauges should be scraped once per second for full resolution data. 
The sum counters enable accurate computation of mean absolute offset and standard deviation using PromQL:
- Mean absolute offset = rate(satpulse_offset_abs_sum_nanoseconds_total) / rate(satpulse_samples_good_total)
- Standard deviation = sqrt(rate(satpulse_offset_sum_squares_nanoseconds_total) / rate(satpulse_samples_good_total))

### Satellite Metrics (from gpsprot.SatellitesMsg)

- `satpulse_satellites_visible` (gauge, labels: gnss) - Number of satellites in view per constellation
- `satpulse_satellites_used` (gauge, labels: gnss) - Number of satellites used in solution per constellation
- `satpulse_satellite_signal_strength` (gauge, labels: gnss, sv_id, signal) - Signal strength per satellite and signal (CNR)

### Survey Metrics (from gpsprot.SurveyMsg)

- `satpulse_survey_in_progress` (gauge) - 1 if survey is in progress, 0 otherwise
- `satpulse_survey_accuracy_meters` (gauge) - Current survey position accuracy in meters

### System Metrics

- `satpulse_start_time_seconds` (gauge) - Unix timestamp when daemon started
- `satpulse_last_pps_time_seconds` (gauge) - Time in seconds since last PPS pulse received

### Clock Step Metrics (from SampleData.Era)

- `satpulse_clock_steps_total` (counter) - Total number of clock step adjustments (derived from Era changes)
- `satpulse_clock_step_uncertain` (gauge) - 1 if step count is uncertain, 0 otherwise (from Era.StepCount())

### Future Metrics (from issue #112)

Additional observability data that could be implemented in future iterations:

- Time Dilution of Precision (TDOP) from UBX-NAV-DOP
- IO status: bytes received/transmitted, overrun errors from UBX-MON-IO/UBX-MON-COMMS
- Antenna status from UBX-MON-RF/UBX-MON-HW
- Jamming/spoofing status from UBX-SEC-SIG/UBX-MON-HW
- Informational messages from receiver (NMEA TXT, UBX-INF-*)
- Fix type (2D/3D/time only) from UBX-NAV-PVT

## Benefits

1. **Separation of Concerns**: Core logic is decoupled from observability
2. **Extensibility**: Easy to add new observability backends
3. **Performance**: Can disable unused observers
4. **Flexibility**: Users can choose which endpoints to expose
5. **Standard Monitoring**: Prometheus integration enables standard monitoring stacks

## Future Cleanup

After implementing the observability architecture, several areas could be simplified by leveraging the new interfaces:

1. **Refactor summary statistics generation** in `internal/mon/monitor.go` to use the `Sampler` interface instead of custom statistics code

2. **Implement MultiSampler** - a fan-out implementation similar to `MultiObserver` but only for the `Sampler` interface, useful for cases where only sample data is needed

3. **Move clock log file generation** to use the `Sampler` interface - this would require extending the `Sampler` interface to include a `ReopenLog()` method, allowing the clock logging to be implemented as another sampler with proper log rotation support

These changes would further consolidate observability concerns and reduce code duplication across the monitoring subsystem.

