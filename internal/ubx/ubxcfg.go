package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

const nPort = 6

type RawConfig struct {
	ver *Version
	// a bit for each port that might be in use
	usePorts uint8
	tmode2   *bin.CfgTmode2
	tmode3   *bin.CfgTmode3
	tp5      *bin.CfgTp5
	gnss     *bin.CfgGNSS
	rate     *bin.CfgRate
	nav5     *bin.CfgNav5
	prt      *bin.CfgPrt
	msgRate  map[bin.MsgID][nPort]byte
}

func (raw *RawConfig) Config() *gpsmsg.Config {
	if raw == nil {
		return nil
	}
	cfg := &gpsmsg.Config{}
	raw.convTmode2(cfg)
	if raw.tmode2 == nil {
		raw.convTmode3(cfg)
	}
	raw.convTp5(cfg)
	raw.convGNSS(cfg)
	raw.convRate(cfg)
	raw.convNav5(cfg)
	return cfg
}

func (raw *RawConfig) convTmode2(cfg *gpsmsg.Config) {
	tm := raw.tmode2
	if tm == nil {
		return
	}
	switch tm.TimeMode {
	case bin.CfgTmode2Disabled:
		gpsmsg.CfgTimeMode.Set(cfg, gpsmsg.TimeModeDisabled)
	case bin.CfgTmode2SurveyIn:
		gpsmsg.CfgTimeMode.Set(cfg, gpsmsg.TimeModeSurvey)
		gpsmsg.CfgSurveyMinDur.Set(cfg, time.Duration(tm.SvinMinDur)*time.Second)
		gpsmsg.CfgSurveyAccLimit.Set(cfg, gpsmsg.Length(tm.SvinAccLimit)*gpsmsg.Millimeter)
	case bin.CfgTmode2FixedMode:
		gpsmsg.CfgTimeMode.Set(cfg, gpsmsg.TimeModeFixed)
		if tm.Flags&bin.CfgTmode2LLA == 0 {
			gpsmsg.CfgFixedPosECEF.Set(cfg, gpsmsg.Point3D{
				X: gpsmsg.Length(tm.EcefXOrLat) * gpsmsg.Centimeter,
				Y: gpsmsg.Length(tm.EcefYOrLon) * gpsmsg.Centimeter,
				Z: gpsmsg.Length(tm.EcefZOrAlt) * gpsmsg.Centimeter,
			})
		}
		gpsmsg.CfgFixedPosAcc.Set(cfg, gpsmsg.Length(tm.FixedPosAcc)*gpsmsg.Millimeter)
	}
}

func (raw *RawConfig) convTmode3(cfg *gpsmsg.Config) {
	tm := raw.tmode3
	if tm == nil {
		return
	}
	switch tm.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3Disabled:
		gpsmsg.CfgTimeMode.Set(cfg, gpsmsg.TimeModeDisabled)
	case bin.CfgTmode3SurveyIn:
		gpsmsg.CfgTimeMode.Set(cfg, gpsmsg.TimeModeSurvey)
		gpsmsg.CfgSurveyMinDur.Set(cfg, time.Duration(tm.SvinMinDur)*time.Second)
		gpsmsg.CfgSurveyAccLimit.Set(cfg, gpsmsg.Length(tm.SvinAccLimit)*(gpsmsg.Millimeter/10))
	case bin.CfgTmode3FixedMode:
		gpsmsg.CfgTimeMode.Set(cfg, gpsmsg.TimeModeFixed)
		if tm.Flags&bin.CfgTmode3LLA == 0 {
			gpsmsg.CfgFixedPosECEF.Set(cfg, gpsmsg.Point3D{
				X: lengthHP(tm.EcefXOrLat, tm.EcefXOrLatHP),
				Y: lengthHP(tm.EcefYOrLon, tm.EcefYOrLonHP),
				Z: lengthHP(tm.EcefZOrAlt, tm.EcefZOrAltHP),
			})
		}
		gpsmsg.CfgFixedPosAcc.Set(cfg, gpsmsg.Length(tm.FixedPosAcc)*(gpsmsg.Millimeter/10))
	}
}

func lengthHP(l int32, h int8) gpsmsg.Length {
	return gpsmsg.Length(l)*gpsmsg.Centimeter + gpsmsg.Length(h)*(gpsmsg.Millimeter/10)
}

func (raw *RawConfig) convTp5(cfg *gpsmsg.Config) {
	tp := raw.tp5
	if tp == nil {
		return
	}
	gpsmsg.CfgAntennaCableDelay.Set(cfg, time.Duration(tp.AntCableDelay)*time.Nanosecond)
	gpsmsg.CfgTimePulsePolarityRising.Set(cfg, tp.Flags&bin.CfgTp5Polarity != 0)
	flags := tp.Flags
	if flags&bin.CfgTp5LockGpsFreq != 0 && flags&bin.CfgTp5AlignToTow != 0 {
		switch flags & bin.CfgTp5GridUTCGNSS {
		case bin.CfgTp5GridGPS:
			gpsmsg.CfgTimePulseGNSS.Set(cfg, gpsmsg.GPS)
		case bin.CfgTp5GridGLONASS:
			gpsmsg.CfgTimePulseGNSS.Set(cfg, gpsmsg.GLONASS)
		case bin.CfgTp5GridBeiDou:
			gpsmsg.CfgTimePulseGNSS.Set(cfg, gpsmsg.BeiDou)
		case bin.CfgTp5GridGalileo:
			gpsmsg.CfgTimePulseGNSS.Set(cfg, gpsmsg.Galileo)
		}
	}
	period, width := tpPeriodWidth(tp.FreqPeriod, tp.PulseLenRatio, flags)
	onlyWhenLocked := false
	if flags&bin.CfgTp5LockedOtherSet != 0 {
		onlyWhenLocked = period == 0 || width == 0
		period, width = tpPeriodWidth(tp.FreqPeriodLock, tp.PulseLenRatioLock, flags)
	}
	gpsmsg.CfgTimePulsePeriod.Set(cfg, period)
	gpsmsg.CfgTimePulseWidth.Set(cfg, width)
	gpsmsg.CfgTimePulseOnlyWhenLocked.Set(cfg, onlyWhenLocked)
}

func tpPeriodWidth(freqPeriod, lenRatio uint32, flags bin.CfgTp5Flags) (time.Duration, time.Duration) {
	var period time.Duration
	if flags&bin.CfgTp5IsFreq != 0 {
		if freqPeriod == 0 {
			period = 0
		} else {
			period = time.Second / time.Duration(freqPeriod)
		}
	} else {
		period = time.Duration(freqPeriod) * time.Microsecond
	}
	var width time.Duration
	if flags&bin.CfgTp5IsLength != 0 {
		width = time.Duration(lenRatio) * time.Microsecond
	} else {
		width = (period * time.Duration(lenRatio)) >> 32
	}
	return period, width
}

func (raw *RawConfig) convGNSS(cfg *gpsmsg.Config) {
	gnss := raw.gnss
	if gnss == nil {
		return
	}
	enabled := make([]gpsmsg.MajorGNSS, 0)
	for _, blk := range gnss.Blocks {
		if blk.Enable != 0 {
			if g, ok := majorGNSS(blk.GNSSID); ok {
				enabled = append(enabled, g)
			}
		}
	}
	gpsmsg.CfgEnabledGNSS.Set(cfg, enabled)
}

func (raw *RawConfig) convRate(cfg *gpsmsg.Config) {
	rate := raw.rate
	if rate == nil {
		return
	}
	period := time.Duration(raw.rate.MeasRate) * time.Millisecond
	if raw.ver.protVerAtLeast(18, 0) && period != 0 {
		period /= time.Duration(raw.rate.NavRate)
	}
	gpsmsg.CfgSolutionPeriod.Set(cfg, period)
}

func (raw *RawConfig) convNav5(cfg *gpsmsg.Config) {
	nav5 := raw.nav5
	if nav5 == nil {
		return
	}
	stationary := false
	if nav5.DynModel == bin.CfgNav5DynStationary {
		stationary = true
	}
	gpsmsg.CfgStationary.Set(cfg, stationary)
	var utc gpsmsg.MajorGNSS
	switch nav5.UtcStandard {
	case bin.CfgNav5UtcAuto:
		utc = 0
	case bin.CfgNav5UtcUSNO:
		utc = gpsmsg.GPS
	case bin.CfgNav5UtcSU:
		utc = gpsmsg.GLONASS
	case bin.CfgNav5UtcNTSC:
		utc = gpsmsg.BeiDou
	case bin.CfgNav5UtcEU:
		utc = gpsmsg.Galileo
	}
	gpsmsg.CfgUtcStandard.Set(cfg, utc)
}

func (c *RawConfig) SetVersion(ver *Version) {
	c.ver = ver
}

func (c *RawConfig) SetPortsInUse(ports []bin.PortID) {
	c.usePorts = 0
	for _, port := range ports {
		c.usePorts |= (1 << port)
	}
}

func (cfg *RawConfig) AddMsg(m bin.Msg) bool {
	if cfg == nil {
		return false
	}
	switch mt := m.(type) {
	case *bin.CfgTmode2:
		cfg.tmode2 = mt
	case *bin.CfgTmode3:
		cfg.tmode3 = mt
	case *bin.CfgTp5:
		cfg.tp5 = mt
	case *bin.CfgGNSS:
		cfg.gnss = mt
	case *bin.CfgRate:
		cfg.rate = mt
	case *bin.CfgNav5:
		cfg.nav5 = mt
	case *bin.CfgMsg:
		cfg.addMsgRate(mt.MsgID, mt.Rate)
	case *bin.CfgPrt:
		cfg.prt = mt
	default:
		return false
	}
	return true
}

func (cfg *RawConfig) Port() (bin.PortID, bool) {
	if cfg == nil || cfg.prt == nil {
		return 0, false
	}
	return cfg.prt.PortID, true
}

func (cfg *RawConfig) addMsgRate(msgID bin.MsgID, rate [6]byte) {
	if cfg.msgRate == nil {
		cfg.msgRate = make(map[bin.MsgID][6]byte)
	}
	cfg.msgRate[msgID] = rate
}

func majorGNSS(g bin.GNSSID) (gpsmsg.MajorGNSS, bool) {
	switch g {
	case bin.GPS:
		return gpsmsg.GPS, true
	case bin.GLONASS:
		return gpsmsg.GLONASS, true
	case bin.BeiDou:
		return gpsmsg.BeiDou, true
	case bin.Galileo:
		return gpsmsg.Galileo, true
	}
	return 0, false
}
