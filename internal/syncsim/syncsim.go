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
	Duration    float64       // simulation duration in seconds
	PHC         PHCConfig     // PHC oscillator parameters
	GPS         GPSConfig     // GPS PPS parameters
	MinDelay    float64       // minimum pulse delivery delay in seconds
	MaxDelay    float64       // maximum pulse delivery delay in seconds
	MsgDelay    float64       // GPS message delay after pulse in seconds
	MsgJitter   float64       // GPS message delay jitter in seconds
	PulseWidth  float64       // pulse width in seconds (0 for single-edge mode)
	ToggleTimes []float64     // absolute simulation times to toggle pulse/message delivery on/off
	Outlier     OutlierConfig // PPS outlier injection configuration
	Shift       ShiftConfig   // PPS phase shift configuration
}

// PHCConfig holds PHC oscillator parameters (matches ticc-model TOML format)
type PHCConfig struct {
	FreqOffset       float64      `toml:"freqOffset"`       // ppb
	FreqDrift        float64      `toml:"freqDrift"`        // ppb/day
	WhiteFreqNoise   float64      `toml:"whiteFreqNoise"`   // ppb
	FlickerFreqNoise float64      `toml:"flickerFreqNoise"` // ppb
	SineFM           SineFMConfig `toml:"sineFM"`           // sinusoidal FM parameters
}

// SineFMConfig holds sinusoidal frequency modulation parameters
type SineFMConfig struct {
	Amp    float64 `toml:"amp"`    // ppb
	Period float64 `toml:"period"` // seconds
	Phase  float64 `toml:"phase"`  // radians
}

// CreateSimulator returns an OscillatorSimulator combining all PHC error sources.
// Applies components in order: offset, white noise, flicker noise, drift, sine FM.
func (c PHCConfig) CreateSimulator() clocksim.OscillatorSimulator {
	oscs := []clocksim.OscillatorSimulator{
		clocksim.FreqOffset(c.FreqOffset),
	}
	if c.WhiteFreqNoise > 0 {
		oscs = append(oscs, clocksim.WhiteFreqNoise(c.WhiteFreqNoise, 42))
	}
	if c.FlickerFreqNoise > 0 {
		oscs = append(oscs, clocksim.FlickerFreqNoise(c.FlickerFreqNoise, 43))
	}
	if c.FreqDrift != 0 {
		oscs = append(oscs, clocksim.FreqDrift(c.FreqDrift))
	}
	if c.SineFM.Amp > 0 {
		oscs = append(oscs, clocksim.SineFM(c.SineFM.Amp, c.SineFM.Period, 0))
	}
	return clocksim.CombineOscillators(oscs...)
}

// GPSConfig holds GPS PPS error parameters (matches ticc-model TOML format)
type GPSConfig struct {
	Jitter       float64             `toml:"jitter"`       // nanoseconds (white noise stddev)
	Sawtooth     SawtoothConfig      `toml:"sawtooth"`     // sawtooth error parameters
	AR1          AR1Config           `toml:"ar1"`          // AR(1) colored noise parameters
	Periodic     []PeriodicComponent `toml:"periodic"`     // periodic components (up to 3)
	PrePulseTime float64             `toml:"prePulseTime"` // seconds before pulse to send UBX-TIM-TP, defaults to 0.95
}

// SawtoothConfig holds GPS sawtooth error parameters
type SawtoothConfig struct {
	Amp           float64      `toml:"amp"`           // nanoseconds (tick size)
	PhaseInit     float64      `toml:"phaseInit"`     // initial phase [0,1), defaults to 0.5
	InternalClock SineFMConfig `toml:"internalClock"` // GPS receiver's internal oscillator
}

// AR1Config holds AR(1) colored noise parameters
type AR1Config struct {
	Alpha float64 `toml:"alpha"` // dimensionless (autocorrelation coefficient)
	Noise float64 `toml:"noise"` // nanoseconds (stddev)
}

// PeriodicComponent represents a single periodic GPS error component
type PeriodicComponent struct {
	Period float64 `toml:"period"` // seconds
	Amp    float64 `toml:"amp"`    // nanoseconds
}

// HWConfig holds hardware characteristics for PHC and GPS (TOML-loadable)
type HWConfig struct {
	PHC PHCConfig `toml:"phc"`
	GPS GPSConfig `toml:"gps"`
}

// LoadHWConfig loads hardware configuration from a TOML file.
// Starts with DefaultConfig() values and merges TOML values on top,
// so users can specify only the fields they want to override.
func LoadHWConfig(path string) (*HWConfig, error) {
	// Start with defaults
	defaults := DefaultConfig()
	hw := &HWConfig{
		PHC: defaults.PHC,
		GPS: defaults.GPS,
	}

	// Parse TOML and overlay user values
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	err = toml.NewDecoder(f).DisallowUnknownFields().Decode(hw)
	if err != nil {
		return nil, err
	}
	return hw, nil
}

// CreateSimulator returns a PPSSimulator combining all GPS error sources.
// Applies components in order: jitter, sawtooth, AR(1), periodic components.
// Does NOT include Shift, Outlier, or Sawtooth - those are added separately in Simulate().
// Sawtooth is created separately with oscillator coupling.
func (c GPSConfig) CreateSimulator() clocksim.PPSSimulator {
	sims := []clocksim.PPSSimulator{}
	if c.Jitter > 0 {
		sims = append(sims, clocksim.JitterPPS(time.Duration(c.Jitter)*time.Nanosecond, 123))
	}
	if c.AR1.Alpha > 0 {
		sims = append(sims, clocksim.AR1ColoredNoisePPS(c.AR1.Alpha, c.AR1.Noise, 124))
	}
	for _, p := range c.Periodic {
		sims = append(sims, clocksim.PeriodicPPS(p.Amp, p.Period))
	}
	return clocksim.CombinePPS(sims...)
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

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Duration: 60.0,
		PHC: PHCConfig{
			FreqOffset:     2000.0,
			FreqDrift:      -150.0,
			WhiteFreqNoise: 20.0,
		},
		GPS: GPSConfig{
			Jitter: 10.0,
			Sawtooth: SawtoothConfig{
				PhaseInit: 0.5,
				InternalClock: SineFMConfig{
					Amp:    2.0,             // 2 ppb amplitude
					Period: 600.0,           // 10 minute period
					Phase:  math.Pi / 3,     // π/3 radians - typical mid-range value
				},
			},
		},
		MinDelay:   5e-6,
		MaxDelay:   250e-6,
		MsgDelay:   0.1,
		MsgJitter:  0.01,
		PulseWidth: 0, // default to single-edge mode
		Outlier: OutlierConfig{
			Offset: 2000 * time.Nanosecond, // 2µs default outlier magnitude
		},
	}
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
	otherPPSSims := []clocksim.PPSSimulator{simCfg.GPS.CreateSimulator()}

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

	otherPPS := clocksim.CombinePPS(otherPPSSims...)

	// Create sawtooth separately with GPS receiver's internal oscillator
	var sawtoothPPS clocksim.PPSSimulator
	if simCfg.GPS.Sawtooth.Amp > 0 {
		ampSec := simCfg.GPS.Sawtooth.Amp * 1e-9 // Convert ns to seconds
		// Create GPS receiver's internal oscillator (independent from PHC)
		// Uses Phase field from config (defaults applied by LoadHWConfig)
		gpsOsc := clocksim.SineFM(
			simCfg.GPS.Sawtooth.InternalClock.Amp,
			simCfg.GPS.Sawtooth.InternalClock.Period,
			simCfg.GPS.Sawtooth.InternalClock.Phase,
		)
		sawtoothPPS = clocksim.SawtoothPPS(gpsOsc, ampSec, simCfg.GPS.Sawtooth.PhaseInit)
	}

	// Prepare dual-edge mode parameters
	var pulseWidth time.Duration
	var trailingEdgeSim clocksim.PPSSimulator
	var pulseType phcsync.PulseType

	if simCfg.PulseWidth > 0 {
		pulseWidth = time.Duration(simCfg.PulseWidth * 1e9)
		trailingEdgeSim = clocksim.JitterPPS(2*time.Nanosecond, 789)
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
		"phcFreqDrift", simCfg.PHC.FreqDrift,
		"phcWhiteNoise", simCfg.PHC.WhiteFreqNoise,
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
					sawtoothNs := lastReading.Sawtooth.Next * 1e9
					tMsg := tStart.Add(time.Duration(data.PPS * 1e9))
					timeMsg := &gpsprot.TimeMsg{
						TAITime:     tMsg,
						GNSS:        gpsprot.GPS,
						Ref:         gpsprot.PrePulse,
						Tag:         ubx.Tag,
						NativeMsgID: "UBX-TIM-TP",
						PulseOffset: &sawtoothNs,
					}
					msgTRead := time.Unix(0, 0).Add(time.Duration(event.Time * 1e9))
					timeMsgBuf.Time(timeMsg, msgTRead)
					lg.Debug("delivering PrePulse message",
						"second", int(data.PPS),
						"taiTime", tMsg,
						"pulseOffset", sawtoothNs)
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
		prePulseTime := cfg.GPS.PrePulseTime
		if prePulseTime == 0 {
			prePulseTime = 0.95
		}
		for pps := 1.0; pps < cfg.Duration; pps += 1.0 {
			pulseDelay := cfg.MinDelay + rng.Float64()*(cfg.MaxDelay-cfg.MinDelay)
			risingTime := pps + pulseDelay

			// Emit PrePulse event only if sawtooth is configured
			if cfg.GPS.Sawtooth.Amp > 0 {
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
