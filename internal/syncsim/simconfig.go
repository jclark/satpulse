package syncsim

import (
	"math"
	"os"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/pelletier/go-toml/v2"
)

// Config holds simulation parameters
type Config struct {
	PHC               PHCConfig       `toml:"phc"`   // PHC oscillator parameters
	GPS               GPSConfig       `toml:"gps"`   // GPS PPS parameters
	Sync              phcsync.Config  `toml:"sync"`  // controller config
	MinDelay          float64         `toml:"minDelay"`          // minimum pulse delivery delay in seconds
	MaxDelay          float64         `toml:"maxDelay"`          // maximum pulse delivery delay in seconds
	MsgDelay          float64         `toml:"msgDelay"`          // GPS message delay after pulse in seconds
	MsgJitter         float64         `toml:"msgJitter"`         // GPS message delay jitter in seconds
	PulseWidth        float64         `toml:"pulseWidth"`        // pulse width in seconds (0 for single-edge mode)
	PrePulseTime      float64         `toml:"prePulseTime"`      // seconds before pulse to send UBX-TIM-TP PrePulse message
	PostPulseMsgDelay float64         `toml:"postPulseMsgDelay"` // seconds after pulse to send PostPulse sawtooth message
	SawtoothMsgType   gpsprot.TimeRef `toml:"sawtoothMsgType"`   // type of sawtooth message: PrePulse, PostPulse, or NoPulse
	ToggleTimes       []float64       `toml:"toggleTimes"`       // absolute simulation times to toggle pulse/message delivery on/off
	Outlier           OutlierConfig   `toml:"outlier"`           // PPS outlier injection configuration
	Shift             ShiftConfig     `toml:"shift"`             // PPS phase shift configuration
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		PHC:               DefaultPHCConfig(),
		GPS:               DefaultGPSConfig(),
		Sync:              phcsync.DefaultConfig(),
		MinDelay:          5e-6,
		MaxDelay:          250e-6,
		MsgDelay:          0.1,
		MsgJitter:         0.01,
		PulseWidth:        0,                // default to single-edge mode
		PrePulseTime:      0.95,             // send PrePulse message 0.95s before PPS edge
		PostPulseMsgDelay: 0.1,              // send PostPulse message 0.1s after PPS edge
		SawtoothMsgType:   gpsprot.PrePulse, // default to PrePulse (current behavior)
		Outlier: OutlierConfig{
			Offset: 2000, // 2µs default outlier magnitude (in nanoseconds)
		},
	}
}

// PHCConfig holds PHC oscillator parameters
type PHCConfig struct {
	FreqOffset   clocksim.PPB `toml:"freqOffset"`   // ppb
	Drift        float64      `toml:"drift"`        // ppb/day
	WhiteNoise   clocksim.PPB `toml:"whiteNoise"`   // ppb
	FlickerNoise clocksim.PPB `toml:"flickerNoise"` // ppb
	RandomWalk   clocksim.PPB `toml:"randomWalk"`   // ppb/√s
	Sinusoid     []Sinusoid   `toml:"sinusoid"`     // sinusoidal components
}

// Sinusoid represents a sinusoidal error component
type Sinusoid struct {
	Period    float64 `toml:"period"`    // seconds
	Amp       float64 `toml:"amp"`       // ppb for PHC, nanoseconds for GPS
	PhaseInit float64 `toml:"phaseInit"` // initial phase [0,1), defaults to 0
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

// GPSConfig holds GPS PPS error parameters
type GPSConfig struct {
	Jitter     clocksim.Nanoseconds `toml:"jitter"`     // nanoseconds (white noise stddev)
	Sawtooth   SawtoothConfig       `toml:"sawtooth"`   // sawtooth error parameters
	AR1        []AR1Config          `toml:"ar1"`        // AR(1) colored noise parameters (on phase)
	AR1FM      AR1FMConfig          `toml:"ar1FM"`      // AR(1) FM parameters (OU on frequency, sigma in ppb)
	RandomWalk float64              `toml:"randomWalk"` // random walk FM coefficient in ppb/√s
	Drift      DriftConfig          `toml:"drift"`      // Carpenter-Lee bounded drift parameters
	Resonator  ResonatorConfig      `toml:"resonator"`  // damped oscillator on phase (bounded)
	Sinusoid   []Sinusoid           `toml:"sinusoid"`   // sinusoidal components
}

// IsZero returns true if all GPS parameters are zero (no PPS error configured).
func (c GPSConfig) IsZero() bool {
	return c.Jitter == 0 && c.Sawtooth.Amp == 0 && len(c.AR1) == 0 &&
		c.AR1FM.Tau == 0 && c.RandomWalk == 0 && c.Drift.Tau == 0 &&
		c.Resonator.Period == 0 && len(c.Sinusoid) == 0
}

// ResonatorConfig holds damped oscillator on phase parameters.
// Implements a bounded phase process that oscillates around zero.
type ResonatorConfig struct {
	Period float64              `toml:"period"` // seconds - resonator period (ω₀ = 2π/period)
	Sigma  clocksim.Nanoseconds `toml:"sigma"`  // nanoseconds - RMS phase deviation
	Zeta   float64              `toml:"zeta"`   // dimensionless - damping ratio (default 0.3)
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

// SawtoothConfig holds GPS sawtooth error parameters
type SawtoothConfig struct {
	Amp           float64  `toml:"amp"`           // nanoseconds (tick size)
	PhaseInit     float64  `toml:"phaseInit"`     // initial phase [0,1), defaults to 0.5
	InternalClock Sinusoid `toml:"internalClock"` // GPS receiver's internal oscillator
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


// AR1Config holds AR(1) colored noise parameters using tau/sigma parameterisation.
// tau is the correlation time constant in seconds.
// sigma is the steady-state RMS in nanoseconds.
type AR1Config struct {
	Tau   float64              `toml:"tau"`   // seconds (correlation time constant)
	Sigma clocksim.Nanoseconds `toml:"sigma"` // nanoseconds (steady-state RMS)
}

// AR1FMConfig holds AR(1) frequency modulation parameters.
// tau is the correlation time constant in seconds.
// sigma is the steady-state RMS in ppb (parts per billion).
type AR1FMConfig struct {
	Tau   float64      `toml:"tau"`   // seconds (correlation time constant)
	Sigma clocksim.PPB `toml:"sigma"` // ppb (steady-state RMS of frequency bias)
}

// DriftConfig holds Carpenter-Lee 2nd-order Gauss-Markov drift parameters.
// User-facing parameters (tau, sigma, zeta) are converted to internal parameters
// (omega_n, zeta, sigma_drift) for the simulator.
type DriftConfig struct {
	Tau   float64              `toml:"tau"`   // seconds - bounded drift timescale (1/omega_n)
	Sigma clocksim.Nanoseconds `toml:"sigma"` // nanoseconds - RMS of bounded drift phase
	Zeta  float64              `toml:"zeta"`  // dimensionless - damping ratio (default 0.7)
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
	return cfg.Sync.Validate()
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

// OutlierConfig configures PPS outlier injection for testing.
// Used to test outlier detection algorithms.
type OutlierConfig struct {
	Times  []float64            // seconds at which to inject outliers (rounded to nearest integer)
	Offset clocksim.Nanoseconds // magnitude of outlier phase offset
}

// ShiftConfig configures a temporary PPS phase shift for testing.
// Used to simulate ionospheric disturbances or other gradual timing biases.
type ShiftConfig struct {
	StartTime float64              // start time in seconds
	Ramp      float64              // ramp up/down duration in seconds (symmetric)
	Duration  float64              // total duration in seconds (includes ramp up + hold + ramp down)
	Shift     clocksim.Nanoseconds // phase shift amount
}

// InOutage returns true if time t falls within an outage period.
// Odd-indexed intervals (1, 3, 5...) are outage periods.
func (c Config) InOutage(t float64) bool {
	for i, toggle := range c.ToggleTimes {
		if t < toggle {
			return i%2 == 1
		}
	}
	return len(c.ToggleTimes)%2 == 1
}

