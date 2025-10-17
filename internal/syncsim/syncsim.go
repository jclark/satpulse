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

	// Virtual clock starts at t=0, max ±500ppm (like Intel i210)
	vclock := clocksim.NewVirtualClock(raw, pps, 0, 500000)

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

		// Advance simulation time to when pulse timestamp is delivered to userspace
		pulseDeliveryTime := nextPPS + pulseDelay
		vclock.AdvanceTo(pulseDeliveryTime)

		// Read the timestamp that was just delivered
		if !vclock.TimestampAvailable() {
			lg.Error("expected timestamp not available", "nextPPS", nextPPS, "simTime", pulseDeliveryTime)
			break
		}

		timestamp, trueTime, ok := testClock.ReadTimestampWithEra()
		if !ok {
			lg.Error("failed to read timestamp")
			break
		}

		// Create TRead (monotonic time when read occurred)
		// In real system this would be time.Now(), here we simulate it
		tRead := time.Unix(0, 0).Add(time.Duration(pulseDeliveryTime * 1e9))

		// Read current PHC time for TReadPHC
		tReadPHC := testClock.Now()

		// Create PulseEdge
		edge := phcsync.PulseEdge{
			Timestamp: timestamp,
			TRead:     tRead,
			TReadPHC:  tReadPHC,
		}

		// Compute true TAI time when the PPS occurred in the simulation.
		trueTAITime := ptime.Time((simCfg.GPSStartTime + trueTime) * 1e9)
		taiOffset := timestamp.T.Sub(trueTAITime)

		// Feed pulse to controller
		lg.Debug("delivering pulse",
			"second", int(nextPPS),
			"timestamp", timestamp.T,
			"era", timestamp.Era,
			"taiOffset", taiOffset)
		ctrl.PulseEdge(edge)

		// Now generate and deliver GPS time message
		// GPS time for this second (TAI)
		gpsTime := ptime.Time((simCfg.GPSStartTime + nextPPS) * 1e9)

		// Create time message
		timeMsg := &gpsprot.TimeMsg{
			TAITime:     gpsTime,
			GNSS:        gpsprot.GPS,
			Ref:         gpsprot.PostPulse,
			Tag:         ubx.Tag,
			NativeMsgID: "NAV-PVT",
		}

		// Message arrives some time after pulse
		msgDelayTime := simCfg.MsgDelay + rng.NormFloat64()*simCfg.MsgJitter
		if msgDelayTime < 0 {
			msgDelayTime = 0
		}
		msgArrivalTime := nextPPS + msgDelayTime

		// Advance to message arrival
		vclock.AdvanceTo(msgArrivalTime)

		// Message read time (monotonic)
		msgTRead := time.Unix(0, 0).Add(time.Duration(msgArrivalTime * 1e9))

		// Deliver message to buffer
		timeMsgBuf.Time(timeMsg, msgTRead)
		lg.Debug("delivering time message",
			"second", int(nextPPS),
			"gpsTime", gpsTime,
			"msgTRead", msgTRead)

		// Notify controller
		ctrl.TimeMessage()

		sampleCount++

		// Periodic tick (simulate 0.25s timer)
		// In real system this would be called by a ticker
		// Here we just call it after each sample
		ctrl.Tick()

		if ctrl.Tracking() {
			stats.add(taiOffset)
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
