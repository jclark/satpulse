package phcsync

import (
	"math"
	"time"
)

// piServo implement a simple Proportional-Integral (PI) servo for frequency adjustment.
type piServo struct {
	offSum  float64
	kp, ki  float64
	maxFreq float64
}

func newPiServo(freq, kp, ki, maxFreq float64) *piServo {
	return &piServo{
		kp:      kp,
		ki:      ki,
		maxFreq: maxFreq,
		offSum:  -freq / ki,
	}
}

// reset sets the integral term so that when the offset is zero the servo output is freq.
// This provides bumpless transfer when switching between average-frequency hold and normal PI control.
func (s *piServo) reset(freq float64) {
	s.offSum = -freq / s.ki
}

// sample processes a sample and returns frequency adjustment in PPB
// off is local - ref
func (s *piServo) sample(off time.Duration) float64 {
	fOff := float64(off)
	out := s.kp*fOff + s.ki*(s.offSum+fOff)
	if math.Abs(out) <= s.maxFreq {
		// only update integral term if not clamped
		s.offSum += fOff
		return -out
	}
	if out < 0 {
		return s.maxFreq
	}
	return -s.maxFreq
}
