package ubx

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

const nPort = 6

type CfgOld struct {
	tmode2  *bin.CfgTmode2
	tmode3  *bin.CfgTmode3
	tp5     *bin.CfgTp5
	gnss    *bin.CfgGNSS
	rate    *bin.CfgRate
	nav5    *bin.CfgNav5
	prt     *bin.CfgPrt
	msgRate map[bin.MsgID][nPort]byte
}

func (raw *CfgOld) SetMsgRate(msgID bin.MsgID, rate byte) {
	if raw == nil || raw.prt == nil {
		return
	}
	prt := raw.prt.PortID
	if prt >= nPort {
		return
	}
	if raw.msgRate == nil {
		raw.msgRate = make(map[bin.MsgID][nPort]byte)
	}
	rates := raw.msgRate[msgID]
	rates[int(prt)] = rate
	raw.msgRate[msgID] = rates
}

func (raw *CfgOld) msgEnabled(msgID bin.MsgID) bool {
	if raw == nil || raw.prt == nil {
		return false
	}
	prt := raw.prt.PortID
	if prt >= nPort {
		return false
	}
	if raw.msgRate == nil {
		return false
	}
	rates := raw.msgRate[msgID]
	return rates[int(prt)] == 1
}

func (raw *CfgOld) prtNMEAOutDisabled(origPrt *bin.CfgPrt) bool {
	if origPrt == nil {
		return false
	}
	return origPrt.OutProtoMask&bin.CfgPrtProtoNMEA != 0 && raw.prt.OutProtoMask&bin.CfgPrtProtoNMEA == 0
}

func (raw *CfgOld) cookPrt(cm *gpsprot.ConfigMap) {
	prt := raw.prt
	if prt == nil {
		return
	}
	gpsprot.CfgNMEAEnabled.Set(cm, prt.OutProtoMask&bin.CfgPrtProtoNMEA != 0)
	if prt.PortID == bin.PortUART1 || prt.PortID == bin.PortUART2 {
		gpsprot.CfgBaudRate.Set(cm, prt.BaudRate)
	}
}

func (raw *CfgOld) changePrt(cm *gpsprot.ConfigMap) *bin.CfgPrt {
	if raw.prt == nil {
		return nil
	}

	prt := *raw.prt
	if nmeaEnabled, exists := gpsprot.CfgNMEAEnabled.Get(cm); exists {
		if nmeaEnabled {
			prt.OutProtoMask |= bin.CfgPrtProtoNMEA
		} else {
			prt.OutProtoMask &^= bin.CfgPrtProtoNMEA
		}
	}
	if baudRate, exists := gpsprot.CfgBaudRate.Get(cm); exists {
		if prt.PortID == bin.PortUART1 || prt.PortID == bin.PortUART2 {
			prt.BaudRate = baudRate
		}
	}
	if prt == *raw.prt {
		return nil
	}
	return &prt
}

func (raw *CfgOld) cookTmode2(cm *gpsprot.ConfigMap) {
	tm := raw.tmode2
	if tm == nil {
		return
	}
	switch tm.TimeMode {
	case bin.CfgTmode2Disabled:
		gpsprot.CfgTimeMode.Set(cm, gpsprot.TimeModeDisabled)
	case bin.CfgTmode2SurveyIn:
		gpsprot.CfgTimeMode.Set(cm, gpsprot.TimeModeSurvey)
	case bin.CfgTmode2FixedMode:
		gpsprot.CfgTimeMode.Set(cm, gpsprot.TimeModeFixed)
		if tm.Flags&bin.CfgTmode2LLA == 0 {
			gpsprot.CfgFixedPosECEF.Set(cm, gpsprot.Point3D{
				gpsprot.Length(tm.EcefXOrLat) * gpsprot.Centimeter,
				gpsprot.Length(tm.EcefYOrLon) * gpsprot.Centimeter,
				gpsprot.Length(tm.EcefZOrAlt) * gpsprot.Centimeter,
			})
		}
		gpsprot.CfgFixedPosAcc.Set(cm, gpsprot.Length(tm.FixedPosAcc)*gpsprot.Millimeter)
	}
}

func (raw *CfgOld) changeTmode2(cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions) (*bin.CfgTmode2, bool) {
	if raw.tmode2 == nil {
		return nil, false
	}
	tm := *raw.tmode2

	mode := gpsprot.TimeModeDisabled
	switch tm.TimeMode {
	case bin.CfgTmode2SurveyIn:
		mode = gpsprot.TimeModeSurvey
	case bin.CfgTmode2FixedMode:
		mode = gpsprot.TimeModeFixed
	}
	if v, ok := gpsprot.CfgTimeMode.Get(cm); ok {
		mode = v
	}
	survey := false
	if opts.Survey.When.Contains(mode) {
		survey = true
		if tm.TimeMode != bin.CfgTmode2FixedMode {
			mode = gpsprot.TimeModeDisabled
		}
	}

	switch mode {
	case gpsprot.TimeModeDisabled:
		tm.TimeMode = bin.CfgTmode2Disabled
	case gpsprot.TimeModeSurvey:
		tm.TimeMode = bin.CfgTmode2SurveyIn
	case gpsprot.TimeModeFixed:
		tm.TimeMode = bin.CfgTmode2FixedMode
	}

	if ecef, exists := gpsprot.CfgFixedPosECEF.Get(cm); exists {
		var hp int8
		err := changeECEF(ecef, &tm.EcefXOrLat, &tm.EcefYOrLon, &tm.EcefZOrAlt, &hp, &hp, &hp)
		if err != nil {
			return nil, false
		}
		tm.Flags &^= bin.CfgTmode2LLA
	}

	if tm == *raw.tmode2 {
		return nil, survey
	}
	return &tm, survey
}

func (raw *CfgOld) surveyTmode2(opts gpsprot.ConfigOptions) *bin.CfgTmode2 {
	tm := *raw.tmode2
	tm.TimeMode = bin.CfgTmode2SurveyIn
	tm.SvinMinDur = uint32(opts.Survey.MinDur.Round(time.Second) / time.Second)
	q, _ := divModRound(int64(opts.Survey.AccLimit), int64(gpsprot.Millimeter))
	tm.SvinAccLimit = uint32(q)
	return &tm
}

func (raw *CfgOld) changeTmode3(cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions) (*bin.CfgTmode3, bool) {
	if raw.tmode3 == nil {
		return nil, false
	}
	tm := *raw.tmode3
	mode := gpsprot.TimeModeDisabled
	switch tm.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3SurveyIn:
		mode = gpsprot.TimeModeSurvey
	case bin.CfgTmode3FixedMode:
		mode = gpsprot.TimeModeFixed
	}
	if v, ok := gpsprot.CfgTimeMode.Get(cm); ok {
		mode = v
	}
	survey := false
	if opts.Survey.When.Contains(mode) {
		survey = true
		if tm.Flags&bin.CfgTmode3Mode != bin.CfgTmode3FixedMode {
			mode = gpsprot.TimeModeDisabled
		}
	}
	tm.Flags &^= bin.CfgTmode3Mode
	switch mode {
	case gpsprot.TimeModeDisabled:
		// do nothing
	case gpsprot.TimeModeSurvey:
		tm.Flags |= bin.CfgTmode3SurveyIn
	case gpsprot.TimeModeFixed:
		tm.Flags |= bin.CfgTmode3FixedMode
	}
	if ecef, exists := gpsprot.CfgFixedPosECEF.Get(cm); exists {
		err := changeECEF(ecef, &tm.EcefXOrLat, &tm.EcefYOrLon, &tm.EcefZOrAlt,
			&tm.EcefXOrLatHP, &tm.EcefYOrLonHP, &tm.EcefZOrAltHP)
		if err != nil {
			return nil, false
		}
		tm.Flags &^= bin.CfgTmode3LLA
	}

	if tm == *raw.tmode3 {
		return nil, survey
	}
	return &tm, survey
}

func (raw *CfgOld) surveyTmode3(opts gpsprot.ConfigOptions) *bin.CfgTmode3 {
	tm := *raw.tmode3
	tm.Flags &^= bin.CfgTmode3Mode
	tm.Flags |= bin.CfgTmode3SurveyIn
	tm.SvinMinDur = uint32(opts.Survey.MinDur.Round(time.Second) / time.Second)
	q, _ := divModRound(int64(opts.Survey.AccLimit), int64(gpsprot.Millimeter/10))
	tm.SvinAccLimit = uint32(q)
	return &tm
}

func changeECEF(ecef gpsprot.Point3D, x, y, z *int32, xhp, yhp, zhp *int8) (err error) {
	var lo [3]int32
	var hi [3]int8
	for i := 0; i < 3; i++ {
		lo[i], hi[i], err = splitLength(ecef[0])
		if err != nil {
			return
		}
	}
	*x, *y, *z, *xhp, *yhp, *zhp = lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]
	return
}

func (raw *CfgOld) cookTmode3(cm *gpsprot.ConfigMap) {
	tm := raw.tmode3
	if tm == nil {
		return
	}
	switch tm.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3Disabled:
		gpsprot.CfgTimeMode.Set(cm, gpsprot.TimeModeDisabled)
	case bin.CfgTmode3SurveyIn:
		gpsprot.CfgTimeMode.Set(cm, gpsprot.TimeModeSurvey)
	case bin.CfgTmode3FixedMode:
		gpsprot.CfgTimeMode.Set(cm, gpsprot.TimeModeFixed)
		if tm.Flags&bin.CfgTmode3LLA == 0 {
			gpsprot.CfgFixedPosECEF.Set(cm, gpsprot.Point3D{
				lengthHP(tm.EcefXOrLat, tm.EcefXOrLatHP),
				lengthHP(tm.EcefYOrLon, tm.EcefYOrLonHP),
				lengthHP(tm.EcefZOrAlt, tm.EcefZOrAltHP),
			})
		}
		gpsprot.CfgFixedPosAcc.Set(cm, gpsprot.Length(tm.FixedPosAcc)*(gpsprot.Millimeter/10))
	}
}

func (raw *CfgOld) cookTp5(cm *gpsprot.ConfigMap) {
	tp := raw.tp5
	if tp == nil {
		return
	}
	gpsprot.CfgAntennaCableDelay.Set(cm, time.Duration(tp.AntCableDelay)*time.Nanosecond)
	gpsprot.CfgTimePulsePolarityRising.Set(cm, tp.Flags&bin.CfgTp5Polarity != 0)
	flags := tp.Flags
	gnss := tp5FlagsGNSS(flags)
	gpsprot.CfgTimePulseAlignToGNSS.Set(cm, gnss != 0)
	if gnss != 0 {
		gpsprot.CfgPrimaryGNSS.Set(cm, gnss)
	}
	period, width := tpPeriodWidth(tp.FreqPeriod, tp.PulseLenRatio, flags)
	onlyWhenLocked := false
	if flags&bin.CfgTp5LockedOtherSet != 0 {
		onlyWhenLocked = width == 0
		period, width = tpPeriodWidth(tp.FreqPeriodLock, tp.PulseLenRatioLock, flags)
	}
	gpsprot.CfgTimePulsePeriod.Set(cm, period)
	// report inactive pulse as pulse width 0
	if flags&bin.CfgTp5Active == 0 {
		width = 0
	}
	gpsprot.CfgTimePulseWidth.Set(cm, width)
	gpsprot.CfgTimePulseOnlyWhenLocked.Set(cm, onlyWhenLocked)
}

func tp5FlagsGNSS(flags bin.CfgTp5Flags) gpsprot.GNSS {
	if flags&bin.CfgTp5AlignToTow == 0 || flags&bin.CfgTp5LockGpsFreq == 0 {
		return 0
	}
	grid := flags & bin.CfgTp5GridUTCGNSS
	switch grid {
	case bin.CfgTp5GridGPS:
		return gpsprot.GPS
	case bin.CfgTp5GridGLONASS:
		return gpsprot.GLO
	case bin.CfgTp5GridBeiDou:
		return gpsprot.BDS
	case bin.CfgTp5GridGalileo:
		return gpsprot.GAL
	}
	return 0
}

func tpPeriodWidth(freqPeriod, lenRatio uint32, flags bin.CfgTp5Flags) (time.Duration, time.Duration) {
	var period time.Duration
	if flags&bin.CfgTp5IsFreq != 0 {
		if freqPeriod == 0 {
			period = 0
		} else {
			period = (time.Second / time.Duration(freqPeriod)).Round(time.Microsecond)
		}
	} else {
		period = time.Duration(freqPeriod) * time.Microsecond
	}
	var width time.Duration
	if flags&bin.CfgTp5IsLength != 0 {
		width = time.Duration(lenRatio) * time.Microsecond
	} else {
		width = ((period * time.Duration(lenRatio)) >> 32).Round(time.Microsecond)
	}
	return period, width
}

func (raw *CfgOld) changeTp5(cm *gpsprot.ConfigMap) *bin.CfgTp5 {
	if raw.tp5 == nil {
		return nil
	}

	// Copy the current tp5
	tp := *raw.tp5

	// Handle CfgTimePulsePolarityRising
	rising, exists := gpsprot.CfgTimePulsePolarityRising.Get(cm)
	if exists {
		if rising {
			tp.Flags |= bin.CfgTp5Polarity
		} else {
			tp.Flags &^= bin.CfgTp5Polarity
		}
	}

	// Handle CfgTimePulseAlignGNSS
	if align, exists := gpsprot.CfgTimePulseAlignToGNSS.Get(cm); exists {
		gnssFlags := bin.CfgTp5AlignToTow | bin.CfgTp5LockGpsFreq
		if align {
			tp.Flags |= gnssFlags
			gnss := raw.changeTp5GNSS(cm)
			switch gnss {
			case gpsprot.GPS:
				tp.Flags |= bin.CfgTp5GridGPS
			case gpsprot.GLO:
				tp.Flags |= bin.CfgTp5GridGLONASS
			case gpsprot.BDS:
				tp.Flags |= bin.CfgTp5GridBeiDou
			case gpsprot.GAL:
				tp.Flags |= bin.CfgTp5GridGalileo
			}
		} else {
			tp.Flags &^= gnssFlags
		}
	}

	// Handle CfgTimePulseOnlyWhenLocked
	// Also set up where to write the perioad and width if we change them
	lenRatioPtr := &tp.PulseLenRatio
	freqPeriodPtr := &tp.FreqPeriod
	onlyWhenLocked, exists := gpsprot.CfgTimePulseOnlyWhenLocked.Get(cm)
	if exists {
		if onlyWhenLocked {
			lenRatioPtr = &tp.PulseLenRatioLock
			freqPeriodPtr = &tp.FreqPeriodLock
			tp.PulseLenRatio = 0
			if tp.Flags&bin.CfgTp5LockedOtherSet == 0 {
				tp.Flags |= bin.CfgTp5LockedOtherSet
				// we are changing from unsplit to split, so copy the unlocked period
				// just in case we don't change the period and the FreqPeriodLock was something bogus
				tp.FreqPeriodLock = tp.FreqPeriod
			}
		} else {
			tp.Flags &^= bin.CfgTp5LockedOtherSet
		}
	} else if tp.Flags&bin.CfgTp5LockedOtherSet != 0 {
		lenRatioPtr = &tp.PulseLenRatioLock
		freqPeriodPtr = &tp.FreqPeriodLock
	}

	// Handle CfgTimePulsePeriod
	// If onlyWhenLocked is set, then we change both the locked and unlocked periods
	period, periodExists := gpsprot.CfgTimePulsePeriod.Get(cm)
	if period <= 0 {
		periodExists = false
	}
	if periodExists {
		// if CfgTimePulseOnlyWhenLocked wasn't specified, then onlyWhenLocked will be false
		// so onlyWhenLocked tests that it was specified as true
		if onlyWhenLocked && period != 0 && time.Second%period != 0 {
			// if we have a period we cannot express in Hz, then switch over to using a length
			// but only if we are setting both the locked and unlocked periods
			tp.Flags &^= bin.CfgTp5IsFreq
		}
		if tp.Flags&bin.CfgTp5IsFreq == 0 {
			*freqPeriodPtr = uint32(period.Round(time.Microsecond) / time.Microsecond)
		} else {
			// XXX ought to round
			*freqPeriodPtr = uint32(time.Second / period)
		}
		if onlyWhenLocked {
			// unlocked pulse width will be zero, so it doesn't really matter what the unlocked period is
			// but we'll set it be the same as the locked period
			// just in case above we changed the CfgTp5IsFreq flags above
			tp.FreqPeriod = tp.FreqPeriodLock
		}
	}

	// Handle CfgTimePulseWidth
	if width, exists := gpsprot.CfgTimePulseWidth.Get(cm); exists {
		// width 0 means inactive
		if width == 0 {
			tp.Flags &^= bin.CfgTp5Active
		} else if width < 0 {
			// invalid ignore it
		} else {
			tp.Flags |= bin.CfgTp5Active
			// XXX don't want separate active flag, so make width of non-zero imply active
			if tp.Flags&bin.CfgTp5IsLength != 0 {
				*lenRatioPtr = uint32(width.Round(time.Microsecond) / time.Microsecond)
			} else {
				// need to write the pulse width as a ratio of the period
				if !periodExists {
					period, _ = tpPeriodWidth(*freqPeriodPtr, *lenRatioPtr, tp.Flags)
				}
				if period == 0 {
					*lenRatioPtr = 0
				} else {
					*lenRatioPtr = uint32(((width << 32) + period/2) / period)
				}
			}
		}
	}

	// if we didn't change anything, then there's nothing to do
	if tp == *raw.tp5 {
		return nil
	}
	return &tp
}

func (raw *CfgOld) changeTp5GNSS(cm *gpsprot.ConfigMap) gpsprot.GNSS {
	g, _ := gpsprot.CfgPrimaryGNSS.Get(cm)
	// if the primary GNSS is explicitly specified, then use that (regardless of whether it's enabled)
	if g.IsMajor() {
		return g
	}
	// otherwise, choose a suitable enabled GNSS

	enabled := gnssEnabledSet(raw.gnss)

	// try the one in the existing TP5 flags
	g = tp5FlagsGNSS(raw.tp5.Flags | bin.CfgTp5AlignToTow | bin.CfgTp5LockGpsFreq)
	if enabled.Contains(g) {
		return g
	}

	// try using the one implied by the UTC Standard in nav5
	g = nav5GNSS(raw.nav5)
	if enabled.Contains(g) {
		return g
	}

	// choose an enabled one based on this preference order
	// GLONASS is last because it's unusual leap second handling is bad for PTP
	// GPS is first because it's the most common
	// Galileo is kept closely aligned with GPS, so it's a good second choice if GPS isn't enabled
	prefer := []gpsprot.GNSS{gpsprot.GPS, gpsprot.GAL, gpsprot.BDS, gpsprot.GLO}

	for _, g := range prefer {
		if enabled.Contains(g) {
			return g
		}
	}
	return gpsprot.GPS
}

func (raw *CfgOld) cookGNSS(cm *gpsprot.ConfigMap) {
	gnss := raw.gnss
	if gnss == nil {
		return
	}
	gpsprot.CfgGNSSEnabled.Set(cm, gnssEnabledSet(gnss))
}

func gnssEnabledSet(gnss *bin.CfgGNSS) gpsprot.GNSSSet {
	if gnss == nil {
		return 0
	}
	var enabled gpsprot.GNSSSet
	for _, blk := range gnss.Blocks {
		if blk.Enable != 0 {
			if g := idToGNSS(blk.GNSSID); g != 0 {
				enabled |= gpsprot.GNSSFlag(g)
			}
		}
	}
	return enabled
}

func (raw *CfgOld) cookRate(cm *gpsprot.ConfigMap, ver *Version) {
	rate := raw.rate
	if rate == nil {
		return
	}
	gpsprot.CfgSolutionPeriod.Set(cm, rateSolutionPeriod(rate, ver))
}

func rateSolutionPeriod(rate *bin.CfgRate, ver *Version) time.Duration {
	period := time.Duration(rate.MeasRate) * time.Millisecond
	if ver.protVerAtLeast(18, 0) && rate.NavRate != 0 {
		period /= time.Duration(rate.NavRate)
	}
	return period
}

func (raw *CfgOld) changeRate(cm *gpsprot.ConfigMap, ver *Version) *bin.CfgRate {
	if raw.rate == nil {
		return nil
	}
	rate := *raw.rate
	if period, exists := gpsprot.CfgSolutionPeriod.Get(cm); exists {
		setSolutionPeriod(&rate, period, ver)
	}
	if gnss, exists := gpsprot.CfgPrimaryGNSS.Get(cm); exists {
		switch gnss {
		case gpsprot.GPS:
			rate.TimeRef = bin.CfgRateGPS
		case gpsprot.GLO:
			rate.TimeRef = bin.CfgRateGLONASS
		case gpsprot.BDS:
			rate.TimeRef = bin.CfgRateBeiDou
		case gpsprot.GAL:
			rate.TimeRef = bin.CfgRateGalileo
		}
	}
	if rate == *raw.rate {
		return nil
	}
	return &rate
}

func setSolutionPeriod(rate *bin.CfgRate, period time.Duration, ver *Version) {
	// don't unnecessatily change navRate
	if rateSolutionPeriod(rate, ver) == period {
		return
	}
	measRate := period.Round(time.Millisecond) / time.Millisecond
	if measRate <= 0 || measRate > 0xffff {
		return
	}
	rate.MeasRate = uint16(measRate)
	rate.NavRate = 1
}

func (raw *CfgOld) cookNav5(cm *gpsprot.ConfigMap) {
	nav5 := raw.nav5
	if nav5 == nil {
		return
	}
	stationary := false
	if nav5.DynModel == bin.CfgNav5DynStationary {
		stationary = true
	}
	if _, exist := gpsprot.CfgPrimaryGNSS.Get(cm); !exist {
		gnss := nav5GNSS(nav5)
		if gnss != 0 {
			gpsprot.CfgPrimaryGNSS.Set(cm, gnss)
		}
	}
	gpsprot.CfgStationary.Set(cm, stationary)
}

func nav5GNSS(nav5 *bin.CfgNav5) gpsprot.GNSS {
	if nav5 == nil {
		return 0
	}
	switch nav5.UtcStandard {
	case bin.CfgNav5UtcUSNO:
		return gpsprot.GPS
	case bin.CfgNav5UtcSU:
		return gpsprot.GLO
	case bin.CfgNav5UtcNTSC:
		return gpsprot.BDS
	case bin.CfgNav5UtcEU:
		return gpsprot.GAL
	}
	return 0
}

func (raw *CfgOld) changeNav5(cm *gpsprot.ConfigMap) *bin.CfgNav5 {
	if raw.nav5 == nil {
		return nil
	}

	nav5 := *raw.nav5
	nav5.Mask = 0
	if stationary, exists := gpsprot.CfgStationary.Get(cm); exists {
		if stationary {
			nav5.DynModel = bin.CfgNav5DynStationary
		} else if nav5.DynModel == bin.CfgNav5DynStationary {
			nav5.DynModel = bin.CfgNav5DynPortable
		}
		nav5.Mask |= bin.CfgNav5MaskDyn
	}

	if gnss, exists := gpsprot.CfgPrimaryGNSS.Get(cm); exists {
		nav5.Mask |= bin.CfgNav5MaskUtc
		switch gnss {
		case gpsprot.GPS:
			nav5.UtcStandard = bin.CfgNav5UtcUSNO
		case gpsprot.GLO:
			nav5.UtcStandard = bin.CfgNav5UtcSU
		case gpsprot.BDS:
			nav5.UtcStandard = bin.CfgNav5UtcNTSC
		case gpsprot.GAL:
			nav5.UtcStandard = bin.CfgNav5UtcEU
		default:
			nav5.Mask &^= bin.CfgNav5MaskUtc
		}
	}

	if nav5 == *raw.nav5 {
		return nil
	}
	return &nav5
}

func (raw *CfgOld) addMsgRate(msgID bin.MsgID, rate [6]byte) {
	if raw.msgRate == nil {
		raw.msgRate = make(map[bin.MsgID][6]byte)
	}
	raw.msgRate[msgID] = rate
}

func idToGNSS(g bin.GNSSID) gpsprot.GNSS {
	switch g {
	case bin.GPS:
		return gpsprot.GPS
	case bin.GLO:
		return gpsprot.GLO
	case bin.BDS:
		return gpsprot.BDS
	case bin.GAL:
		return gpsprot.GAL
	case bin.NavIC:
		return gpsprot.NAVIC
	case bin.QZSS:
		return gpsprot.QZSS
	case bin.SBAS:
		return gpsprot.SBAS
	}
	return 0
}
