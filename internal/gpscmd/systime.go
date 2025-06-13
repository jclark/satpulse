package gpscmd

import (
	"errors"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ntptime"
	"github.com/jclark/satpulse/internal/ptime"
)

func EstimateSystemTime() (*gpsprot.TimeEstimate, error) {
	pre := time.Now()
	state, err := ntptime.Get()
	post := time.Now()
	
	if err != nil {
		return nil, err
	}
	if !state.Synchronized {
		return nil, errors.New("system clock not synchronized")
	}
	leapTonight := ptime.LeapSecondNone
	switch state.LeapSecondStatus {
	case ntptime.LeapSecondInsTonight:
		leapTonight = ptime.LeapSecondPositive
	case ntptime.LeapSecondDelTonight:
		leapTonight = ptime.LeapSecondNegative
	case ntptime.LeapSecondInProgress:
		return nil, errors.New("leap second in progress")
	}
	return &gpsprot.TimeEstimate{
		EstimatedTime:  state.Time,
		TimeOfEstimate: pre.Add(post.Sub(pre) / 2), // midpoint of pre and post
		Accuracy:       state.MaxError,
		LeapSecond:     ptime.LeapSecondState{
			UTCOffset:   int16(state.TAIOffset),
			LeapTonight: leapTonight,
		},
	}, nil
}
