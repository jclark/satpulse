// Package unc converts Unicore raw observation messages to RINEX records.
package unc

import (
	"fmt"
	"math"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

const lockTimeTolerance = 0.05

// Options controls Unicore to RINEX conversion.
type Options struct{}

// Converter converts OBSVM messages to RINEX observations.
type Converter struct {
	sink  rinex.Sink
	state map[signalKey]signalState
}

type signalKey struct {
	sat rinex.SatelliteID
	sig rinex.SignalID
}

type signalState struct {
	t       rinex.Time
	lock    float32
	pending bool
	seen    bool
}

// New creates a Converter that writes records to sink.
// It panics if sink is nil.
func New(sink rinex.Sink, _ Options) *Converter {
	if sink == nil {
		panic("nil RINEX sink")
	}
	return &Converter{
		sink:  sink,
		state: make(map[signalKey]signalState),
	}
}

// ConvertObsVM converts one Unicore OBSVM message.
func (c *Converter) ConvertObsVM(h *uncmsg.MsgHdr, m *uncmsg.ObsVM) (bool, error) {
	t := rinex.TimeFromGPSWeekSeconds(int64(h.Week), float64(h.MillisecondsOfWeek)/1000)
	seen := false
	for _, meas := range m.Obs {
		ok, err := c.convertObs(t, meas)
		if err != nil {
			return seen, err
		}
		if ok {
			seen = true
		}
	}
	return seen, nil
}

func (c *Converter) convertObs(t rinex.Time, meas uncmsg.ObsVMObs) (bool, error) {
	st := meas.ChTrStatus
	sysID := st.SysID()
	sys := uncmsg.RINEXSys(sysID)
	satNum := uncmsg.RINEXSatNum(sysID, meas.PRN)
	sig := uncmsg.RINEXObsSig(sysID, st.SignalType(), st.L2C())
	if sys == "" || satNum == 0 || sig == "" {
		return false, nil
	}
	obs := rinex.SignalObservation{
		T:   t,
		Sat: rinex.SatelliteID(fmt.Sprintf("%s%02d", sys, satNum)),
		Sig: rinex.SignalID(sig),
	}
	if sysID == uncmsg.SysGLO {
		obs.Frq = opt.Make(int8(meas.SystemFreq) - 7)
	}
	if st.PseudorangeValid() && finite64(meas.PSR) {
		obs.PR = opt.Make(meas.PSR)
	}
	cpOK := st.CarrierPhaseValid() && finite64(meas.ADR)
	ll := c.lli(obs.Sat, obs.Sig, t, meas.LockTime, cpOK)
	if cpOK {
		obs.CP = opt.Make(-meas.ADR)
		obs.LL = ll
	}
	if finite32(meas.Dopp) {
		obs.Do = opt.Make(float64(meas.Dopp))
	}
	if meas.CN0 != 0 {
		obs.CN0 = opt.Make(float32(meas.CN0) / 100)
	}
	if len(obs.ObservationCodes()) == 0 {
		return false, nil
	}
	return true, c.sink.Observation(obs)
}

func (c *Converter) lli(sat rinex.SatelliteID, sig rinex.SignalID, t rinex.Time, lock float32, phase bool) bool {
	k := signalKey{sat: sat, sig: sig}
	st := c.state[k]
	if lock == 0 || st.seen && lockSlip(t, lock, st) {
		st.pending = true
	}
	ll := false
	if phase && st.pending {
		ll = true
		st.pending = false
	}
	st.t = t
	st.lock = lock
	st.seen = true
	c.state[k] = st
	return ll
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
