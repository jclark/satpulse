package main

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/tai"
)

type Servo struct {
	clk               *phc.Clock
	sampler           sampler
	reset             *resetter
	comp              *compensator
	piControl         *piController
	freqAdj           float64
	maxFreqAdj        float64
	adjSetOffsetDelay time.Duration
}

type sampler interface {
	sample(ref, local tai.Time, epoch phc.Epoch)
}

func NewServo(clk *phc.Clock) (*Servo, error) {
	s := Servo{}
	s.clk = clk
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

func (s *Servo) Sample(ref, local tai.Time, epoch phc.Epoch) {
	off := local.Sub(ref)
	fmt.Printf("Servo: GPS  %v, PHC %v, master offset: %v\n", ref, local, off)
	s.sampler.sample(ref, local, epoch)
}

func (s *Servo) setFreqAdj(fa float64) {
	fa = clamp(fa, s.maxFreqAdj)
	err := s.clk.SetFreqAdj(fa)
	if err != nil {
		fmt.Printf("error adjusting frequency: %+v\n", err)
	}
	s.freqAdj = fa
}

func (s *Servo) adjTime(off time.Duration) phc.Epoch {
	off += s.adjSetOffsetDelay
	epoch, err := s.clk.AdjTime(off)
	fmt.Printf("stepped clock by %v\n", off)
	if err != nil {
		fmt.Printf("error adjusting time: %+v\n", err)
	}
	return epoch
}

type piController struct {
	servo  *Servo
	epoch  phc.Epoch
	offSum float64
}

// the values here come from linuxptp
const kp = 0.7
const ki = 0.3

func (p *piController) sample(ref, local tai.Time, epoch phc.Epoch) {
	if epoch != p.epoch {
		return
	}
	off := float64(local.Sub(ref))
	p.offSum += off
	out := kp*off + ki*p.offSum
	// fmt.Printf("off %v, offSum %v, out %v\n", off, p.offSum, out)
	p.servo.setFreqAdj(-out)
}

func (p *piController) init(freqAdj float64) {
	p.offSum = -freqAdj / ki
}

const observePeriod = time.Second * 8

type resetter struct {
	servo  *Servo
	ref1   tai.Time
	local1 tai.Time
}

func (r *resetter) sample(ref, local tai.Time, epoch phc.Epoch) {
	if r.ref1.IsZero() {
		r.ref1 = ref
		r.local1 = local
		return
	}
	refPeriod := ref.Sub(r.ref1)
	localPeriod := local.Sub(r.local1)
	if localPeriod < observePeriod {
		return
	}

	freqAdj := r.servo.freqAdj
	fmt.Printf("cur freqAdj %v\n", freqAdj)
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

	fmt.Printf("new freqAdj %v\n", freqAdj)
	r.servo.setFreqAdj(freqAdj)

	r.servo.comp.epoch = r.servo.adjTime(ref.Sub(local))
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
	epoch phc.Epoch
}

func (c *compensator) sample(ref, local tai.Time, epoch phc.Epoch) {
	if epoch != c.epoch {
		return
	}
	off := ref.Sub(local)
	// this causes future calls to adjTime to be adjusted by this
	c.servo.adjSetOffsetDelay = off
	fmt.Printf("adj_setoffset delay is %v\n", off)
	// this call will have the offset applied twice, once here
	// and once in Servo.adjTime
	nextEpoch := c.servo.adjTime(off)
	fmt.Printf("adjusted time again new epoch %v\n", nextEpoch)
	c.servo.piControl.epoch = nextEpoch
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
