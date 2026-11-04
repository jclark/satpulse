// Package rnxnov converts NovAtel-format RANGE raw observation logs to RINEX
// records.
package rnxnov

import (
	"math"

	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
)

const lockTimeTolerance = 0.05

// Converter converts RANGE logs to RINEX observations.
// It is not safe for concurrent use, and expects one receiver's logs in time
// order.
type Converter struct {
	sink     rinex.Sink
	rinex    func(novmsg.SatSystem, uint16, uint8) (sat, sig string, phaseShift float64)
	state    map[signalKey]signalState
	unmapped int
}

type signalKey struct {
	sat rinex.SatelliteID
	sig rinex.SignalID
}

type signalState struct {
	t       rinex.Time
	lock    float32
	arc     uint32
	pending bool
}

// New creates a Converter that writes records to sink, mapping satellites
// and signals with novmsg.RangeRINEX.
// It panics if sink is nil.
func New(sink rinex.Sink) *Converter {
	return newConverter(sink, novmsg.RangeRINEX)
}

// NewSino creates a Converter for SinoGNSS receivers, which maps satellites
// and signals with novmsg.SinoRangeRINEX.
// It panics if sink is nil.
func NewSino(sink rinex.Sink) *Converter {
	return newConverter(sink, novmsg.SinoRangeRINEX)
}

func newConverter(sink rinex.Sink, f func(novmsg.SatSystem, uint16, uint8) (string, string, float64)) *Converter {
	if sink == nil {
		panic("nil RINEX sink")
	}
	return &Converter{
		sink:  sink,
		rinex: f,
		state: make(map[signalKey]signalState),
	}
}

// ConvertRange converts one RANGE log. It reports whether any observation
// was written.
func (c *Converter) ConvertRange(h *novmsg.MsgHdr[novmsg.Port], m *novmsg.Range) (bool, error) {
	t := rinex.TimeFromGPSWeekMillis(int64(h.Week), uint32(h.MillisecondsOfWeek))
	seen := false
	for _, rec := range m.Obs {
		ok, err := c.convertObs(t, rec)
		if err != nil {
			return seen, err
		}
		if ok {
			seen = true
		}
	}
	return seen, nil
}

// Unmapped returns the number of RANGE records skipped so far because the
// Converter has no RINEX satellite or signal for them.
func (c *Converter) Unmapped() int {
	return c.unmapped
}

func (c *Converter) convertObs(t rinex.Time, rec novmsg.RangeObs) (bool, error) {
	st := rec.Status
	sysID := st.SatSystem()
	sat, sig, phaseShift := c.rinex(sysID, rec.PRN, st.SignalType())
	if sat == "" {
		c.unmapped++
		return false, nil
	}
	obs := rinex.SignalObservation{
		T:   t,
		Sat: rinex.SatelliteID(sat),
		Sig: rinex.SignalID(sig),
	}
	if sysID == novmsg.SatSystemGLONASS && rec.GloFreq <= 13 {
		obs.Frq = opt.Make(int8(rec.GloFreq) - 7)
	}
	if st.CodeLocked() && finite64(rec.PSR) {
		obs.PR = opt.Make(rec.PSR)
	}
	cpOK := st.PhaseLocked() && finite64(rec.ADR)
	obs.Arc = c.arc(obs.Sat, obs.Sig, t, rec.LockTime, cpOK)
	if cpOK {
		// ADR is accumulated Doppler range, opposite in sign to RINEX
		// carrier phase.
		obs.CP = opt.Make(-rec.ADR + phaseShift)
		obs.HC = !st.ParityKnown()
	}
	if finite32(rec.Dopp) {
		obs.Do = opt.Make(float64(rec.Dopp))
	}
	if rec.CN0 != 0 {
		obs.CN0 = opt.Make(rec.CN0)
	}
	if len(obs.ObservationCodes()) == 0 {
		return false, nil
	}
	return true, c.sink.Observation(obs)
}

func (c *Converter) arc(sat rinex.SatelliteID, sig rinex.SignalID, t rinex.Time, lock float32, phase bool) uint32 {
	k := signalKey{sat: sat, sig: sig}
	st, seen := c.state[k]
	ll := st.pending || seen && (lock == 0 || lockSlip(t, lock, st))
	if ll {
		st.arc++
	}
	st.pending = ll && !phase
	st.t = t
	st.lock = lock
	c.state[k] = st
	return st.arc
}

func lockSlip(t rinex.Time, lock float32, st signalState) bool {
	dt := t - st.t
	return dt > 0 && float64(lock-st.lock)+lockTimeTolerance <= float64(dt)/1e7
}

func finite32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

func finite64(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
