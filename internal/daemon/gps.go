package daemon

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsprot"
)

type GPSConfig struct {
	Config             bool         `toml:"config"`
	TimeMode           bool         `toml:"timeMode"`
	Resurvey           bool         `toml:"resurvey"`
	SurveyTime         uint32       `toml:"surveyTime"`
	SurveyAcc          float64      `toml:"surveyAcc"`
	FixedPosECEF       geopos.ECEF  `toml:"fixedPosECEF"`
	FixedPosAcc        float64      `toml:"fixedPosAcc"`
	AntennaCableDelay  float64      `toml:"antennaCableDelay"`  // in nanoseconds
	AntennaCableLength float64      `toml:"antennaCableLength"` // in meters
	AntennaCableVF     float64      `toml:"antennaCableVF"`     // velocity factor
	GNSS               gpsprot.GNSS `toml:"gnss"`
	PulseWidth         float64      `toml:"pulseWidth"`
}

const defaultAccuracy = 20.0 // in meters

var gpsDefault = GPSConfig{
	TimeMode:           true,
	Resurvey:           false,
	SurveyTime:         2000, // 2000 seconds
	SurveyAcc:          defaultAccuracy,
	FixedPosAcc:        defaultAccuracy,
	AntennaCableDelay:  math.NaN(),
	AntennaCableLength: math.NaN(),
	AntennaCableVF:     0.66, // typical value for RG-58 cable
	PulseWidth:         math.NaN(),
}

func (c *GPSConfig) target() (*gpsprot.ConfigTarget, error) {
	target := gpsprot.NewConfigTarget(c.Config)
	if !c.Config {
		return target, nil
	}
	err := c.getTimeMode(target)
	if err != nil {
		return nil, err
	}
	cm := &target.Map
	err = c.getPrimaryGNSS(cm)
	if err != nil {
		return nil, err
	}
	err = c.getDelay(cm)
	if err != nil {
		return nil, err
	}
	err = c.getFixedPos(cm)
	if err != nil {
		return nil, err
	}
	return target, nil
}

func (c *GPSConfig) getTimeMode(target *gpsprot.ConfigTarget) error {
	opts := &target.Opts
	if !c.TimeMode {
		gpsprot.CfgTimeMode.Set(&target.Map, gpsprot.TimeModeDisabled)
		opts.Survey.When = 0
	} else {
		opts.Survey.When = gpsprot.TimeModeFlags(gpsprot.TimeModeDisabled)
		if c.Resurvey {
			opts.Survey.When |= gpsprot.TimeModeFlags(gpsprot.TimeModeSurvey)
		}
		opts.Survey.MinDur = time.Second * time.Duration(c.SurveyTime)
		opts.Survey.AccLimit = gpsprot.Meters(c.SurveyAcc)
		if opts.Survey.AccLimit < gpsprot.Millimeter {
			return fmt.Errorf("survey accuracy %v is too small", opts.Survey.AccLimit)
		}
	}
	return nil
}

func (c *GPSConfig) getPrimaryGNSS(cm *gpsprot.ConfigMap) error {
	if c.GNSS == 0 {
		return nil
	}
	if !c.GNSS.IsMajor() {
		return fmt.Errorf("primary GNSS must be a major GNSS (%v is not)", c.GNSS)
	}
	gpsprot.CfgPrimaryGNSS.Set(cm, c.GNSS)
	return nil
}

func (c *GPSConfig) getFixedPos(cm *gpsprot.ConfigMap) error {
	if c.FixedPosECEF.IsZero() {
		return nil
	}
	err := c.FixedPosECEF.CheckOnEarth()
	if err != nil {
		return fmt.Errorf("%v: invalid fixed position: %w", c.FixedPosECEF, err)
	}
	var fixedPos gpsprot.Point3D
	for i := 0; i < 3; i++ {
		fixedPos[i] = gpsprot.Meters(c.FixedPosECEF[i])
	}
	gpsprot.CfgFixedPosECEF.Set(cm, fixedPos)
	acc := gpsprot.Meters(c.FixedPosAcc)
	if acc < gpsprot.Millimeter {
		return fmt.Errorf("fixed position accuracy %v is too small", c.FixedPosAcc)
	}
	if acc > gpsprot.Meter*1000 {
		return fmt.Errorf("fixed position accuracy %v is too large", c.FixedPosAcc)
	}
	gpsprot.CfgFixedPosAcc.Set(cm, acc)
	return nil
}

const speedOfLight = 0.299792458 // in meters per nanosecond

const maxAntennaCableDelay = 30000 // in nanoseconds; needs to fit in signed 16-bit integer

func (c *GPSConfig) getDelay(cm *gpsprot.ConfigMap) error {
	delay := 0.0
	specified := false
	if !math.IsNaN(c.AntennaCableLength) {
		specified = true
		if c.AntennaCableVF <= 0.0 || c.AntennaCableVF > 1.0 {
			return fmt.Errorf("invalid GPS antenna cable velocity factor %v", c.AntennaCableVF)
		}
		delay = c.AntennaCableLength / (speedOfLight * c.AntennaCableVF)
	}
	if !math.IsNaN(c.AntennaCableDelay) {
		specified = true
		delay += c.AntennaCableDelay
	}
	if !specified {
		return nil
	}
	if !(math.Abs(delay) <= maxAntennaCableDelay) {
		return fmt.Errorf("invalid cable delay %v (cable delay is in nanoseconds)", c.AntennaCableDelay)
	}
	gpsprot.CfgAntennaCableDelay.Set(cm, time.Duration(delay))
	return nil
}

func (c *GPSConfig) pulseWidth() (time.Duration, error) {
	if math.IsNaN(c.PulseWidth) {
		return 0, nil
	}
	d := time.Duration(c.PulseWidth * float64(time.Second))
	if d <= 0 || d >= time.Second {
		return 0, fmt.Errorf("GPS pulse width must be > 0 and < 1.0: %v", c.PulseWidth)
	}
	return d, nil
}
