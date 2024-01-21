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
	reset             *resetter
	comp              *compensator
	piControl         *piController
	freqOff           float64 // frequency offset in PPB
	maxFreqOff        float64
	adjSetOffsetDelay time.Duration
}

type sampler interface {
	sample(ref ptime.Time, local ptime.ClockTime, delayed bool)
}

func New(clk Clock, lg *slog.Logger) (*Servo, error) {
	s := Servo{}
	s.clk = clk
	s.lg = lg
	rst := new(resetter)
	pi := new(piController)
	comp := new(compensator)
	rst.servo = &s
	pi.servo = &s
	comp.servo = &s
	s.piControl = pi
	s.reset = rst
	s.comp = comp
	s.sampler = rst
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
	s.sampler.sample(ref, local, delayed)
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
		s.lg.Error("error adjusting the PHC time", "err", err, "totalOff", totalOff)
	} else {
		s.lg.Info("adjusted the PHC time", "totalOff", totalOff, "off", off, "delay", s.adjSetOffsetDelay)
	}
	return era
}

type piController struct {
	servo  *Servo
	era    ptime.Era
	offSum float64
}

// the values here come from linuxptp
const kp = 0.7
const ki = 0.3

func (p *piController) sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	if local.Era != p.era || delayed {
		return
	}
	off := local.T.Sub(ref)
	fOff := float64(off)
	p.offSum += fOff
	out := kp*fOff + ki*p.offSum
	// fmt.Printf("off %v, offSum %v, out %v\n", off, p.offSum, out)
	p.servo.lg.Info("adjusted PHC frequency using the PI servo", "off", off, "freq", p.servo.freqOff+out)
	p.servo.setFreqOff(-out)
}

func (p *piController) init(freq float64) {
	p.offSum = -freq / ki
}

const observePeriod = time.Second * 4

type resetter struct {
	servo  *Servo
	ref1   ptime.Time
	local1 ptime.Time
}

func (r *resetter) sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	if r.ref1.IsZero() {
		r.ref1 = ref
		r.local1 = local.T
		return
	}
	refPeriod := ref.Sub(r.ref1)
	localPeriod := local.T.Sub(r.local1)
	if localPeriod < observePeriod || delayed {
		return
	}

	freqOff := r.servo.freqOff
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

	r.servo.setFreqOff(freqOff)

	r.servo.comp.era = r.servo.adjTime(ref.Sub(local.T))
	r.servo.sampler = r.servo.comp
	r.servo.piControl.init(freqOff)
}

// A compensator compensates for the inaccuracy of ADJ_SETOFFSET.
// In the ethernet drivers we are interested in, ADJ_SETOFFSET works by
// reading a time value, applying a delta to the read value,
// and then writing the value back. This takes time, which the drivers
// don't attempt to compensate for. It's about 4.5 microseconds with an i210.
// To compensate for this, after the resetter sets the time correctly, we look at the offset
// between the next pulse of the PHC clock and the GPS time, and assume
// that this is the delay. We then set the clock again compensating for
// this delay.
type compensator struct {
	servo *Servo
	era   ptime.Era
}

func (c *compensator) sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	if local.Era != c.era || delayed {
		return
	}
	off := ref.Sub(local.T)
	// this causes future calls to adjTime to be adjusted by this
	c.servo.adjSetOffsetDelay = off
	// this call will have the offset applied twice, once here
	// and once in Servo.adjTime
	nextEra := c.servo.adjTime(off)
	c.servo.piControl.era = nextEra
	c.servo.sampler = c.servo.piControl
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
