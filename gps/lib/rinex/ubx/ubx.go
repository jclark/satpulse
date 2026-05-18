// Package ubx converts u-blox raw observation messages to RINEX records.
package ubx

import (
	"fmt"
	"math"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

const (
	gen8PhaseThreshold   = 6
	gen9PhaseThreshold   = 15
	defaultSlipThreshold = 15
)

// Options controls UBX to RINEX conversion.
type Options struct {
	// PhaseThreshold is the cpStdev value at which phase is omitted.
	// If zero, the RTKLIB Explorer Gen8/Gen9 thresholds are used.
	PhaseThreshold byte
	SlipThreshold  byte
}

// Converter converts UBX-RXM-RAWX messages to RINEX observations.
type Converter struct {
	opts  Options
	sink  rinex.Sink
	state map[signalKey]signalState
	gen9  bool
}

type signalKey struct {
	sat rinex.SatelliteID
	sig rinex.SignalID
}

type signalState struct {
	lock       uint16
	subHalfCyc bool
	pending    bool
	seen       bool
}

// New creates a Converter that writes records to sink.
func New(sink rinex.Sink, opts Options) *Converter {
	if opts.SlipThreshold == 0 {
		opts.SlipThreshold = defaultSlipThreshold
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
	threshold := c.phaseThreshold()
	gen9 := false
	for _, meas := range m.Meas {
		obs, ok := c.observation(t, threshold, meas)
		gen9 = gen9 || meas.SigID > 1
		if !ok {
			continue
		}
		if err := c.sink.Observation(obs); err != nil {
			return err
		}
	}
	c.gen9 = c.gen9 || gen9
	return nil
}

func (c *Converter) phaseThreshold() byte {
	if c.opts.PhaseThreshold != 0 {
		return c.opts.PhaseThreshold
	}
	if c.gen9 {
		return gen9PhaseThreshold
	}
	return gen8PhaseThreshold
}

func (c *Converter) observation(t rinex.Time, phaseThreshold byte, meas ubxbin.RxmRawxMeas) (rinex.SignalObservation, bool) {
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
	phase := meas.TrkStat&ubxbin.RxmRawxCpValid != 0 && meas.CpStdev&ubxbin.RxmRawxCpStdMask < phaseThreshold && meas.CpMes != -0.5 && finite64(meas.CpMes)
	lli := c.lli(obs.Sat, obs.Sig, meas, phase)
	if phase {
		obs.CP = opt.Make(carrierPhase(meas))
		if lli != 0 {
			obs.LLI = opt.Make(lli)
		}
	} else if lli != 0 {
		obs.LLI = opt.Make(lli)
	}
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

func carrierPhase(meas ubxbin.RxmRawxMeas) float64 {
	cp := meas.CpMes
	if meas.GNSSID == ubxbin.BDS && (meas.SVID <= 5 || meas.SVID >= 59) {
		cp += 0.5
	}
	return cp
}

func (c *Converter) lli(sat rinex.SatelliteID, sig rinex.SignalID, meas ubxbin.RxmRawxMeas, phase bool) rinex.LLI {
	k := signalKey{sat: sat, sig: sig}
	st := c.state[k]
	sub := meas.TrkStat&ubxbin.RxmRawxSubHalfCyc != 0
	subChanged := sub != st.subHalfCyc
	if meas.LockTime == 0 || st.seen && meas.LockTime < st.lock || subChanged || meas.CpStdev&ubxbin.RxmRawxCpStdMask >= c.opts.SlipThreshold {
		st.pending = true
	}
	lli := rinex.LLI(0)
	if subChanged {
		lli = rinex.LLILostLock
	}
	if phase && st.pending {
		lli |= rinex.LLILostLock
		st.pending = false
	}
	if phase && halfCycleUnresolved(meas) {
		lli |= rinex.LLIHalfCycleAmbiguity
	}
	st.lock = meas.LockTime
	st.subHalfCyc = sub
	st.seen = true
	c.state[k] = st
	return lli
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
