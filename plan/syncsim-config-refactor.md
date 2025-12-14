# Syncsim Configuration Refactoring Plan

## Overview

This document describes the refactoring of how `syncsimcmd.go` provides configuration to `syncsim.go`. The goal is to:

1. Read configuration from a TOML file (like `daemon/config.go`)
2. Remove command-line options that set Config fields
3. Organize Config into logical sections with comprehensive documentation
4. Add `check` annotations for validation
5. Generate a JSON schema
6. Keep consistency with tsgen - both commands share the same config format

---

## Phase 1: Unify Config Structure

This phase removes `HWConfig`, unifies both commands on a single `Config` type,
and changes the CLI interface. All changes must be done together to maintain
a compilable codebase.

### Current State

**Current Config Structure** ([simconfig.go](internal/syncsim/simconfig.go)):

```go
type Config struct {
    Duration          float64         // should be CLI arg, not config
    PHC               PHCConfig       // hardware section - KEEP
    GPS               GPSConfig       // hardware section - KEEP
    MinDelay          float64         // pulse timing
    MaxDelay          float64         // pulse timing
    MsgDelay          float64         // message timing
    MsgJitter         float64         // message timing
    PulseWidth        float64         // pulse timing
    PrePulseTime      float64         // message timing
    PostPulseMsgDelay float64         // message timing
    SawtoothMsgType   gpsprot.TimeRef // message timing
    ToggleTimes       []float64       // fault injection
    Outlier           OutlierConfig   // fault injection
    Shift             ShiftConfig     // fault injection
    TSLog             io.Writer       // NOT config - runtime param
}
```

**Current tsgen Interface** ([tsgen.go](internal/syncsim/cmd/tsgen/tsgen.go)):
```
tsgen <hw-config.toml> <duration-seconds>
```
tsgen uses only `HWConfig` (PHC + GPS sections) with duration as positional arg.

**Current syncsim Interface** ([syncsimcmd.go](internal/syncsim/cmd/syncsim/syncsimcmd.go)):
The current CLI has ~50 flags - this needs simplification.

### Design Principles

1. **Duration is a CLI argument**, not config - matches tsgen pattern
2. **Single Config type** - no separate HWConfig; both commands load the same type
3. **Same TOML file works for both** - tsgen loads but ignores sections it doesn't need
4. **TSLog is a runtime parameter**, not config
5. **DisallowUnknownFields for both** - catches typos; works because both load same type

### Config Changes in This Phase

Minimal changes to `Config` struct:
1. Remove `Duration` field (becomes CLI positional arg)
2. Remove `TSLog io.Writer` field (becomes `Simulate` parameter)
3. Add `Sync phcsync.Config` field (controller config loaded from TOML `[sync]` section)

```go
type Config struct {
    PHC               PHCConfig       // unchanged
    GPS               GPSConfig       // unchanged
    Sync              phcsync.Config  // NEW: controller config from TOML
    MinDelay          float64         // unchanged (reorganized in Phase 3)
    MaxDelay          float64         // unchanged
    MsgDelay          float64         // unchanged
    MsgJitter         float64         // unchanged
    PulseWidth        float64         // unchanged
    PrePulseTime      float64         // unchanged
    PostPulseMsgDelay float64         // unchanged
    SawtoothMsgType   gpsprot.TimeRef // unchanged
    ToggleTimes       []float64       // unchanged
    Outlier           OutlierConfig   // unchanged
    Shift             ShiftConfig     // unchanged
    // Duration removed - now CLI arg
    // TSLog removed - now Simulate parameter
}
```

### Target CLI Interface

```
syncsim [options] <config.toml> <duration>
tsgen   [options] <config.toml> <duration>
```

**syncsim runtime-only options:**
```
  -h, --help           Show help
  -V, --version        Show version
  --debug              Enable debug logging
  --stats N            Log stats every N seconds (0 to disable)
  --clock-log PATH     Write clock offsets to PATH
  --ts-log PATH        Write PHC timestamps to PATH (JSON Lines)
```

**Removed flags** (all hardware and simulation parameter flags):
- `--hw`, `--duration`
- `--phc-*` flags (freq-offset, drift, white, flicker, random-walk)
- `--jitter`, `--ar1-*`, `--gps-random-walk`, `--sawtooth*`
- `--min-delay`, `--max-delay`, `--msg-delay`, `--msg-jitter`, `--pulse-width`
- `--tracking-*` flags
- `--toggle`, `--outlier-*`

### Implementation Steps

#### 1.1: Update Config struct
1. Remove `Duration` field from `Config` (becomes CLI positional arg)
2. Remove `TSLog io.Writer` from `Config` (becomes `Simulate` parameter)
3. Add `Sync phcsync.Config` field to `Config`
4. Update `DefaultConfig()` to include `Sync: phcsync.DefaultConfig()`

#### 1.2: Update Simulate function
1. Change signature from:
   ```go
   func Simulate(observers []obs.Observer, phcCfg phcsync.Config, simCfg Config, curTime *time.Time, lg *slog.Logger) (Stats, error)
   ```
   to:
   ```go
   func Simulate(observers []obs.Observer, cfg Config, duration float64, tsLog io.Writer, curTime *time.Time, lg *slog.Logger) (Stats, error)
   ```
2. Replace `phcCfg` references with `cfg.Sync`
3. Replace `simCfg.Duration` with `duration` parameter
4. Replace `simCfg.TSLog` with `tsLog` parameter

#### 1.3: Update config loading
1. Rename `LoadHWConfig` → `LoadConfig`
2. Change signature to load into `*Config` instead of `*HWConfig`
3. Remove `HWConfig` type entirely

#### 1.4: Update syncsimcmd.go
1. Change CLI from flags-based to positional args: `syncsim [options] <config.toml> <duration>`
2. Remove all hardware/simulation parameter flags (~40 flags removed)
3. Keep runtime-only flags: `--debug`, `--stats`, `--clock-log`, `--ts-log`
4. Load full `Config` from TOML (includes `[sync]` section)
5. Update `Simulate` call with new signature

#### 1.5: Update tsgen
1. Change from `syncsim.HWConfig` to `syncsim.Config`
2. Change from `syncsim.LoadHWConfig` to `syncsim.LoadConfig`
3. Continue using only `cfg.PHC` and `cfg.GPS` fields
4. Extra sections (sync, pulse, msg, fault) silently ignored due to `DisallowUnknownFields`
   loading the same `Config` type

#### 1.6: Update tests
1. Update any tests that use `HWConfig` to use `Config`
2. Update any tests that call `Simulate` with old signature
3. Verify all tests pass

#### 1.7: Add check validation on load
1. `LoadConfig` calls `cfg.Sync.Validate()` (phcsync.Config already has check annotations)
2. Return validation errors from LoadConfig

---

## Phase 2: Add Unit Types (Nanoseconds, PPB)

This phase adds type-safe unit types to clocksim and updates config fields
to use them, making the config self-documenting.

### Unit Type Definitions

**Location:** `internal/clocksim/units.go`

```go
// Nanoseconds is a float64 representing a duration in nanoseconds.
// Float64 allows sub-nanosecond precision for noise parameters (e.g., AR1.Sigma = 0.25ns).
type Nanoseconds float64

// Seconds converts Nanoseconds to seconds (float64).
func (n Nanoseconds) Seconds() float64 {
    return float64(n) / 1e9
}

// PPB is a float64 representing a fractional frequency in parts-per-billion.
// 1 PPB = 1e-9 relative frequency offset.
type PPB float64

// Fractional converts PPB to dimensionless fractional frequency.
// Replaces manual `ppb / 1e9` or `ppb * 1e-9` conversions in clocksim code.
func (p PPB) Fractional() float64 {
    return float64(p) / 1e9
}
```

**Design principles:**
- clocksim internally uses `float64` for seconds - no changes to simulation logic
- `Nanoseconds` and `PPB` are distinct types requiring explicit conversion at API boundaries
- Conversion methods replace scattered `/ 1e9` and `* 1e-9` throughout clocksim
- `time.Duration` for exact integer ns arithmetic (e.g., `AdjTime`, timestamps)

### Implementation Steps

#### 2.1: Add Nanoseconds Type
1. Create `internal/clocksim/units.go` with `Nanoseconds` type and `.Seconds()` method
2. Update clocksim function signatures that take nanosecond parameters:
   - `JitterGPS(stddev time.Duration)` → `JitterGPS(stddev Nanoseconds)`
   - `SinusoidGPS(ampNs, ...)` → `SinusoidGPS(amp Nanoseconds, ...)`
   - `AR1ColoredNoiseGPS(..., noiseStddevNs float64)` → `AR1ColoredNoiseGPS(..., noiseStddev Nanoseconds)`
   - `ShiftPPS(..., shift time.Duration)` → `ShiftPPS(..., shift Nanoseconds)`
   - `SingleOutlierPPS(..., offset time.Duration)` → `SingleOutlierPPS(..., offset Nanoseconds)`
3. Update syncsim to use `Nanoseconds` in config types (Jitter, Sigma for phase, Amp, Offset)
4. Update all call sites in syncsim.go
5. Run tests to verify

#### 2.2: Add PPB Type
1. Add `PPB` type to `internal/clocksim/units.go` with `.Fractional()` method
2. Update clocksim function signatures that take ppb parameters:
   - `WhiteNoiseOsc(stddevPPB float64)` → `WhiteNoiseOsc(stddev PPB)`
   - `FlickerNoiseOsc(stddevPPB float64)` → `FlickerNoiseOsc(stddev PPB)`
   - `RandomWalkOsc(stddevPPB float64)` → `RandomWalkOsc(stddev PPB)`
   - `SinusoidOsc(ampPPB, ...)` → `SinusoidOsc(amp PPB, ...)`
   - `FreqOffsetOsc(ppb float64)` → `FreqOffsetOsc(offset PPB)`
   - `AR1FMGPS(..., sigmaPPB float64)` → `AR1FMGPS(..., sigma PPB)`
3. Update syncsim to use `PPB` in config types (FreqOffset, WhiteNoise, FlickerNoise)
4. Update all call sites in syncsim.go
5. Run tests to verify

---

## Phase 3: Restructure Config for Nice TOML

This phase reorganizes flat fields into logical nested sections. Each step
includes the struct definition to add and instructions for wiring it up.
Each step should compile and pass tests before moving to the next.

After this phase, Config will have clean nested sections:
```go
type Config struct {
    PHC   PHCConfig      `toml:"phc"`   // oscillator error model
    GPS   GPSConfig      `toml:"gps"`   // GPS timing error model
    Sync  phcsync.Config `toml:"sync"`  // controller config (from Phase 1)
    Pulse PulseConfig    `toml:"pulse"` // NEW: replaces MinDelay, MaxDelay, PulseWidth
    Msg   MsgConfig      `toml:"msg"`   // NEW: replaces MsgDelay, MsgJitter, PrePulseTime, etc.
    Fault FaultConfig    `toml:"fault"` // NEW: replaces ToggleTimes, Outlier, Shift
}
```

### Step 3.1: Add Seconds type alias

Add to `internal/syncsim/simconfig.go`:

```go
// Seconds is a true alias for float64, used for documentation in config structs.
// No conversion needed - clocksim uses float64 for seconds internally.
type Seconds = float64
```

### Step 3.2: Create SawtoothType enum

Add to `internal/syncsim/simconfig.go`:

```go
// SawtoothType specifies how sawtooth correction messages are delivered.
type SawtoothType int

const (
    SawtoothPrePulse  SawtoothType = iota // correction before pulse (UBX-TIM-TP)
    SawtoothPostPulse                      // correction after pulse (UBX-TIM-TOS)
    SawtoothNone                           // no correction messages
)

func (s SawtoothType) String() string {
    switch s {
    case SawtoothPrePulse:
        return "prepulse"
    case SawtoothPostPulse:
        return "postpulse"
    case SawtoothNone:
        return "none"
    default:
        return fmt.Sprintf("SawtoothType(%d)", s)
    }
}

func (s SawtoothType) MarshalText() ([]byte, error) {
    return []byte(s.String()), nil
}

func (s *SawtoothType) UnmarshalText(text []byte) error {
    switch string(text) {
    case "prepulse":
        *s = SawtoothPrePulse
    case "postpulse":
        *s = SawtoothPostPulse
    case "none":
        *s = SawtoothNone
    default:
        return fmt.Errorf("invalid sawtooth type: %q (must be prepulse, postpulse, or none)", text)
    }
    return nil
}
```

### Step 3.3: Create PulseConfig

Add this struct, then:
1. Add `Pulse PulseConfig` field to Config
2. Remove flat fields: `MinDelay`, `MaxDelay`, `PulseWidth`
3. Update `Simulate` to use `cfg.Pulse.*`
4. Update `DefaultConfig()` with defaults shown below
5. Verify tests pass

```go
// PulseConfig configures PPS pulse timing characteristics.
type PulseConfig struct {
    // MinDelay is the minimum delay from the true GPS second to when the
    // PPS pulse edge is delivered to the PHC timestamper. Real GPS receivers
    // have internal processing delays that prevent the pulse from arriving
    // exactly at the second boundary. The actual delay for each pulse is
    // uniformly distributed between MinDelay and MaxDelay.
    // Typical values are 5-50 microseconds (0.000005 to 0.00005).
    MinDelay Seconds `toml:"minDelay" check:">=0,<1"`

    // MaxDelay is the maximum delay from the true GPS second to when the
    // PPS pulse edge is delivered to the PHC timestamper. Must be >= MinDelay.
    MaxDelay Seconds `toml:"maxDelay" check:">=0,<1"`

    // Width is the pulse width for dual-edge timestamping mode.
    // When Width > 0, the simulator generates both rising and falling edges.
    // When Width = 0, only rising edges are generated (single-edge mode).
    // Typical GPS receivers use 100ms (0.1) pulse width.
    Width Seconds `toml:"width" check:">=0,<1"`
}

// Default:
Pulse: PulseConfig{
    MinDelay: 5e-6,   // 5 µs
    MaxDelay: 250e-6, // 250 µs
    Width:    0,      // single-edge mode
},
```

### Step 3.4: Create MsgConfig

Add this struct, then:
1. Add `Msg MsgConfig` field to Config
2. Remove flat fields: `MsgDelay`, `MsgJitter`, `PrePulseTime`, `PostPulseMsgDelay`, `SawtoothMsgType`
3. Update `Simulate` to use `cfg.Msg.*`
4. Update `DefaultConfig()`
5. Verify tests pass

```go
// MsgConfig configures GPS message delivery timing.
// GPS receivers send time messages (e.g., UBX-NAV-PVT) after each PPS pulse.
type MsgConfig struct {
    // Delay is the mean delay from the PPS pulse to when the GPS time message
    // is received. Real GPS receivers take 50-250ms to transmit after the pulse.
    Delay Seconds `toml:"delay" check:">=0,<1"`

    // Jitter is the standard deviation of the message arrival time.
    Jitter Seconds `toml:"jitter" check:">=0,<1"`

    // SawtoothType specifies how sawtooth correction messages are delivered:
    //   - SawtoothPrePulse ("prepulse"): Correction arrives before the pulse (UBX-TIM-TP)
    //   - SawtoothPostPulse ("postpulse"): Correction arrives after the pulse (UBX-TIM-TOS)
    //   - SawtoothNone ("none"): No correction messages
    SawtoothType SawtoothType `toml:"sawtoothType"`

    // PrePulseTime is the time before the PPS edge when a PrePulse correction
    // message is delivered. Only used when SawtoothType is SawtoothPrePulse.
    PrePulseTime Seconds `toml:"prePulseTime" check:">0,<1"`

    // PostPulseDelay is the delay after the PPS edge when a PostPulse correction
    // message is delivered. Only used when SawtoothType is SawtoothPostPulse.
    PostPulseDelay Seconds `toml:"postPulseDelay" check:">=0,<1"`
}

// Default:
Msg: MsgConfig{
    Delay:          0.1,              // 100 ms
    Jitter:         0.01,             // 10 ms stddev
    SawtoothType:   SawtoothPrePulse,
    PrePulseTime:   0.95,             // 950 ms before pulse
    PostPulseDelay: 0.1,              // 100 ms after pulse
},
```

### Step 3.5: Create FaultConfig

Add these structs, then:
1. Add `Fault FaultConfig` field to Config
2. Remove flat fields: `ToggleTimes`, `Outlier`, `Shift`
3. Convert `ToggleTimes` (absolute times) → `Fault.Toggle.Durations` (relative durations)
4. Update `Simulate` to use `cfg.Fault.*`
5. Update `DefaultConfig()`
6. Verify tests pass

```go
// FaultConfig configures fault injection for testing controller resilience.
type FaultConfig struct {
    Toggle  ToggleConfig  `toml:"toggle"`  // signal outages
    Outlier OutlierConfig `toml:"outlier"` // phase outliers
    Shift   ShiftConfig   `toml:"shift"`   // gradual phase shifts
}

// ToggleConfig configures signal outage simulation.
type ToggleConfig struct {
    // Durations is a sequence of relative durations in seconds.
    // The simulation alternates between signal-on and signal-off states.
    // First duration is signal-on, second is signal-off, etc.
    // Example: [60, 10, 60] means: 60s on, 10s off, 60s on.
    Durations []Seconds `toml:"durations"`
}

// OutlierConfig configures discrete phase outlier injection.
type OutlierConfig struct {
    // Times is a list of simulation times at which to inject an outlier.
    Times []Seconds `toml:"times"`

    // Offset is the magnitude of the phase offset to inject.
    // Typical multipath outliers are 1000-10000 ns (1-10 µs).
    Offset Nanoseconds `toml:"offset" check:">=0"`
}

// ShiftConfig configures gradual phase shift injection.
type ShiftConfig struct {
    // StartTime is the simulation time when the shift begins.
    StartTime Seconds `toml:"startTime" check:">=0"`

    // Ramp is the ramp-up/ramp-down duration.
    Ramp Seconds `toml:"ramp" check:">=0"`

    // Duration is the total duration including both ramp periods.
    Duration Seconds `toml:"duration" check:">=0"`

    // Shift is the maximum phase shift magnitude at the plateau.
    Shift Nanoseconds `toml:"shift"`
}

// Default:
Fault: FaultConfig{
    Outlier: OutlierConfig{
        Offset: 2000, // 2 µs
    },
},
```

### Step 3.6: Enhance PHCConfig documentation

Update existing `PHCConfig` with comprehensive doc comments and `check` annotations.
Also update `Sinusoid` struct. No structural changes, just documentation.

```go
// PHCConfig configures the PTP Hardware Clock oscillator error model.
// The PHC's instantaneous frequency is modeled as:
//   nominal_freq * (1 + freq_offset + drift*t + white + flicker + random_walk + sinusoids)
// All frequency parameters are in parts-per-billion (ppb) relative to nominal.
type PHCConfig struct {
    // FreqOffset is the constant frequency offset in ppb.
    // Typical values: ±1000-10000 ppb for factory-trimmed oscillators.
    FreqOffset PPB `toml:"freqOffset" check:">=-1000000,<=1000000"`

    // Drift is the linear frequency drift rate in ppb per day.
    Drift float64 `toml:"drift" check:">=-1000,<=1000"` // ppb/day

    // WhiteNoise is the standard deviation of white frequency noise in ppb.
    // Typical values are 1-20 ppb for good crystals.
    WhiteNoise PPB `toml:"whiteNoise" check:">=0,<=10000"`

    // FlickerNoise is the standard deviation of flicker (1/f) frequency noise in ppb.
    // Typical values are 0.1-5 ppb for crystals.
    FlickerNoise PPB `toml:"flickerNoise" check:">=0,<=10000"`

    // RandomWalk is the random walk FM coefficient in ppb/√s.
    // Typical values are 0.01-1 ppb/√s.
    RandomWalk float64 `toml:"randomWalk" check:">=0,<=10000"` // ppb/√s

    // Sinusoid is a list of sinusoidal frequency modulation components.
    Sinusoid []Sinusoid `toml:"sinusoid"`
}

// Sinusoid configures a sinusoidal modulation component.
// Contribution: Amp * sin(2π * (t/Period + PhaseInit))
type Sinusoid struct {
    Period    Seconds `toml:"period" check:">0,<=1000000"`    // oscillation period
    Amp       float64 `toml:"amp" check:">=0,<=10000"`        // ppb for PHC, ns for GPS
    PhaseInit float64 `toml:"phaseInit" check:">=0,<1"`       // initial phase [0,1)
}
```

### Step 3.7: Enhance GPSConfig documentation

Update existing `GPSConfig` and nested types with comprehensive doc comments
and `check` annotations. No structural changes, just documentation.

```go
// GPSConfig configures the GPS PPS timing error model.
// All phase parameters are in nanoseconds.
type GPSConfig struct {
    // Jitter is white phase noise stddev in nanoseconds.
    // Typical: 0.1-5 ns survey-grade, 5-50 ns consumer-grade.
    Jitter Nanoseconds `toml:"jitter" check:">=0,<=1000"`

    // AR1 is a list of AR(1) colored phase noise processes.
    AR1 []AR1Config `toml:"ar1"`

    // AR1FM is an AR(1) frequency modulation process (sigma in ppb).
    AR1FM AR1Config `toml:"ar1FM"`

    // RandomWalk FM coefficient in ppb/√s.
    RandomWalk float64 `toml:"randomWalk" check:">=0,<=1000"` // ppb/√s

    // Drift configures bounded drift (2nd-order Gauss-Markov).
    Drift DriftConfig `toml:"drift"`

    // Resonator configures a damped oscillator on phase.
    Resonator ResonatorConfig `toml:"resonator"`

    // Sinusoid is a list of sinusoidal phase modulation components.
    Sinusoid []Sinusoid `toml:"sinusoid"`

    // Sawtooth configures GPS receiver quantization sawtooth error.
    Sawtooth SawtoothConfig `toml:"sawtooth"`
}

// AR1Config configures an AR(1) noise process.
type AR1Config struct {
    Tau   Seconds `toml:"tau" check:">=0,<=1000000"`   // correlation time constant
    Sigma float64 `toml:"sigma" check:">=0,<=10000"`   // RMS (ns for phase, ppb for FM)
}

// DriftConfig configures bounded drift (Carpenter-Lee model).
type DriftConfig struct {
    Tau   Seconds     `toml:"tau" check:">=0,<=1000000"`
    Sigma Nanoseconds `toml:"sigma" check:">=0,<=10000"`
    Zeta  float64     `toml:"zeta" check:">=0,<=10"`    // damping ratio, default 0.7
}

// ResonatorConfig configures a damped harmonic oscillator.
type ResonatorConfig struct {
    Period Seconds     `toml:"period" check:">=0,<=1000000"`
    Sigma  Nanoseconds `toml:"sigma" check:">=0,<=10000"`
    Zeta   float64     `toml:"zeta" check:">=0,<=10"`   // damping ratio, default 0.3
}

// SawtoothConfig configures GPS receiver quantization sawtooth error.
type SawtoothConfig struct {
    Amp           Nanoseconds `toml:"amp" check:">=0,<=1000"`    // amplitude (0.5e9/f_osc)
    PhaseInit     float64     `toml:"phaseInit" check:">=0,<1"`  // initial phase [0,1)
    InternalClock Sinusoid    `toml:"internalClock"`             // oscillator error model
}
```

### Step 3.8: Update test configs and examples

1. Convert existing `.toml` test files to new nested format
2. Create example configs demonstrating different scenarios
3. Verify all tests pass

---

## Phase 4: Cleanups

### Phase 4.1: Unify Default Config

Consolidate `DefaultConfig()` and `DefaultZeroConfig()` into a single `DefaultConfig()` that:
- Produces zero noise/errors by default for PHC, GPS, and Fault sections
- Includes reasonable defaults for non-noise parameters (like sawtooth.PhaseInit, sawtooth.InternalClock)
- Provides sensible defaults for Pulse and Msg sections (operational parameters)

**Changes:**

1. Remove `DefaultZeroConfig()`, `DefaultZeroGPSConfig()`, `DefaultZeroSawtoothConfig()`
2. Rename current `DefaultZeroConfig()` behavior to be the new `DefaultConfig()`
3. Update `DefaultPHCConfig()` to return zero noise but keep FreqOffset=2000 default
   (deterministic offset, not noise - doesn't affect ADEV shape)
4. Update `DefaultGPSConfig()` to return zero noise but keep sawtooth defaults
5. Add `DefaultPulseConfig()`:
   ```go
   func DefaultPulseConfig() PulseConfig {
       return PulseConfig{
           MinDelay: 5e-6,   // 5 µs
           MaxDelay: 250e-6, // 250 µs
           Width:    0,      // single-edge mode
       }
   }
   ```
6. Add `DefaultMsgConfig()`:
   ```go
   func DefaultMsgConfig() MsgConfig {
       return MsgConfig{
           Delay:          0.1,              // 100 ms
           Jitter:         0.01,             // 10 ms stddev
           SawtoothType:   SawtoothPrePulse,
           PrePulseTime:   0.95,             // 950 ms before pulse
           PostPulseDelay: 0.1,              // 100 ms after pulse
       }
   }
   ```
7. Add `DefaultFaultConfig()` returning zero struct (no faults by default)
8. Remove default Offset from `OutlierConfig` - user must specify if using outliers
9. Update `DefaultConfig()` to use all the above
10. Update tsgen to use `DefaultConfig()` instead of `DefaultZeroConfig()`
11. Remove redundant InternalClock defaulting logic from tsgen.go (lines 60-65)
12. Update `TestLoadConfigMergesDefaults` to reflect new defaults

**Rationale:**
- Both commands use identical defaulting, making behavior predictable
- Zero noise by default is explicit - user specifies exactly what noise they want
- Non-noise defaults (pulse timing, message timing) are still convenient
- tsgen's `IsZero()` checks continue to work for deciding what to output

### Phase 4.2: Add --show-default-config Option

Add a command-line option to output the default TOML configuration with comments,
helping users understand the expected config structure since CLI flags were removed.

**Naming convention:** Following satpulsetool pattern (`--show-config` / `-c`), use:
- `--show-default-config` / `-C`

Short option `-C` is available in both syncsim and daemon (daemon uses: -h, -v, -s, -w, -V, -d).

**Files to modify:**
1. [syncsimcmd.go](internal/syncsim/cmd/syncsim/syncsimcmd.go) - Add flag and handler
2. [simconfig.go](internal/syncsim/simconfig.go) - Add `comment:"..."` tags to struct fields

**Changes to syncsimcmd.go:**

1. Add flag:
   ```go
   flags.BoolVarP(&showDefaultConfig, "show-default-config", "C", false, "print default config as TOML and exit")
   ```

2. Handle the flag (after version check, before positional arg check):
   ```go
   if showDefaultConfig {
       cfg := syncsim.DefaultConfig()
       if err := toml.NewEncoder(os.Stdout).Encode(cfg); err != nil {
           cmd.ErrPrintln("syncsim", err)
           os.Exit(1)
       }
       os.Exit(0)
   }
   ```

3. Add TOML import:
   ```go
   toml "github.com/pelletier/go-toml/v2"
   ```

4. Update help text to mention the option:
   ```go
   fmt.Fprintf(os.Stderr, "Usage: syncsim [options] <config.toml> <duration>\n\n"+
       "Use -C/--show-default-config to see the default configuration.\n\nOptions:\n%s", flags.FlagUsages())
   ```

**Changes to simconfig.go:**

Add `comment:"..."` tags to struct fields. The go-toml/v2 library includes
these as comments above each field in the generated TOML. Derive short
comment text from the existing doc comments.

Example:
```go
FreqOffset clocksim.PPB `toml:"freqOffset" check:"..." comment:"Constant frequency offset (ppb)"`
```

Note: phcsync.Config (in internal/phcsync/) may also need comment tags.

### Phase 4.3: Separate Sinusoid Types (DONE)

Split the shared `Sinusoid` struct into type-safe variants:
- `FreqSinusoid` with `Amp clocksim.PPB` for frequency modulation (PHC sinusoids, GPS internal clock)
- `PhaseSinusoid` with `Amp clocksim.Nanoseconds` for phase modulation (GPS sinusoids)

Also fixed zeta defaults:
- Added `DefaultDriftConfig()` returning `DriftConfig{Zeta: 0.7}`
- Added `DefaultResonatorConfig()` returning `ResonatorConfig{Zeta: 0.3}`
- Removed "(default X)" from zeta comment tags

### Phase 4.4: Other Cleanups

Can we merge tsgen and syncsim commands?

---

## Phase 5: Generate JSON Schema

### Target Output

Output to `configs/syncsim-schema.json`. Reference the existing `config-schema.json` for the `sync` section.

### JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "syncsim configuration",
  "type": "object",
  "properties": {
    "duration": {
      "type": "number",
      "exclusiveMinimum": 0,
      "maximum": 1000000,
      "description": "Simulation duration in seconds"
    },
    "phc": { "$ref": "#/definitions/phcConfig" },
    "gps": { "$ref": "#/definitions/gpsConfig" },
    "sync": { "$ref": "config-schema.json#/properties/sync" },
    "pulse": { "$ref": "#/definitions/pulseConfig" },
    "msg": { "$ref": "#/definitions/msgConfig" },
    "fault": { "$ref": "#/definitions/faultConfig" }
  },
  "definitions": {
    "phcConfig": {
      "type": "object",
      "description": "PHC oscillator error model configuration",
      "properties": {
        "freqOffset": {
          "type": "number",
          "minimum": -1000000,
          "maximum": 1000000,
          "description": "Constant frequency offset in ppb"
        },
        "drift": {
          "type": "number",
          "minimum": -1000,
          "maximum": 1000,
          "description": "Linear frequency drift rate in ppb/day"
        },
        "whiteNoise": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "White frequency noise stddev in ppb"
        },
        "flickerNoise": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "Flicker (1/f) frequency noise stddev in ppb"
        },
        "randomWalk": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "Random walk FM coefficient in ppb/√s"
        },
        "sinusoid": {
          "type": "array",
          "items": { "$ref": "#/definitions/sinusoid" }
        }
      }
    },
    "sinusoid": {
      "type": "object",
      "properties": {
        "period": {
          "type": "number",
          "exclusiveMinimum": 0,
          "maximum": 1000000,
          "description": "Oscillation period in seconds"
        },
        "amp": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "Amplitude (ppb for PHC, ns for GPS)"
        },
        "phaseInit": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Initial phase as fraction of cycle [0,1)"
        }
      }
    },
    "gpsConfig": {
      "type": "object",
      "description": "GPS PPS timing error model configuration",
      "properties": {
        "jitter": {
          "type": "number",
          "minimum": 0,
          "maximum": 1000,
          "description": "White phase noise stddev in nanoseconds"
        },
        "ar1": {
          "type": "array",
          "items": { "$ref": "#/definitions/ar1Config" }
        },
        "ar1FM": { "$ref": "#/definitions/ar1Config" },
        "randomWalk": {
          "type": "number",
          "minimum": 0,
          "maximum": 1000,
          "description": "Random walk FM coefficient in ppb/√s"
        },
        "drift": { "$ref": "#/definitions/driftConfig" },
        "resonator": { "$ref": "#/definitions/resonatorConfig" },
        "sinusoid": {
          "type": "array",
          "items": { "$ref": "#/definitions/sinusoid" }
        },
        "sawtooth": { "$ref": "#/definitions/sawtoothConfig" }
      }
    },
    "ar1Config": {
      "type": "object",
      "properties": {
        "tau": {
          "type": "number",
          "minimum": 0,
          "maximum": 1000000,
          "description": "Correlation time constant in seconds"
        },
        "sigma": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "Steady-state RMS (ns for phase, ppb for FM)"
        }
      }
    },
    "driftConfig": {
      "type": "object",
      "properties": {
        "tau": {
          "type": "number",
          "minimum": 0,
          "maximum": 1000000,
          "description": "Characteristic timescale in seconds"
        },
        "sigma": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "RMS phase deviation in nanoseconds"
        },
        "zeta": {
          "type": "number",
          "minimum": 0,
          "maximum": 10,
          "description": "Damping ratio (default 0.7)"
        }
      }
    },
    "resonatorConfig": {
      "type": "object",
      "properties": {
        "period": {
          "type": "number",
          "minimum": 0,
          "maximum": 1000000,
          "description": "Natural oscillation period in seconds"
        },
        "sigma": {
          "type": "number",
          "minimum": 0,
          "maximum": 10000,
          "description": "RMS phase deviation in nanoseconds"
        },
        "zeta": {
          "type": "number",
          "minimum": 0,
          "maximum": 10,
          "description": "Damping ratio (default 0.3)"
        }
      }
    },
    "sawtoothConfig": {
      "type": "object",
      "properties": {
        "amp": {
          "type": "number",
          "minimum": 0,
          "maximum": 1000,
          "description": "Sawtooth amplitude in nanoseconds"
        },
        "phaseInit": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Initial phase [0,1), default 0.5"
        },
        "internalClock": {
          "$ref": "#/definitions/sinusoid",
          "description": "GPS internal oscillator error model"
        }
      }
    },
    "pulseConfig": {
      "type": "object",
      "description": "PPS pulse timing configuration",
      "properties": {
        "minDelay": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Minimum pulse delay in seconds"
        },
        "maxDelay": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Maximum pulse delay in seconds"
        },
        "width": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Pulse width in seconds (0 for single-edge)"
        }
      }
    },
    "msgConfig": {
      "type": "object",
      "description": "GPS message timing configuration",
      "properties": {
        "delay": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Mean message delay after pulse in seconds"
        },
        "jitter": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Message delay stddev in seconds"
        },
        "sawtoothType": {
          "type": "string",
          "enum": ["prepulse", "postpulse", "none"],
          "description": "How sawtooth correction is delivered"
        },
        "prePulseTime": {
          "type": "number",
          "exclusiveMinimum": 0,
          "exclusiveMaximum": 1,
          "description": "Time before pulse for prepulse message"
        },
        "postPulseDelay": {
          "type": "number",
          "minimum": 0,
          "exclusiveMaximum": 1,
          "description": "Delay after pulse for postpulse message"
        }
      }
    },
    "faultConfig": {
      "type": "object",
      "description": "Fault injection configuration",
      "properties": {
        "toggle": {
          "type": "object",
          "properties": {
            "durations": {
              "type": "array",
              "items": { "type": "number", "exclusiveMinimum": 0 },
              "description": "Alternating on/off durations in seconds"
            }
          }
        },
        "outlier": {
          "type": "object",
          "properties": {
            "times": {
              "type": "array",
              "items": { "type": "number" },
              "description": "Times to inject outliers"
            },
            "offset": {
              "type": "string",
              "pattern": "^-?[0-9]+(\\.[0-9]+)?(ns|µs|us|ms|s)$",
              "description": "Outlier offset magnitude"
            }
          }
        },
        "shift": {
          "type": "object",
          "properties": {
            "startTime": {
              "type": "number",
              "minimum": 0,
              "description": "When shift begins"
            },
            "ramp": {
              "type": "string",
              "pattern": "^-?[0-9]+(\\.[0-9]+)?(ns|µs|us|ms|s)$",
              "description": "Ramp up/down duration"
            },
            "duration": {
              "type": "string",
              "pattern": "^-?[0-9]+(\\.[0-9]+)?(ns|µs|us|ms|s)$",
              "description": "Total shift duration"
            },
            "shift": {
              "type": "string",
              "pattern": "^-?[0-9]+(\\.[0-9]+)?(ns|µs|us|ms|s)$",
              "description": "Maximum phase shift"
            }
          }
        }
      }
    }
  }
}
```

### Implementation Steps

1. Create schema generation from Go struct + check annotations
2. Reference `config-schema.json` for `sync` section
3. Output to `configs/syncsim-schema.json`

---

## Example TOML Configuration

```toml
# syncsim/tsgen configuration for testing tracking mode robustness
# Usage: syncsim config.toml 300   (runs for 300 seconds)
#        tsgen config.toml 600     (generates 600 seconds of timestamps)

# PHC oscillator: typical Intel i210 characteristics
[phc]
freqOffset = 2000.0    # 2 ppm initial offset
whiteNoise = 7.0       # ppb
flickerNoise = 1.0     # ppb
randomWalk = 1.0       # ppb/√s

# GPS receiver: u-blox ZED-F9T characteristics
[gps]
jitter = 0.25          # ns

[gps.sawtooth]
amp = 15.0             # ns (≈ 32 MHz oscillator)

[[gps.ar1]]
tau = 600.0            # 10-minute correlation
sigma = 5.0            # ns

# === Sections below are used by syncsim only (ignored by tsgen) ===

# Controller tuning (parallels [sync] in satpulse.toml)
[sync.track]
kp = 0.5
ki = 0.1
madWindow = 10
madMultiple = 25.0

# Pulse delivery timing
[pulse]
minDelay = 5e-6        # 5 µs
maxDelay = 250e-6      # 250 µs
width = 0.1            # 100 ms (dual-edge mode)

# Message delivery timing
[msg]
delay = 0.1            # 100 ms
jitter = 0.01          # 10 ms stddev
sawtoothType = "prepulse"
prePulseTime = 0.95    # 950 ms before pulse

# Fault injection: 10-second outage at t=100
[fault.toggle]
durations = [100.0, 10.0]

# Single outlier at t=200
[fault.outlier]
times = [200.0]
offset = 2000    # nanoseconds (2 µs)
```
