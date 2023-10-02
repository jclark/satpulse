package tsync

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/gps4ptp/internal/ptime"
	"github.com/jclark/gps4ptp/internal/sse"
)

type Clock interface {
	SetFreqAdj(fa float64) error
	FreqAdj() (float64, error)
	MaxFreqAdj() float64
	AdjTime(d time.Duration) (ptime.Era, error)
}

type Servo struct {
	clk               Clock
	lg                *slog.Logger
	sseCh             chan<- sse.Event
	sampler           sampler
	reset             *resetter
	comp              *compensator
	piControl         *piController
	freqAdj           float64
	maxFreqAdj        float64
	adjSetOffsetDelay time.Duration
}

type SampleEvent struct {
	Offset            float64 `json:"offset"` // in nanoseconds
	Freq              float64 `json:"freq"`   // in parts per billion
	StepCount         uint32  `json:"stepCount"`
	StepCountChanging bool    `json:"stepCountChanging,omitempty"`
}

type sampler interface {
	sample(ref ptime.Time, local ptime.ClockTime, delayed bool)
}

func NewServo(clk Clock, lg *slog.Logger, sseCh chan<- sse.Event) (*Servo, error) {
	s := Servo{}
	s.clk = clk
	s.lg = lg
	s.sseCh = sseCh
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
	freqAdj, err := clk.FreqAdj()
	if err != nil {
		return nil, err
	}
	s.freqAdj = freqAdj
	s.maxFreqAdj = clk.MaxFreqAdj()
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

	if s.sseCh == nil {
		return
	}
	stepCount, changing := local.Era.StepCount()

	event, err := sse.Make("phc", &SampleEvent{
		Offset:            float64(off),
		StepCount:         uint32(stepCount),
		StepCountChanging: changing,
		Freq:              s.freqAdj,
	})
	if err != nil {
		s.lg.Error("error creating sample event", "err", err)
		return
	}
	s.sseCh <- event
}

func (s *Servo) setFreqAdj(fa float64) {
	fa = clamp(fa, s.maxFreqAdj)
	if fa == s.freqAdj {
		return
	}
	err := s.clk.SetFreqAdj(fa)
	if err != nil {
		s.lg.Error("error adjusting the PHC frequency", "err", err, "freqAdj", fa)
		return
	}
	s.lg.Debug("adjusted the PHC frequency", "oldFreqAdj", s.freqAdj, "newFreqAdj", fa, "diff", fa-s.freqAdj)
	s.freqAdj = fa
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
	p.servo.lg.Info("adjusted PHC frequency using the PI servo", "off", off, "freq", p.servo.freqAdj+out)
	p.servo.setFreqAdj(-out)
}

func (p *piController) init(freqAdj float64) {
	p.offSum = -freqAdj / ki
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

	freqAdj := r.servo.freqAdj
	/*
		Semantics of freqAdj in ppb is that a period that the local clock measured with freqAdj 0 as N seconds
		will be measured with freqAdj A as N*F seconds where F is (1e9 + A)/1e9.

		We know that N*F1 = (L2 - L1) where N is the time that the unadjusted clock would measure and L1, L2 are the
		local clocks at the beginning and end of the period and F1 is (1e9 + A1)/1e9, where A1 is current freqAdj.
		We want to find F2 such that N*F2 = (R2 - R1).
		So we have (L2 - L1)/F1 = (R2 - R1)/F2. So F2 = F1*(R2 - R1)/(L2 - L1)
		So A2 = (A1 + 1e9)*(R2 - R1)/(L2 - L1) - 1e9
	*/

	ratio := float64(refPeriod) / float64(localPeriod)
	freqAdj = (1e9+freqAdj)*ratio - 1e9

	r.servo.setFreqAdj(freqAdj)

	r.servo.comp.era = r.servo.adjTime(ref.Sub(local.T))
	r.servo.sampler = r.servo.comp
	r.servo.piControl.init(freqAdj)
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
