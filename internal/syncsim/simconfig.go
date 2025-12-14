package syncsim

import (
	"errors"
	"fmt"
	"io"
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
	PHC   PHCConfig      `toml:"phc" comment:"PHC oscillator error model"`
	GPS   GPSConfig      `toml:"gps" comment:"GPS time pulse error model"`
	Sync  phcsync.Config `toml:"sync" comment:"PHC sync controller parameters"`
	Pulse PulseConfig    `toml:"pulse" comment:"Pulse delivery timing"`
	Msg   MsgConfig      `toml:"msg" comment:"GPS message delivery timing"`
	Fault FaultConfig    `toml:"fault" comment:"Fault injection parameters"`
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

// DefaultConfig returns a Config with zero noise but sensible operational defaults.
// PHC and GPS have zero noise parameters (user specifies exactly what noise they want).
// Pulse and Msg have reasonable timing defaults. Fault has no faults configured.
func DefaultConfig() Config {
	return Config{
		PHC:   DefaultPHCConfig(),
		GPS:   DefaultGPSConfig(),
		Sync:  phcsync.DefaultConfig(),
		Pulse: DefaultPulseConfig(),
		Msg:   DefaultMsgConfig(),
		Fault: DefaultFaultConfig(),
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
	FreqOffset clocksim.PPB `toml:"freqOffset" check:">=-1_000_000,<=1_000_000" comment:"Constant frequency offset (ppb)"`

	// Drift is the linear frequency drift rate in ppb per day.
	Drift float64 `toml:"drift" check:">=-1_000_000,<=1_000_000" comment:"Linear frequency drift (ppb/day)"`

	// WhiteNoise is the standard deviation of white frequency noise in ppb.
	// Typical values are 1-20 ppb for good crystals.
	WhiteNoise clocksim.PPB `toml:"whiteNoise" check:">=0,<=10000" comment:"White frequency noise stddev (ppb)"`

	// FlickerNoise is the standard deviation of flicker (1/f) frequency noise in ppb.
	// Typical values are 0.1-5 ppb for crystals.
	FlickerNoise clocksim.PPB `toml:"flickerNoise" check:">=0,<=10000" comment:"Flicker (1/f) frequency noise stddev (ppb)"`

	// RandomWalk is the random walk FM coefficient in ppb/√s.
	// Typical values are 0.01-1 ppb/√s.
	RandomWalk clocksim.PPB `toml:"randomWalk" check:">=0,<=10000" comment:"Random walk FM coefficient (ppb/√s)"`

	// Sinusoid is a list of sinusoidal frequency modulation components.
	Sinusoid []FreqSinusoid `toml:"sinusoid" comment:"Sinusoidal frequency modulation components"`
}

// FreqSinusoid configures a sinusoidal frequency modulation component.
// Contribution to frequency: Amp * sin(2π * (t/Period + PhaseInit))
type FreqSinusoid struct {
	// Period is the oscillation period in seconds.
	Period Seconds `toml:"period" check:">0,<=1000000" comment:"Oscillation period (s)"`

	// Amp is the frequency amplitude in parts-per-billion.
	Amp clocksim.PPB `toml:"amp" check:">=0,<=10000" comment:"Frequency amplitude (ppb)"`

	// PhaseInit is the initial phase as a fraction of cycle [0,1).
	PhaseInit float64 `toml:"phaseInit" check:">=0,<1" comment:"Initial phase [0,1)"`
}

func (s FreqSinusoid) IsZero() bool { return s.Amp == 0 }

// PhaseSinusoid configures a sinusoidal phase modulation component.
// Contribution to phase: Amp * sin(2π * (t/Period + PhaseInit))
type PhaseSinusoid struct {
	// Period is the oscillation period in seconds.
	Period Seconds `toml:"period" check:">0,<=1000000" comment:"Oscillation period (s)"`

	// Amp is the phase amplitude in nanoseconds.
	Amp clocksim.Nanoseconds `toml:"amp" check:">=0,<=10000" comment:"Phase amplitude (ns)"`

	// PhaseInit is the initial phase as a fraction of cycle [0,1).
	PhaseInit float64 `toml:"phaseInit" check:">=0,<1" comment:"Initial phase [0,1)"`
}

func (s PhaseSinusoid) IsZero() bool { return s.Amp == 0 }

// IsZero returns true if all PHC parameters are zero (no oscillator error configured).
func (c PHCConfig) IsZero() bool {
	if c.FreqOffset != 0 || c.Drift != 0 || c.WhiteNoise != 0 ||
		c.FlickerNoise != 0 || c.RandomWalk != 0 {
		return false
	}
	for _, s := range c.Sinusoid {
		if !s.IsZero() {
			return false
		}
	}
	return true
}

// DefaultPHCConfig returns a PHCConfig with zero noise.
// Includes a zero sinusoid entry so users can see the expected structure.
func DefaultPHCConfig() PHCConfig {
	return PHCConfig{
		Sinusoid: []FreqSinusoid{{}},
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
			oscs = append(oscs, clocksim.SinusoidOsc(s.Amp, s.Period, s.PhaseInit))
		}
	}
	return clocksim.CombineOsc(oscs...)
}

// GPSConfig configures the GPS PPS timing error model.
// All phase parameters are in nanoseconds.
type GPSConfig struct {
	// Jitter is white phase noise stddev in nanoseconds.
	// Typical: 0.1-5 ns survey-grade, 5-50 ns consumer-grade.
	Jitter clocksim.Nanoseconds `toml:"jitter" check:">=0,<=1000" comment:"White phase noise stddev (ns)"`

	// Sawtooth configures GPS receiver quantization sawtooth error.
	Sawtooth SawtoothConfig `toml:"sawtooth" comment:"GPS receiver quantization sawtooth error"`

	// AR1 is a list of AR(1) colored phase noise processes.
	AR1 []AR1Config `toml:"ar1" comment:"AR(1) colored phase noise processes"`

	// AR1FM is an AR(1) frequency modulation process (sigma in ppb).
	AR1FM AR1FMConfig `toml:"ar1FM" comment:"AR(1) frequency modulation process"`

	// RandomWalk is the random walk FM coefficient in ppb/√s.
	RandomWalk float64 `toml:"randomWalk" check:">=0,<=1000" comment:"Random walk FM coefficient (ppb/√s)"`

	// Drift configures bounded drift (2nd-order Gauss-Markov).
	Drift DriftConfig `toml:"drift" comment:"Bounded drift (2nd-order Gauss-Markov)"`

	// Resonator configures a damped oscillator on phase.
	Resonator ResonatorConfig `toml:"resonator" comment:"Damped oscillator on phase"`

	// Sinusoid is a list of sinusoidal phase modulation components.
	Sinusoid []PhaseSinusoid `toml:"sinusoid" comment:"Sinusoidal phase modulation components"`
}

// IsZero returns true if all GPS parameters are zero (no PPS error configured).
func (c GPSConfig) IsZero() bool {
	if c.Jitter != 0 || c.Sawtooth.Amp != 0 || c.AR1FM.Tau != 0 ||
		c.RandomWalk != 0 || c.Drift.Tau != 0 || c.Resonator.Period != 0 {
		return false
	}
	for _, a := range c.AR1 {
		if !a.IsZero() {
			return false
		}
	}
	for _, s := range c.Sinusoid {
		if !s.IsZero() {
			return false
		}
	}
	return true
}

// ResonatorConfig configures a damped harmonic oscillator on phase.
// Implements a bounded phase process that oscillates around zero.
type ResonatorConfig struct {
	// Period is the natural oscillation period in seconds.
	Period Seconds `toml:"period" check:">=0,<=1000000" comment:"Natural oscillation period (s)"`

	// Sigma is the RMS phase deviation in nanoseconds.
	Sigma clocksim.Nanoseconds `toml:"sigma" check:">=0,<=10000" comment:"RMS phase deviation (ns)"`

	// Zeta is the damping ratio.
	Zeta float64 `toml:"zeta" check:">=0,<=10" comment:"Damping ratio"`
}

// DefaultResonatorConfig returns a ResonatorConfig with sensible zeta default.
func DefaultResonatorConfig() ResonatorConfig {
	return ResonatorConfig{Zeta: 0.3}
}

// InternalParams converts user-facing (period, sigma, zeta) to internal (omegaN, zeta, sigmaNoise).
func (c ResonatorConfig) InternalParams() (omegaN, zeta, sigmaNoise float64) {
	return clocksim.ResonatorUserToInternal(c.Period, float64(c.Sigma), c.Zeta)
}

// DefaultGPSConfig returns a GPSConfig with zero noise but sensible defaults.
// Users specifying sawtooth.amp will get working InternalClock values automatically.
// Drift and Resonator have sensible zeta defaults for when user specifies tau/sigma.
// Includes zero AR1 and Sinusoid entries so users can see the expected structure.
func DefaultGPSConfig() GPSConfig {
	return GPSConfig{
		Sawtooth:  DefaultSawtoothConfig(),
		Drift:     DefaultDriftConfig(),
		Resonator: DefaultResonatorConfig(),
		AR1:       []AR1Config{{}},
		Sinusoid:  []PhaseSinusoid{{}},
	}
}

// SawtoothConfig configures GPS receiver quantization sawtooth error.
type SawtoothConfig struct {
	// Amp is the sawtooth amplitude in nanoseconds (≈ 0.5e9/f_osc).
	Amp float64 `toml:"amp" check:">=0,<=1000" comment:"Sawtooth amplitude (ns, ≈ 0.5e9/f_osc)"`

	// PhaseInit is the initial phase [0,1), default 0.5.
	PhaseInit float64 `toml:"phaseInit" check:">=0,<1" comment:"Initial phase [0,1)"`

	// InternalClock models the GPS receiver's internal oscillator error.
	InternalClock FreqSinusoid `toml:"internalClock" comment:"GPS internal oscillator error model"`
}

// DefaultSawtoothConfig returns a SawtoothConfig with zero amplitude but sensible defaults
// for PhaseInit and InternalClock. Users specifying sawtooth.amp will get working values.
func DefaultSawtoothConfig() SawtoothConfig {
	return SawtoothConfig{
		Amp:       0,   // zero by default - user specifies if needed
		PhaseInit: 0.5, // sensible default
		InternalClock: FreqSinusoid{
			Amp:       2.0,       // 2 ppb amplitude
			Period:    600.0,     // 10 minute period
			PhaseInit: 1.0 / 6.0, // π/3 radians = 1/6 cycle in [0,1)
		},
	}
}

// AR1Config configures an AR(1) phase noise process.
type AR1Config struct {
	// Tau is the correlation time constant in seconds.
	Tau Seconds `toml:"tau" check:">=0,<=1000000" comment:"Correlation time constant (s)"`

	// Sigma is the steady-state RMS in nanoseconds.
	Sigma clocksim.Nanoseconds `toml:"sigma" check:">=0,<=10000" comment:"Steady-state RMS (ns)"`
}

func (c AR1Config) IsZero() bool { return c.Tau == 0 || c.Sigma == 0 }

// AR1FMConfig configures an AR(1) frequency modulation process.
type AR1FMConfig struct {
	// Tau is the correlation time constant in seconds.
	Tau Seconds `toml:"tau" check:">=0,<=1000000" comment:"Correlation time constant (s)"`

	// Sigma is the steady-state RMS of frequency bias in ppb.
	Sigma clocksim.PPB `toml:"sigma" check:">=0,<=10000" comment:"Steady-state RMS of frequency bias (ppb)"`
}

// DriftConfig configures bounded drift (Carpenter-Lee 2nd-order Gauss-Markov).
type DriftConfig struct {
	// Tau is the characteristic timescale in seconds.
	Tau Seconds `toml:"tau" check:">=0,<=1000000" comment:"Characteristic timescale (s)"`

	// Sigma is the RMS phase deviation in nanoseconds.
	Sigma clocksim.Nanoseconds `toml:"sigma" check:">=0,<=10000" comment:"RMS phase deviation (ns)"`

	// Zeta is the damping ratio.
	Zeta float64 `toml:"zeta" check:">=0,<=10" comment:"Damping ratio"`
}

// DefaultDriftConfig returns a DriftConfig with sensible zeta default.
func DefaultDriftConfig() DriftConfig {
	return DriftConfig{Zeta: 0.7}
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

// DefaultPulseConfig returns default pulse timing parameters.
func DefaultPulseConfig() PulseConfig {
	return PulseConfig{
		MinDelay: 5e-6,   // 5 µs
		MaxDelay: 250e-6, // 250 µs
		Width:    0,      // single-edge mode
	}
}

// DefaultMsgConfig returns default message timing parameters.
func DefaultMsgConfig() MsgConfig {
	return MsgConfig{
		Delay:          0.1,              // 100 ms
		Jitter:         0.01,             // 10 ms stddev
		SawtoothType:   SawtoothPrePulse, // default to PrePulse
		PrePulseTime:   0.95,             // 950 ms before pulse
		PostPulseDelay: 0.1,              // 100 ms after pulse
	}
}

// DefaultFaultConfig returns a FaultConfig with no faults configured.
func DefaultFaultConfig() FaultConfig {
	return FaultConfig{}
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

// WriteDefaultConfig writes the default configuration as TOML to w.
// The output includes comments derived from struct field comment tags.
func WriteDefaultConfig(w io.Writer) error {
	cfg := DefaultConfig()
	return toml.NewEncoder(w).Encode(cfg)
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
		if s.Amp > 0 {
			sims = append(sims, clocksim.SinusoidGPS(s.Amp, s.Period, s.PhaseInit))
		}
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
	MinDelay Seconds `toml:"minDelay" check:">=0,<1" comment:"Min pulse delay from GPS second (s)"`

	// MaxDelay is the maximum delay from the true GPS second to when the
	// PPS pulse edge is delivered to the PHC timestamper. Must be >= MinDelay.
	MaxDelay Seconds `toml:"maxDelay" check:">=0,<1" comment:"Max pulse delay from GPS second (s)"`

	// Width is the pulse width for dual-edge timestamping mode.
	// When Width > 0, the simulator generates both rising and falling edges.
	// When Width = 0, only rising edges are generated (single-edge mode).
	// Typical GPS receivers use 100ms (0.1) pulse width.
	Width Seconds `toml:"width" check:">=0,<1" comment:"Pulse width (0=single-edge mode)"`
}

// MsgConfig configures GPS message delivery timing.
// GPS receivers send time messages (e.g., UBX-NAV-PVT) after each PPS pulse.
type MsgConfig struct {
	// Delay is the mean delay from the PPS pulse to when the GPS time message
	// is received. Real GPS receivers take 50-250ms to transmit after the pulse.
	Delay Seconds `toml:"delay" check:">=0,<1" comment:"Mean message delay after pulse (s)"`

	// Jitter is the standard deviation of the message arrival time.
	Jitter Seconds `toml:"jitter" check:">=0,<1" comment:"Message delay stddev (s)"`

	// SawtoothType specifies how sawtooth correction messages are delivered:
	//   - SawtoothPrePulse ("prepulse"): Correction arrives before the pulse (UBX-TIM-TP)
	//   - SawtoothPostPulse ("postpulse"): Correction arrives after the pulse (UBX-TIM-TOS)
	//   - SawtoothNone ("none"): No correction messages
	SawtoothType SawtoothType `toml:"sawtoothType" comment:"How sawtooth correction is delivered (prepulse/postpulse/none)"`

	// PrePulseTime is the time before the PPS edge when a PrePulse correction
	// message is delivered. Only used when SawtoothType is SawtoothPrePulse.
	PrePulseTime Seconds `toml:"prePulseTime" check:">=0,<1" comment:"Time before pulse for prepulse message (s)"`

	// PostPulseDelay is the delay after the PPS edge when a PostPulse correction
	// message is delivered. Only used when SawtoothType is SawtoothPostPulse.
	PostPulseDelay Seconds `toml:"postPulseDelay" check:">=0,<1" comment:"Delay after pulse for postpulse message (s)"`
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
	Toggle  ToggleConfig  `toml:"toggle" comment:"Signal outage simulation"`
	Outlier OutlierConfig `toml:"outlier" comment:"Discrete phase outlier injection"`
	Shift   ShiftConfig   `toml:"shift" comment:"Gradual phase shift injection"`
}

// ToggleConfig configures signal outage simulation.
type ToggleConfig struct {
	// Durations is a sequence of relative durations in seconds.
	// The simulation alternates between signal-on and signal-off states.
	// First duration is signal-on, second is signal-off, etc.
	// Example: [60, 10, 60] means: 60s on, 10s off, 60s on.
	Durations []Seconds `toml:"durations" comment:"Alternating on/off durations (s)"`
}

// OutlierConfig configures discrete phase outlier injection.
type OutlierConfig struct {
	// Times is a list of simulation times at which to inject an outlier.
	Times []Seconds `toml:"times" comment:"Simulation times to inject outliers"`

	// Offset is the magnitude of the phase offset to inject in nanoseconds.
	// Typical multipath outliers are 1000-10000 ns (1-10 µs).
	Offset clocksim.Nanoseconds `toml:"offset" check:">=0" comment:"Outlier offset magnitude (ns)"`
}

// ShiftConfig configures gradual phase shift injection.
type ShiftConfig struct {
	// StartTime is the simulation time when the shift begins.
	StartTime Seconds `toml:"startTime" check:">=0" comment:"When shift begins (s)"`

	// Ramp is the ramp-up/ramp-down duration in seconds.
	Ramp Seconds `toml:"ramp" check:">=0" comment:"Ramp up/down duration (s)"`

	// Duration is the total duration including both ramp periods in seconds.
	Duration Seconds `toml:"duration" check:">=0" comment:"Total shift duration (s)"`

	// Shift is the maximum phase shift magnitude at the plateau in nanoseconds.
	Shift clocksim.Nanoseconds `toml:"shift" comment:"Max phase shift at plateau (ns)"`
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
