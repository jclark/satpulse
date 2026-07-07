// Package rnxsbf converts Septentrio SBF raw observation messages to RINEX records.
package rnxsbf

import (
	"fmt"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

const speedOfLight = 299792458.0

// Converter converts SBF MeasEpoch blocks to RINEX observations.
type Converter struct {
	sink    rinex.Sink
	state   map[signalKey]signalState
	ts      sbfbin.TimeStamp
	pending *sbfbin.MeasEpoch
}

type signalKey struct {
	sat rinex.SatelliteID
	sig rinex.SignalID
}

type signalState struct {
	lock    uint16
	ceil    uint16 // clip ceiling of the sub-block type lock came from
	cumLoss uint8
	arc     uint32
	pending bool
	seen    bool
	cumSeen bool
}

// master carries the Type1 master values a Type2 slave sub-block needs to
// reconstruct its absolute measurements.
type master struct {
	pr, do     float64
	prOK, doOK bool
	freqHz     float64 // master carrier frequency; 0 when unknown
	frq        opt.Val[int8]
}

// extraKey correlates a MeasExtra sub-block with a MeasEpoch sub-block of the
// same epoch: the two blocks are not guaranteed to list sub-blocks in the
// same order, so they are matched on receiver channel and signal number.
type extraKey struct {
	rxChannel uint8
	sigNum    uint8
}

type extraInfo struct {
	cn0HighRes  float64
	cumLossCont uint8
}

// New creates a Converter that writes records to sink.
func New(sink rinex.Sink) *Converter {
	return &Converter{sink: sink, state: make(map[signalKey]signalState)}
}

// ConvertBlock converts one SBF block, reporting whether it was a MeasEpoch.
// A MeasEpoch is held until the next measurement block decides its pairing:
// the MeasExtra with the same timestamp converts it with the refinement, and
// any other MeasEpoch or MeasExtra shows none is coming and converts it
// without one. Blocks of other types are ignored and leave the held MeasEpoch
// in place. Call Flush after the last block of the stream.
func (c *Converter) ConvertBlock(b *sbfbin.Block) (bool, error) {
	switch p := b.Params.(type) {
	case *sbfbin.MeasEpoch:
		if err := c.Flush(); err != nil {
			return true, err
		}
		c.ts = b.TimeStamp
		c.pending = p
		return true, nil
	case *sbfbin.MeasExtra:
		if c.pending == nil || b.TimeStamp != c.ts {
			return false, c.Flush()
		}
		m := c.pending
		c.pending = nil
		return false, c.ConvertMeasEpoch(c.ts, m, p)
	}
	return false, nil
}

// Flush converts a MeasEpoch held by ConvertBlock whose MeasExtra can no
// longer arrive, such as at the end of the input stream.
func (c *Converter) Flush() error {
	if c.pending == nil {
		return nil
	}
	m := c.pending
	c.pending = nil
	return c.ConvertMeasEpoch(c.ts, m, nil)
}

// ConvertMeasEpoch converts one SBF MeasEpoch block, and the MeasExtra block
// for the same epoch if available, to RINEX observations. ts is the MeasEpoch
// block-header timestamp; pass a nil extra when MeasExtra output is not
// enabled or has not arrived for this epoch.
func (c *Converter) ConvertMeasEpoch(ts sbfbin.TimeStamp, m *sbfbin.MeasEpoch, extra *sbfbin.MeasExtra) error {
	if ts.TOW == sbfbin.TOWDNU || ts.WNc == sbfbin.WNcDNU {
		return nil
	}
	t := rinex.TimeFromGPSWeekMillis(int64(ts.WNc), ts.TOW)
	hr := extraInfos(extra)
	for i := range m.Type1 {
		t1 := &m.Type1[i]
		sys := sbfbin.RINEXSys(t1.SVID)
		num := sbfbin.RINEXSatNum(t1.SVID)
		if sys == "" || num == 0 {
			continue
		}
		sat := rinex.SatelliteID(fmt.Sprintf("%s%02d", sys, num))
		obs, mst, ok := c.masterObservation(t, sat, sys, m.CommonFlags, t1, hr)
		if ok {
			if err := c.sink.Observation(obs); err != nil {
				return err
			}
		}
		for j := range m.Type2[i] {
			obs, ok := c.slaveObservation(t, sat, sys, m.CommonFlags, t1.RxChannel, &m.Type2[i][j], mst, hr)
			if !ok {
				continue
			}
			if err := c.sink.Observation(obs); err != nil {
				return err
			}
		}
	}
	return nil
}

func extraInfos(extra *sbfbin.MeasExtra) map[extraKey]extraInfo {
	if extra == nil {
		return nil
	}
	hasCN0 := extra.HasCN0HighRes()
	m := make(map[extraKey]extraInfo, len(extra.Channels))
	for i := range extra.Channels {
		s := &extra.Channels[i]
		info := extraInfo{cumLossCont: s.CumLossCont}
		if hasCN0 {
			info.cn0HighRes = s.CN0HighRes()
		}
		m[extraKey{s.RxChannel, s.SignalNumber()}] = info
	}
	return m
}

// masterObservation builds the observation for a Type1 master sub-block. It
// always returns the master values needed by the Type2 slaves; ok is false
// when there is no observation to emit for the master itself.
func (c *Converter) masterObservation(t rinex.Time, sat rinex.SatelliteID, sys string, flags sbfbin.CommonFlags, t1 *sbfbin.MeasEpochChannelType1, hr map[extraKey]extraInfo) (rinex.SignalObservation, master, bool) {
	var mst master
	mst.pr, mst.prOK = t1.Pseudorange()
	mst.do, mst.doOK = t1.DopplerHz()
	if k, ok := t1.GLONASSFreqNr(); ok {
		mst.frq = opt.Make(k)
	}
	sigSys, sigCode := sbfbin.RINEXSig(t1.SignalNumber(), flags)
	if sigCode == "" || sigSys != sys {
		return rinex.SignalObservation{}, mst, false
	}
	sig := rinex.SignalID(sigCode)
	if f, ok := rinex.SignalFrequencyHz(sys, sig, mst.frq.Ptr()); ok {
		mst.freqHz = f
	}
	obs := rinex.SignalObservation{T: t, Sat: sat, Sig: sig}
	obs.Frq = mst.frq
	if mst.prOK {
		obs.PR = opt.Make(mst.pr)
	}
	cpOK := false
	if off, ok := t1.CarrierOffsetCycles(); ok && mst.prOK && mst.freqHz > 0 {
		obs.CP = opt.Make(mst.pr/(speedOfLight/mst.freqHz) + off)
		cpOK = true
	}
	info, infoOK := hr[extraKey{t1.RxChannel, t1.SignalNumber()}]
	obs.Arc, obs.HC = c.arcHC(sat, sig, t1.LockTime, sbfbin.MeasType1LockTimeClipped, t1.LockTime != sbfbin.MeasType1LockTimeDNU, info.cumLossCont, infoOK, t1.ObsInfo.HalfCycleAmbiguity(), cpOK)
	if mst.doOK {
		obs.Do = opt.Make(mst.do)
	}
	if v, ok := t1.CN0dBHz(); ok {
		obs.CN0 = opt.Make(float32(v + info.cn0HighRes))
	}
	if !obs.PR.IsSet() && !obs.CP.IsSet() && !obs.Do.IsSet() && !obs.CN0.IsSet() {
		return rinex.SignalObservation{}, mst, false
	}
	return obs, mst, true
}

// slaveObservation builds the observation for a Type2 slave sub-block,
// reconstructing absolute values from the deltas relative to its master.
func (c *Converter) slaveObservation(t rinex.Time, sat rinex.SatelliteID, sys string, flags sbfbin.CommonFlags, rxChannel uint8, t2 *sbfbin.MeasEpochChannelType2, mst master, hr map[extraKey]extraInfo) (rinex.SignalObservation, bool) {
	sigSys, sigCode := sbfbin.RINEXSig(t2.SignalNumber(), flags)
	if sigCode == "" || sigSys != sys {
		return rinex.SignalObservation{}, false
	}
	sig := rinex.SignalID(sigCode)
	frq := opt.Val[int8]{}
	if n := t2.SignalNumber(); n >= sbfbin.SigNumGLONASSL1CA && n <= sbfbin.SigNumGLONASSL2CA {
		// The FDMA channel is a per-satellite property, so when the slave's
		// own ObsInfo does not carry it, the master's channel applies.
		if k, ok := t2.GLONASSFreqNr(); ok {
			frq = opt.Make(k)
		} else {
			frq = mst.frq
		}
	}
	freqHz, freqOK := rinex.SignalFrequencyHz(sys, sig, frq.Ptr())
	obs := rinex.SignalObservation{T: t, Sat: sat, Sig: sig}
	obs.Frq = frq
	var pr float64
	prOK := false
	if off, ok := t2.PseudorangeOffset(); ok && mst.prOK {
		pr = mst.pr + off
		obs.PR = opt.Make(pr)
		prOK = true
	}
	cpOK := false
	if off, ok := t2.CarrierOffsetCycles(); ok && prOK && freqOK {
		obs.CP = opt.Make(pr/(speedOfLight/freqHz) + off)
		cpOK = true
	}
	info, infoOK := hr[extraKey{rxChannel, t2.SignalNumber()}]
	obs.Arc, obs.HC = c.arcHC(sat, sig, uint16(t2.LockTime), sbfbin.MeasType2LockTimeClipped, t2.LockTime != sbfbin.MeasType2LockTimeDNU, info.cumLossCont, infoOK, t2.ObsInfo.HalfCycleAmbiguity(), cpOK)
	if off, ok := t2.DopplerOffsetHz(); ok && mst.doOK && mst.freqHz > 0 && freqOK {
		obs.Do = opt.Make(mst.do*(freqHz/mst.freqHz) + off)
	}
	if v, ok := t2.CN0dBHz(); ok {
		obs.CN0 = opt.Make(float32(v + info.cn0HighRes))
	}
	if !obs.PR.IsSet() && !obs.CP.IsSet() && !obs.Do.IsSet() && !obs.CN0.IsSet() {
		return rinex.SignalObservation{}, false
	}
	return obs, true
}

// arcHC tracks per-signal loss-of-lock state across epochs. A lock-time reset
// (zero, or a decrease since the last valid value), or any change in the
// MeasExtra CumLossCont counter (which the receiver increments at each initial
// lock after (re)acquisition or detected cycle slip), marks a pending arc
// increment, applied at the next epoch that actually reports a carrier phase.
// A Do-Not-Use lock-time (lockOK false) or an absent MeasExtra entry (cumOK
// false) leaves the corresponding state untouched. CumLossCont catches slips
// the lock-time comparison cannot see, such as a slip followed by an outage
// long enough for the counter to re-clip.
//
// A signal can move between the Type1 master and a Type2 slave position when
// the receiver re-selects the master, and the two encodings clip the same
// underlying lock time at different ceilings (65534 vs 254), so the decrease
// comparison clamps both sides to the smaller of the two ceilings involved.
func (c *Converter) arcHC(sat rinex.SatelliteID, sig rinex.SignalID, lock, ceil uint16, lockOK bool, cumLoss uint8, cumOK, halfCycle, phase bool) (uint32, bool) {
	k := signalKey{sat: sat, sig: sig}
	st := c.state[k]
	if lockOK && st.seen {
		clip := min(ceil, st.ceil)
		if lock == 0 || min(lock, clip) < min(st.lock, clip) {
			st.pending = true
		}
	}
	if cumOK && st.cumSeen && cumLoss != st.cumLoss {
		st.pending = true
	}
	if phase && st.pending {
		st.arc++
		st.pending = false
	}
	if lockOK {
		st.lock = lock
		st.ceil = ceil
		st.seen = true
	}
	if cumOK {
		st.cumLoss = cumLoss
		st.cumSeen = true
	}
	c.state[k] = st
	return st.arc, phase && halfCycle
}
