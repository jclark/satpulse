package main

import (
	"fmt"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/tai"
)

type Servo struct {
	clk        *phc.Clock
	cur        state
	train      state
	normal     state
	freqAdj    float64
	maxFreqAdj float64
}

type state interface {
	sample(ref, local tai.Time)
}

func NewServo(clk *phc.Clock) (*Servo, error) {
	s := Servo{}
	s.clk = clk
	trainState := new(trainState)
	normalState := new(normalState)
	trainState.servo = &s
	normalState.servo = &s
	s.normal = normalState
	s.train = trainState
	s.cur = s.train
	freqAdj, err := clk.FreqAdj()
	if err != nil {
		return nil, err
	}
	s.freqAdj = freqAdj
	return &s, nil
}

func (s *Servo) Sample(ref, local tai.Time) {
	off := local.Sub(ref)
	fmt.Printf("Servo: GPS  %v, PHC %v, master offset: %v\n", ref, local, off)
	s.cur.sample(ref, local)
}

func (s *Servo) setFreqAdj(fa float64) {
	err := s.clk.SetFreqAdj(fa)
	if err != nil {
		fmt.Printf("error adjusting frequency: %+v\n", err)
	}
	s.freqAdj = fa
}

type normalState struct {
	servo *Servo
}

func (s *normalState) sample(ref, local tai.Time) {

}

type trainState struct {
	servo  *Servo
	ref1   tai.Time
	local1 tai.Time
}

const trainPeriod = time.Second * 8

func (s *trainState) sample(ref, local tai.Time) {
	if s.ref1.IsZero() {
		s.ref1 = ref
		s.local1 = local
		return
	}
	refPeriod := ref.Sub(s.ref1)
	localPeriod := local.Sub(s.local1)
	if localPeriod < trainPeriod {
		return
	}
	ratio := float64(refPeriod) / float64(localPeriod)
	freqAdj := s.servo.freqAdj

	freqAdj = ((1e9 + freqAdj) * ratio) - 1e9

	fmt.Printf("new freqAdj %v\n", freqAdj)
	s.servo.setFreqAdj(freqAdj)
	s.servo.cur = s.servo.normal
}
