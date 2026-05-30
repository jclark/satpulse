// Package rnxubx converts u-blox raw observation messages to RINEX records.
package rnxubx

import (
	"fmt"
	"math"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// Options controls UBX to RINEX conversion.
type Options struct {
	SlipThreshold   byte
	BDSGeoHalfCycle bool
	// SlipOnSubHalfCycle restores the RTKLIB behaviour of marking a cycle slip
	// on any change of the RAWX subHalfCyc bit. By default a subHalfCyc change
	// marks a slip only when the half cycle is resolved on both the previous and
	// current valid phase, so that a toggle caused by half-cycle resolution
	// (already advertised by LLI bit 1) does not reset the carrier phase arc.
	SlipOnSubHalfCycle bool
}

// Converter converts UBX-RXM-RAWX messages to RINEX observations.
type Converter struct {
	opts  Options
	sink  rinex.Sink
	state map[signalKey]signalState
}

type signalKey struct {
	sat rinex.SatelliteID
	sig rinex.SignalID
}

type signalState struct {
	lock       uint16
	subHalfCyc bool
	halfCyc    bool
	cpValid    bool
	arc        uint32
	pending    bool
	seen       bool
}

// New creates a Converter that writes records to sink.
func New(sink rinex.Sink, opts Options) *Converter {
	if opts.SlipThreshold == 0 {
		opts.SlipThreshold = 15
	}
	return &Converter{
		opts:  opts,
		sink:  sink,
		state: make(map[signalKey]signalState),
	}
}

// ConvertRAWX converts one UBX-RXM-RAWX message.
func (c *Converter) ConvertRAWX(m *ubxbin.RxmRawx) error {
	t := rinex.TimeFromGPSWeekSeconds(int64(m.Week), m.RcvTow)
	for _, meas := range m.Meas {
		obs, ok := c.observation(t, meas)
		if !ok {
			continue
		}
		if err := c.sink.Observation(obs); err != nil {
			return err
		}
	}
	return nil
}

func (c *Converter) observation(t rinex.Time, meas ubxbin.RxmRawxMeas) (rinex.SignalObservation, bool) {
	sys := ubxbin.RINEXSys(meas.GNSSID)
	satNum := ubxbin.RINEXSatNum(meas.GNSSID, meas.SVID)
	sig := ubxbin.RINEXSig(meas.GNSSID, meas.SigID)
	if sys == "" || satNum == 0 || sig == "" {
		return rinex.SignalObservation{}, false
	}
	obs := rinex.SignalObservation{
		T:   t,
		Sat: rinex.SatelliteID(fmt.Sprintf("%s%02d", sys, satNum)),
		Sig: rinex.SignalID(sig),
	}
	if meas.GNSSID == ubxbin.GLO {
		obs.Frq = opt.Make(int8(meas.FreqID) - 7)
	}
	if meas.TrkStat&ubxbin.RxmRawxPrValid != 0 && finite64(meas.PrMes) {
		obs.PR = opt.Make(meas.PrMes)
	}
	cp, cpOK := carrierPhase(meas, c.opts.BDSGeoHalfCycle)
	arc, hc := c.arcHC(obs.Sat, obs.Sig, meas, cpOK)
	if cpOK {
		obs.CP = opt.Make(cp)
	}
	obs.Arc = arc
	obs.HC = hc
	if finite32(meas.DoMes) {
		obs.Do = opt.Make(float64(meas.DoMes))
	}
	if meas.CNO != 0 {
		obs.CN0 = opt.Make(float32(meas.CNO))
	}
	if len(obs.ObservationCodes()) == 0 {
		return rinex.SignalObservation{}, false
	}
	return obs, true
}

func carrierPhase(meas ubxbin.RxmRawxMeas, bdsGeoHalfCycle bool) (float64, bool) {
	if meas.TrkStat&ubxbin.RxmRawxCpValid == 0 || !finite64(meas.CpMes) {
		return 0, false
	}
	// RTKLIB rejects a RAWX carrier phase of -0.5 cycles here for reasons unknown.
	cp := meas.CpMes
	if bdsGeoHalfCycle && isBDSGeo(meas) {
		// RTKLIB applies this BDS GEO half-cycle shift; its source gives no rationale.
		cp += 0.5
	}
	return cp, true
}

func isBDSGeo(meas ubxbin.RxmRawxMeas) bool {
	return meas.GNSSID == ubxbin.BDS && (meas.SVID <= 5 || meas.SVID >= 59)
}

func (c *Converter) arcHC(sat rinex.SatelliteID, sig rinex.SignalID, meas ubxbin.RxmRawxMeas, phase bool) (arc uint32, hc bool) {
	k := signalKey{sat: sat, sig: sig}
	st := c.state[k]
	sub := meas.TrkStat&ubxbin.RxmRawxSubHalfCyc != 0
	half := meas.TrkStat&ubxbin.RxmRawxHalfCyc != 0
	subChanged := sub != st.subHalfCyc
	subSlip := subChanged
	if !c.opts.SlipOnSubHalfCycle {
		subSlip = subChanged && st.seen && st.cpValid && st.halfCyc && phase && half
	}
	ll := false
	if meas.LockTime == 0 || st.seen && meas.LockTime < st.lock || subSlip || meas.CpStdev&ubxbin.RxmRawxCpStdMask >= c.opts.SlipThreshold {
		st.pending = true
	}
	if subSlip {
		ll = true
	}
	if phase && st.pending {
		ll = true
		st.pending = false
	}
	if ll {
		st.arc++
	}
	if phase && halfCycleUnresolved(meas) {
		hc = true
	}
	st.lock = meas.LockTime
	st.subHalfCyc = sub
	st.halfCyc = half
	st.cpValid = phase
	st.seen = true
	c.state[k] = st
	return st.arc, hc
}

func halfCycleUnresolved(meas ubxbin.RxmRawxMeas) bool {
	if meas.GNSSID == ubxbin.SBAS {
		return meas.LockTime <= 8000
	}
	return meas.TrkStat&ubxbin.RxmRawxHalfCyc == 0
}

func finite32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

func finite64(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
