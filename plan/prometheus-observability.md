# Prometheus Metrics Implementation Plan

## Overview

This plan describes implementing Prometheus metrics support for SatPulse (GitHub issue [#78](https://github.com/jclark/satpulse/issues/78)) by creating a harmonized observability interface that unifies SSE events, Prometheus metrics, and potentially logging.

## Design Goals

1. Create a `Sampler` interface in the `internal/mon` package for clock synchronization samples
2. Create a unified `Observer` interface in the `internal/obs` package that extends `Sampler` and `gpsprot.Handler`
3. Move SSE-specific code out of core packages into dedicated observability implementations
4. Support multiple observability backends (SSE, Prometheus) through a fan-out pattern
5. Enable/disable UI and metrics endpoints via configuration
6. Support log rotation through the observer interface

## Architecture

### Core Interfaces

#### `internal/mon/sampler.go`
```go
package mon

// Sampler handles clock synchronization samples
type Sampler interface {
    // Sample reports a clock synchronization sample
    Sample(ref ptime.Time, local ptime.ClockTime, delayed bool, kind SampleKind, offset time.Duration, freq float64, syncState SyncState)
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
    gpsprot.Handler
    
    // Release cleans up resources (e.g., closes SSE channels)
    Release()
    
    // ReopenLog closes and reopen logs on SIGHUP, allowing log rotation
    ReopenLog()
}

// MultiObserver fans out calls to multiple observers
type MultiObserver struct {
    observers []Observer
}

func NewMultiObserver(observers ...Observer) *MultiObserver

```

### SSE Implementation

#### `internal/obs/sseobs/sse.go`
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
```

### Prometheus Implementation

#### `internal/obs/promobs/prometheus.go`
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
```

### Daemon Integration

#### Configuration Changes

Update `docs/config.md` and `configs/config-schema.json` to add per-endpoint configuration for HTTP endpoints.

#### `internal/daemon/config.go`
```go
type HTTPConfig struct {
    Listen  string `toml:"listen"`
    PProf   bool   `toml:"pprof"`
    UI      bool   `toml:"ui"`      // Serve web UI (default: true)
    Metrics bool   `toml:"metrics"`  // Serve /metrics endpoint (default: true)
}

// SetDefaults sets default values for HTTPConfig
func (c *HTTPConfig) SetDefaults() {
    if c.Listen != "" {
        // Only set defaults if this is a configured endpoint
        c.UI = true
        c.Metrics = true
    }
}

```

#### `internal/daemon/daemon.go` Changes

1. Add focused observer creation functions:
```go
// createPromObserver creates Prometheus observer if any HTTP endpoint needs metrics
func createPromObserver(cfg Config) *promobs.PrometheusObserver {
    for _, httpCfg := range cfg.HTTP {
        if httpCfg.Metrics {
            return promobs.New()
        }
    }
    return nil
}

// createSSEObserver creates SSE observer if any HTTP endpoint needs UI
func createSSEObserver(cfg Config, lg *slog.Logger) *sseobs.SSEObserver {
    for _, httpCfg := range cfg.HTTP {
        if httpCfg.UI {
            return sseobs.New(lg)
        }
    }
    return nil
}

// combineObservers combines individual observers into appropriate single observer
func combineObservers(promObs *promobs.PrometheusObserver, sseObs *sseobs.SSEObserver) obs.Observer {
    if promObs != nil && sseObs != nil {
        return obs.NewMultiObserver(promObs, sseObs)
    } else if promObs != nil {
        return promObs
    } else if sseObs != nil {
        return sseObs
    } else {
        return obs.Null{}
    }
}
```

2. Use the focused functions in main daemon code:
```go
// In the main daemon function
promObs := createPromObserver(cfg)
sseObs := createSSEObserver(cfg, lg)
observer := combineObservers(promObs, sseObs)

// Create broadcast from SSE observer's channel if needed (same as current code)
var sseBcast *bcast.Bcast[sse.Event]
if sseObs != nil {
    sseBcast = startBcast(ctx, lg, &wg, sseObs.Channel())
}

// Pass observer to dispatcher and monitor
dispatcher, err := gpsevent.NewDispatcher(lg, pktProcs, mon, ls, phcFlags, pulseWidth, observer, eventLogPath, tStart)

mon, err := mon.NewMonitor(servo, lg, mon.MonitorConfig{
    // ... existing fields ...
    Sampler: observer,  // Observer implements Sampler
})

// Pass broadcast and promObs to HTTP setup
err = startHTTP(ctx, lg, wg, cfg.HTTP, sseBcast, promObs)
```

3. Update HTTP server setup in `internal/daemon/http.go`:
```go
// Function signature only needs to add promObs parameter (already has broadcast)
func startHTTP(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg []HTTPConfig, b *bcast.Bcast[sse.Event], promObs *promobs.PrometheusObserver) error

// In startHTTP function, configure each endpoint based on its settings
for i, listener := range listeners {
    listener := listener
    mux := http.NewServeMux()
    
    if cfg[i].PProf {
        registerPprofHandlers(mux)
    }
    
    // Only register UI routes if enabled for this endpoint  
    if cfg[i].UI && b != nil {
        mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
            sseHandleRequest(ctx, lg, w, r, b, initEvent)
        })
        mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
            fileServer.ServeHTTP(w, r)
        })
    }
    
    // Only register metrics endpoint if enabled for this endpoint
    if cfg[i].Metrics && promObs != nil {
        mux.Handle("/metrics", promObs.Handler())
    }
    
    server := &http.Server{Handler: mux}
    servers[i] = server
    // ... rest of server setup
}
```

## Implementation Steps

1. **Create Sampler interface** in `internal/mon/sampler.go`
2. **Create obs package** with unified Observer interface and MultiObserver
3. **Implement SSE observer** in `internal/obs/sseobs/`:
   - Move SSE event creation from `gpsevent/dispatcher.go`
   - Move sample event creation from `mon/monitor.go`
4. **Implement basic Prometheus observer** in `internal/obs/promobs/`:
   - Start with simple metrics: sync_state and clock_offset_nanoseconds
   - Use custom registry (no global state)
   - Get basic infrastructure working
5. **Update gpsevent.Dispatcher**:
   - Replace `sseCh` with `Observer` interface
   - Remove SSE-specific code
   - Call observer methods instead
6. **Update mon.Monitor**:
   - Replace `sseCh` with `Sampler` interface
   - Remove SSE-specific code
   - Call sampler.Sample() instead
7. **Update daemon package**:
   - Add UI and Metrics configuration
   - Create appropriate observers based on config
   - Update HTTP server routing
8. **Update configuration**:
   - Add ui and metrics boolean fields to HTTPConfig in schema
   - Update docs/config.md to document the new fields
9. **Expand Prometheus metrics incrementally**:
   - First: Add remaining Sample-based metrics (frequency, histogram, stats)
   - Second: Add GPS message-based metrics (satellites, survey)
   - Third: Add system metrics (start time, last PPS)

## Prometheus Metrics Specification

Based on GitHub issue [#78](https://github.com/jclark/satpulse/issues/78) requirements and additional ideas from issue [#112](https://github.com/jclark/satpulse/issues/112):

### Clock Synchronization Metrics (from Sampler Interface)

- `satpulse_sync_state` (gauge) - Synchronization status:
  - 0 = out of sync
  - 1 = in sync
- `satpulse_clock_offset_nanoseconds` (gauge) - Current signed PHC/GPS offset in nanoseconds
- `satpulse_clock_offset_abs_nanoseconds` (histogram) - Histogram of absolute offset values in nanoseconds
- `satpulse_clock_frequency_ppb` (gauge) - Current frequency adjustment in parts per billion

### Statistics Metrics (from Monitor stats)

Current stats that are logged over intervals (default 30s):

- `satpulse_offset_max_abs_nanoseconds` (gauge) - Maximum absolute offset in current interval
- `satpulse_offset_mean_abs_nanoseconds` (gauge) - Mean absolute offset in current interval  
- `satpulse_offset_rms_nanoseconds` (gauge) - RMS offset in current interval
- `satpulse_frequency_mean_ppb` (gauge) - Mean frequency adjustment in current interval
- `satpulse_frequency_stddev_ppb` (gauge) - Standard deviation of frequency in current interval
- `satpulse_samples_missing_total` (counter) - Total missing samples in current interval
- `satpulse_samples_outlier_total` (counter) - Total outlier samples in current interval

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

3. **Move clock log file generation** to use the `Sampler` interface - this would require extending the `Sampler` interface to include a `ReopenLog()` method, allowing the clock logging to be implemented as another sampler

These changes would further consolidate observability concerns and reduce code duplication across the monitoring subsystem.

