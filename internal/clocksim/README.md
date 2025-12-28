# clocksim - Clock Simulation for Testing

This package provides infrastructure for testing PHC synchronization code without real hardware.

## Architecture

The simulation is layered to mirror the real hardware/software stack:

### Real System
```
GNSS PPS → PHC Hardware → ts.Clock → servo.Clock interface → phcsync.Controller
```

### Simulated System
```
PPSSimulator → VirtualClock → TestClock → servo.Clock interface → phcsync.Controller
```

## Components

### OscillatorSimulator
Models the behavior of a hardware oscillator (crystal/TCXO/OCXO).

- **Input**: True simulation time (seconds)
- **Output**: Fractional frequency error at that instant

The output is the **frequency error** (not integrated phase). For example, if output is `+1e-6`, the clock runs 1µs fast per second of true time. `RawClock` integrates this frequency error to produce phase.

**Key insight:** Oscillator provides frequency error; integration gives phase.

**Example oscillators:**
- `Perfect()` - Zero frequency error (runs at correct rate)
- `ConstantDrift(ppm)` - Constant frequency offset
- `WhiteFreqNoise(stddevPPM, seed)` - Random frequency variations
- `CombineOscillators(...)` - Combines multiple error sources

### PPSSimulator
Models GNSS PPS timing errors (atmospheric delays, jitter, etc).

- **Input**: True time (typically integer seconds)
- **Output**: Phase error in seconds (already integrated)

The phase error is added to the true time to get the actual PPS occurrence time.

**Key insight:** PPS provides phase error directly (not frequency error).

**Example simulators:**
- `PerfectPPS()` - No timing error
- `WhiteNoisePPS(stddev, seed)` - Gaussian jitter

### RawClock
Wraps an OscillatorSimulator and integrates frequency error to produce phase.
Represents the unadjusted PHC hardware oscillator with an initial offset.

Uses trapezoidal rule integration: `phase(t) = startPhase + integral(1 + freqError(t)) dt`

### VirtualClock
Simulates a disciplined PHC that can be adjusted via frequency offset and time steps.

**Key features:**
- Models Linux PHC implementation (tracks state from last adjustment)
- Lazy PPS timestamp generation as simulation time advances
- Timestamps queued in FIFO (like real hardware)
- No history required - only tracks last adjustment point

**Methods:**
- `AdvanceTo(newTime)` - Advance simulation time to newTime, generate timestamps for PPS events (panics if time goes backwards)
- `SetFreqOffset(ppb)` - Adjust PHC frequency (models `ADJ_SETFREQ`)
- `AdjTime(duration)` - Step the clock (models `ADJ_SETOFFSET` with realistic kernel delay)
- `ReadTimestamp()` - Read next timestamp from queue
- `TimestampAvailable()` - Check if timestamps are queued

**Realistic kernel behavior:**
- `AdjTime` simulates read-modify-write delay (~5µs ± 1µs jitter), causing imperfect time steps
- Simulation time advances during `AdjTime` to model kernel execution time

**Precision:**
- All phase values stored as `int64` nanoseconds (matching `ptime.Time`)
- Oscillator frequency errors integrated in float64 for accuracy, converted to ns at end
- Preserves nanosecond precision even at large time values (1.4e9 seconds)

### TestClock
Implements `servo.Clock` interface for testing.
Wraps VirtualClock and adds era tracking, mirroring how `ts.Clock` wraps `phc.Clock`.

**Era semantics:**
- Even eras (2, 4, 6...): Certain, timestamps can be compared
- Odd eras (3, 5, 7...): Uncertain, during clock step
- `AdjTime()` increments era twice (like real `ts.Clock`)

## Usage Example

```go
// Create oscillator running 10ppm fast
osc := ConstantDrift(10.0)
raw := NewRawClock(osc, 0)

// PPS with 10ns jitter
pps := WhiteNoisePPS(10e-9, 42)

// Create virtual clock
vclock := NewVirtualClock(raw, pps, 0, 500000) // start at t=0, ±500ppm max

// Create test clock (implements servo.Clock)
testClock := NewTestClock(vclock)

// Use with servo
s, _ := servo.New(testClock, logger)

// Simulation loop - advance time and process timestamps
simTime := 0.0
for simTime < 100.0 {
    simTime += 1.0  // Advance to next PPS
    vclock.AdvanceTo(simTime)

    // Read timestamp and pass to servo
    if vclock.TimestampAvailable() {
        ts, _, _ := testClock.ReadTimestampWithEra()
        gpsTime := ptime.Time(simTime * 1e9) // GPS time for this PPS
        s.Sample(gpsTime, ts, false)
    }
}
```

## Design Rationale

### Why layered architecture?
Mirrors real system structure, making tests realistic and making it easy to understand the mapping between simulation and reality.

### Why lazy PPS generation?
- GNSS phase error can be negative (PPS before nominal second)
- Can't predict PPS times far in advance
- Generates timestamps only when simulation time passes PPS time
- Ensures `computeVirtPhase()` always called with time ≤ current simTime

### Why track only last adjustment?
- Mirrors Linux kernel PHC implementation
- Avoids unbounded memory growth
- Sufficient for computing current clock state

### Why separate VirtualClock and TestClock?
- VirtualClock = PHC hardware simulation (no era tracking)
- TestClock = Software era tracking layer (like `ts.Clock`)
- Clean separation of concerns, matches real architecture

## Testing

Run tests:
```bash
go test ./internal/clocksim
```

Tests cover:
- Basic timestamp generation
- Frequency adjustments
- Time stepping
- Multiple PPS events
- PPS jitter

## Test Programs

Test programs use `//go:build ignore` to avoid compilation with the package:

- **servotest.go**: Simulates full servo convergence with realistic timing
  ```bash
  go run internal/clocksim/servotest.go
  ```

- **tstest.go**: Generates realistic PHC timestamps
  ```bash
  go run internal/clocksim/tstest.go
  ```

- **rawclocktest.go**: Tests RawClock integration and drift
  ```bash
  go run internal/clocksim/rawclocktest.go
  ```
