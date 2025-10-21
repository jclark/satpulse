package syncsim

import (
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/statsobs"
	"github.com/jclark/satpulse/internal/timemsg"
	"github.com/jclark/satpulse/internal/ubx"
)

// Config holds simulation parameters
type Config struct {
	Duration     float64 // simulation duration in seconds
	OscDrift     float64 // oscillator drift in ppb
	OscNoise     float64 // oscillator frequency noise stddev in ppb
	PPSJitter    float64 // PPS timing jitter in nanoseconds
	MinDelay     float64 // minimum pulse delivery delay in seconds
	MaxDelay     float64 // maximum pulse delivery delay in seconds
	MsgDelay     float64 // GPS message delay after pulse in seconds
	MsgJitter    float64 // GPS message delay jitter in seconds
	GPSStartTime float64 // GPS time at start of simulation in seconds
	PulseWidth   float64 // pulse width in seconds (0 for single-edge mode)
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Duration:     60.0,
		OscDrift:     2000.0,
		OscNoise:     20.0,
		PPSJitter:    10.0,
		MinDelay:     5e-6,
		MaxDelay:     250e-6,
		MsgDelay:     0.1,
		MsgJitter:    0.01,
		GPSStartTime: 0, // caller should set this
		PulseWidth:   0, // default to single-edge mode
	}
}

// Stats holds simulation results
type Stats struct {
	statsobs.Stats                // embedded - detailed tracking statistics from observer
	SampleCount    int            // total samples fed to controller
	TrackingStdDev time.Duration  // stddev from true time (simulation-only)
}

// Simulate runs a phcsync simulation with the given configuration.
// It returns statistics about the simulation run.
func Simulate(observers []obs.Observer, phcCfg phcsync.Config, simCfg Config, lg *slog.Logger) (Stats, error) {
	// Create oscillator with drift and noise
	osc := clocksim.CombineOscillators(
		clocksim.ConstantDrift(simCfg.OscDrift),
		clocksim.WhiteFreqNoise(simCfg.OscNoise, 42),
	)

	// PHC starts at epoch (1970-01-01T00:00:00 TAI) - way off from GPS
	raw := clocksim.NewRawClock(osc, 0)

	// PPS with jitter
	pps := clocksim.WhiteNoisePPS(time.Duration(simCfg.PPSJitter)*time.Nanosecond, 123)

	// Prepare dual-edge mode parameters
	var pulseWidth time.Duration
	var trailingEdgeSim clocksim.PPSSimulator
	var pulseType phcsync.PulseType

	if simCfg.PulseWidth > 0 {
		pulseWidth = time.Duration(simCfg.PulseWidth * 1e9)
		trailingEdgeSim = clocksim.WhiteNoisePPS(2*time.Nanosecond, 789)
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
	vclock := clocksim.NewVirtualClock(raw, pps, 0, 500000, pulseWidth, trailingEdgeSim)

	// Test clock with era tracking
	testClock := clocksim.NewTestClock(vclock)

	// Leap second (current value as of 2017)
	ls := ptime.LeapSecond{
		UTCOffBefore:  37,
		UTCOffAfter:   37,
		OffChangeTime: 1483228800, // 2017-01-01
	}

	// Create timemsg.Buffer
	timeMsgBuf := timemsg.NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)

	// Create internal stats observer
	statsObs := statsobs.NewStatsObserver()

	// Combine with user-provided observers
	allObservers := append([]obs.Observer{statsObs}, observers...)
	multiObs := obs.NewMultiObserver(allObservers...)

	// Create controller
	ctrl, err := phcsync.NewController(
		testClock,
		timeMsgBuf,
		multiObs,
		nil, // no grandmaster
		nil, // no refclock
		phcCfg,
		ls,
		pulseType,
		lg,
	)
	if err != nil {
		return Stats{}, err
	}
	defer ctrl.Close()

	lg.Info("starting phcsync simulation",
		"duration", simCfg.Duration,
		"pulseDelay", "5µs-250µs",
		"msgDelay", simCfg.MsgDelay,
		"oscDrift", simCfg.OscDrift,
		"oscNoise", simCfg.OscNoise,
		"ppsJitter", simCfg.PPSJitter,
		"gpsStartTime", simCfg.GPSStartTime,
		"phcStartTime", 0.0)

	rng := rand.New(rand.NewSource(999))
	sampleCount := 0
	nextPPS := 1.0 // First PPS at t=1.0
	stats := &offsetStats{}

	for nextPPS < simCfg.Duration {
		// Generate random pulse delivery delay (uniform between min and max)
		pulseDelay := simCfg.MinDelay + rng.Float64()*(simCfg.MaxDelay-simCfg.MinDelay)

		// Calculate when various events happen
		risingEdgeTime := nextPPS + pulseDelay
		msgDelayTime := simCfg.MsgDelay + rng.NormFloat64()*simCfg.MsgJitter
		if msgDelayTime < 0 {
			msgDelayTime = 0
		}
		msgArrivalTime := nextPPS + msgDelayTime

		// Track offset of first (rising) edge for statistics
		var risingEdgeOffset time.Duration

		// In dual-edge mode, handle both edges and message in chronological order
		if pulseType.EdgesPerPulse == 2 {
			trailingEdgeTime := nextPPS + simCfg.PulseWidth + pulseDelay

			// Determine chronological order of events
			// Rising edge always comes first (at risingEdgeTime)
			// Then either trailing edge or message

			// Step 1: Advance to rising edge and deliver it
			vclock.AdvanceTo(risingEdgeTime)
			if !vclock.TimestampAvailable() {
				lg.Error("expected rising edge timestamp not available", "nextPPS", nextPPS)
				break
			}

			timestamp, trueTime, ok := testClock.ReadTimestampWithEra()
			if !ok {
				lg.Error("failed to read rising edge timestamp")
				break
			}

			tRead := time.Unix(0, 0).Add(time.Duration(risingEdgeTime * 1e9))
			tReadPHC := testClock.Now()

			edge := phcsync.PulseEdge{
				Timestamp: timestamp,
				TRead:     tRead,
				TReadPHC:  tReadPHC,
			}

			trueTAITime := ptime.Time((simCfg.GPSStartTime + trueTime) * 1e9)
			risingEdgeOffset = timestamp.T.Sub(trueTAITime)

			lg.Debug("delivering pulse",
				"second", int(nextPPS),
				"edgeIdx", 0,
				"timestamp", timestamp.T,
				"era", timestamp.Era,
				"taiOffset", risingEdgeOffset)
			ctrl.PulseEdge(edge)

			// Step 2: Advance to trailing edge and deliver it (message may come before or after)
			vclock.AdvanceTo(trailingEdgeTime)
			if !vclock.TimestampAvailable() {
				lg.Error("expected trailing edge timestamp not available", "nextPPS", nextPPS)
				break
			}

			timestamp, trueTime, ok = testClock.ReadTimestampWithEra()
			if !ok {
				lg.Error("failed to read trailing edge timestamp")
				break
			}

			tRead = time.Unix(0, 0).Add(time.Duration(trailingEdgeTime * 1e9))
			tReadPHC = testClock.Now()

			edge = phcsync.PulseEdge{
				Timestamp: timestamp,
				TRead:     tRead,
				TReadPHC:  tReadPHC,
			}

			trueTAITime = ptime.Time((simCfg.GPSStartTime + trueTime) * 1e9)
			taiOffset := timestamp.T.Sub(trueTAITime)

			lg.Debug("delivering pulse",
				"second", int(nextPPS),
				"edgeIdx", 1,
				"timestamp", timestamp.T,
				"era", timestamp.Era,
				"taiOffset", taiOffset)
			ctrl.PulseEdge(edge)

			// Step 3: Advance to message arrival (if not already past it)
			if msgArrivalTime > trailingEdgeTime {
				vclock.AdvanceTo(msgArrivalTime)
			}
		} else {
			// Single-edge mode: just advance to rising edge
			vclock.AdvanceTo(risingEdgeTime)
			if !vclock.TimestampAvailable() {
				lg.Error("expected timestamp not available", "nextPPS", nextPPS)
				break
			}

			timestamp, trueTime, ok := testClock.ReadTimestampWithEra()
			if !ok {
				lg.Error("failed to read timestamp")
				break
			}

			tRead := time.Unix(0, 0).Add(time.Duration(risingEdgeTime * 1e9))
			tReadPHC := testClock.Now()

			edge := phcsync.PulseEdge{
				Timestamp: timestamp,
				TRead:     tRead,
				TReadPHC:  tReadPHC,
			}

			trueTAITime := ptime.Time((simCfg.GPSStartTime + trueTime) * 1e9)
			risingEdgeOffset = timestamp.T.Sub(trueTAITime)

			lg.Debug("delivering pulse",
				"second", int(nextPPS),
				"timestamp", timestamp.T,
				"era", timestamp.Era,
				"taiOffset", risingEdgeOffset)
			ctrl.PulseEdge(edge)

			// Advance to message arrival
			vclock.AdvanceTo(msgArrivalTime)
		}

		// Now deliver GPS time message
		gpsTime := ptime.Time((simCfg.GPSStartTime + nextPPS) * 1e9)

		timeMsg := &gpsprot.TimeMsg{
			TAITime:     gpsTime,
			GNSS:        gpsprot.GPS,
			Ref:         gpsprot.PostPulse,
			Tag:         ubx.Tag,
			NativeMsgID: "NAV-PVT",
		}

		msgTRead := time.Unix(0, 0).Add(time.Duration(msgArrivalTime * 1e9))

		timeMsgBuf.Time(timeMsg, msgTRead)
		lg.Debug("delivering time message",
			"second", int(nextPPS),
			"gpsTime", gpsTime,
			"msgTRead", msgTRead)

		ctrl.TimeMessage()

		sampleCount++

		ctrl.Tick()

		if ctrl.Tracking() {
			stats.add(risingEdgeOffset)
		}

		nextPPS += 1.0
	}

	// Get tracking stats from observer
	trackingStats := statsObs.Stats()

	return Stats{
		Stats:          trackingStats,
		SampleCount:    sampleCount,
		TrackingStdDev: time.Duration(stats.stdDevRounded()),
	}, nil
}

type offsetStats struct {
	count      int64
	sum        time.Duration
	sumSquares int64
}

func (s *offsetStats) add(d time.Duration) {
	ns := d.Nanoseconds()
	s.count++
	s.sum += d
	s.sumSquares += ns * ns
}

func (s *offsetStats) stdDevRounded() int64 {
	std := s.stdDev()
	return int64(math.Round(std))
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
