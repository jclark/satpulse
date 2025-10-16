//go:build ignore

package main

import (
	"flag"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/logobs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/timemsg"
	"github.com/jclark/satpulse/internal/ubx"
)

var (
	duration     = flag.Float64("duration", 60.0, "simulation duration in seconds")
	oscDrift     = flag.Float64("drift", 10000.0, "oscillator drift in ppb")
	oscNoise     = flag.Float64("noise", 1.0, "oscillator frequency noise stddev in ppb")
	ppsJitter    = flag.Float64("jitter", 10e-9, "PPS timing jitter in seconds")
	minDelay     = flag.Float64("min-delay", 5e-6, "minimum pulse delivery delay in seconds")
	maxDelay     = flag.Float64("max-delay", 250e-6, "maximum pulse delivery delay in seconds")
	msgDelay     = flag.Float64("msg-delay", 0.1, "GPS message delay after pulse in seconds")
	msgJitter    = flag.Float64("msg-jitter", 0.01, "GPS message delay jitter in seconds")
	statsInt     = flag.Int("stats", 10, "statistics interval in seconds (0 to disable)")
	clockLogPath = flag.String("clock-log", "/tmp/synctest-clock.log", "path to clock log file")
	logLevel     = flag.String("log-level", "INFO", "log level (DEBUG, INFO, WARN, ERROR)")
)

func main() {
	flag.Parse()

	// Configure logging
	level := slog.LevelInfo
	switch *logLevel {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}
	lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	// GPS time starts near present (2024-10-08 is roughly GPS time ~1.4e9 seconds)
	const gpsStartTime = 1.4e9

	// Create oscillator with drift and noise
	osc := clocksim.CombineOscillators(
		clocksim.ConstantDrift(*oscDrift),
		clocksim.WhiteFreqNoise(*oscNoise, 42),
	)

	// PHC starts at epoch (1970-01-01T00:00:00 TAI) - way off from GPS
	raw := clocksim.NewRawClock(osc, 0)

	// PPS with jitter
	pps := clocksim.WhiteNoisePPS(*ppsJitter, 123)

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

	// Create samplers
	statsObs := logobs.NewStatsLogObserver(lg, *statsInt)
	clockObs, err := logobs.NewClockLogObserver(lg, *clockLogPath, ls)
	if err != nil {
		lg.Error("failed to create clock log observer", "err", err)
		os.Exit(1)
	}
	defer clockObs.Release()

	// Create multi-sampler
	sampler := &multiSampler{samplers: []phcsync.Sampler{statsObs, clockObs}}

	// Create controller
	cfg := phcsync.DefaultConfig()
	ctrl, err := phcsync.NewController(
		testClock,
		timeMsgBuf,
		sampler,
		nil, // no grandmaster
		nil, // no refclock
		cfg,
		ls,
		lg,
	)
	if err != nil {
		lg.Error("failed to create controller", "err", err)
		os.Exit(1)
	}
	defer ctrl.Close()

	lg.Info("starting phcsync simulation",
		"duration", *duration,
		"pulseDelay", "5µs-250µs",
		"msgDelay", *msgDelay,
		"oscDrift", *oscDrift,
		"oscNoise", *oscNoise,
		"ppsJitter", *ppsJitter,
		"gpsStartTime", gpsStartTime,
		"phcStartTime", 0.0,
		"clockLog", *clockLogPath)

	rng := rand.New(rand.NewSource(999))
	sampleCount := 0
	nextPPS := 1.0 // First PPS at t=1.0

	for nextPPS < *duration {
		// Generate random pulse delivery delay (uniform between min and max)
		pulseDelay := *minDelay + rng.Float64()*(*maxDelay-*minDelay)

		// Advance simulation time to when pulse timestamp is delivered to userspace
		pulseDeliveryTime := nextPPS + pulseDelay
		vclock.AdvanceTo(pulseDeliveryTime)

		// Read the timestamp that was just delivered
		if !vclock.TimestampAvailable() {
			lg.Error("expected timestamp not available", "nextPPS", nextPPS, "simTime", pulseDeliveryTime)
			break
		}

		timestamp, ok := testClock.ReadTimestampWithEra()
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

		// Feed pulse to controller
		lg.Debug("delivering pulse",
			"second", int(nextPPS),
			"timestamp", timestamp.T,
			"era", timestamp.Era)
		ctrl.PulseEdge(edge)

		// Now generate and deliver GPS time message
		// GPS time for this second (TAI)
		gpsTime := ptime.Time((gpsStartTime + nextPPS) * 1e9)

		// Create time message
		timeMsg := &gpsprot.TimeMsg{
			TAITime:     gpsTime,
			GNSS:        gpsprot.GPS,
			Ref:         gpsprot.PostPulse,
			Tag:         ubx.Tag,
			NativeMsgID: "NAV-PVT",
		}

		// Message arrives some time after pulse
		msgDelayTime := *msgDelay + rng.NormFloat64()*(*msgJitter)
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

		nextPPS += 1.0
	}

	// Final flush of stats
	statsObs.Release()

	// Get final frequency from clock
	finalFreq, _ := testClock.FreqOffset()

	lg.Info("simulation complete",
		"samples", sampleCount,
		"finalFreq", finalFreq)
}

// multiSampler combines multiple samplers
type multiSampler struct {
	samplers []phcsync.Sampler
}

func (m *multiSampler) Sample(data phcsync.SampleData) {
	for _, s := range m.samplers {
		s.Sample(data)
	}
}
