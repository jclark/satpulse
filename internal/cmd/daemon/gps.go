package daemon

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsprot"
)

type GPSConfig struct {
	InputOnly  bool    `toml:"inputOnly"`
	TimeMode   bool    `toml:"timeMode"`
	Resurvey   bool    `toml:"resurvey"`
	SurveyTime uint32  `toml:"surveyTime"`
	SurveyAcc  float64 `toml:"surveyAcc"`
}

var gpsDefault = GPSConfig{
	TimeMode:   true,
	Resurvey:   false,
	SurveyTime: 2000, // 2000 seconds
	SurveyAcc:  20,   // 20 meters
}

func (r GPSConfig) gpsprot() (*gpsprot.ConfigMap, gpsprot.ConfigOptions, error) {
	m := gpscfg.RequiredConfig()
	opts := gpscfg.RequiredOptions()
	if !r.TimeMode {
		gpsprot.CfgTimeMode.Set(m, gpsprot.TimeModeDisabled)
		opts.Survey.When = 0
	} else {
		opts.Survey.When = gpsprot.TimeModeFlags(gpsprot.TimeModeDisabled)
		if r.Resurvey {
			opts.Survey.When |= gpsprot.TimeModeFlags(gpsprot.TimeModeSurvey)
		}
		opts.Survey.MinDur = time.Second * time.Duration(r.SurveyTime)
		opts.Survey.AccLimit = gpsprot.Meters(r.SurveyAcc)
		if opts.Survey.AccLimit < gpsprot.Millimeter {
			return nil, opts, fmt.Errorf("survey accuracy %v is too small", opts.Survey.AccLimit)
		}
	}
	opts.InputOnly = r.InputOnly
	return m, opts, nil
}
