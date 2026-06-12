// Package syncsim provides a discrete-event simulator for testing the phcsync controller.
//
// # Architecture
//
// The simulator uses an event-driven architecture built on Go 1.23 iterators:
//
//	generatePulseEvents()        ─┐
//	generatePulseMsgEvents()     ─┤
//	generateMessageEvents()      ─┼─> mergeEvents() ──> for event := range events
//	generateTickEvents()         ─┘
//
// Event streams are generated independently and merged chronologically:
//   - Pulse events: GPS PPS edges (rising, and trailing for dual-edge mode)
//   - Pulse-related messages tied to GPS PPS edges
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
//
// 6. Pulse delivery events pair with PHC timestamp readings in FIFO order. If an
// injected fault delays an edge past its scheduled delivery event, delivery is
// deferred to the first event after the edge fires, so a pulse is never delivered
// with a stale timestamp.
package syncsim

import (
	"fmt"
	"io"
	"iter"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/jclark/satpulse/time/lib/allan"
	"github.com/jclark/satpulse/time/clocksim"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/statsobs"
	"github.com/jclark/satpulse/time/internal/timemsg"
)

const (
	tickInterval = 0.25

	// Must be used by both pulse event generators, advancing once per PPS,
	// so pulse edges and pulse messages use the same delay for each PPS.
	pulseDelaySeed = 999
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
	prevMode    phcsync.Mode
	samples     [phcsync.NModes]int
	transitions [phcsync.NModes]int
}

func (m *modeObserver) Sample(s phcsync.Sample) {
	m.samples[s.Mode]++
	if s.Mode != m.prevMode {
		m.transitions[s.Mode]++
		m.prevMode = s.Mode
	}
}

// Stats holds simulation results
type Stats struct {
	statsobs.Stats                       // embedded - detailed tracking statistics from observer
	SampleCount     int                  // total samples fed to controller
	TrackingStdDev  float64              // stddev from true time in nanoseconds (simulation-only)
	TrackingMean    float64              // mean offset from true time in nanoseconds (simulation-only)
	TrackingAbsMax  time.Duration        // max absolute offset from true time (simulation-only)
	TrackingADev    float64              // Allan deviation of tracking offsets (simulation-only)
	ModeSamples     map[phcsync.Mode]int // samples per mode (non-zero only)
	ModeTransitions map[phcsync.Mode]int // transitions into each mode (non-zero only)
}

// String formats Stats for human-readable output.
func (s Stats) String() string {
	str := s.Stats.String() +
		fmt.Sprintf("sampleCount = %d\n"+
			"trackingStdDev = %.2f\n"+
			"trackingMean = %.2f\n"+
			"trackingAbsMax = %d\n"+
			"trackingADev = %.6e\n",
			s.SampleCount,
			s.TrackingStdDev,
			s.TrackingMean,
			s.TrackingAbsMax.Nanoseconds(),
			s.TrackingADev)
	for m := phcsync.Mode(0); m < phcsync.NModes; m++ {
		if n := s.ModeSamples[m]; n > 0 {
			str += fmt.Sprintf("%sSamples = %d\n", m, n)
		}
	}
	for m := phcsync.Mode(0); m < phcsync.NModes; m++ {
		if n := s.ModeTransitions[m]; n > 0 {
			str += fmt.Sprintf("%sEntered = %d\n", m, n)
		}
	}
	return str
}

// Simulate runs a phcsync simulation with the given configuration.
// tsLog is an optional writer for PHC timestamp log (JSON Lines format).
// curTime is updated as the simulation progresses, allowing callers to use it for logging.
// It returns statistics about the simulation run.
func Simulate(observers []obs.Observer, cfg Config, tsLog io.Writer, curTime *time.Time, lg *slog.Logger) (Stats, error) {
	// Validate phcsync configuration
	if err := cfg.Sync.Validate(); err != nil {
		return Stats{}, err
	}
	// Normalize outages once for efficient InOutage checks
	outages := cfg.Fault.NormalizeOutages()

	// Create oscillator from PHC config
	osc := cfg.PHC.CreateSimulator()

	// PHC starts at epoch (1970-01-01T00:00:00 TAI) - way off from GPS
	raw := clocksim.NewRawClock(osc, 0)

	// Build other PPS simulators (without sawtooth)
	otherPPSSims := []clocksim.GPSSimulator{cfg.GPS.CreateSimulator(cfg.Sim.StartTime)}

	// Add excursions if configured
	for _, exc := range cfg.Fault.Excursion {
		if !exc.IsZero() {
			otherPPSSims = append(otherPPSSims, clocksim.ExcursionPPS(
				exc.StartTime,
				exc.Duration,
				exc.Amplitude,
				exc.Rise.Duration,
				exc.Rise.EffectivePower(),
				exc.Fall.Duration,
				exc.Fall.EffectivePower(),
			))
		}
	}

	// Add outliers if configured
	for _, outlier := range cfg.Fault.Outlier {
		if !outlier.IsZero() {
			otherPPSSims = append(otherPPSSims, clocksim.SingleOutlierPPS(outlier.Time, outlier.Offset))
		}
	}

	otherPPS := clocksim.CombineGPS(otherPPSSims...)

	// Create sawtooth separately with GPS receiver's internal oscillator
	var sawtoothPPS clocksim.GPSSimulator
	if cfg.GPS.Sawtooth.Amp > 0 {
		ampSec := cfg.GPS.Sawtooth.Amp * 1e-9 // Convert ns to seconds
		ic := cfg.GPS.Sawtooth.InternalClock
		gpsOsc := clocksim.SinusoidOsc(ic.Amp, ic.Period, ic.PhaseInit)
		sawtoothPPS = clocksim.SawtoothGPS(gpsOsc, ampSec, cfg.GPS.Sawtooth.PhaseInit)
	}

	// Prepare dual-edge mode parameters
	var pulseWidth time.Duration
	var trailingEdgeSim clocksim.GPSSimulator
	var edgesPerPulse int

	if cfg.Pulse.Width > 0 {
		pulseWidth = time.Duration(cfg.Pulse.Width * 1e9)
		trailingEdgeSim = clocksim.JitterGPS(2, 789) // 2 nanoseconds
		edgesPerPulse = 2
	} else {
		edgesPerPulse = 1
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

	// Create mode observer
	modeObs := &modeObserver{}

	// Create internal stats observer
	statsObs := statsobs.NewStatsObserver()

	// Combine all observers
	allObservers := append([]obs.Observer{statsObs, modeObs}, observers...)
	multiObs := obs.NewMultiObserver(allObservers...)

	// Create controller before the buffer so the buffer can be sized
	// from the controller's requirement.
	ctrl, err := phcsync.NewController(
		testClock,
		multiObs,
		nil, // no grandmaster
		cfg.Sync,
		ls,
		edgesPerPulse,
		lg,
	)
	if err != nil {
		return Stats{}, err
	}
	timeMsgBuf := timemsg.NewBuffer(lg, ctrl.RequiredMsgWindow(), ls, gpsprot.GPS)
	ctrl.SetTimeMsgBuffer(timeMsgBuf)
	defer ctrl.Close()

	durSec := cfg.Sim.Duration
	lg.Info("starting phcsync simulation",
		"duration", durSec,
		"pulseMinDelay", cfg.Pulse.MinDelay,
		"pulseMaxDelay", cfg.Pulse.MaxDelay,
		"msgDelay", cfg.Msg.Delay,
		"phcFreqOffset", cfg.PHC.FreqOffset,
		"phcDrift", cfg.PHC.Drift,
		"phcWhiteNoise", cfg.PHC.WhiteNoise,
		"ppsJitter", cfg.GPS.Jitter,
		"startTime", curTime.Format(time.RFC3339),
		"phcStartTime", 0.0)

	// Generate event streams
	// Note: ticks start at t=0.25, modeling real system behavior where ticks
	// run continuously from the start. Early ticks are safe - see generateTickEvents.
	pulseGen := generatePulseEvents(cfg, durSec, edgesPerPulse)
	pulseMsgGen := generatePulseMsgEvents(cfg, durSec)
	msgGen := generateNavSolutionMsgEvents(cfg, durSec)
	tickGen := generateTickEvents(durSec)

	// Merge and process events
	events := mergeEvents(pulseGen, pulseMsgGen, msgGen, tickGen)

	sampleCount := 0
	stats := &offsetStats{adev: *allan.NewAccum(1.0)}
	var lastReading clocksim.TimestampReading
	trackingStarted := false
	var pending []Event

	for event := range events {
		// Update current time for logging
		tCur := tStart.Add(time.Duration(event.Time * 1e9))
		*curTime = ls.TimeToSys(tCur)

		// Main loop is responsible for ALL vclock advancement
		vclock.AdvanceTo(event.Time)

		// Pulse events pair with timestamp readings in FIFO order. A pulse
		// whose edge has not yet fired (injected shift exceeding the pulse
		// delay) stays pending until the first event after the edge, then
		// is processed here at the current event's time.
		if event.Type == EventPulse {
			pending = append(pending, event)
		}
		for len(pending) > 0 && vclock.TimestampAvailable() {
			lastReading, _ = testClock.ReadTimestamp()
			data := pending[0].Data.(PulseEventData)
			pending = pending[1:]

			tTrue := tStart.Add(time.Duration(lastReading.TrueTime * 1e9))
			offset := lastReading.Timestamp.T.Sub(tTrue)

			// Output PHC timestamp if writer configured (leading edge, after first tracking)
			if data.EdgeIdx == 0 {
				if !trackingStarted && ctrl.Mode() == phcsync.ModeTracking {
					trackingStarted = true
				}
				if trackingStarted && tsLog != nil {
					nsec := int64(lastReading.Timestamp.T - tStart)
					fmt.Fprintf(tsLog, `{"chan":"A","timestamp":"%d.%09d"}`+"\n", nsec/1e9, nsec%1e9)
				}
			}

			// Track offset for leading edge in tracking mode
			if data.EdgeIdx == 0 && ctrl.Mode() == phcsync.ModeTracking {
				stats.add(offset)
			}

			// Deliver to controller if not in outage
			if !InOutage(outages, data.PPS) {
				tSys := time.Unix(0, 0).Add(time.Duration(event.Time * 1e9))
				tReadPHC := testClock.Now()

				tr := phctime.Sample{
					PHC: tReadPHC,
					Sys: tSys,
				}
				lg.Debug("delivering pulse to controller",
					"second", int(data.PPS),
					"edgeIdx", data.EdgeIdx,
					"timestamp", lastReading.Timestamp.T,
					"era", lastReading.Timestamp.Era,
					"offset", offset)
				ctrl.Pulse(lastReading.Timestamp, tr)
			}
		}

		// Dispatch based on event type
		switch event.Type {
		case EventTick:
			handleTickEvent(event.Time, ctrl)

		case EventPrePulseMsg:
			data := event.Data.(PrePulseMsgEventData)
			if !InOutage(outages, data.PPS) {
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
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TP",
						PulseOffset: opt.Make(pulseOffset),
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
			if !InOutage(outages, data.PPS) {
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
						Tag:         gpsreg.TagUBX,
						NativeMsgID: "TIM-TOS",
						PulseOffset: opt.Make(pulseOffset),
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

		case EventNavSolutionMsg:
			data := event.Data.(NavSolutionMsgEventData)
			if !InOutage(outages, data.PPS) {
				handleNavSolutionMsgEvent(event.Time, data, timeMsgBuf, ctrl, tStart, lg)
				sampleCount++
			}
		}
	}

	// Get stats from observers
	trackingStats := statsObs.Stats()

	// Convert mode arrays to maps (non-zero only)
	modeSamples := make(map[phcsync.Mode]int)
	modeTransitions := make(map[phcsync.Mode]int)
	for m := phcsync.Mode(0); m < phcsync.NModes; m++ {
		if n := modeObs.samples[m]; n > 0 {
			modeSamples[m] = n
		}
		if n := modeObs.transitions[m]; n > 0 {
			modeTransitions[m] = n
		}
	}

	return Stats{
		Stats:           trackingStats,
		SampleCount:     sampleCount,
		TrackingStdDev:  stats.stdDev(),
		TrackingMean:    stats.mean(),
		TrackingAbsMax:  stats.absMax,
		TrackingADev:    stats.adev.ADev(),
		ModeSamples:     modeSamples,
		ModeTransitions: modeTransitions,
	}, nil
}

type offsetStats struct {
	count      int64
	sum        time.Duration
	absMax     time.Duration
	sumSquares int64
	adev       allan.Accum[float64]
}

func (s *offsetStats) add(d time.Duration) {
	ns := d.Nanoseconds()
	s.count++
	s.sum += d
	s.absMax = max(d.Abs(), s.absMax)
	s.sumSquares += ns * ns
	s.adev.Add(d.Seconds())
}

func (s *offsetStats) mean() float64 {
	if s.count == 0 {
		return 0
	}
	return float64(s.sum) / float64(s.count)
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
func generatePulseEvents(cfg Config, duration float64, edgesPerPulse int) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(pulseDelaySeed))
		for pps := 1.0; pps < duration; pps += 1.0 {
			pulseDelay := cfg.Pulse.MinDelay + rng.Float64()*(cfg.Pulse.MaxDelay-cfg.Pulse.MinDelay)
			risingTime := pps + pulseDelay

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
				trailingTime := pps + cfg.Pulse.Width + pulseDelay
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

func generatePulseMsgEvents(cfg Config, duration float64) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		if cfg.GPS.Sawtooth.Amp <= 0 || cfg.Msg.SawtoothType == SawtoothNone {
			return
		}
		rng := rand.New(rand.NewSource(pulseDelaySeed))
		prePulseTime := cfg.Msg.PrePulseTime
		if prePulseTime == 0 {
			prePulseTime = 0.95
		}
		for pps := 1.0; pps < duration; pps += 1.0 {
			pulseDelay := cfg.Pulse.MinDelay + rng.Float64()*(cfg.Pulse.MaxDelay-cfg.Pulse.MinDelay)
			risingTime := pps + pulseDelay

			if cfg.Msg.SawtoothType == SawtoothPrePulse {
				if !yield(Event{
					Time: risingTime - prePulseTime,
					Type: EventPrePulseMsg,
					Data: PrePulseMsgEventData{PPS: pps, Sawtooth: 0}, // filled by main loop
				}) {
					return
				}
			} else if cfg.Msg.SawtoothType == SawtoothPostPulse {
				if !yield(Event{
					Time: risingTime + cfg.Msg.PostPulseDelay,
					Type: EventPostPulseMsg,
					Data: PostPulseMsgEventData{PPS: pps},
				}) {
					return
				}
			}
		}
	}
}

// generateMessageEvents creates a push-style iterator that yields GPS message events.
// Generates ALL messages - does NOT filter by outages (filtering happens in main loop).
func generateNavSolutionMsgEvents(cfg Config, duration float64) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(888))
		for pps := 1.0; pps < duration; pps += 1.0 {
			msgDelayTime := cfg.Msg.Delay + rng.NormFloat64()*cfg.Msg.Jitter
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
func generateTickEvents(duration float64) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for t := tickInterval; t < duration; t += tickInterval {
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

// mergeEvents takes four event generators and merges them chronologically
// Returns events in time order until all generators are exhausted
func mergeEvents(
	pulseGen iter.Seq[Event],
	pulseMsgGen iter.Seq[Event],
	msgGen iter.Seq[Event],
	tickGen iter.Seq[Event],
) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		pulseNext, pulseStop := iter.Pull(pulseGen)
		pulseMsgNext, pulseMsgStop := iter.Pull(pulseMsgGen)
		msgNext, msgStop := iter.Pull(msgGen)
		tickNext, tickStop := iter.Pull(tickGen)
		defer pulseStop()
		defer pulseMsgStop()
		defer msgStop()
		defer tickStop()
		pulseEvent, pulseOK := pulseNext()
		pulseMsgEvent, pulseMsgOK := pulseMsgNext()
		msgEvent, msgOK := msgNext()
		tickEvent, tickOK := tickNext()
		for pulseOK || pulseMsgOK || msgOK || tickOK {
			var nextEvent Event
			if pulseOK &&
				(!pulseMsgOK || pulseEvent.Time <= pulseMsgEvent.Time) &&
				(!msgOK || pulseEvent.Time <= msgEvent.Time) &&
				(!tickOK || pulseEvent.Time <= tickEvent.Time) {
				nextEvent = pulseEvent
				pulseEvent, pulseOK = pulseNext()
			} else if pulseMsgOK &&
				(!msgOK || pulseMsgEvent.Time <= msgEvent.Time) &&
				(!tickOK || pulseMsgEvent.Time <= tickEvent.Time) {
				nextEvent = pulseMsgEvent
				pulseMsgEvent, pulseMsgOK = pulseMsgNext()
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

// handleNavSolutionMsgEvent delivers a NAV-PVT style time message to the controller.
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
		Tag:         gpsreg.TagUBX,
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
