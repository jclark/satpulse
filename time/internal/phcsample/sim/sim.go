// Package sim provides a discrete-event simulator for testing the
// phcsample.Generator. It parallels time/internal/syncsim in scope:
// it reuses clocksim for oscillator and PPS simulation and it reuses
// the Go-level configuration types from syncsim for oscillator, GPS,
// pulse, message, and fault parameters. The rig feeds the Generator
// with synthesized PulseEdge events and UTC time messages and scores
// each returned offset against ground-truth true time.
//
// There is no TOML loader. Tests and evaluation programs construct
// Config values programmatically.
package sim

import (
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/clocksim"
	"github.com/jclark/satpulse/time/internal/phcsample"
	"github.com/jclark/satpulse/time/internal/syncsim"
	"github.com/jclark/satpulse/time/internal/timemsg"
	"github.com/jclark/satpulse/time/lib/allan"
	"github.com/jclark/satpulse/time/lib/check"
	"github.com/jclark/satpulse/time/phctime"
)

// Config holds phcsample simulation parameters. The oscillator, GPS,
// pulse, message, and fault subconfigs come directly from syncsim so
// both simulators share a single noise and fault-injection model.
type Config struct {
	Sim    syncsim.SimConfig
	PHC    syncsim.PHCConfig
	GPS    syncsim.GPSConfig
	Sample phcsample.Config
	Pulse  syncsim.PulseConfig
	Msg    syncsim.MsgConfig
	Fault  syncsim.FaultConfig

	// MsgRate is the number of time messages emitted per simulated
	// second. A value of 1 matches the classic one-per-pulse model;
	// higher values exercise the "messages can outnumber pulses"
	// case where the wallClock regression absorbs sub-second grid
	// points.
	MsgRate int `check:">=1,<=20"`

	// MsgOutage drops GPS time messages for the listed intervals
	// without affecting pulse-edge delivery. Useful for exercising
	// the "messages stop but pulses continue" case.
	MsgOutage []syncsim.OutageConfig

	// EdgeOutage drops PPS edges for the listed intervals without
	// affecting GPS message delivery. Useful for exercising the
	// "pulses stop but messages continue" case.
	EdgeOutage []syncsim.OutageConfig
}

// Validate checks every field in the reused syncsim config surface
// against its check-tag constraints (including elements of each
// []struct slice, which check.Validate does not descend into on
// its own) and then delegates to phcsample.Config.Validate for the
// Sample section. Fails fast on out-of-range settings rather than
// running with undefined behaviour.
func (c *Config) Validate() error {
	var msgs []string
	if errs := check.Validate(c); errs != nil {
		msgs = append(msgs, errs...)
	}
	msgs = append(msgs, validateSlice("phc.sinusoid", c.PHC.Sinusoid)...)
	msgs = append(msgs, validateSlice("gps.ar1", c.GPS.AR1)...)
	msgs = append(msgs, validateSlice("gps.sinusoid", c.GPS.Sinusoid)...)
	msgs = append(msgs, validateSlice("fault.outage", c.Fault.Outage)...)
	msgs = append(msgs, validateSlice("fault.outlier", c.Fault.Outlier)...)
	msgs = append(msgs, validateSlice("fault.excursion", c.Fault.Excursion)...)
	msgs = append(msgs, validateSlice("msgOutage", c.MsgOutage)...)
	msgs = append(msgs, validateSlice("edgeOutage", c.EdgeOutage)...)
	if err := c.Sample.Validate(); err != nil {
		msgs = append(msgs, err.Error())
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New(strings.Join(msgs, "\n\t"))
}

// validateSlice runs check.Validate on each element of items and
// prefixes each error with "<prefix>[<i>].", matching the tag-path
// style check.Validate uses for nested structs. Elements that report
// IsZero() == true are skipped, because syncsim's Default*Config
// helpers deliberately include a single zero sentinel entry in
// slices like PHC.Sinusoid and Fault.Excursion as a TOML documentation
// aid; those sentinels are filtered out at runtime by type-specific
// guards (s.Amp > 0 etc.) and are not meant to satisfy the per-field
// range tags.
func validateSlice[T any](prefix string, items []T) []string {
	var msgs []string
	for i := range items {
		if z, ok := any(items[i]).(interface{ IsZero() bool }); ok && z.IsZero() {
			continue
		}
		errs := check.Validate(&items[i])
		for _, m := range errs {
			msgs = append(msgs, fmt.Sprintf("%s[%d].%s", prefix, i, m))
		}
	}
	return msgs
}

// DefaultConfig returns a Config with zero PHC/GPS noise, phcsample's
// default tuning, and a default 1000s simulation duration.
func DefaultConfig() Config {
	return Config{
		Sim:    syncsim.DefaultSimConfig(),
		PHC:    syncsim.DefaultPHCConfig(),
		GPS:    syncsim.DefaultGPSConfig(),
		Sample: phcsample.DefaultConfig(),
		Pulse:   syncsim.DefaultPulseConfig(),
		Msg:     syncsim.DefaultMsgConfig(),
		Fault:   syncsim.DefaultFaultConfig(),
		MsgRate: 1,
	}
}

// Stats summarises one simulation run. Residuals are the difference
// between each sample's offset and the simulator's ground-truth
// offset, expressed in nanoseconds.
type Stats struct {
	Samples  int           // successful Generate returns
	NotReady int           // ErrNotReady returns
	Errors   int           // other error returns
	Mean     float64       // mean residual (ns)
	StdDev   float64       // residual stddev (ns)
	AbsMax   time.Duration // max absolute residual
	ADev     float64       // Allan deviation of residuals (seconds)
}

// String formats Stats in a block suitable for t.Logf.
func (s Stats) String() string {
	return fmt.Sprintf("samples = %d\nnotReady = %d\nerrors = %d\nmean = %.2f\nstdDev = %.2f\nabsMax = %d\naDev = %.6e\n",
		s.Samples, s.NotReady, s.Errors, s.Mean, s.StdDev, s.AbsMax.Nanoseconds(), s.ADev)
}

// Simulate runs a phcsample simulation with cfg and returns stats
// scoring the emitted sample stream against ground-truth true time.
// curTime is updated as the simulation progresses so callers can
// read it for log timestamps.
func Simulate(cfg Config, curTime *time.Time, lg *slog.Logger) (Stats, error) {
	if err := cfg.Validate(); err != nil {
		return Stats{}, err
	}

	combinedOutages := cfg.Fault.NormalizeOutages()
	msgOnlyOutages := normalizeOutages(cfg.MsgOutage)
	edgeOnlyOutages := normalizeOutages(cfg.EdgeOutage)

	osc := cfg.PHC.CreateSimulator()
	raw := clocksim.NewRawClock(osc, 0)

	otherSims := []clocksim.GPSSimulator{cfg.GPS.CreateSimulator(cfg.Sim.StartTime)}
	for _, exc := range cfg.Fault.Excursion {
		if !exc.IsZero() {
			otherSims = append(otherSims, clocksim.ExcursionPPS(
				exc.StartTime, exc.Duration, exc.Amplitude,
				exc.Rise.Duration, exc.Rise.EffectivePower(),
				exc.Fall.Duration, exc.Fall.EffectivePower(),
			))
		}
	}
	for _, outlier := range cfg.Fault.Outlier {
		if !outlier.IsZero() {
			otherSims = append(otherSims, clocksim.SingleOutlierPPS(outlier.Time, outlier.Offset))
		}
	}
	otherPPS := clocksim.CombineGPS(otherSims...)

	var sawtoothPPS clocksim.GPSSimulator
	if cfg.GPS.Sawtooth.Amp > 0 {
		ic := cfg.GPS.Sawtooth.InternalClock
		gpsOsc := clocksim.SinusoidOsc(ic.Amp, ic.Period, ic.PhaseInit)
		ampSec := cfg.GPS.Sawtooth.Amp * 1e-9
		sawtoothPPS = clocksim.SawtoothGPS(gpsOsc, ampSec, cfg.GPS.Sawtooth.PhaseInit)
	}

	var pulseWidth time.Duration
	var trailingEdgeSim clocksim.GPSSimulator
	edgesPerPulse := 1
	if cfg.Pulse.Width > 0 {
		pulseWidth = time.Duration(cfg.Pulse.Width * 1e9)
		trailingEdgeSim = clocksim.JitterGPS(2, 789)
		edgesPerPulse = 2
	}
	vclock := clocksim.NewVirtualClock(raw, sawtoothPPS, otherPPS, 0, 500000, pulseWidth, trailingEdgeSim)
	testClock := clocksim.NewTestClock(vclock)

	ls := ptime.LeapSecond2016()
	tStart, _ := ls.SysToTime(*curTime)
	simOrigin := *curTime

	// monoBase anchors the sim's synthetic monotonic clock. Time
	// values derived with monoBase.Add(d) carry a monotonic reading
	// (inherited from time.Now()), so their differences use the
	// monotonic component just like production code that reads
	// CLOCK_MONOTONIC. The wall-clock side of each such value is
	// never used by the sim; only Sub/Before comparisons are, which
	// time.Time routes through the monotonic component whenever
	// both operands have one. By contrast, time.Unix(0, 0).Add(d)
	// carries only a wall reading - that is what the simulator
	// passes for the sys argument of Generate, mirroring the
	// non-monotonic CLOCK_REALTIME reading production takes from
	// the PHC cross-sample.
	monoBase := time.Now()

	// Ground-truth offset is constant: chrony expects
	// (true_wall_time - sys) in seconds. We synthesise
	// sys = time.Unix(0,0) + eventTime*1s while true_wall_time =
	// simOrigin + eventTime*1s, so the difference reduces to
	// simOrigin - 1970.
	expectedOffset := simOrigin.Sub(time.Unix(0, 0)).Seconds()

	stats := &residualStats{adev: *allan.NewAccum(1.0)}
	buf := timemsg.NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)
	gen := phcsample.NewGenerator(cfg.Sample, edgesPerPulse, lg)
	buf.SetMsgUTCTimer(gen)

	lg.Info("starting phcsample simulation",
		"duration", cfg.Sim.Duration,
		"edgesPerPulse", edgesPerPulse,
		"startTime", curTime.Format(time.RFC3339),
	)

	events := mergeEvents(
		generatePulseEvents(cfg, cfg.Sim.Duration, edgesPerPulse),
		generateMessageEvents(cfg, cfg.Sim.Duration),
	)

	var lastReading clocksim.TimestampReading
	for event := range events {
		*curTime = simOrigin.Add(time.Duration(event.Time * 1e9))
		vclock.AdvanceTo(event.Time)
		if vclock.TimestampAvailable() {
			lastReading, _ = testClock.ReadTimestamp()
		}

		switch event.Type {
		case EventPulse:
			data := event.Data.(PulseEventData)
			if syncsim.InOutage(combinedOutages, data.PPS) || syncsim.InOutage(edgeOnlyOutages, data.PPS) {
				continue
			}
			tMono := monoBase.Add(time.Duration(event.Time * 1e9))
			tSysWall := time.Unix(0, 0).Add(time.Duration(event.Time * 1e9))
			tReadPHC := testClock.Now()
			gen.Pulse(phcsample.PulseEdge{
				Timestamp: lastReading.Timestamp,
				TRead: phctime.Sample{
					PHC: tReadPHC,
					Sys: tMono,
				},
			})
			// Generate consumes the PHC/sys cross-sample taken at
			// read time, not the pulse-edge timestamp itself. In
			// production this is TReadWall.PHC.T / TReadWall.Sys;
			// in the sim it is the testClock.Now() reading above
			// paired with a non-monotonic wall-clock sys.
			off, err := gen.Generate(tReadPHC.T, tSysWall)
			switch {
			case err == nil:
				stats.addResidual(off - expectedOffset)
			case errors.Is(err, phcsample.ErrNotReady):
				stats.notReady++
			default:
				stats.errors++
			}
		case EventMessage:
			data := event.Data.(MessageEventData)
			if syncsim.InOutage(combinedOutages, data.Time) || syncsim.InOutage(msgOnlyOutages, data.Time) {
				continue
			}
			deliverMessage(buf, data, event.Time, monoBase, tStart, ls)
		}
	}

	return stats.final(), nil
}

// Event is a simulation event with a time and a payload.
type Event struct {
	Time float64
	Type EventType
	Data any
}

// EventType identifies the payload kind carried by an Event.
type EventType int

const (
	EventPulse EventType = iota
	EventMessage
)

// PulseEventData is the payload of EventPulse.
type PulseEventData struct {
	EdgeIdx int
	PPS     float64
}

// MessageEventData is the payload of EventMessage. Time is the
// nominal UTC-grid instant (in sim seconds) that the message
// reports; at MsgRate=1 this is a PPS moment, at higher rates it
// is a sub-second grid point.
type MessageEventData struct {
	Time float64
}

type residualStats struct {
	n        int
	sumNs    float64
	sumSqNs  float64
	absMaxNs float64
	adev     allan.Accum[float64]
	notReady int
	errors   int
}

func (s *residualStats) addResidual(seconds float64) {
	ns := seconds * 1e9
	s.n++
	s.sumNs += ns
	s.sumSqNs += ns * ns
	if abs := math.Abs(ns); abs > s.absMaxNs {
		s.absMaxNs = abs
	}
	s.adev.Add(seconds)
}

func (s *residualStats) final() Stats {
	out := Stats{
		Samples:  s.n,
		NotReady: s.notReady,
		Errors:   s.errors,
		AbsMax:   time.Duration(s.absMaxNs),
		ADev:     s.adev.ADev(),
	}
	if s.n > 0 {
		out.Mean = s.sumNs / float64(s.n)
	}
	if s.n > 1 {
		n := float64(s.n)
		variance := (s.sumSqNs - (s.sumNs*s.sumNs)/n) / (n - 1)
		if variance < 0 {
			variance = 0
		}
		out.StdDev = math.Sqrt(variance)
	}
	return out
}

func deliverMessage(buf *timemsg.Buffer, data MessageEventData, eventTime float64, monoBase time.Time, tStart ptime.Time, ls ptime.LeapSecond) {
	taiAtMsg := tStart.Add(time.Duration(data.Time * 1e9))
	utc := ls.TimeToUTC(taiAtMsg)
	msg := &gpsprot.TimeMsg{
		TAITime:     taiAtMsg,
		UTCTime:     &utc,
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         gpsreg.TagUBX,
		NativeMsgID: "NAV-PVT",
	}
	// Monotonic-bearing tRead: production reads this from
	// time.Now() on packet receipt, which carries a monotonic
	// component. wallClock fits tRead->utc and relies on Sub
	// using the monotonic component.
	tRead := monoBase.Add(time.Duration(eventTime * 1e9))
	buf.Time(msg, tRead)
}

func normalizeOutages(list []syncsim.OutageConfig) []syncsim.OutageConfig {
	if len(list) == 0 {
		return nil
	}
	tmp := syncsim.FaultConfig{Outage: list}
	return tmp.NormalizeOutages()
}

func generatePulseEvents(cfg Config, duration float64, edgesPerPulse int) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(999))
		for pps := 1.0; pps < duration; pps += 1.0 {
			pulseDelay := cfg.Pulse.MinDelay + rng.Float64()*(cfg.Pulse.MaxDelay-cfg.Pulse.MinDelay)
			rising := pps + pulseDelay
			if !yield(Event{Time: rising, Type: EventPulse, Data: PulseEventData{EdgeIdx: 0, PPS: pps}}) {
				return
			}
			if edgesPerPulse == 2 {
				trailing := rising + cfg.Pulse.Width
				if !yield(Event{Time: trailing, Type: EventPulse, Data: PulseEventData{EdgeIdx: 1, PPS: pps}}) {
					return
				}
			}
		}
	}
}

func generateMessageEvents(cfg Config, duration float64) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(888))
		rate := max(cfg.MsgRate, 1)
		period := 1.0 / float64(rate)
		for nominal := period; nominal < duration; nominal += period {
			delay := cfg.Msg.Delay + rng.NormFloat64()*cfg.Msg.Jitter
			if delay < 0 {
				delay = 0
			}
			msgTime := nominal + delay
			if !yield(Event{Time: msgTime, Type: EventMessage, Data: MessageEventData{Time: nominal}}) {
				return
			}
		}
	}
}

func mergeEvents(a, b iter.Seq[Event]) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		aNext, aStop := iter.Pull(a)
		bNext, bStop := iter.Pull(b)
		defer aStop()
		defer bStop()
		aEv, aOK := aNext()
		bEv, bOK := bNext()
		for aOK || bOK {
			var ev Event
			if aOK && (!bOK || aEv.Time <= bEv.Time) {
				ev = aEv
				aEv, aOK = aNext()
			} else {
				ev = bEv
				bEv, bOK = bNext()
			}
			if !yield(ev) {
				return
			}
		}
	}
}
