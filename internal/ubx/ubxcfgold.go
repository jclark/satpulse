package ubx

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

const nPort = 6

type CfgOld struct {
	tmode   *bin.CfgTmode
	tmode2  *bin.CfgTmode2
	tmode3  *bin.CfgTmode3
	tp5     *bin.CfgTp5
	gnss    *bin.CfgGNSS
	rate    *bin.CfgRate
	nav5    *bin.CfgNav5
	prt     *bin.CfgPrt
	msgRate map[bin.MsgID][nPort]byte
}

// cfgOldProps says when a field of CfgOld may be needed when getting or setting a property in a ConfigProps.
// A PropID will be included in the slice for a field in cfgOldProps if and only if
// getting or setting that property may need to use the corresponding field in CfgOld.
var cfgOldProps = struct {
	tmode, tp5, gnss, nav5, prt []gpsprot.PropIDs
}{
	// tmode applies to tmode2 and tmode3 as well
	tmode: []gpsprot.PropIDs{
		gpsprot.PropIDTimeMode,
		gpsprot.PropIDFixedPosECEF,
		gpsprot.PropIDFixedPosAcc,
	},
	tp5: []gpsprot.PropIDs{
		gpsprot.PropIDAntennaCableDelay,
		gpsprot.PropIDTimePulsePolarityRising,
		gpsprot.PropIDTimePulseAlignToGNSS,
		gpsprot.PropIDTimePulsePeriod,
		gpsprot.PropIDTimePulseWidth,
		gpsprot.PropIDTimePulseOnlyWhenLocked,
		gpsprot.PropIDPrimaryGNSS,
	},
	gnss: []gpsprot.PropIDs{
		// XXX PropIDPrimaryGNSS doesn't currently look at the gnss field (I think)
		// PropIDPrimaryGNSS is regarded as unset, if tp5 isn't aligned to a GNSS and nav5 also doesn't have a preferred GNSS
		gpsprot.PropIDSignalsEnabled,
		gpsprot.PropIDTimePulseAlignToGNSS, // cookTp5 can end up looking at the .gnss field for this
	},
	nav5: []gpsprot.PropIDs{
		gpsprot.PropIDPrimaryGNSS,
		gpsprot.PropIDStationary,
	},
	prt: []gpsprot.PropIDs{
		gpsprot.PropIDBaudRate,
	},
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

func (raw *CfgOld) cookPrt(cp *gpsprot.ConfigProps) {
	prt := raw.prt
	if prt == nil {
		return
	}
	if prt.PortID == bin.PortUART1 || prt.PortID == bin.PortUART2 {
		cp.SetBaudRate(prt.BaudRate)
	}
}

func (raw *CfgOld) changePrt(target *gpsprot.ConfigTarget) *bin.CfgPrt {
	if raw.prt == nil {
		return nil
	}

	prt := *raw.prt
	cp := &target.Props
	if target.Opts.NMEAMsg.IsSet() {
		if target.Opts.NMEAMsg.Get() != 0 {
			prt.OutProtoMask |= bin.CfgPrtProtoNMEA
		} else {
			prt.OutProtoMask &^= bin.CfgPrtProtoNMEA
		}
	}
	if baudRate, exists := cp.GetBaudRate(); exists {
		if prt.PortID == bin.PortUART1 || prt.PortID == bin.PortUART2 {
			prt.BaudRate = baudRate
		}
	}
	if prt == *raw.prt {
		return nil
	}
	return &prt
}

func (raw *CfgOld) cookTmode(cp *gpsprot.ConfigProps) {
	tm := raw.tmode
	if tm == nil {
		return
	}
	switch tm.TimeMode {
	case bin.CfgTmodeDisabled:
		cp.SetTimeMode(gpsprot.TimeModeDisabled)
	case bin.CfgTmodeSurveyIn:
		cp.SetTimeMode(gpsprot.TimeModeSurvey)
	case bin.CfgTmodeFixedMode:
		cp.SetFixedPosECEF(gpsprot.Point3D{
			gpsprot.Length(tm.FixedPosX) * gpsprot.Centimeter,
			gpsprot.Length(tm.FixedPosY) * gpsprot.Centimeter,
			gpsprot.Length(tm.FixedPosZ) * gpsprot.Centimeter,
		})
		acc := math.Sqrt(float64(tm.FixedPosVar))
		cp.SetFixedPosAcc(gpsprot.Length(acc) * gpsprot.Millimeter)
	}
}

func (raw *CfgOld) changeTmode(target *gpsprot.ConfigTarget) (*bin.CfgTmode, bool) {
	if raw.tmode == nil {
		return nil, false
	}
	tm := *raw.tmode

	mode := gpsprot.TimeModeDisabled
	switch tm.TimeMode {
	case bin.CfgTmodeSurveyIn:
		mode = gpsprot.TimeModeSurvey
	case bin.CfgTmodeFixedMode:
		mode = gpsprot.TimeModeFixed
	}
	cp := &target.Props
	if timeMode, exists := cp.GetTimeMode(); exists {
		mode = timeMode
	}
	survey := false
	if target.Opts.Survey.When.Contains(mode) {
		survey = true
		if tm.TimeMode != bin.CfgTmodeFixedMode {
			mode = gpsprot.TimeModeDisabled
		}
	}
	switch mode {
	case gpsprot.TimeModeDisabled:
		tm.TimeMode = bin.CfgTmodeDisabled
	case gpsprot.TimeModeSurvey:
		tm.TimeMode = bin.CfgTmodeSurveyIn
	case gpsprot.TimeModeFixed:
		tm.TimeMode = bin.CfgTmodeFixedMode
	}

	if ecef, exists := cp.GetFixedPosECEF(); exists {
		var hp int8
		err := changeECEF(ecef, &tm.FixedPosX, &tm.FixedPosY, &tm.FixedPosZ, &hp, &hp, &hp)
		if err != nil {
			return nil, false
		}
	}
	if tm == *raw.tmode {
		return nil, survey
	}
	return &tm, survey
}

func (raw *CfgOld) surveyTmode(opts gpsprot.ConfigOptions) *bin.CfgTmode {
	tm := *raw.tmode
	tm.TimeMode = bin.CfgTmodeSurveyIn
	tm.SvinMinDur = uint32(opts.Survey.MinDur.Round(time.Second) / time.Second)
	q, _ := divModRound(int64(opts.Survey.AccLimit), int64(gpsprot.Millimeter))
	q = q * q
	if q > math.MaxUint32 {
		q = math.MaxUint32
	}
	tm.SvinVarLimit = uint32(q * q)
	return &tm
}

func (raw *CfgOld) cookTmode2(cp *gpsprot.ConfigProps) {
	tm := raw.tmode2
	if tm == nil {
		return
	}
	switch tm.TimeMode {
	case bin.CfgTmode2Disabled:
		cp.SetTimeMode(gpsprot.TimeModeDisabled)
	case bin.CfgTmode2SurveyIn:
		cp.SetTimeMode(gpsprot.TimeModeSurvey)
	case bin.CfgTmode2FixedMode:
		cp.SetTimeMode(gpsprot.TimeModeFixed)
		if tm.Flags&bin.CfgTmode2LLA == 0 {
			cp.SetFixedPosECEF(gpsprot.Point3D{
				gpsprot.Length(tm.EcefXOrLat) * gpsprot.Centimeter,
				gpsprot.Length(tm.EcefYOrLon) * gpsprot.Centimeter,
				gpsprot.Length(tm.EcefZOrAlt) * gpsprot.Centimeter,
			})
		}
		cp.SetFixedPosAcc(gpsprot.Length(tm.FixedPosAcc) * gpsprot.Millimeter)
	}
}

func (raw *CfgOld) changeTmode2(target *gpsprot.ConfigTarget) (*bin.CfgTmode2, bool) {
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
	cp := &target.Props
	if timeMode, exists := cp.GetTimeMode(); exists {
		mode = timeMode
	}
	survey := false
	if target.Opts.Survey.When.Contains(mode) {
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

	if ecef, exists := cp.GetFixedPosECEF(); exists {
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

func (raw *CfgOld) changeTmode3(target *gpsprot.ConfigTarget) (*bin.CfgTmode3, bool) {
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
	cp := &target.Props
	if timeMode, exists := cp.GetTimeMode(); exists {
		mode = timeMode
	}
	survey := false
	if target.Opts.Survey.When.Contains(mode) {
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
	if ecef, exists := cp.GetFixedPosECEF(); exists {
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

func (raw *CfgOld) cookTmode3(cp *gpsprot.ConfigProps) {
	tm := raw.tmode3
	if tm == nil {
		return
	}
	switch tm.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3Disabled:
		cp.SetTimeMode(gpsprot.TimeModeDisabled)
	case bin.CfgTmode3SurveyIn:
		cp.SetTimeMode(gpsprot.TimeModeSurvey)
	case bin.CfgTmode3FixedMode:
		cp.SetTimeMode(gpsprot.TimeModeFixed)
		if tm.Flags&bin.CfgTmode3LLA == 0 {
			cp.SetFixedPosECEF(gpsprot.Point3D{
				lengthHP(tm.EcefXOrLat, tm.EcefXOrLatHP),
				lengthHP(tm.EcefYOrLon, tm.EcefYOrLonHP),
				lengthHP(tm.EcefZOrAlt, tm.EcefZOrAltHP),
			})
		}
		cp.SetFixedPosAcc(gpsprot.Length(tm.FixedPosAcc) * (gpsprot.Millimeter / 10))
	}
}

func (raw *CfgOld) cookTp5(cp *gpsprot.ConfigProps) {
	tp := raw.tp5
	if tp == nil {
		return
	}
	cp.SetAntennaCableDelay(time.Duration(tp.AntCableDelay) * time.Nanosecond)
	cp.SetTimePulsePolarityRising(tp.Flags&bin.CfgTp5Polarity != 0)
	flags := tp.Flags
	gnss := tp5FlagsGNSS(flags)
	cp.SetTimePulseAlignToGNSS(gnss != 0)
	if gnss != 0 {
		cp.SetPrimaryGNSS(gnss)
	}
	period, width := tpPeriodWidth(tp.FreqPeriod, tp.PulseLenRatio, flags)
	onlyWhenLocked := false
	if flags&bin.CfgTp5LockedOtherSet != 0 {
		onlyWhenLocked = width == 0
		period, width = tpPeriodWidth(tp.FreqPeriodLock, tp.PulseLenRatioLock, flags)
	}
	cp.SetTimePulsePeriod(period)
	// report inactive pulse as pulse width 0
	if flags&bin.CfgTp5Active == 0 {
		width = 0
	}
	cp.SetTimePulseWidth(width)
	cp.SetTimePulseOnlyWhenLocked(onlyWhenLocked)
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

func (raw *CfgOld) changeTp5(cp *gpsprot.ConfigProps) *bin.CfgTp5 {
	if raw.tp5 == nil {
		return nil
	}

	// Copy the current tp5
	tp := *raw.tp5

	// Handle CfgTimePulsePolarityRising
	rising, exists := cp.GetTimePulsePolarityRising()
	if exists {
		if rising {
			tp.Flags |= bin.CfgTp5Polarity
		} else {
			tp.Flags &^= bin.CfgTp5Polarity
		}
	}

	// Handle CfgTimePulseAlignGNSS
	if align, exists := cp.GetTimePulseAlignToGNSS(); exists {
		gnssFlags := bin.CfgTp5AlignToTow | bin.CfgTp5LockGpsFreq
		if align {
			tp.Flags |= gnssFlags
			gnss := raw.changeTp5GNSS(cp)
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
	// Also set up where to write the period and width if we change them
	lenRatioPtr := &tp.PulseLenRatio
	freqPeriodPtr := &tp.FreqPeriod
	onlyWhenLocked, exists := cp.GetTimePulseOnlyWhenLocked()
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
	period, periodExists := cp.GetTimePulsePeriod()
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
	if width, exists := cp.GetTimePulseWidth(); exists {
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

func (raw *CfgOld) changeTp5GNSS(cp *gpsprot.ConfigProps) gpsprot.GNSS {
	g, _ := cp.GetPrimaryGNSS()
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

func (raw *CfgOld) cookGNSS(cp *gpsprot.ConfigProps) {
	gnss := raw.gnss
	if gnss == nil {
		return
	}
	var ss gpsprot.SignalSet
	for _, blk := range gnss.Blocks {
		if blk.Enable&1 == 1 {
			ss |= gnssCfgMaskSignals(blk.GNSSID, blk.SigCfgMask)
		}
	}
	cp.SetSignalsEnabled(ss)
}

var gnssSigMask = []struct {
	gnss bin.GNSSID
	mask bin.CfgGNSSSigMask
	sig  gpsprot.Signal
}{
	{bin.GPS, bin.CfgGNSSGPSL1CA, gpsprot.SigGPSL1CA},
	{bin.GPS, bin.CfgGNSSGPSL2C, gpsprot.SigGPSL2C},
	{bin.GPS, bin.CfgGNSSGPSL5, gpsprot.SigGPSL5},
	{bin.SBAS, bin.CfgGNSSSBASL1CA, gpsprot.SigSBASL1CA},
	{bin.GAL, bin.CfgGNSSGALE1, gpsprot.SigGALE1},
	{bin.GAL, bin.CfgGNSSGALE5a, gpsprot.SigGALE5a},
	{bin.GAL, bin.CfgGNSSGALE5b, gpsprot.SigGALE5b},
	{bin.BDS, bin.CfgGNSSBDSB1I, gpsprot.SigBDSB1I},
	{bin.BDS, bin.CfgGNSSBDSB2I, gpsprot.SigBDSB2I},
	{bin.BDS, bin.CfgGNSSBDSB2A, gpsprot.SigBDSB2a},
	{bin.GLO, bin.CfgGNSSGLOL1, gpsprot.SigGLOL1},
	{bin.GLO, bin.CfgGNSSGLOL2, gpsprot.SigGLOL2},
	{bin.QZSS, bin.CfgGNSSQZSSL1CA, gpsprot.SigQZSSL1CA},
	{bin.QZSS, bin.CfgGNSSQZSSL1S, gpsprot.SigQZSSL1S},
	{bin.QZSS, bin.CfgGNSSQZSSL2C, gpsprot.SigQZSSL2C},
	{bin.QZSS, bin.CfgGNSSQZSSL5, gpsprot.SigQZSSL5},
}

func gnssCfgMaskSignals(g bin.GNSSID, mask bin.CfgGNSSSigMask) gpsprot.SignalSet {
	ss := gpsprot.SignalSet(0)
	// not an efficient algorithm, but it doesn't matter
	for _, m := range gnssSigMask {
		if m.gnss == g && m.mask&mask != 0 {
			ss |= gpsprot.SignalSetOf(m.sig)
		}
	}
	return ss
}

func gnssEnabledSet(gnss *bin.CfgGNSS) gpsprot.GNSSSet {
	if gnss == nil {
		return 0
	}
	var enabled gpsprot.GNSSSet
	for _, blk := range gnss.Blocks {
		if blk.Enable != 0 {
			if g := idToGNSS(blk.GNSSID); g != 0 {
				enabled |= gpsprot.GNSSSetOf(g)
			}
		}
	}
	return enabled
}

func (raw *CfgOld) changeGNSS(cp *gpsprot.ConfigProps, ver *Version, monGNSS *monGNSS) (*bin.CfgGNSS, error) {
	if raw.gnss == nil {
		return nil, nil
	}
	signals, exists := cp.GetSignalsEnabled()
	if !exists {
		return nil, nil
	}
	gnss := *raw.gnss
	blocks := make([]bin.CfgGNSSBlock, len(gnss.Blocks))
	copy(blocks, gnss.Blocks)
	gnss.Blocks = blocks
	nMajor := 0
	// Note that `blk, i := range blocks` won't work here because we modify blocks[i]
	for i := range blocks {
		blk := &blocks[i]
		if blk.GNSSID == bin.IMES {
			// don't mess with IMES
			// it's not a GNSS, and our configuration doesn't touch it
			continue
		}
		cfgMask := bin.CfgGNSSSigMask(0)
		g := idToGNSS(blk.GNSSID)
		if g != 0 && (ver.GNSS.Contains(g) || (ver.GNSS == 0 && blk.GNSSID == bin.GPS)) {
			// the signal corresponding to the 0x1 bit in the SigCfgMask is always supported if the GNSS is available
			if gnssCfgMaskSignals(blk.GNSSID, 0x01)&signals != 0 {
				cfgMask |= 0x1
			}
			if blk.Enable&0x1 != 0 && blk.SigCfgMask&^0x01 != 0 {
				for i := 1; i < 8; i++ {
					m := bin.CfgGNSSSigMask(1 << i)
					if blk.SigCfgMask&m != 0 {
						if signals&gnssCfgMaskSignals(blk.GNSSID, m) != 0 {
							cfgMask |= m
						}
					}
				}
			}
			// Figure out whether to enable QZSS L1S
			// 19.2 is documented as first version supporting UBX-NAV-SLAS, which is specific to QZSS L1S
			if blk.GNSSID == bin.QZSS && signals&gpsprot.SignalSetOf(gpsprot.SigQZSSL1S) != 0 && ver.protVerAtLeast(19, 2) {
				cfgMask |= bin.CfgGNSSQZSSL1S
			}
		}
		blk.SigCfgMask = cfgMask
		if cfgMask != 0 {
			blk.Enable = 0x1
			if g.IsMajor() {
				nMajor++
			}
		} else {
			blk.Enable = 0
		}
	}
	if nMajor == 0 {
		return nil, fmt.Errorf("GPS receiver does not support specified GNSS signals")
	}
	if monGNSS != nil && nMajor >= monGNSS.maxSimultaneousMajorGNSS {
		if nMajor == 4 || monGNSS.maxSimultaneousMajorGNSS == 3 {
			// handle this case by disabling GLONASS
			for i := range blocks {
				blk := &blocks[i]
				if blk.GNSSID == bin.GLO {
					blk.Enable = 0
					blk.SigCfgMask = 0
					break
				}
			}
		} else {
			// no obvious way to handle this; but I don't think it should ever happen
			return nil, fmt.Errorf("%d major GNSSs enabled; exceeds maximum %d", nMajor, monGNSS.maxSimultaneousMajorGNSS)
		}
	}
	if !ver.protVerGreater(23, 0) {
		adjustTrackingChannels(&gnss)
	}
	// if no changes, then no need to send a message
	if gnss.CfgGNSSFixed == raw.gnss.CfgGNSSFixed && slices.Equal(gnss.Blocks, raw.gnss.Blocks) {
		return nil, nil
	}
	return &gnss, nil
}

// adjustTrackingChannels makes sure that the tracking channels comply with constraints in the spec.
// This is only called for protocol versions where the relevant fields are not read-only.
// This is conservative, and won't make any changes if the tracking channels do not violate the constraints.
func adjustTrackingChannels(gnss *bin.CfgGNSS) {
	numTrkChUse := min(gnss.NumTrkChHw, gnss.NumTrkChUse)

	resTotal := 0
	for i := range gnss.Blocks {
		blk := &gnss.Blocks[i]
		if blk.GNSSID == bin.IMES {
			continue
		}
		if blk.MaxTrkCh > numTrkChUse {
			blk.MaxTrkCh = numTrkChUse
		} else if blk.MaxTrkCh < 4 && idToGNSS(blk.GNSSID).IsMajor() {
			// It is required to be at least 4 for a major GNSS
			// If it's somehow less than 4, then make it something reasonable;
			// default is usually half the available tracking channels.
			blk.MaxTrkCh = max(4, numTrkChUse/2)
		}
		// Sum of reserved must be less than numTrkChUse
		// The spec is not explicit whether this applies to only the enabled GNSS or all of them.
		// But experimentation shows that it applies to enabled only.
		if blk.Enable&0x1 != 0 {
			resTotal += int(blk.ResTrkCh)
		}
	}
	if resTotal <= int(numTrkChUse) {
		return
	}
	if resTotal <= int(gnss.NumTrkChHw) {
		// problem was NumTrkChUse was too small
		gnss.NumTrkChUse = gnss.NumTrkChHw
		return
	}
	// There's some bigger problem, so fix up the ResTrkCh to something reasonable
	for i := range gnss.Blocks {
		blk := &gnss.Blocks[i]
		if blk.Enable&0x1 == 0 || blk.GNSSID == bin.IMES {
			continue
		}
		if idToGNSS(blk.GNSSID).IsMajor() {
			blk.ResTrkCh = gnss.NumTrkChHw / 4
			// Seems like a bad idea to have ResTrkCh > MaxTrkCh
			// even though spec doesn't explicitly disallow it
			if blk.ResTrkCh > blk.MaxTrkCh {
				blk.MaxTrkCh = gnss.NumTrkChHw / 2
			}
		} else {
			blk.ResTrkCh = 0
		}
	}
}

func (raw *CfgOld) changeRate(ct *gpsprot.ConfigTarget, ver *Version) *bin.CfgRate {
	if raw.rate == nil {
		return nil
	}
	rate := *raw.rate
	// XXX also survey progress message
	if ct.Opts.EnablesMsgs() {
		rate.MeasRate = 1000 // 1 second
		rate.NavRate = 1
	}
	if gnss, exists := ct.Props.GetPrimaryGNSS(); exists {
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

func (raw *CfgOld) cookNav5(cp *gpsprot.ConfigProps) {
	nav5 := raw.nav5
	if nav5 == nil {
		return
	}
	stationary := false
	if nav5.DynModel == bin.CfgNav5DynStationary {
		stationary = true
	}
	if _, exist := cp.GetPrimaryGNSS(); !exist {
		gnss := nav5GNSS(nav5)
		if gnss != 0 {
			cp.SetPrimaryGNSS(gnss)
		}
	}
	cp.SetStationary(stationary)
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

func (raw *CfgOld) changeNav5(cp *gpsprot.ConfigProps) *bin.CfgNav5 {
	if raw.nav5 == nil {
		return nil
	}

	nav5 := *raw.nav5
	nav5.Mask = 0
	if stationary, exists := cp.GetStationary(); exists {
		if stationary {
			nav5.DynModel = bin.CfgNav5DynStationary
		} else if nav5.DynModel == bin.CfgNav5DynStationary {
			nav5.DynModel = bin.CfgNav5DynPortable
		}
		nav5.Mask |= bin.CfgNav5MaskDyn
	}

	if gnss, exists := cp.GetPrimaryGNSS(); exists {
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

func monGNSSSet(mon bin.MonGnssMajorGnss) gpsprot.GNSSSet {
	g := gpsprot.GNSSSet(0)
	if mon&bin.MonGnssGPS != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.GPS)
	}
	if mon&bin.MonGnssGlonass != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.GLO)
	}
	if mon&bin.MonGnssBeidou != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.BDS)
	}
	if mon&bin.MonGnssGalileo != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.GAL)
	}
	return g
}
