package daemon

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsmsg"
)

type ReceiverConfig struct {
	TimeMode   bool    `toml:"timeMode"`
	Resurvey   bool    `toml:"resurvey"`
	SurveyTime uint32  `toml:"surveyTime"`
	SurveyAcc  float64 `toml:"surveyAcc"`
}

var receiverDefault = ReceiverConfig{
	TimeMode:   true,
	Resurvey:   false,
	SurveyTime: 2000, // 2000 seconds
	SurveyAcc:  20,   // 20 meters
}

func gpsConfig(r ReceiverConfig) (*gpsmsg.ConfigMap, gpsmsg.ConfigOptions, error) {
	m := gpscfg.RequiredConfig()
	opts := gpscfg.RequiredOptions()
	if !r.TimeMode {
		gpsmsg.CfgTimeMode.Set(m, gpsmsg.TimeModeDisabled)
		opts.Survey.When = 0
	} else {
		opts.Survey.When = gpsmsg.TimeModeFlags(gpsmsg.TimeModeDisabled)
		if r.Resurvey {
			opts.Survey.When |= gpsmsg.TimeModeFlags(gpsmsg.TimeModeSurvey)
		}
		opts.Survey.MinDur = time.Second * time.Duration(r.SurveyTime)
		opts.Survey.AccLimit = gpsmsg.Meters(r.SurveyAcc)
		if opts.Survey.AccLimit < gpsmsg.Millimeter {
			return nil, opts, fmt.Errorf("survey accuracy %v is too small", opts.Survey.AccLimit)
		}
	}
	return m, opts, nil
}
