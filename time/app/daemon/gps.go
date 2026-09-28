package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// cfgFeatures describes aspects of the non-GPS daemon configuration
// that affect GPS configuration.
type cfgFeatures int

const (
	cfgTimePulse    cfgFeatures = 1 << iota // configure time pulse output on receiver
	cfgTimePulseMsg                         // configure messages reporting the time pulse
	cfgPosition                             // position data is used
	cfgSatellites                           // satellite data is used
	cfgNtripStream                          // Ntrip caster or push needs signals/mode read back from receiver
	cfgRTCMMSM7To4                          // Ntrip caster or push converts MSM7 output to MSM4
)

type GPSConfig struct {
	Config             bool          `toml:"config"`
	Mobile             bool          `toml:"mobile"`
	Resurvey           bool          `toml:"resurvey"`
	SurveyTime         uint32        `toml:"surveyTime"`
	SurveyAcc          float64       `toml:"surveyAcc"`
	FixedPosECEF       geopos.ECEF   `toml:"fixedPosECEF"`
	FixedPosLLH        *[3]float64   `toml:"fixedPosLLH"`
	FixedPosAcc        float64       `toml:"fixedPosAcc"`
	AntennaCableDelay  float64       `toml:"antennaCableDelay"`  // in nanoseconds
	AntennaCableLength float64       `toml:"antennaCableLength"` // in meters
	AntennaCableVF     float64       `toml:"antennaCableVF"`     // velocity factor
	TimeGNSS           gpsprot.GNSS  `toml:"timeGNSS"`
	MinElevation       float64       `toml:"minElevation"` // in degrees
	RTCMBaseID         uint16        `toml:"rtcmBaseID"`
	PulseWidth         float64       `toml:"pulseWidth"`
	SatellitesOutput   *bool         `toml:"satellitesOutput"`
	RTCMOutput         *bool         `toml:"rtcmOutput"`
	RTCMPreferMSM7     *bool         `toml:"rtcmPreferMSM7"`
	Vendor             gpsreg.Vendor `toml:"vendor"`
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
	MinElevation:       math.NaN(),
	RTCMBaseID:         0xFFFF,
	PulseWidth:         math.NaN(),
}

// target creates a GPS configuration target based on the current GPSConfig.
// If the returned error is errSatsOutNotEnabled, the caller should log it as a warning and continue.
func (c *GPSConfig) target(speed int, cf cfgFeatures) (*gpsprot.ConfigTarget, error) {
	target := gpsprot.NewConfigTarget()
	if !c.Config {
		return target, nil
	}
	if cf&cfgTimePulse != 0 {
		target.Props.SetPPS(defaultPPSWidth)
	}
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
	err = c.getMinElevation(cp)
	if err != nil {
		return nil, err
	}
	err = c.getRTCMBaseID(cp)
	if err != nil {
		return nil, err
	}
	if cf&cfgNtripStream != 0 {
		// Both properties feed STR-record fields built by
		// ntrip.StreamRecordBuilder: SignalsEnabled drives the
		// nav-system, carrier, and synthesised format-details
		// fields; Mode supplies the receiver's fixed position so
		// the lat/lon fields can be derived (Mode is otherwise
		// only in props when the operator set gps.fixedPosECEF).
		target.Get |= gpsprot.PropIDSignalsEnabled | gpsprot.PropIDMode
	}
	// setMsgOptions last because its error may be a non-fatal warning
	err = c.setMsgOptions(&target.Opts, speed, cf)
	return target, err
}

// setMsgOptions configures the message options on the ConfigTarget.
func (c *GPSConfig) setMsgOptions(opts *gpsprot.ConfigOptions, speed int, cf cfgFeatures) error {
	// Config is very slow on 8th gen if NMEA is enabled
	opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	if cf&cfgTimePulseMsg != 0 {
		opts.PVTMsg = gpsprot.PVTMsgTimingPTP
	} else {
		opts.PVTMsg = gpsprot.PVTMsgTimingSerialUTC
	}
	if cf&cfgPosition != 0 {
		opts.PVTMsg |= gpsprot.PVTMsgPos
		if c.Mobile {
			opts.PVTMsg |= gpsprot.PVTMsgVel
		}
	}
	opts.RTCMMsg = c.rtcmMsg(cf)
	var err error
	opts.SatsMsg, err = c.satsMsg(speed, cf&cfgSatellites != 0)
	return err
}

func (c *GPSConfig) getMode(target *gpsprot.ConfigTarget) error {
	opts := &target.Opts
	opts.Survey.MinDur = time.Second * time.Duration(c.SurveyTime)
	opts.Survey.AccLimit = gpsprot.Meters(c.SurveyAcc)
	if opts.Survey.AccLimit < gpsprot.Millimeter {
		return configErrorf("survey accuracy %v is too small", opts.Survey.AccLimit)
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
	if c.FixedPosECEF.IsZero() && c.FixedPosLLH == nil {
		opts.SetStatic = true
		return nil
	}
	if !c.FixedPosECEF.IsZero() && c.FixedPosLLH != nil {
		return configErrorf("must not specify both fixedPosECEF and fixedPosLLH")
	}
	if c.FixedPosLLH != nil {
		return c.setFixedModeLLH(cp)
	}
	return c.setFixedModeECEF(cp)
}

func (c *GPSConfig) setFixedModeECEF(cp *gpsprot.ConfigProps) error {
	err := c.FixedPosECEF.CheckOnEarth()
	if err != nil {
		return configErrorf("%v: invalid fixed position: %w", c.FixedPosECEF, err)
	}
	var fixedPos gpsprot.Point3D
	for i := range 3 {
		fixedPos[i] = gpsprot.Meters(c.FixedPosECEF[i])
	}
	acc, err := c.getFixedPosAcc()
	if err != nil {
		return err
	}
	cp.SetMode(gpsprot.Mode{
		Static:       true,
		PosType:      gpsprot.PosTypeECEF,
		FixedPosECEF: fixedPos,
		FixedPosAcc:  acc,
	})
	return nil
}

func (c *GPSConfig) setFixedModeLLH(cp *gpsprot.ConfigProps) error {
	coords := c.FixedPosLLH
	lat := gpsprot.DegreesFromFloat(coords[0])
	lon := gpsprot.DegreesFromFloat(coords[1])
	height := gpsprot.Meters(coords[2])
	if lat < -90*gpsprot.Degrees || lat > 90*gpsprot.Degrees {
		return configErrorf("fixed position latitude %v out of range [-90, 90]", coords[0])
	}
	if lon < -180*gpsprot.Degrees || lon > 180*gpsprot.Degrees {
		return configErrorf("fixed position longitude %v out of range [-180, 180]", coords[1])
	}
	if height < -500*gpsprot.Meter || height > 10000*gpsprot.Meter {
		return configErrorf("fixed position height %v out of range [-500, 10000]", coords[2])
	}
	acc, err := c.getFixedPosAcc()
	if err != nil {
		return err
	}
	cp.SetMode(gpsprot.Mode{
		Static:      true,
		PosType:     gpsprot.PosTypeLLH,
		FixedPosLLH: [2]gpsprot.Angle{lat, lon},
		Height:      height,
		FixedPosAcc: acc,
	})
	return nil
}

func (c *GPSConfig) getFixedPosAcc() (gpsprot.Length, error) {
	acc := gpsprot.Meters(c.FixedPosAcc)
	if acc < gpsprot.Millimeter {
		return 0, configErrorf("fixed position accuracy %v is too small", c.FixedPosAcc)
	}
	if acc > gpsprot.Meter*1000 {
		return 0, configErrorf("fixed position accuracy %v is too large", c.FixedPosAcc)
	}
	return acc, nil
}

func (c *GPSConfig) getTimeGNSS(cp *gpsprot.ConfigProps) error {
	if c.TimeGNSS == 0 {
		return nil
	}
	if !c.TimeGNSS.IsMajor() {
		return configErrorf("time GNSS must be a major GNSS (%v is not)", c.TimeGNSS)
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
			return configErrorf("invalid GPS antenna cable velocity factor %v", c.AntennaCableVF)
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
		return configErrorf("invalid cable delay %v (cable delay is in nanoseconds)", c.AntennaCableDelay)
	}
	cp.SetAntennaCableDelay(time.Duration(delay))
	return nil
}

func (c *GPSConfig) getMinElevation(cp *gpsprot.ConfigProps) error {
	if math.IsNaN(c.MinElevation) {
		return nil
	}
	if c.MinElevation < -90 || c.MinElevation > 90 {
		return configErrorf("minimum elevation %v out of range [-90, 90]", c.MinElevation)
	}
	cp.SetMinElevation(gpsprot.DegreesFromFloat(c.MinElevation))
	return nil
}

func (c *GPSConfig) getRTCMBaseID(cp *gpsprot.ConfigProps) error {
	if c.RTCMBaseID == 0xFFFF {
		return nil
	}
	if c.RTCMBaseID > 4095 {
		return configErrorf("RTCM base ID %d out of range [0, 4095]", c.RTCMBaseID)
	}
	cp.SetRTCMBaseID(c.RTCMBaseID)
	return nil
}

var errSatsOutNotEnabled = errors.New("satellites output will not be enabled")

const minSpeedSatellitesOutput = 38400

// satsMsg returns the satellites message option based on the configuration and speed.
// If the error is errSatsOutNotEnabled, and the caller should log it as a warning and continue.
func (c *GPSConfig) satsMsg(speed int, wantSatellitesOutput bool) (v opt.Val[gpsprot.SatsMsgFlags], err error) {
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
			v.Set(full)
		} else {
			v.Set(gpsprot.SatsMsgNone)
		}
	} else if *c.SatellitesOutput {
		v.Set(full)
	} else {
		v.Set(gpsprot.SatsMsgNone)
	}
	return
}

func (c *GPSConfig) rtcmMsg(cf cfgFeatures) (v opt.Val[gpsprot.RTCMMsgFlags]) {
	if c.RTCMOutput == nil {
		return
	}
	if !*c.RTCMOutput {
		v.Set(gpsprot.RTCMMsgNone)
		return
	}
	preferMSM7 := cf&cfgRTCMMSM7To4 != 0
	if c.RTCMPreferMSM7 != nil {
		preferMSM7 = *c.RTCMPreferMSM7
	}
	if preferMSM7 {
		v.Set(gpsprot.RTCMMsgAutoMSM7)
	} else {
		v.Set(gpsprot.RTCMMsgAuto)
	}
	return
}

func (c *GPSConfig) validate(lg *slog.Logger) {
	if !math.IsNaN(c.PulseWidth) {
		lg.Warn("gps.pulseWidth is deprecated and will be ignored; pulse width is now auto-detected")
	}
}
