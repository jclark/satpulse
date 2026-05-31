package rinex

const smoothSpeedOfLight = 299792458.0

// SmoothingSink wraps a Sink and applies divergence-free carrier smoothing to
// pseudoranges before passing observations through. Each smoothed code band is
// paired with one partner band whose carrier supplies an ionosphere-matched
// reference, so the smoothing window is unbounded within a continuous arc. The
// partner is L1 for any other band, and L5 (then L2, then the lowest other
// band present) for L1 itself. A satellite with no usable partner band passes
// through unsmoothed.
//
// Observations must arrive grouped by epoch in time order, as the converters
// and file readers emit them; SmoothingSink buffers one epoch at a time.
type SmoothingSink struct {
	sink  Sink
	buf   []SignalObservation
	haveT bool
	curT  Time
	epoch int64
	state map[signalKey]*smoothState
}

type smoothState struct {
	n         int
	pr        float64 // smoothed pseudorange, metres
	prevPhiDf float64 // previous divergence-free phase reference, metres
	arc       uint32
	parterArc uint32
	lastEpoch int64
}

// NewSmoothingSink creates a Sink that divergence-free carrier-smooths pseudoranges.
func NewSmoothingSink(sink Sink) *SmoothingSink {
	return &SmoothingSink{sink: sink, state: make(map[signalKey]*smoothState)}
}

// Metadata passes m through to the wrapped sink.
func (s *SmoothingSink) Metadata(m Metadata) error {
	return s.sink.Metadata(m)
}

// Observation buffers obs, flushing the previous epoch when the time advances.
func (s *SmoothingSink) Observation(obs SignalObservation) error {
	if s.haveT && obs.T != s.curT {
		if err := s.flushEpoch(); err != nil {
			return err
		}
	}
	s.curT = obs.T
	s.haveT = true
	s.buf = append(s.buf, obs)
	return nil
}

// Flush flushes the buffered epoch and the wrapped sink.
func (s *SmoothingSink) Flush() error {
	if err := s.flushEpoch(); err != nil {
		return err
	}
	return s.sink.Flush()
}

func (s *SmoothingSink) flushEpoch() error {
	if len(s.buf) == 0 {
		return nil
	}
	s.epoch++
	carrier := make(map[signalKey]float64) // metres, for any obs with valid carrier
	freq := make(map[signalKey]float64)
	for i := range s.buf {
		o := &s.buf[i]
		if !o.CP.IsSet() {
			continue
		}
		f, ok := o.FrequencyHz()
		if !ok {
			continue
		}
		k := signalKey{sat: o.Sat, sig: o.Sig}
		carrier[k] = o.CP.Get() * smoothSpeedOfLight / f
		freq[k] = f
	}
	for i := range s.buf {
		o := &s.buf[i]
		if !o.PR.IsSet() || !o.CP.IsSet() {
			continue
		}
		s.smooth(o, carrier, freq)
	}
	for _, o := range s.buf {
		if err := s.sink.Observation(o); err != nil {
			return err
		}
	}
	s.buf = s.buf[:0]
	return nil
}

func (s *SmoothingSink) smooth(o *SignalObservation, carrier, freq map[signalKey]float64) {
	k := signalKey{sat: o.Sat, sig: o.Sig}
	fi, ok := freq[k]
	if !ok {
		return
	}
	pk, ok := s.partner(o.Sat, o.Sig, freq)
	if !ok {
		return
	}
	fj := freq[pk]
	phiI := carrier[k]
	phiJ := carrier[pk]
	gamma := (fi / fj) * (fi / fj)
	b := 2 / (gamma - 1)
	phiDf := phiI + b*(phiI-phiJ)
	st := s.state[k]
	reset := st == nil ||
		st.lastEpoch != s.epoch-1 ||
		o.Arc != st.arc ||
		s.buf[s.index(pk)].Arc != st.parterArc
	if st == nil {
		st = &smoothState{}
		s.state[k] = st
	}
	if reset {
		st.n = 1
		st.pr = o.PR.Get()
	} else {
		st.n++
		fn := float64(st.n)
		st.pr = o.PR.Get()/fn + (fn-1)/fn*(st.pr+(phiDf-st.prevPhiDf))
	}
	st.prevPhiDf = phiDf
	st.arc = o.Arc
	st.parterArc = s.buf[s.index(pk)].Arc
	st.lastEpoch = s.epoch
	o.PR.Set(st.pr)
}

// partner returns the key of the band to pair with sig on sat for divergence-free
// smoothing. Non-L1 bands pair with L1; L1 pairs with L5, then L2, then the
// lowest other band present. It reports false when no partner band has a valid
// carrier this epoch.
func (s *SmoothingSink) partner(sat SatelliteID, sig SignalID, freq map[signalKey]float64) (signalKey, bool) {
	if len(sig) == 0 {
		return signalKey{}, false
	}
	band := sig[0]
	if band != '1' {
		return s.bandKey(sat, '1', freq)
	}
	for _, pref := range []byte{'5', '2'} {
		if k, ok := s.bandKey(sat, pref, freq); ok {
			return k, true
		}
	}
	return s.lowestOtherBand(sat, '1', freq)
}

func (s *SmoothingSink) bandKey(sat SatelliteID, band byte, freq map[signalKey]float64) (signalKey, bool) {
	for k := range freq {
		if k.sat == sat && len(k.sig) > 0 && k.sig[0] == band {
			return k, true
		}
	}
	return signalKey{}, false
}

func (s *SmoothingSink) lowestOtherBand(sat SatelliteID, exclude byte, freq map[signalKey]float64) (signalKey, bool) {
	var best signalKey
	found := false
	for k := range freq {
		if k.sat != sat || len(k.sig) == 0 || k.sig[0] == exclude {
			continue
		}
		if !found || k.sig[0] < best.sig[0] {
			best = k
			found = true
		}
	}
	return best, found
}

func (s *SmoothingSink) index(k signalKey) int {
	for i := range s.buf {
		if s.buf[i].Sat == k.sat && s.buf[i].Sig == k.sig {
			return i
		}
	}
	return 0
}
