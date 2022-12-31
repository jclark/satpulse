package main

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/tai"
)

const i210SetOffsetFudge = time.Nanosecond * 4600

type Servo struct {
	clk        *phc.Clock
	cur        sampler
	reset      sampler
	piControl  sampler
	freqAdj    float64
	maxFreqAdj float64
}

type sampler interface {
	sample(ref, local tai.Time, epoch phc.Epoch)
}

func NewServo(clk *phc.Clock) (*Servo, error) {
	s := Servo{}
	s.clk = clk
	rst := new(resetter)
	pi := new(piController)
	rst.servo = &s
	pi.servo = &s
	s.piControl = pi
	s.reset = rst
	s.cur = s.reset
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
	s.cur.sample(ref, local, epoch)
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
	off += i210SetOffsetFudge
	epoch, err := s.clk.AdjTime(off)
	fmt.Printf("stepped clock by %v\n", off)
	if err != nil {
		fmt.Printf("error adjusting time: %+v\n", err)
	}
	return epoch
}

type piController struct {
	servo *Servo
}

func (s *piController) sample(ref, local tai.Time, epoch phc.Epoch) {

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

	nextEpoch := r.servo.adjTime(ref.Sub(local))
	fmt.Printf("Adjusted time new epoch %v\n", nextEpoch)

	r.servo.cur = r.servo.piControl
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
