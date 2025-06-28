package daemon

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
)

// gpsTimePulseFlags contains flags for GPS time pulse configuration
type gpsTimePulseFlags int

const (
	// gpsTimePulseEnable enables time pulse output
	gpsTimePulseEnable gpsTimePulseFlags = 1 << iota
	// gpsTimePulseGetWidth queries the time pulse width
	gpsTimePulseGetWidth
)

type GPSConfig struct {
	Config             bool         `toml:"config"`
	Stationary         bool         `toml:"stationary"`
	Resurvey           bool         `toml:"resurvey"`
	SurveyTime         uint32       `toml:"surveyTime"`
	SurveyAcc          float64      `toml:"surveyAcc"`
	FixedPosECEF       geopos.ECEF  `toml:"fixedPosECEF"`
	FixedPosAcc        float64      `toml:"fixedPosAcc"`
	AntennaCableDelay  float64      `toml:"antennaCableDelay"`  // in nanoseconds
	AntennaCableLength float64      `toml:"antennaCableLength"` // in meters
	AntennaCableVF     float64      `toml:"antennaCableVF"`     // velocity factor
	TimeGNSS           gpsprot.GNSS `toml:"timeGNSS"`
	PulseWidth         float64      `toml:"pulseWidth"`
	SatellitesOutput   *bool        `toml:"satellitesOutput"`
	RTCMOutput         *bool        `toml:"rtcmOutput"`
	NMEANumbering      string       `toml:"nmeaNumbering"`
}

const defaultAccuracy = 20.0 // in meters

var gpsDefault = GPSConfig{
	Stationary:         true,
	Resurvey:           false,
	SurveyTime:         2000, // 2000 seconds
	SurveyAcc:          defaultAccuracy,
	FixedPosAcc:        defaultAccuracy,
	AntennaCableDelay:  math.NaN(),
	AntennaCableLength: math.NaN(),
	AntennaCableVF:     0.66, // typical value for RG-58 cable
	PulseWidth:         math.NaN(),
}

func (c *GPSConfig) target(speed int, wantSatellitesOutput bool, tpFlags gpsTimePulseFlags) (*gpsprot.ConfigTarget, time.Duration, error) {
	target := gpsprot.NewConfigTarget()
	pulseWidth, err := c.pulseWidth()
	if err != nil {
		return nil, 0, err
	}
	if !c.Config {
		return target, pulseWidth, nil
	}
	timePulseEnabled := tpFlags&gpsTimePulseEnable != 0
	if timePulseEnabled {
		target.Props.SetPPS()
	}
	gpsevent.SetMsgOptions(target, timePulseEnabled)
	if tpFlags&gpsTimePulseGetWidth != 0 && pulseWidth == 0 {
		target.Get |= gpsprot.PropIDTimePulseWidth
	}
	err = c.getTimeMode(target)
	if err != nil {
		return nil, 0, err
	}
	cp := &target.Props
	err = c.getTimeGNSS(cp)
	if err != nil {
		return nil, 0, err
	}
	err = c.getDelay(cp)
	if err != nil {
		return nil, 0, err
	}
	err = c.getFixedPos(cp)
	if err != nil {
		return nil, 0, err
	}
	target.Opts.SatsMsg = c.satsMsg(speed, wantSatellitesOutput)
	target.Opts.RTCMMsg = c.rtcmMsg()
	return target, pulseWidth, nil
}

func (c *GPSConfig) CreatePacketProcessors() (map[gpsprot.Tag]gpsprot.PacketProcessor, error) {
	nmeaNumbering := []gpsprot.NMEASVNumberingRange{}
	if c.NMEANumbering != "" {
		nmeaNumbering = gpsreg.FindNMEASVNumbering(c.NMEANumbering)
		if nmeaNumbering == nil {
			return nil, fmt.Errorf("unknown NMEA SV numbering: %s", c.NMEANumbering)
		}
	}
	return gpsreg.CreatePacketProcessors(nmeaNumbering), nil
}

func (c *GPSConfig) getTimeMode(target *gpsprot.ConfigTarget) error {
	opts := &target.Opts
	cp := &target.Props

	if !c.Stationary {
		cp.SetTimeMode(gpsprot.TimeModeDisabled)
		opts.Survey.When = 0
	} else {
		cp.SetStationary(true)
		if !c.FixedPosECEF.IsZero() {
			cp.SetTimeMode(gpsprot.TimeModeFixed)
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
	}
	return nil
}

func (c *GPSConfig) getTimeGNSS(cp *gpsprot.ConfigProps) error {
	if c.TimeGNSS == 0 {
		return nil
	}
	if !c.TimeGNSS.IsMajor() {
		return fmt.Errorf("time GNSS must be a major GNSS (%v is not)", c.TimeGNSS)
	}
	cp.SetTimeGNSS(c.TimeGNSS)
	return nil
}

func (c *GPSConfig) getFixedPos(cp *gpsprot.ConfigProps) error {
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
	cp.SetFixedPosECEF(fixedPos)
	acc := gpsprot.Meters(c.FixedPosAcc)
	if acc < gpsprot.Millimeter {
		return fmt.Errorf("fixed position accuracy %v is too small", c.FixedPosAcc)
	}
	if acc > gpsprot.Meter*1000 {
		return fmt.Errorf("fixed position accuracy %v is too large", c.FixedPosAcc)
	}
	cp.SetFixedPosAcc(acc)
	return nil
}

const speedOfLight = 0.299792458 // in meters per nanosecond

const maxAntennaCableDelay = 30000 // in nanoseconds; needs to fit in signed 16-bit integer

func (c *GPSConfig) getDelay(cp *gpsprot.ConfigProps) error {
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
	cp.SetAntennaCableDelay(time.Duration(delay))
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

const minSpeedSatellitesOutput = 38400

func (c *GPSConfig) satsMsg(speed int, wantSatellitesOutput bool) (opt gpsprot.Option[gpsprot.SatsMsgFlags]) {
	if c.SatellitesOutput == nil {
		if speed < minSpeedSatellitesOutput {
			// If the speed is too slow, then we won't have automatically enabled it,
			// so we don't need to disable it.
			return
		}
		if wantSatellitesOutput {
			opt.Set(gpsprot.SatsMsgSV)
		} else {
			opt.Set(gpsprot.SatsMsgNone)
		}
	} else if *c.SatellitesOutput {
		opt.Set(gpsprot.SatsMsgSV)
	} else {
		opt.Set(gpsprot.SatsMsgNone)
	}
	return
}

func (c *GPSConfig) rtcmMsg() (opt gpsprot.Option[gpsprot.RTCMMsgFlags]) {
	if c.RTCMOutput == nil {
		return
	}
	if *c.RTCMOutput {
		opt.Set(gpsprot.RTCMMsgAuto)
	} else {
		opt.Set(gpsprot.RTCMMsgNone)
	}
	return
}
