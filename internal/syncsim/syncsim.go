// Package syncsim provides a discrete-event simulator for testing the phcsync controller.
//
// # Architecture
//
// The simulator uses an event-driven architecture built on Go 1.23 iterators:
//
//	generatePulseEvents()    ─┐
//	generateMessageEvents()  ─┼─> mergeEvents() ──> for event := range events
//	generateTickEvents()     ─┘
//
// Three event streams are generated independently and merged chronologically:
//   - Pulse events: GPS PPS edges (rising, and trailing for dual-edge mode)
//   - Message events: GPS time messages containing TAI time
//   - Tick events: System ticks every 250ms (like real daemon)
//
// # Key Design Principles
//
// 1. Event generators are declarative - they describe WHAT events happen and WHEN,
// but NOT which events are delivered. The main loop decides delivery based on outages.
//
// 2. Pulse timestamps are ALWAYS read (even during outages) to track PHC drift during
// signal loss. The decision to deliver to the controller happens separately.
//
// 3. Ticks start immediately at t=0.25, modeling real system behavior. Early ticks are
// safe because the controller returns early when in ModeReset.
//
// 4. Mode tracking uses the observer pattern, which sees samples BEFORE mode transitions.
// This correctly attributes samples to the mode that processed them.
//
// 5. Single-edge and dual-edge modes are handled uniformly - the generator produces
// 1 or 2 pulse events per PPS, and the event loop processes them identically.
package syncsim

import (
	"iter"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/statsobs"
	"github.com/jclark/satpulse/internal/timemsg"
	"github.com/jclark/satpulse/internal/ubx"
	"github.com/pelletier/go-toml/v2"
)

// Config holds simulation parameters
type Config struct {
	Duration          float64         // simulation duration in seconds
	PHC               PHCConfig       // PHC oscillator parameters
	GPS               GPSConfig       // GPS PPS parameters
	MinDelay          float64         // minimum pulse delivery delay in seconds
	MaxDelay          float64         // maximum pulse delivery delay in seconds
	MsgDelay          float64         // GPS message delay after pulse in seconds
	MsgJitter         float64         // GPS message delay jitter in seconds
	PulseWidth        float64         // pulse width in seconds (0 for single-edge mode)
	PrePulseTime      float64         // seconds before pulse to send UBX-TIM-TP PrePulse message
	PostPulseMsgDelay float64         // seconds after pulse to send PostPulse sawtooth message
	SawtoothMsgType   gpsprot.TimeRef // type of sawtooth message: PrePulse, PostPulse, or NoPulse
	ToggleTimes       []float64       // absolute simulation times to toggle pulse/message delivery on/off
	Outlier           OutlierConfig   // PPS outlier injection configuration
	Shift             ShiftConfig     // PPS phase shift configuration
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Duration:          60.0,
		PHC:               DefaultPHCConfig(),
		GPS:               DefaultGPSConfig(),
		MinDelay:          5e-6,
		MaxDelay:          250e-6,
		MsgDelay:          0.1,
		MsgJitter:         0.01,
		PulseWidth:        0,                // default to single-edge mode
		PrePulseTime:      0.95,             // send PrePulse message 0.95s before PPS edge
		PostPulseMsgDelay: 0.1,              // send PostPulse message 0.1s after PPS edge
		SawtoothMsgType:   gpsprot.PrePulse, // default to PrePulse (current behavior)
		Outlier: OutlierConfig{
			Offset: 2000 * time.Nanosecond, // 2µs default outlier magnitude
		},
	}
}

// PHCConfig holds PHC oscillator parameters
type PHCConfig struct {
	FreqOffset   float64    `toml:"freqOffset"`   // ppb
	Drift        float64    `toml:"drift"`        // ppb/day
	WhiteNoise   float64    `toml:"whiteNoise"`   // ppb
	FlickerNoise float64    `toml:"flickerNoise"` // ppb
	RandomWalk   float64    `toml:"randomWalk"`   // ppb/√s
	Sinusoid     []Sinusoid `toml:"sinusoid"`     // sinusoidal components
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
			oscs = append(oscs, clocksim.SinusoidOsc(s.Amp, s.Period, s.PhaseInit))
		}
	}
	return clocksim.CombineOsc(oscs...)
}

// GPSConfig holds GPS PPS error parameters
type GPSConfig struct {
	Jitter     float64        `toml:"jitter"`     // nanoseconds (white noise stddev)
	Sawtooth   SawtoothConfig `toml:"sawtooth"`   // sawtooth error parameters
	AR1        AR1Config      `toml:"ar1"`        // AR(1) colored noise parameters
	RandomWalk float64        `toml:"randomWalk"` // random walk FM coefficient in ppb/√s
	Sinusoid   []Sinusoid     `toml:"sinusoid"`   // sinusoidal components
}

// IsZero returns true if all GPS parameters are zero (no PPS error configured).
func (c GPSConfig) IsZero() bool {
	return c.Jitter == 0 && c.Sawtooth.Amp == 0 && c.AR1.Tau == 0 &&
		c.RandomWalk == 0 && len(c.Sinusoid) == 0
}

func DefaultGPSConfig() GPSConfig {
	return GPSConfig{
		Jitter:     0.25,
		RandomWalk: 0.000143,
		Sawtooth: SawtoothConfig{
			Amp:       15,
			PhaseInit: 0.5,
			InternalClock: Sinusoid{
				Amp:       2.0,       // 2 ppb amplitude
				Period:    600.0,     // 10 minute period
				PhaseInit: 1.0 / 6.0, // π/3 radians = 1/6 cycle in [0,1)
			},
		},
	}
}

// SawtoothConfig holds GPS sawtooth error parameters
type SawtoothConfig struct {
	Amp           float64  `toml:"amp"`           // nanoseconds (tick size)
	PhaseInit     float64  `toml:"phaseInit"`     // initial phase [0,1), defaults to 0.5
	InternalClock Sinusoid `toml:"internalClock"` // GPS receiver's internal oscillator
}

// AR1Config holds AR(1) colored noise parameters using tau/sigma parameterisation.
// tau is the correlation time constant in seconds.
// sigma is the steady-state RMS in nanoseconds.
type AR1Config struct {
	Tau   float64 `toml:"tau"`   // seconds (correlation time constant)
	Sigma float64 `toml:"sigma"` // nanoseconds (steady-state RMS)
}

// AlphaNoise returns (alpha, noise_stddev_ns) for use with AR1ColoredNoiseGPS.
// alpha is the autocorrelation coefficient, noise is the driving noise stddev.
// Assumes 1-second sample interval.
func (c AR1Config) AlphaNoise() (alpha, noise float64) {
	if c.Tau <= 0 || c.Sigma <= 0 {
		return 0, 0
	}
	alpha = math.Exp(-1.0 / c.Tau)
	alpha = min(alpha, 1.0-1e-12)
	noise = c.Sigma * math.Sqrt(max(0, 1-alpha*alpha))
	return alpha, noise
}

// HWConfig holds hardware characteristics for PHC and GPS (TOML-loadable)
type HWConfig struct {
	PHC PHCConfig `toml:"phc"`
	GPS GPSConfig `toml:"gps"`
}

// DefaultHWConfig returns an HWConfig with sensible default hardware characteristics.
func DefaultHWConfig() HWConfig {
	return HWConfig{
		PHC: DefaultPHCConfig(),
		GPS: DefaultGPSConfig(),
	}
}

// LoadHWConfig loads hardware configuration from a TOML file into hw.
// The caller initializes hw (zero-valued or with defaults), and TOML values are merged on top.
func LoadHWConfig(path string, hw *HWConfig) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewDecoder(f).DisallowUnknownFields().Decode(hw)
}

// CreateSimulator returns a GPSSimulator combining all GPS error sources.
// Applies components in order: jitter, AR(1), random walk, sinusoids.
// Does NOT include Shift, Outlier, or Sawtooth - those are added separately in Simulate().
// Sawtooth is created separately with oscillator coupling.
func (c GPSConfig) CreateSimulator() clocksim.GPSSimulator {
	sims := []clocksim.GPSSimulator{}
	if c.Jitter > 0 {
		sims = append(sims, clocksim.JitterGPS(time.Duration(c.Jitter)*time.Nanosecond, 123))
	}
	if alpha, noise := c.AR1.AlphaNoise(); alpha > 0 {
		sims = append(sims, clocksim.AR1ColoredNoiseGPS(alpha, noise, 124))
	}
	if c.RandomWalk > 0 {
		// Convert from ppb/√s to dimensionless/√s
		hPlus1 := c.RandomWalk * 1e-9
		sims = append(sims, clocksim.RandomWalkFMGPS(hPlus1, 125))
	}
	for _, s := range c.Sinusoid {
		sims = append(sims, clocksim.SinusoidGPS(s.Amp, s.Period, s.PhaseInit))
	}
	return clocksim.CombineGPS(sims...)
}

// OutlierConfig configures PPS outlier injection for testing.
// Used to test outlier detection algorithms.
type OutlierConfig struct {
	Times  []float64     // seconds at which to inject outliers (rounded to nearest integer)
	Offset time.Duration // magnitude of outlier phase offset
}

// ShiftConfig configures a temporary PPS phase shift for testing.
// Used to simulate ionospheric disturbances or other gradual timing biases.
type ShiftConfig struct {
	StartTime float64       // start time in seconds
	Ramp      time.Duration // ramp up/down duration (symmetric)
	Duration  time.Duration // total duration (includes ramp up + hold + ramp down)
	Shift     time.Duration // phase shift amount
}

// inOutage returns true if time t falls within an outage period.
// Odd-indexed intervals (1, 3, 5...) are outage periods.
func inOutage(t float64, toggleTimes []float64) bool {
	for i, toggle := range toggleTimes {
		if t < toggle {
			return i%2 == 1
		}
	}
	return len(toggleTimes)%2 == 1
}

const (
	tickInterval = 0.25
	NoPulse      = gpsprot.NavSolution // use NavSolution value to indicate no sawtooth messages
)

// Event represents a simulation event with a time
type Event struct {
	Time float64
	Type EventType
	Data any
}

type EventType int

const (
	EventPulse EventType = iota
	EventNavSolutionMsg
	EventTick
	EventPrePulseMsg
	EventPostPulseMsg
)

type PulseEventData struct {
	EdgeIdx int
	PPS     float64
}

type NavSolutionMsgEventData struct {
	PPS float64
}

type PrePulseMsgEventData struct {
	PPS      float64
	Sawtooth float64
}

type PostPulseMsgEventData struct {
	PPS float64
}

type TickEventData struct {
}

type modeObserver struct {
	obs.DefaultObserver
	initSamples       int
	convergingSamples int
	trackingSamples   int
}

func (m *modeObserver) Sample(s phcsync.Sample) {
	switch s.Mode {
	case phcsync.ModeReset:
		m.initSamples++
	case phcsync.ModeConverging:
		m.convergingSamples++
	case phcsync.ModeTracking:
		m.trackingSamples++
	}
}

// Stats holds simulation results
type Stats struct {
	statsobs.Stats                  // embedded - detailed tracking statistics from observer
	SampleCount       int           // total samples fed to controller
	TrackingStdDev    time.Duration // stddev from true time (simulation-only)
	TrackingMean      time.Duration // mean offset from true time (simulation-only)
	TrackingAbsMax    time.Duration // max absolute offset from true time (simulation-only)
	InitSamples       int           // samples processed in reset mode (includes initial sync and recovery)
	ConvergingSamples int           // samples processed in converging mode
	TrackingSamples   int           // samples processed in tracking mode
}

// Simulate runs a phcsync simulation with the given configuration.
// curTime is updated as the simulation progresses, allowing callers to use it for logging.
// It returns statistics about the simulation run.
func Simulate(observers []obs.Observer, phcCfg phcsync.Config, simCfg Config, curTime *time.Time, lg *slog.Logger) (Stats, error) {
	// Validate phcsync configuration
	if err := phcCfg.Validate(); err != nil {
		return Stats{}, err
	}
	// Create oscillator from PHC config
	osc := simCfg.PHC.CreateSimulator()

	// PHC starts at epoch (1970-01-01T00:00:00 TAI) - way off from GPS
	raw := clocksim.NewRawClock(osc, 0)

	// Build other PPS simulators (without sawtooth)
	otherPPSSims := []clocksim.GPSSimulator{simCfg.GPS.CreateSimulator()}

	// Add shift if configured
	if simCfg.Shift.Shift != 0 {
		otherPPSSims = append(otherPPSSims, clocksim.ShiftPPS(
			simCfg.Shift.StartTime,
			simCfg.Shift.Ramp,
			simCfg.Shift.Duration,
			simCfg.Shift.Shift,
		))
	}

	// Add outliers if configured
	for _, second := range simCfg.Outlier.Times {
		otherPPSSims = append(otherPPSSims, clocksim.SingleOutlierPPS(second, simCfg.Outlier.Offset))
	}

	otherPPS := clocksim.CombineGPS(otherPPSSims...)

	// Create sawtooth separately with GPS receiver's internal oscillator
	var sawtoothPPS clocksim.GPSSimulator
	if simCfg.GPS.Sawtooth.Amp > 0 {
		ampSec := simCfg.GPS.Sawtooth.Amp * 1e-9 // Convert ns to seconds
		// Create GPS receiver's internal oscillator (independent from PHC)
		// Uses PhaseInit field from config (defaults applied by LoadHWConfig)
		gpsOsc := clocksim.SinusoidOsc(
			simCfg.GPS.Sawtooth.InternalClock.Amp,
			simCfg.GPS.Sawtooth.InternalClock.Period,
			simCfg.GPS.Sawtooth.InternalClock.PhaseInit,
		)
		sawtoothPPS = clocksim.SawtoothGPS(gpsOsc, ampSec, simCfg.GPS.Sawtooth.PhaseInit)
	}

	// Prepare dual-edge mode parameters
	var pulseWidth time.Duration
	var trailingEdgeSim clocksim.GPSSimulator
	var pulseType phcsync.PulseType

	if simCfg.PulseWidth > 0 {
		pulseWidth = time.Duration(simCfg.PulseWidth * 1e9)
		trailingEdgeSim = clocksim.JitterGPS(2*time.Nanosecond, 789)
		pulseType = phcsync.PulseType{
			EdgesPerPulse: 2,
			PulseWidth:    pulseWidth,
		}
	} else {
		pulseType = phcsync.PulseType{
			EdgesPerPulse: 1,
			PulseWidth:    0,
		}
	}

	// Virtual clock starts at t=0, max ±500ppm (like Intel i210)
	vclock := clocksim.NewVirtualClock(raw, sawtoothPPS, otherPPS, 0, 500000, pulseWidth, trailingEdgeSim)

	// Test clock with era tracking
	testClock := clocksim.NewTestClock(vclock)

	// Leap second (current value as of 2017)
	ls := ptime.LeapSecond{
		UTCOffBefore:  37,
		UTCOffAfter:   37,
		OffChangeTime: 1483228800, // 2017-01-01
	}

	// Convert start time to TAI once
	tStart, _ := ls.SysToTime(*curTime)

	// Create timemsg.Buffer
	timeMsgBuf := timemsg.NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)

	// Create mode observer
	modeObs := &modeObserver{}

	// Create internal stats observer
	statsObs := statsobs.NewStatsObserver()

	// Combine all observers
	allObservers := append([]obs.Observer{statsObs, modeObs}, observers...)
	multiObs := obs.NewMultiObserver(allObservers...)

	// Create controller
	ctrl, err := phcsync.NewController(
		testClock,
		multiObs,
		nil, // no grandmaster
		phcCfg,
		ls,
		pulseType,
		lg,
	)
	if err != nil {
		return Stats{}, err
	}
	ctrl.SetTimeMsgBuffer(timeMsgBuf)
	defer ctrl.Close()

	lg.Info("starting phcsync simulation",
		"duration", simCfg.Duration,
		"pulseDelay", "5µs-250µs",
		"msgDelay", simCfg.MsgDelay,
		"phcFreqOffset", simCfg.PHC.FreqOffset,
		"phcDrift", simCfg.PHC.Drift,
		"phcWhiteNoise", simCfg.PHC.WhiteNoise,
		"ppsJitter", simCfg.GPS.Jitter,
		"startTime", curTime.Format(time.RFC3339),
		"phcStartTime", 0.0)

	// Generate event streams
	// Note: ticks start at t=0.25, modeling real system behavior where ticks
	// run continuously from the start. Early ticks are safe - see generateTickEvents.
	pulseGen := generatePulseEvents(simCfg, pulseType.EdgesPerPulse)
	msgGen := generateNavSolutionMsgEvents(simCfg)
	tickGen := generateTickEvents(simCfg)

	// Merge and process events
	events := mergeEvents(pulseGen, msgGen, tickGen)

	sampleCount := 0
	stats := &offsetStats{}
	var lastReading clocksim.TimestampReading

	for event := range events {
		// Update current time for logging
		tCur := tStart.Add(time.Duration(event.Time * 1e9))
		*curTime = ls.TimeToSys(tCur)

		// Main loop is responsible for ALL vclock advancement
		vclock.AdvanceTo(event.Time)

		// Read timestamp if available (after vclock advancement)
		if vclock.TimestampAvailable() {
			lastReading, _ = testClock.ReadTimestamp()
		}

		// Dispatch based on event type
		switch event.Type {
		case EventTick:
			handleTickEvent(event.Time, ctrl)

		case EventPrePulseMsg:
			data := event.Data.(PrePulseMsgEventData)
			if !inOutage(data.PPS, simCfg.ToggleTimes) {
				// Only create PrePulse message if sawtooth configured
				if lastReading.Sawtooth != nil {
					// Sawtooth.Next is rawSaw where pulse_time = true_second + rawSaw.
					// PulseOffset is defined as: true_second = pulse_time + PulseOffset.
					// Therefore: PulseOffset = -rawSaw
					pulseOffset := -lastReading.Sawtooth.Next * 1e9
					tMsg := tStart.Add(time.Duration(data.PPS * 1e9))
					timeMsg := &gpsprot.TimeMsg{
						TAITime:     tMsg,
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
						PulseOffset: &pulseOffset,
					}
					msgTRead := time.Unix(0, 0).Add(time.Duration(event.Time * 1e9))
					timeMsgBuf.Time(timeMsg, msgTRead)
					ctrl.TimeMessage()
					lg.Debug("delivering PrePulse message",
						"second", int(data.PPS),
						"taiTime", tMsg,
						"pulseOffset", pulseOffset)
				}
			}

		case EventPostPulseMsg:
			data := event.Data.(PostPulseMsgEventData)
			if !inOutage(data.PPS, simCfg.ToggleTimes) {
				// Only create PostPulse message if sawtooth configured
				if lastReading.Sawtooth != nil {
					// KEY DIFFERENCE: PostPulse uses Sawtooth.Current (not Next)
					// because the message arrives AFTER the pulse has occurred
					// Sawtooth.Current is rawSaw where pulse_time = true_second + rawSaw.
					// PulseOffset is defined as: true_second = pulse_time + PulseOffset.
					// Therefore: PulseOffset = -rawSaw
					pulseOffset := -lastReading.Sawtooth.Current * 1e9
					tMsg := tStart.Add(time.Duration(data.PPS * 1e9))
					timeMsg := &gpsprot.TimeMsg{
						TAITime:     tMsg,
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PostPulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TOS",
						PulseOffset: &pulseOffset,
					}
					msgTRead := time.Unix(0, 0).Add(time.Duration(event.Time * 1e9))
					timeMsgBuf.Time(timeMsg, msgTRead)
					ctrl.TimeMessage()
					lg.Debug("delivering PostPulse message",
						"second", int(data.PPS),
						"taiTime", tMsg,
						"pulseOffset", pulseOffset)
				}
			}

		case EventPulse:
			data := event.Data.(PulseEventData)

			// Use stored timestamp reading from tick
			tTrue := tStart.Add(time.Duration(lastReading.TrueTime * 1e9))
			offset := lastReading.Timestamp.T.Sub(tTrue)

			// Track offset for leading edge in tracking mode
			if data.EdgeIdx == 0 && ctrl.Mode() == phcsync.ModeTracking {
				stats.add(offset)
			}

			// Deliver to controller if not in outage
			if !inOutage(data.PPS, simCfg.ToggleTimes) {
				tSys := time.Unix(0, 0).Add(time.Duration(event.Time * 1e9))
				tReadPHC := testClock.Now()

				edge := phcsync.PulseEdge{
					Timestamp: lastReading.Timestamp,
					TRead: ptime.Sample{
						Clock: tReadPHC,
						Sys:   tSys,
					},
				}
				lg.Debug("delivering pulse to controller",
					"second", int(data.PPS),
					"edgeIdx", data.EdgeIdx,
					"timestamp", lastReading.Timestamp.T,
					"era", lastReading.Timestamp.Era,
					"offset", offset)
				ctrl.PulseEdge(edge)
			}

		case EventNavSolutionMsg:
			data := event.Data.(NavSolutionMsgEventData)
			if !inOutage(data.PPS, simCfg.ToggleTimes) {
				handleNavSolutionMsgEvent(event.Time, data, timeMsgBuf, ctrl, tStart, lg)
				sampleCount++
			}
		}
	}

	// Get stats from observers
	trackingStats := statsObs.Stats()

	return Stats{
		Stats:             trackingStats,
		SampleCount:       sampleCount,
		TrackingStdDev:    stats.stdDevRounded(),
		TrackingAbsMax:    stats.absMax,
		TrackingMean:      stats.mean(),
		InitSamples:       modeObs.initSamples,
		ConvergingSamples: modeObs.convergingSamples,
		TrackingSamples:   modeObs.trackingSamples,
	}, nil
}

type offsetStats struct {
	count      int64
	sum        time.Duration
	absMax     time.Duration
	sumSquares int64
}

func (s *offsetStats) add(d time.Duration) {
	ns := d.Nanoseconds()
	s.count++
	s.sum += d
	s.absMax = max(d.Abs(), s.absMax)
	s.sumSquares += ns * ns
}

func (s *offsetStats) stdDevRounded() time.Duration {
	std := s.stdDev()
	return time.Duration(math.Round(std))
}

func (s *offsetStats) mean() time.Duration {
	if s.count == 0 {
		return 0
	}
	return time.Duration(math.Round(float64(s.sum) / float64(s.count)))
}

func (s *offsetStats) stdDev() float64 {
	if s.count <= 1 {
		return 0
	}
	n := float64(s.count)
	sumSq := float64(s.sumSquares)
	sum := float64(s.sum.Nanoseconds())
	variance := (sumSq - (sum*sum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

// generatePulseEvents creates a push-style iterator that yields pulse edge events.
// Generates ALL pulses - does NOT filter by outages (filtering happens in main loop).
func generatePulseEvents(cfg Config, edgesPerPulse int) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(999))
		prePulseTime := cfg.PrePulseTime
		if prePulseTime == 0 {
			prePulseTime = 0.95
		}
		for pps := 1.0; pps < cfg.Duration; pps += 1.0 {
			pulseDelay := cfg.MinDelay + rng.Float64()*(cfg.MaxDelay-cfg.MinDelay)
			risingTime := pps + pulseDelay

			// Emit PrePulse event only if sawtooth is configured and PrePulse mode
			if cfg.GPS.Sawtooth.Amp > 0 && cfg.SawtoothMsgType == gpsprot.PrePulse {
				prePulseEventTime := risingTime - prePulseTime
				if !yield(Event{
					Time: prePulseEventTime,
					Type: EventPrePulseMsg,
					Data: PrePulseMsgEventData{PPS: pps, Sawtooth: 0}, // filled by main loop
				}) {
					return
				}
			}

			// Emit rising edge pulse event
			if !yield(Event{
				Time: risingTime,
				Type: EventPulse,
				Data: PulseEventData{
					EdgeIdx: 0,
					PPS:     pps,
				},
			}) {
				return
			}

			// Emit PostPulse event only if sawtooth is configured and PostPulse mode
			if cfg.GPS.Sawtooth.Amp > 0 && cfg.SawtoothMsgType == gpsprot.PostPulse {
				postPulseEventTime := risingTime + cfg.PostPulseMsgDelay
				if !yield(Event{
					Time: postPulseEventTime,
					Type: EventPostPulseMsg,
					Data: PostPulseMsgEventData{PPS: pps},
				}) {
					return
				}
			}

			if edgesPerPulse == 2 {
				trailingTime := pps + cfg.PulseWidth + pulseDelay
				if !yield(Event{
					Time: trailingTime,
					Type: EventPulse,
					Data: PulseEventData{
						EdgeIdx: 1,
						PPS:     pps,
					},
				}) {
					return
				}
			}
		}
	}
}

// generateMessageEvents creates a push-style iterator that yields GPS message events.
// Generates ALL messages - does NOT filter by outages (filtering happens in main loop).
func generateNavSolutionMsgEvents(cfg Config) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(888))
		for pps := 1.0; pps < cfg.Duration; pps += 1.0 {
			msgDelayTime := cfg.MsgDelay + rng.NormFloat64()*cfg.MsgJitter
			if msgDelayTime < 0 {
				msgDelayTime = 0
			}
			msgTime := pps + msgDelayTime
			if !yield(Event{
				Time: msgTime,
				Type: EventNavSolutionMsg,
				Data: NavSolutionMsgEventData{PPS: pps},
			}) {
				return
			}
		}
	}
}

// generateTickEvents creates a push-style iterator that yields tick events.
// Ticks happen every 250ms regardless of outages, starting immediately at t=0.25.
// Note: Early ticks (before first sample) are safe because controller.Tick()
// returns early when in ModeReset, and mode only exits Reset after first sample
// sets lastSample (controller.go:203).
func generateTickEvents(cfg Config) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for t := tickInterval; t < cfg.Duration; t += tickInterval {
			if !yield(Event{
				Time: t,
				Type: EventTick,
				Data: TickEventData{},
			}) {
				return
			}
		}
	}
}

// mergeEvents takes three event generators and merges them chronologically
// Returns events in time order until all generators are exhausted
func mergeEvents(
	pulseGen iter.Seq[Event],
	msgGen iter.Seq[Event],
	tickGen iter.Seq[Event],
) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		pulseNext, pulseStop := iter.Pull(pulseGen)
		msgNext, msgStop := iter.Pull(msgGen)
		tickNext, tickStop := iter.Pull(tickGen)
		defer pulseStop()
		defer msgStop()
		defer tickStop()
		pulseEvent, pulseOK := pulseNext()
		msgEvent, msgOK := msgNext()
		tickEvent, tickOK := tickNext()
		for pulseOK || msgOK || tickOK {
			var nextEvent Event
			if pulseOK && (!msgOK || pulseEvent.Time <= msgEvent.Time) && (!tickOK || pulseEvent.Time <= tickEvent.Time) {
				nextEvent = pulseEvent
				pulseEvent, pulseOK = pulseNext()
			} else if msgOK && (!tickOK || msgEvent.Time <= tickEvent.Time) {
				nextEvent = msgEvent
				msgEvent, msgOK = msgNext()
			} else {
				nextEvent = tickEvent
				tickEvent, tickOK = tickNext()
			}
			if !yield(nextEvent) {
				return
			}
		}
	}
}

// readPulseTimestamp reads the timestamp from vclock and calculates offset.
// Called for ALL pulse events (even during outages) to track PHC drift.
// Returns timestamp data that can optionally be delivered to controller.
func handleNavSolutionMsgEvent(
	eventTime float64,
	data NavSolutionMsgEventData,
	timeMsgBuf *timemsg.Buffer,
	ctrl *phcsync.Controller,
	tStart ptime.Time,
	lg *slog.Logger,
) {
	tMsg := tStart.Add(time.Duration(data.PPS * 1e9))
	timeMsg := &gpsprot.TimeMsg{
		TAITime:     tMsg,
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         ubx.Tag,
		NativeMsgID: "NAV-PVT",
	}
	msgTRead := time.Unix(0, 0).Add(time.Duration(eventTime * 1e9))
	timeMsgBuf.Time(timeMsg, msgTRead)
	lg.Debug("delivering time message",
		"second", int(data.PPS),
		"taiTime", tMsg,
		"msgTRead", msgTRead)
	ctrl.TimeMessage()
}

// handleTickEvent processes a tick event
func handleTickEvent(eventTime float64, ctrl *phcsync.Controller) {
	sys := time.Unix(0, 0).Add(time.Duration(eventTime * 1e9))
	ctrl.Tick(sys)
}
