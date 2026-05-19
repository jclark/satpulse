package rinex

import (
	"fmt"
	"time"
)

const decimationRoundTicks = Time(100 * time.Millisecond / (time.Duration(tickNs) * time.Nanosecond))

type signalKey struct {
	sat SatelliteID
	sig SignalID
}

// DecimationSink wraps a Sink and emits only observations on a time grid.
type DecimationSink struct {
	sink     Sink
	interval Time
	lli      map[signalKey]LLI
}

// NewDecimationSink creates a Sink that decimates observations by interval.
func NewDecimationSink(sink Sink, interval time.Duration) (*DecimationSink, error) {
	ticks, err := decimationIntervalTicks(interval)
	if err != nil {
		return nil, err
	}
	return &DecimationSink{
		sink:     sink,
		interval: ticks,
		lli:      make(map[signalKey]LLI),
	}, nil
}

// ValidateDecimationInterval reports whether interval is valid for decimation.
func ValidateDecimationInterval(interval time.Duration) error {
	_, err := decimationIntervalTicks(interval)
	return err
}

// Metadata passes m through to the wrapped sink.
func (s *DecimationSink) Metadata(m Metadata) error {
	return s.sink.Metadata(m)
}

// Observation emits obs only when its rounded epoch label is on the interval grid.
func (s *DecimationSink) Observation(obs SignalObservation) error {
	if s.onGrid(obs.T) {
		s.applyLLI(&obs)
		return s.sink.Observation(obs)
	}
	s.saveLLI(obs)
	return nil
}

// Flush flushes the wrapped sink.
func (s *DecimationSink) Flush() error {
	return s.sink.Flush()
}

func decimationIntervalTicks(interval time.Duration) (Time, error) {
	if interval < time.Second {
		return 0, fmt.Errorf("rinex: decimation interval must be at least 1 second")
	}
	tick := time.Duration(tickNs) * time.Nanosecond
	if interval%tick != 0 {
		return 0, fmt.Errorf("rinex: decimation interval must be a multiple of %s", tick)
	}
	if 24*time.Hour%interval != 0 {
		return 0, fmt.Errorf("rinex: decimation interval must divide one GPS day exactly")
	}
	return Time(interval / tick), nil
}

func (s *DecimationSink) onGrid(t Time) bool {
	t = roundTime(t, decimationRoundTicks)
	_, r := divMod(int64(t), int64(s.interval))
	return r == 0
}

func (s *DecimationSink) saveLLI(obs SignalObservation) {
	if !obs.LLI.IsSet() || obs.LLI.Get() == 0 {
		return
	}
	k := signalKey{sat: obs.Sat, sig: obs.Sig}
	s.lli[k] |= obs.LLI.Get()
}

func (s *DecimationSink) applyLLI(obs *SignalObservation) {
	k := signalKey{sat: obs.Sat, sig: obs.Sig}
	lli := s.lli[k]
	if lli == 0 {
		return
	}
	if obs.LLI.IsSet() {
		lli |= obs.LLI.Get()
	}
	obs.LLI.Set(lli)
	delete(s.lli, k)
}

func roundTime(t, unit Time) Time {
	if t < 0 {
		return -roundTime(-t, unit)
	}
	return ((t + unit/2) / unit) * unit
}
