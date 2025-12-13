package syncsim

import (
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/jclark/satpulse/internal/check"
	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/pelletier/go-toml/v2"
)

// Seconds is a true alias for float64, used for documentation in config structs.
// No conversion needed - clocksim uses float64 for seconds internally.
type Seconds = float64

// Config holds simulation parameters
type Config struct {
	PHC   PHCConfig      `toml:"phc"`   // PHC oscillator parameters
	GPS   GPSConfig      `toml:"gps"`   // GPS PPS parameters
	Sync  phcsync.Config `toml:"sync"`  // controller config
	Pulse PulseConfig    `toml:"pulse"` // pulse timing parameters
	Msg   MsgConfig      `toml:"msg"`   // message timing parameters
	Fault FaultConfig    `toml:"fault"` // fault injection configuration
}

// Validate checks that all config fields are within valid ranges.
// Returns an error if any field fails validation.
func (c *Config) Validate() error {
	var errs []error
	if checkErrs := check.Validate(c); checkErrs != nil {
		for _, msg := range checkErrs {
			errs = append(errs, errors.New(msg))
		}
	}
	if err := c.Sync.Validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		PHC:  DefaultPHCConfig(),
		GPS:  DefaultGPSConfig(),
		Sync: phcsync.DefaultConfig(),
		Pulse: PulseConfig{
			MinDelay: 5e-6,   // 5 µs
			MaxDelay: 250e-6, // 250 µs
			Width:    0,      // single-edge mode
		},
		Msg: MsgConfig{
			Delay:          0.1,              // 100 ms
			Jitter:         0.01,             // 10 ms stddev
			SawtoothType:   SawtoothPrePulse, // default to PrePulse
			PrePulseTime:   0.95,             // 950 ms before pulse
			PostPulseDelay: 0.1,              // 100 ms after pulse
		},
		Fault: FaultConfig{
			Outlier: OutlierConfig{
				Offset: 2000, // 2µs default outlier magnitude (in nanoseconds)
			},
		},
	}
}

// PHCConfig configures the PTP Hardware Clock oscillator error model.
// The PHC's instantaneous frequency is modeled as:
//
//	nominal_freq * (1 + freq_offset + drift*t + white + flicker + random_walk + sinusoids)
//
// All frequency parameters are in parts-per-billion (ppb) relative to nominal.
type PHCConfig struct {
	// FreqOffset is the constant frequency offset in ppb.
	// Typical values: ±1000-10000 ppb for factory-trimmed oscillators.
	FreqOffset clocksim.PPB `toml:"freqOffset" check:">=-1_000_000,<=1_000_000"`

	// Drift is the linear frequency drift rate in ppb per day.
	Drift float64 `toml:"drift" check:">=-1_000_000,<=1_000_000"`

	// WhiteNoise is the standard deviation of white frequency noise in ppb.
	// Typical values are 1-20 ppb for good crystals.
	WhiteNoise clocksim.PPB `toml:"whiteNoise" check:">=0,<=10000"`

	// FlickerNoise is the standard deviation of flicker (1/f) frequency noise in ppb.
	// Typical values are 0.1-5 ppb for crystals.
	FlickerNoise clocksim.PPB `toml:"flickerNoise" check:">=0,<=10000"`

	// RandomWalk is the random walk FM coefficient in ppb/√s.
	// Typical values are 0.01-1 ppb/√s.
	RandomWalk clocksim.PPB `toml:"randomWalk" check:">=0,<=10000"`

	// Sinusoid is a list of sinusoidal frequency modulation components.
	Sinusoid []Sinusoid `toml:"sinusoid"`
}

// Sinusoid configures a sinusoidal modulation component.
// Contribution: Amp * sin(2π * (t/Period + PhaseInit))
type Sinusoid struct {
	// Period is the oscillation period in seconds.
	Period Seconds `toml:"period" check:">0,<=1000000"`

	// Amp is the amplitude (ppb for PHC frequency, ns for GPS phase).
	Amp float64 `toml:"amp" check:">=0,<=10000"`

	// PhaseInit is the initial phase as a fraction of cycle [0,1).
	PhaseInit float64 `toml:"phaseInit" check:">=0,<1"`
}

// IsZero returns true if all PHC parameters are zero (no oscillator error configured).
func (c PHCConfig) IsZero() bool {
	return c.FreqOffset == 0 && c.Drift == 0 && c.WhiteNoise == 0 &&
		c.FlickerNoise == 0 && c.RandomWalk == 0 && len(c.Sinusoid) == 0
}

func DefaultPHCConfig() PHCConfig {
	return PHCConfig{
		FreqOffset:   2000.0,
		FlickerNoise: 1,
		WhiteNoise:   7.0,
		RandomWalk:   1,
	}
}

// CreateSimulator returns an OscSimulator combining all PHC error sources.
// Applies components in order: offset, white noise, flicker noise, random walk, drift, sinusoids.
func (c PHCConfig) CreateSimulator() clocksim.OscSimulator {
	oscs := []clocksim.OscSimulator{
		clocksim.FreqOffsetOsc(c.FreqOffset),
	}
	if c.WhiteNoise > 0 {
		oscs = append(oscs, clocksim.WhiteNoiseOsc(c.WhiteNoise, 42))
	}
	if c.FlickerNoise > 0 {
		oscs = append(oscs, clocksim.FlickerNoiseOsc(c.FlickerNoise, 43))
	}
	if c.RandomWalk > 0 {
		oscs = append(oscs, clocksim.RandomWalkOsc(c.RandomWalk, 44))
	}
	if c.Drift != 0 {
		oscs = append(oscs, clocksim.DriftOsc(c.Drift))
	}
	for _, s := range c.Sinusoid {
		if s.Amp > 0 {
			// For PHC, Amp is in PPB
			oscs = append(oscs, clocksim.SinusoidOsc(clocksim.PPB(s.Amp), s.Period, s.PhaseInit))
		}
	}
	return clocksim.CombineOsc(oscs...)
}

// GPSConfig configures the GPS PPS timing error model.
// All phase parameters are in nanoseconds.
type GPSConfig struct {
	// Jitter is white phase noise stddev in nanoseconds.
	// Typical: 0.1-5 ns survey-grade, 5-50 ns consumer-grade.
	Jitter clocksim.Nanoseconds `toml:"jitter" check:">=0,<=1000"`

	// Sawtooth configures GPS receiver quantization sawtooth error.
	Sawtooth SawtoothConfig `toml:"sawtooth"`

	// AR1 is a list of AR(1) colored phase noise processes.
	AR1 []AR1Config `toml:"ar1"`

	// AR1FM is an AR(1) frequency modulation process (sigma in ppb).
	AR1FM AR1FMConfig `toml:"ar1FM"`

	// RandomWalk is the random walk FM coefficient in ppb/√s.
	RandomWalk float64 `toml:"randomWalk" check:">=0,<=1000"`

	// Drift configures bounded drift (2nd-order Gauss-Markov).
	Drift DriftConfig `toml:"drift"`

	// Resonator configures a damped oscillator on phase.
	Resonator ResonatorConfig `toml:"resonator"`

	// Sinusoid is a list of sinusoidal phase modulation components.
	Sinusoid []Sinusoid `toml:"sinusoid"`
}

// IsZero returns true if all GPS parameters are zero (no PPS error configured).
func (c GPSConfig) IsZero() bool {
	return c.Jitter == 0 && c.Sawtooth.Amp == 0 && len(c.AR1) == 0 &&
		c.AR1FM.Tau == 0 && c.RandomWalk == 0 && c.Drift.Tau == 0 &&
		c.Resonator.Period == 0 && len(c.Sinusoid) == 0
}

// ResonatorConfig configures a damped harmonic oscillator on phase.
// Implements a bounded phase process that oscillates around zero.
type ResonatorConfig struct {
	// Period is the natural oscillation period in seconds.
	Period Seconds `toml:"period" check:">=0,<=1000000"`

	// Sigma is the RMS phase deviation in nanoseconds.
	Sigma clocksim.Nanoseconds `toml:"sigma" check:">=0,<=10000"`

	// Zeta is the damping ratio (default 0.3).
	Zeta float64 `toml:"zeta" check:">=0,<=10"`
}

// InternalParams converts user-facing (period, sigma, zeta) to internal (omegaN, zeta, sigmaNoise).
func (c ResonatorConfig) InternalParams() (omegaN, zeta, sigmaNoise float64) {
	return clocksim.ResonatorUserToInternal(c.Period, float64(c.Sigma), c.Zeta)
}

func DefaultGPSConfig() GPSConfig {
	return GPSConfig{
		Jitter:     0.25,
		RandomWalk: 0.000143,
		Sawtooth:   DefaultSawtoothConfig(),
	}
}

// DefaultZeroGPSConfig returns a default zero GPSConfig.
// See DefaultZeroConfig for the definition of "default zero".
func DefaultZeroGPSConfig() GPSConfig {
	return GPSConfig{
		Sawtooth: DefaultZeroSawtoothConfig(),
	}
}

// SawtoothConfig configures GPS receiver quantization sawtooth error.
type SawtoothConfig struct {
	// Amp is the sawtooth amplitude in nanoseconds (≈ 0.5e9/f_osc).
	Amp float64 `toml:"amp" check:">=0,<=1000"`

	// PhaseInit is the initial phase [0,1), default 0.5.
	PhaseInit float64 `toml:"phaseInit" check:">=0,<1"`

	// InternalClock models the GPS receiver's internal oscillator error.
	InternalClock Sinusoid `toml:"internalClock"`
}

func DefaultSawtoothConfig() SawtoothConfig {
	cfg := DefaultZeroSawtoothConfig()
	cfg.Amp = 15
	return cfg
}

// DefaultZeroSawtoothConfig returns a default zero SawtoothConfig.
// See DefaultZeroConfig for the definition of "default zero".
func DefaultZeroSawtoothConfig() SawtoothConfig {
	return SawtoothConfig{
		PhaseInit: 0.5,
		InternalClock: Sinusoid{
			Amp:       2.0,       // 2 ppb amplitude
			Period:    600.0,     // 10 minute period
			PhaseInit: 1.0 / 6.0, // π/3 radians = 1/6 cycle in [0,1)
		},
	}
}


// AR1Config configures an AR(1) phase noise process.
type AR1Config struct {
	// Tau is the correlation time constant in seconds.
	Tau Seconds `toml:"tau" check:">=0,<=1000000"`

	// Sigma is the steady-state RMS in nanoseconds.
	Sigma clocksim.Nanoseconds `toml:"sigma" check:">=0,<=10000"`
}

// AR1FMConfig configures an AR(1) frequency modulation process.
type AR1FMConfig struct {
	// Tau is the correlation time constant in seconds.
	Tau Seconds `toml:"tau" check:">=0,<=1000000"`

	// Sigma is the steady-state RMS of frequency bias in ppb.
	Sigma clocksim.PPB `toml:"sigma" check:">=0,<=10000"`
}

// DriftConfig configures bounded drift (Carpenter-Lee 2nd-order Gauss-Markov).
type DriftConfig struct {
	// Tau is the characteristic timescale in seconds.
	Tau Seconds `toml:"tau" check:">=0,<=1000000"`

	// Sigma is the RMS phase deviation in nanoseconds.
	Sigma clocksim.Nanoseconds `toml:"sigma" check:">=0,<=10000"`

	// Zeta is the damping ratio (default 0.7).
	Zeta float64 `toml:"zeta" check:">=0,<=10"`
}

// InternalParams converts user-facing (tau, sigma, zeta) to internal (omega_n, zeta, sigma_drift).
// Uses discrete Lyapunov calibration so that Sigma matches the actual RMS of the
// 1 Hz discrete drift simulator output.
func (c DriftConfig) InternalParams() (omegaN, zeta, sigmaDrift float64) {
	return clocksim.DriftUserToInternal(c.Tau, float64(c.Sigma), c.Zeta)
}

// AlphaNoise returns (alpha, noise_stddev) for use with AR1ColoredNoiseGPS.
// alpha is the autocorrelation coefficient, noise is the driving noise stddev in nanoseconds.
// Assumes 1-second sample interval.
func (c AR1Config) AlphaNoise() (alpha float64, noise clocksim.Nanoseconds) {
	if c.Tau <= 0 || c.Sigma <= 0 {
		return 0, 0
	}
	alpha = math.Exp(-1.0 / c.Tau)
	alpha = min(alpha, 1.0-1e-12)
	noise = clocksim.Nanoseconds(float64(c.Sigma) * math.Sqrt(max(0, 1-alpha*alpha)))
	return alpha, noise
}

// DefaultZeroConfig returns a Config representing perfect hardware with zero noise.
// "Default zero" configs have all noise and offset parameters set to zero,
// but include defaults that make it easy to specify valid non-zero configurations
// for parameters. Specifically, a non-zero sawtooth.amp will work without needing
// to specify sawtooth.internalClock parameters.
func DefaultZeroConfig() Config {
	return Config{
		GPS:  DefaultZeroGPSConfig(),
		Sync: phcsync.DefaultConfig(),
	}
}

// LoadConfig loads configuration from a TOML file into cfg.
// The caller initializes cfg (zero-valued or with defaults), and TOML values are merged on top.
func LoadConfig(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := toml.NewDecoder(f).DisallowUnknownFields().Decode(cfg); err != nil {
		return err
	}
	return cfg.Validate()
}

// CreateSimulator returns a GPSSimulator combining all GPS error sources.
// Applies components in order: jitter, AR(1), AR(1) FM, random walk, drift, sinusoids.
// Does NOT include Shift, Outlier, or Sawtooth - those are added separately in Simulate().
// Sawtooth is created separately with oscillator coupling.
func (c GPSConfig) CreateSimulator() clocksim.GPSSimulator {
	sims := []clocksim.GPSSimulator{}
	if c.Jitter > 0 {
		sims = append(sims, clocksim.JitterGPS(c.Jitter, 123))
	}
	for i, ar1 := range c.AR1 {
		if alpha, noise := ar1.AlphaNoise(); alpha > 0 {
			sims = append(sims, clocksim.AR1ColoredNoiseGPS(alpha, noise, int64(124+i)))
		}
	}
	if c.AR1FM.Tau > 0 && c.AR1FM.Sigma > 0 {
		sims = append(sims, clocksim.AR1FMGPS(c.AR1FM.Tau, c.AR1FM.Sigma, 126))
	}
	if c.RandomWalk > 0 {
		// Convert from ppb/√s to dimensionless/√s
		hPlus1 := c.RandomWalk * 1e-9
		sims = append(sims, clocksim.RandomWalkFMGPS(hPlus1, 125))
	}
	if omegaN, zeta, sigmaDrift := c.Drift.InternalParams(); omegaN > 0 {
		sims = append(sims, clocksim.DriftGPS(omegaN, zeta, sigmaDrift, 127))
	}
	if omegaN, zeta, sigmaNoise := c.Resonator.InternalParams(); omegaN > 0 {
		sims = append(sims, clocksim.ResonatorGPS(omegaN, zeta, sigmaNoise, 128))
	}
	for _, s := range c.Sinusoid {
		// For GPS, Amp is in nanoseconds
		sims = append(sims, clocksim.SinusoidGPS(clocksim.Nanoseconds(s.Amp), s.Period, s.PhaseInit))
	}
	return clocksim.CombineGPS(sims...)
}


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
	PrePulseTime Seconds `toml:"prePulseTime" check:">=0,<1"`

	// PostPulseDelay is the delay after the PPS edge when a PostPulse correction
	// message is delivered. Only used when SawtoothType is SawtoothPostPulse.
	PostPulseDelay Seconds `toml:"postPulseDelay" check:">=0,<1"`
}

// SawtoothType specifies how sawtooth correction messages are delivered.
type SawtoothType int

const (
	SawtoothPrePulse  SawtoothType = iota // correction before pulse (UBX-TIM-TP)
	SawtoothPostPulse                     // correction after pulse (UBX-TIM-TOS)
	SawtoothNone                          // no correction messages
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

// MarshalText implements encoding.TextMarshaler for TOML serialization.
func (s SawtoothType) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for TOML deserialization.
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

// TimeRef converts SawtoothType to gpsprot.TimeRef for use with timemsg.Buffer.
func (s SawtoothType) TimeRef() gpsprot.TimeRef {
	switch s {
	case SawtoothPrePulse:
		return gpsprot.PrePulse
	case SawtoothPostPulse:
		return gpsprot.PostPulse
	default:
		return gpsprot.NavSolution // NoPulse - no sawtooth messages
	}
}

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

	// Offset is the magnitude of the phase offset to inject in nanoseconds.
	// Typical multipath outliers are 1000-10000 ns (1-10 µs).
	Offset clocksim.Nanoseconds `toml:"offset" check:">=0"`
}

// ShiftConfig configures gradual phase shift injection.
type ShiftConfig struct {
	// StartTime is the simulation time when the shift begins.
	StartTime Seconds `toml:"startTime" check:">=0"`

	// Ramp is the ramp-up/ramp-down duration in seconds.
	Ramp Seconds `toml:"ramp" check:">=0"`

	// Duration is the total duration including both ramp periods in seconds.
	Duration Seconds `toml:"duration" check:">=0"`

	// Shift is the maximum phase shift magnitude at the plateau in nanoseconds.
	Shift clocksim.Nanoseconds `toml:"shift"`
}

// InOutage returns true if time t falls within an outage period.
// Uses cfg.Fault.Toggle.Durations to determine outage state.
// Durations alternate: first is on, second is off, third is on, etc.
// Example: [60, 10, 60] means 60s on, 10s off, 60s on.
func (c Config) InOutage(t float64) bool {
	elapsed := 0.0
	for i, dur := range c.Fault.Toggle.Durations {
		elapsed += dur
		if t < elapsed {
			// Currently in interval i (0-indexed)
			// Even intervals (0, 2, 4...) are signal-on
			// Odd intervals (1, 3, 5...) are signal-off (outage)
			return i%2 == 1
		}
	}
	// Past all durations: if we ended on odd count, signal is off
	return len(c.Fault.Toggle.Durations)%2 == 1
}

