package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/geopos"
	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
)


type GPSConfig struct {
	Config             bool         `toml:"config"`
	Mobile             bool         `toml:"mobile"`
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
	Vendor             string       `toml:"vendor"`
}

const defaultAccuracy = 20.0 // in meters

// defaultPPSWidth is the default pulse width for PPS signals (100ms)
const defaultPPSWidth = time.Second / 10

var gpsDefault = GPSConfig{
	Mobile:             false,
	Resurvey:           false,
	SurveyTime:         2000, // 2000 seconds
	SurveyAcc:          defaultAccuracy,
	FixedPosAcc:        defaultAccuracy,
	AntennaCableDelay:  math.NaN(),
	AntennaCableLength: math.NaN(),
	AntennaCableVF:     0.66, // typical value for RG-58 cable
	PulseWidth:         math.NaN(),
}

// target creates a GPS configuration target based on the current GPSConfig.
// If the returned error is errSatsOutNotEnabled, the caller should log it as a warning and continue.
func (c *GPSConfig) target(speed int, wantSatellitesOutput bool, timePulseEnabled bool) (*gpsprot.ConfigTarget, error) {
	target := gpsprot.NewConfigTarget()
	if !c.Config {
		return target, nil
	}
	if timePulseEnabled {
		target.Props.SetPPS(defaultPPSWidth)
	}
	gpsevent.SetMsgOptions(target, timePulseEnabled)
	err := c.getMode(target)
	if err != nil {
		return nil, err
	}
	cp := &target.Props
	err = c.getTimeGNSS(cp)
	if err != nil {
		return nil, err
	}
	err = c.getDelay(cp)
	if err != nil {
		return nil, err
	}
	target.Opts.RTCMMsg = c.rtcmMsg()
	target.Opts.SatsMsg, err = c.satsMsg(speed, wantSatellitesOutput)
	return target, err
}

func (c *GPSConfig) CreatePacketProcessors() (map[gpsprot.Tag]gpsprot.PacketProcessor, error) {
	var nmeaNumbering []gpsprot.NMEASVNumberingRange
	if c.Vendor != "" {
		vendor := gpsreg.ParseVendor(c.Vendor)
		if vendor == gpsreg.VendorUnknown {
			return nil, fmt.Errorf("unknown vendor: %s", c.Vendor)
		}
		nmeaNumbering = gpsreg.FindNMEASVNumbering(vendor)
	}
	return gpsreg.CreatePacketProcessors(nmeaNumbering), nil
}

func (c *GPSConfig) getMode(target *gpsprot.ConfigTarget) error {
	opts := &target.Opts
	opts.Survey.MinDur = time.Second * time.Duration(c.SurveyTime)
	opts.Survey.AccLimit = gpsprot.Meters(c.SurveyAcc)
	if opts.Survey.AccLimit < gpsprot.Millimeter {
		return fmt.Errorf("survey accuracy %v is too small", opts.Survey.AccLimit)
	}
	if c.Resurvey {
		opts.Survey.Flags |= gpsprot.SurveyAgain
	}
	cp := &target.Props
	// mobile takes precedence over fixed position
	// might want to temporarily turn off time mode
	if c.Mobile {
		cp.SetMode(gpsprot.Mode{Static: false})
		return nil
	}
	if c.FixedPosECEF.IsZero() {
		opts.SetStatic = true
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
	acc := gpsprot.Meters(c.FixedPosAcc)
	if acc < gpsprot.Millimeter {
		return fmt.Errorf("fixed position accuracy %v is too small", c.FixedPosAcc)
	}
	if acc > gpsprot.Meter*1000 {
		return fmt.Errorf("fixed position accuracy %v is too large", c.FixedPosAcc)
	}
	cp.SetMode(gpsprot.Mode{
		Static:       true,
		PosType:      gpsprot.PosTypeECEF,
		FixedPosECEF: fixedPos,
		FixedPosAcc:  acc,
	})
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

var errSatsOutNotEnabled = errors.New("satellites output will not be enabled")

const minSpeedSatellitesOutput = 38400

// satsMsg returns the satellites message option based on the configuration and speed.
// If the error is errSatsOutNotEnabled, and the caller should log it as a warning and continue.
func (c *GPSConfig) satsMsg(speed int, wantSatellitesOutput bool) (opt gpsprot.Option[gpsprot.SatsMsgFlags], err error) {
	const full = gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal
	if c.SatellitesOutput == nil {
		if speed < minSpeedSatellitesOutput {
			// If the speed is too slow, then we won't have automatically enabled it,
			// so we don't need to disable it.
			if wantSatellitesOutput {
				// But we should tell the user about it. The caller should log this error as a warning and continue.
				err = fmt.Errorf("%w: serial speed %d is too low; minimum speed is %d", errSatsOutNotEnabled, speed, minSpeedSatellitesOutput)
			}
			return
		}
		if wantSatellitesOutput {
			opt.Set(full)
		} else {
			opt.Set(gpsprot.SatsMsgNone)
		}
	} else if *c.SatellitesOutput {
		opt.Set(full)
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

func (c *GPSConfig) validate(lg *slog.Logger) {
	if !math.IsNaN(c.PulseWidth) {
		lg.Warn("gps.pulseWidth is deprecated and will be ignored; pulse width is now auto-detected")
	}
}
