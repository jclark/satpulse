package servo

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

type Clock interface {
	SetFreqOffset(float64) error
	FreqOffset() (float64, error)
	MaxFreqOffset() float64
	AdjTime(d time.Duration) (ptime.Era, error)
}

type Servo struct {
	clk               Clock
	lg                *slog.Logger
	sampler           sampler
	piControlEra      ptime.Era
	freqOff           float64 // frequency offset in PPB
	maxFreqOff        float64
	adjSetOffsetDelay time.Duration
}

type sampler func(ref ptime.Time, local ptime.ClockTime, delayed bool)

func New(clk Clock, lg *slog.Logger) (*Servo, error) {
	s := Servo{}
	s.clk = clk
	s.lg = lg
	s.resetSampler()
	f, err := clk.FreqOffset()
	if err != nil {
		return nil, err
	}
	s.freqOff = f
	s.maxFreqOff = clk.MaxFreqOffset()
	return &s, nil
}

func (s *Servo) Logger() *slog.Logger {
	return s.lg
}

func (s *Servo) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	// PTP defines offsetFromMaster as timeOnSlave - timeOnMaster, so I think this is the right way round.
	off := local.T.Sub(ref)
	s.lg.Debug("sample received by servo", "off", off, "gps", ref, "phc", local.T, "era", local.Era, "delayed", delayed)
	s.sampler(ref, local, delayed)
}

func (s *Servo) Locked(era ptime.Era) bool {
	return era == s.piControlEra && era != 0
}

func (s *Servo) FreqOffset() float64 {
	return s.freqOff
}

func (s *Servo) setFreqOff(f float64) {
	f = clamp(f, s.maxFreqOff)
	if f == s.freqOff {
		return
	}
	err := s.clk.SetFreqOffset(f)
	if err != nil {
		s.lg.Error("error adjusting the PHC frequency", "err", err, "freq", f)
		return
	}
	s.lg.Debug("adjusted the PHC frequency", "oldFreq", s.freqOff, "newFreq", f, "diff", f-s.freqOff)
	s.freqOff = f
}

func (s *Servo) adjTime(off time.Duration) ptime.Era {
	totalOff := off + s.adjSetOffsetDelay
	era, err := s.clk.AdjTime(totalOff)
	if err != nil {
		s.lg.Error("error stepping the PHC time", "err", err, "totalOff", totalOff)
	} else {
		s.lg.Info("stepped the PHC time", "totalOff", totalOff, "off", off, "delay", s.adjSetOffsetDelay)
	}
	return era
}

// the values here come from linuxptp
const kp = 0.7
const ki = 0.3

func (s *Servo) piControlSampler(era ptime.Era, freq float64) {
	offSum := -freq / ki
	s.sampler = func(ref ptime.Time, local ptime.ClockTime, delayed bool) {
		if local.Era != era || delayed {
			return
		}
		off := local.T.Sub(ref)
		fOff := float64(off)
		offSum += fOff
		out := kp*fOff + ki*offSum
		s.setFreqOff(-out)
	}
	s.piControlEra = era
}

const observePeriod = time.Second * 4

// minInitialStep is the minimum offset above which the clock will be stepped on startup.
// The idea behind this value is that is very roughly how accurately you can step the clock,
// given the delay between the time you read the clock and the time you write the new value.
const minInitialStep = time.Microsecond * 5

func (s *Servo) resetSampler() {
	var ref1, local1 ptime.Time
	s.sampler = func(ref ptime.Time, local ptime.ClockTime, delayed bool) {
		if ref1.IsZero() {
			ref1 = ref
			local1 = local.T
			return
		}
		refPeriod := ref.Sub(ref1)
		localPeriod := local.T.Sub(local1)
		if localPeriod < observePeriod || delayed {
			return
		}

		freqOff := s.freqOff
		/*
			Semantics of freqOff in ppb is that a period that the local clock measured with freqOff 0 as N seconds
			will be measured with freqOff P as N*F seconds where F is (1e9 + P)/1e9.

			We know that N*F1 = (L2 - L1) where N is the time that the unadjusted clock would measure and L1, L2 are the
			local clocks at the beginning and end of the period and F1 is (1e9 + O1)/1e9, where O1 is current freqAdj.
			We want to find F2 such that N*F2 = (R2 - R1).
			So we have (L2 - L1)/F1 = (R2 - R1)/F2. So F2 = F1*(R2 - R1)/(L2 - L1)
			So O2 = (O1 + 1e9)*(R2 - R1)/(L2 - L1) - 1e9
		*/

		ratio := float64(refPeriod) / float64(localPeriod)
		freqOff = (1e9+freqOff)*ratio - 1e9

		s.setFreqOff(freqOff)
		off := ref.Sub(local.T)
		if off.Abs() < minInitialStep {
			s.piControlSampler(local.Era, freqOff)
		} else {
			nextEra := s.adjTime(off)
			s.compensateSampler(nextEra, freqOff)
		}
	}
}

// compensateSampler compensates for the inaccuracy of ADJ_SETOFFSET.
// In the ethernet drivers we are interested in, ADJ_SETOFFSET works by
// reading a time value, applying a delta to the read value,
// and then writing the value back. This takes time, which the drivers
// don't attempt to compensate for. It's about 4.5 microseconds with an i210.
// To compensate for this, after the resetSampler sets the time correctly, we look at the offset
// between the next pulse of the PHC clock and the GPS time, and assume
// that this is the delay. We then set the clock again compensating for
// this delay.
func (s *Servo) compensateSampler(era ptime.Era, freqOff float64) {
	s.sampler = func(ref ptime.Time, local ptime.ClockTime, delayed bool) {
		if local.Era != era || delayed {
			return
		}
		off := ref.Sub(local.T)
		// this causes future calls to adjTime to be adjusted by this
		s.adjSetOffsetDelay = off
		// this call will have the offset applied twice, once here
		// and once in Servo.adjTime
		nextEra := s.adjTime(off)
		s.piControlSampler(nextEra, freqOff)
	}
}

func clamp(v, max float64) float64 {
	if math.Abs(v) <= max {
		return v
	}
	if v < 0 {
		return -max
	}
	return max
}
