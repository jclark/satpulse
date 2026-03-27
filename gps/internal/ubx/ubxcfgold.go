package ubx

import (
	"fmt"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

const nPort = 6

type CfgOld struct {
	tmode   *ubxbin.CfgTmode
	tmode2  *ubxbin.CfgTmode2
	tmode3  *ubxbin.CfgTmode3
	tp5     *ubxbin.CfgTp5
	gnss    *ubxbin.CfgGNSS
	rate    *ubxbin.CfgRate
	nav5    *ubxbin.CfgNav5
	prt     *ubxbin.CfgPrt
	msgRate map[ubxbin.MsgID][nPort]byte
}

// cfgOldProps says when a field of CfgOld may be needed when getting or setting a property in a ConfigProps.
// A PropID will be included in the slice for a field in cfgOldProps if and only if
// getting or setting that property may need to use the corresponding field in CfgOld.
var cfgOldProps = struct {
	tmode, tp5, gnss, nav5 []gpsprot.PropIDs
}{
	// tmode applies to tmode2 and tmode3 as well
	tmode: []gpsprot.PropIDs{
		gpsprot.PropIDMode,
	},
	tp5: []gpsprot.PropIDs{
		gpsprot.PropIDAntennaCableDelay,
		gpsprot.PropIDTimePulsePolarityRising,
		gpsprot.PropIDTimePulseAlignToGNSS,
		gpsprot.PropIDTimePulsePeriod,
		gpsprot.PropIDTimePulseWidth,
		gpsprot.PropIDTimePulseOnlyWhenLocked,
		gpsprot.PropIDTimeGNSS,
	},
	gnss: []gpsprot.PropIDs{
		// XXX PropIDTimeGNSS doesn't currently look at the gnss field (I think)
		// PropIDTimeGNSS is regarded as unset, if tp5 isn't aligned to a GNSS and nav5 also doesn't have a preferred GNSS
		gpsprot.PropIDSignalsEnabled,
		gpsprot.PropIDTimePulseAlignToGNSS, // changeTp5 can end up looking at the .gnss field for this
	},
	nav5: []gpsprot.PropIDs{
		gpsprot.PropIDTimeGNSS,
		gpsprot.PropIDMinElevation,
	},
}

func (raw *CfgOld) SetMsgRate(msgID ubxbin.MsgID, rate byte) {
	if raw == nil || raw.prt == nil {
		return
	}
	prt := raw.prt.PortID
	if prt >= nPort {
		return
	}
	if raw.msgRate == nil {
		raw.msgRate = make(map[ubxbin.MsgID][nPort]byte)
	}
	rates := raw.msgRate[msgID]
	rates[int(prt)] = rate
	raw.msgRate[msgID] = rates
}

func (raw *CfgOld) anyMsgEnabled() bool {
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
	for _, rates := range raw.msgRate {
		if rates[int(prt)] != 0 {
			return true
		}
	}
	return false
}

func (raw *CfgOld) prtNMEAOutDisabled(origPrt *ubxbin.CfgPrt) bool {
	if origPrt == nil {
		return false
	}
	return origPrt.OutProtoMask&ubxbin.CfgPrtProtoNMEA != 0 && raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA == 0
}

func (raw *CfgOld) changePrtProto(mc *msgChanges) *ubxbin.CfgPrt {
	if raw.prt == nil {
		return nil
	}
	outProtoMask := mc.changeOutProtoMask(raw.prt.OutProtoMask)
	if outProtoMask == raw.prt.OutProtoMask {
		return nil
	}
	prt := *raw.prt
	prt.OutProtoMask = outProtoMask
	return &prt
}

func (raw *CfgOld) changePrtBaudRate(opts *gpsprot.ConfigOptions) *ubxbin.CfgPrt {
	if opts.BaudRate == 0 || raw.prt == nil || opts.BaudRate == raw.prt.BaudRate ||
		(raw.prt.PortID != ubxbin.PortUART1 && raw.prt.PortID != ubxbin.PortUART2) {
		return nil
	}
	prt := *raw.prt
	prt.BaudRate = opts.BaudRate
	return &prt
}

func (raw *CfgOld) cookTmode(cp *gpsprot.ConfigProps) {
	tmc := raw.getTmodeConfig()
	if tmc == nil {
		return
	}
	cp.SetMode(tmc.getMode())
}

func (raw *CfgOld) getTmodeConfig() *tmodeConfig {
	tmc := tmodeConfig{}
	if raw.tmode3 != nil {
		tmc.fromTmode3(raw.tmode3)
	} else if raw.tmode2 != nil {
		tmc.fromTmode2(raw.tmode2)
	} else if raw.tmode != nil {
		tmc.fromTmode(raw.tmode)
	} else {
		return nil
	}
	return &tmc
}

func (raw *CfgOld) changeTmode(target *gpsprot.ConfigTarget) (ubxbin.Msg, ubxbin.Msg, error) {
	// Get the current raw tmode message
	var tmodeMsg ubxbin.Msg
	if raw.tmode3 != nil {
		tmodeMsg = raw.tmode3
	} else if raw.tmode2 != nil {
		tmodeMsg = raw.tmode2
	} else if raw.tmode != nil {
		tmodeMsg = raw.tmode
	} else {
		// no tmode message, so nothing to change
		return nil, nil, nil
	}
	// Convert the raw tmode message to the intermediate representation.
	cur := &tmodeConfig{}
	cur.fromTmodeMsg(tmodeMsg)
	// Use the intermediate tmodeConfig representation to determine the messages to produce
	var err error
	var tmc [2]*tmodeConfig
	tmc[0], tmc[1], err = createTmodeConfigs(target, cur, resurveyDisable)
	if err != nil {
		return nil, nil, err
	}
	// Convert the intermediate messages back to the raw message type using the current tmodeMsg as a basis.
	var msg [2]ubxbin.Msg
	for i := range 2 {
		if tmc[i] == nil {
			continue
		}
		msg[i], err = tmc[i].toTmodeMsg(tmodeMsg)
		if err != nil {
			return nil, nil, err
		}
	}
	return msg[0], msg[1], nil
}

func (raw *CfgOld) cookTp5(cp *gpsprot.ConfigProps) {
	tp := raw.tp5
	if tp == nil {
		return
	}
	cp.SetAntennaCableDelay(time.Duration(tp.AntCableDelay) * time.Nanosecond)
	cp.SetTimePulsePolarityRising(tp.Flags&ubxbin.CfgTp5Polarity != 0)
	flags := tp.Flags
	gnss := tp5FlagsGNSS(flags)
	cp.SetTimePulseAlignToGNSS(gnss != 0)
	if gnss != 0 {
		cp.SetTimeGNSS(gnss)
	}
	period, width := tpPeriodWidth(tp.FreqPeriod, tp.PulseLenRatio, flags)
	onlyWhenLocked := false
	if flags&ubxbin.CfgTp5LockedOtherSet != 0 {
		onlyWhenLocked = width == 0
		period, width = tpPeriodWidth(tp.FreqPeriodLock, tp.PulseLenRatioLock, flags)
	}
	cp.SetTimePulsePeriod(period)
	// report inactive pulse as pulse width 0
	if flags&ubxbin.CfgTp5Active == 0 {
		width = 0
	}
	cp.SetTimePulseWidth(width)
	cp.SetTimePulseOnlyWhenLocked(onlyWhenLocked)
}

func tp5FlagsGNSS(flags ubxbin.CfgTp5Flags) gpsprot.GNSS {
	if flags&ubxbin.CfgTp5AlignToTow == 0 || flags&ubxbin.CfgTp5LockGpsFreq == 0 {
		return 0
	}
	grid := flags & ubxbin.CfgTp5GridUTCGNSS
	switch grid {
	case ubxbin.CfgTp5GridGPS:
		return gpsprot.GPS
	case ubxbin.CfgTp5GridGLONASS:
		return gpsprot.GLO
	case ubxbin.CfgTp5GridBeiDou:
		return gpsprot.BDS
	case ubxbin.CfgTp5GridGalileo:
		return gpsprot.GAL
	}
	return 0
}

func tpPeriodWidth(freqPeriod, lenRatio uint32, flags ubxbin.CfgTp5Flags) (time.Duration, time.Duration) {
	var period time.Duration
	if flags&ubxbin.CfgTp5IsFreq != 0 {
		if freqPeriod == 0 {
			period = 0
		} else {
			period = (time.Second / time.Duration(freqPeriod)).Round(time.Microsecond)
		}
	} else {
		period = time.Duration(freqPeriod) * time.Microsecond
	}
	var width time.Duration
	if flags&ubxbin.CfgTp5IsLength != 0 {
		width = time.Duration(lenRatio) * time.Microsecond
	} else {
		width = ((period * time.Duration(lenRatio)) >> 32).Round(time.Microsecond)
	}
	return period, width
}

func (raw *CfgOld) changeTp5(cp *gpsprot.ConfigProps) *ubxbin.CfgTp5 {
	if raw.tp5 == nil {
		return nil
	}

	// Copy the current tp5
	tp := *raw.tp5

	// Handle CfgTimePulsePolarityRising
	rising, exists := cp.GetTimePulsePolarityRising()
	if exists {
		if rising {
			tp.Flags |= ubxbin.CfgTp5Polarity
		} else {
			tp.Flags &^= ubxbin.CfgTp5Polarity
		}
	}

	// Handle CfgTimePulseAlignGNSS
	if align, exists := cp.GetTimePulseAlignToGNSS(); exists {
		gnssFlags := ubxbin.CfgTp5AlignToTow | ubxbin.CfgTp5LockGpsFreq
		if align {
			tp.Flags |= gnssFlags
			tp.Flags &^= ubxbin.CfgTp5GridUTCGNSS
			gnss := raw.changeTp5GNSS(cp)
			switch gnss {
			case gpsprot.GPS:
				tp.Flags |= ubxbin.CfgTp5GridGPS
			case gpsprot.GLO:
				tp.Flags |= ubxbin.CfgTp5GridGLONASS
			case gpsprot.BDS:
				tp.Flags |= ubxbin.CfgTp5GridBeiDou
			case gpsprot.GAL:
				tp.Flags |= ubxbin.CfgTp5GridGalileo
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
			if tp.Flags&ubxbin.CfgTp5LockedOtherSet == 0 {
				tp.Flags |= ubxbin.CfgTp5LockedOtherSet
				// we are changing from unsplit to split, so copy the unlocked period
				// just in case we don't change the period and the FreqPeriodLock was something bogus
				tp.FreqPeriodLock = tp.FreqPeriod
			}
		} else {
			tp.Flags &^= ubxbin.CfgTp5LockedOtherSet
		}
	} else if tp.Flags&ubxbin.CfgTp5LockedOtherSet != 0 {
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
			tp.Flags &^= ubxbin.CfgTp5IsFreq
		}
		if tp.Flags&ubxbin.CfgTp5IsFreq == 0 {
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
			tp.Flags &^= ubxbin.CfgTp5Active
		} else if width < 0 {
			// invalid ignore it
		} else {
			tp.Flags |= ubxbin.CfgTp5Active
			// XXX don't want separate active flag, so make width of non-zero imply active
			if tp.Flags&ubxbin.CfgTp5IsLength != 0 {
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
	g, _ := cp.GetTimeGNSS()
	// if the primary GNSS is explicitly specified, then use that (regardless of whether it's enabled)
	if g.IsMajor() {
		return g
	}
	// otherwise, choose a suitable enabled GNSS

	enabled := gnssEnabledSet(raw.gnss)

	// try the one in the existing TP5 flags
	g = tp5FlagsGNSS(raw.tp5.Flags | ubxbin.CfgTp5AlignToTow | ubxbin.CfgTp5LockGpsFreq)
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
	gnss ubxbin.GNSSID
	mask ubxbin.CfgGNSSSigMask
	sig  gpsprot.Signal
}{
	{ubxbin.GPS, ubxbin.CfgGNSSGPSL1CA, gpsprot.SigGPSL1CA},
	{ubxbin.GPS, ubxbin.CfgGNSSGPSL2C, gpsprot.SigGPSL2C},
	{ubxbin.GPS, ubxbin.CfgGNSSGPSL5, gpsprot.SigGPSL5},
	{ubxbin.SBAS, ubxbin.CfgGNSSSBASL1CA, gpsprot.SigSBASL1CA},
	{ubxbin.GAL, ubxbin.CfgGNSSGALE1, gpsprot.SigGALE1},
	{ubxbin.GAL, ubxbin.CfgGNSSGALE5a, gpsprot.SigGALE5a},
	{ubxbin.GAL, ubxbin.CfgGNSSGALE5b, gpsprot.SigGALE5b},
	{ubxbin.BDS, ubxbin.CfgGNSSBDSB1I, gpsprot.SigBDSB1I},
	{ubxbin.BDS, ubxbin.CfgGNSSBDSB2I, gpsprot.SigBDSB2I},
	{ubxbin.BDS, ubxbin.CfgGNSSBDSB2A, gpsprot.SigBDSB2a},
	{ubxbin.GLO, ubxbin.CfgGNSSGLOL1, gpsprot.SigGLOL1},
	{ubxbin.GLO, ubxbin.CfgGNSSGLOL2, gpsprot.SigGLOL2},
	{ubxbin.QZSS, ubxbin.CfgGNSSQZSSL1CA, gpsprot.SigQZSSL1CA},
	{ubxbin.QZSS, ubxbin.CfgGNSSQZSSL1S, gpsprot.SigQZSSL1S},
	{ubxbin.QZSS, ubxbin.CfgGNSSQZSSL2C, gpsprot.SigQZSSL2C},
	{ubxbin.QZSS, ubxbin.CfgGNSSQZSSL5, gpsprot.SigQZSSL5},
}

func gnssCfgMaskSignals(g ubxbin.GNSSID, mask ubxbin.CfgGNSSSigMask) gpsprot.SignalSet {
	ss := gpsprot.SignalSet(0)
	// not an efficient algorithm, but it doesn't matter
	for _, m := range gnssSigMask {
		if m.gnss == g && m.mask&mask != 0 {
			ss |= gpsprot.SignalSetOf(m.sig)
		}
	}
	return ss
}

func gnssEnabledSet(gnss *ubxbin.CfgGNSS) gpsprot.GNSSSet {
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

func (raw *CfgOld) changeGNSS(cp *gpsprot.ConfigProps, ver *Version, monGNSS *monGNSS) (*ubxbin.CfgGNSS, error) {
	if raw.gnss == nil {
		return nil, nil
	}
	signals, exists := cp.GetSignalsEnabled()
	if !exists {
		return nil, nil
	}
	gnss := *raw.gnss
	blocks := make([]ubxbin.CfgGNSSBlock, len(gnss.Blocks))
	copy(blocks, gnss.Blocks)
	gnss.Blocks = blocks
	nMajor := 0
	// Gen 8 has restriction to two carrier frequencies
	// GPS and GAL have the same L1 frequency, but BDS and GLO are both different
	const (
		freqGPSGAL = 0x1
		freqGLO    = 0x2
		freqBDS    = 0x4
	)
	freq := 0
	// Note that `blk, i := range blocks` won't work here because we modify blocks[i]
	for i := range blocks {
		blk := &blocks[i]
		if blk.GNSSID == ubxbin.IMES {
			// don't mess with IMES
			// it's not a GNSS, and our configuration doesn't touch it
			continue
		}
		cfgMask := ubxbin.CfgGNSSSigMask(0)
		g := idToGNSS(blk.GNSSID)
		if g != 0 && (ver.GNSS.Contains(g) || (ver.GNSS == 0 && blk.GNSSID == ubxbin.GPS)) {
			// the signal corresponding to the 0x1 bit in the SigCfgMask is always supported if the GNSS is available
			if gnssCfgMaskSignals(blk.GNSSID, 0x01)&signals != 0 {
				cfgMask |= 0x1
			}
			if blk.Enable&0x1 != 0 && blk.SigCfgMask&^0x01 != 0 {
				for i := 1; i < 8; i++ {
					m := ubxbin.CfgGNSSSigMask(1 << i)
					if blk.SigCfgMask&m != 0 {
						if signals&gnssCfgMaskSignals(blk.GNSSID, m) != 0 {
							cfgMask |= m
						}
					}
				}
			}
			// Figure out whether to enable QZSS L1S
			// 19.2 is documented as first version supporting UBX-NAV-SLAS, which is specific to QZSS L1S
			if blk.GNSSID == ubxbin.QZSS && signals&gpsprot.SignalSetOf(gpsprot.SigQZSSL1S) != 0 && ver.protVerAtLeast(19, 2) {
				cfgMask |= ubxbin.CfgGNSSQZSSL1S
			}
		}
		blk.SigCfgMask = cfgMask
		if cfgMask != 0 {
			blk.Enable = 0x1
			if g.IsMajor() {
				nMajor++
				switch g {
				case gpsprot.GPS, gpsprot.GAL:
					freq |= freqGPSGAL
				case gpsprot.GLO:
					freq |= freqGLO
				case gpsprot.BDS:
					freq |= freqBDS
				}
			}
		} else {
			blk.Enable = 0
		}
	}
	if nMajor == 0 {
		return nil, fmt.Errorf("GPS receiver does not support specified GNSS signals")
	}
	if (monGNSS != nil && nMajor > monGNSS.maxSimultaneousMajorGNSS) || freq == freqGPSGAL|freqGLO|freqBDS {
		// try to disable GLONASS
		for i := range blocks {
			blk := &blocks[i]
			if blk.GNSSID == ubxbin.GLO {
				if blk.Enable&0x1 != 0 {
					blk.Enable = 0
					blk.SigCfgMask = 0
					nMajor--
				}
				break
			}
		}
		// that wasn't enough: not clear which other GNSS to disable
		if monGNSS != nil && nMajor > monGNSS.maxSimultaneousMajorGNSS {
			return nil, fmt.Errorf("receiver supports maximum of %d major GNSS; could not determine which to enable", monGNSS.maxSimultaneousMajorGNSS)
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
func adjustTrackingChannels(gnss *ubxbin.CfgGNSS) {
	numTrkChUse := min(gnss.NumTrkChHw, gnss.NumTrkChUse)

	resTotal := 0
	for i := range gnss.Blocks {
		blk := &gnss.Blocks[i]
		if blk.GNSSID == ubxbin.IMES {
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
		if blk.Enable&0x1 == 0 || blk.GNSSID == ubxbin.IMES {
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

func (raw *CfgOld) changeRate(cp *gpsprot.ConfigProps) *ubxbin.CfgRate {
	if raw.rate == nil {
		return nil
	}
	rate := *raw.rate
	if raw.anyMsgEnabled() {
		rate.MeasRate = 1000 // 1 second
		rate.NavRate = 1
	}
	if gnss, exists := cp.GetTimeGNSS(); exists {
		switch gnss {
		case gpsprot.GPS:
			rate.TimeRef = ubxbin.CfgRateGPS
		case gpsprot.GLO:
			rate.TimeRef = ubxbin.CfgRateGLONASS
		case gpsprot.BDS:
			rate.TimeRef = ubxbin.CfgRateBeiDou
		case gpsprot.GAL:
			rate.TimeRef = ubxbin.CfgRateGalileo
		}
	}
	if rate == *raw.rate {
		return nil
	}
	return &rate
}

func (raw *CfgOld) cookNav5(cp *gpsprot.ConfigProps, ver *Version) {
	nav5 := raw.nav5
	if nav5 == nil {
		return
	}
	if ver.tmodeLevel() == 0 {
		cp.SetMode(gpsprot.Mode{Static: nav5.DynModel == ubxbin.CfgNav5DynStationary})
	}
	if _, exist := cp.GetTimeGNSS(); !exist {
		gnss := nav5GNSS(nav5)
		if gnss != 0 {
			cp.SetTimeGNSS(gnss)
		}
	}
	cp.SetMinElevation(gpsprot.Angle(nav5.MinElev) * gpsprot.Degrees)
}

func nav5GNSS(nav5 *ubxbin.CfgNav5) gpsprot.GNSS {
	if nav5 == nil {
		return 0
	}
	switch nav5.UtcStandard {
	case ubxbin.CfgNav5UtcUSNO:
		return gpsprot.GPS
	case ubxbin.CfgNav5UtcSU:
		return gpsprot.GLO
	case ubxbin.CfgNav5UtcNTSC:
		return gpsprot.BDS
	case ubxbin.CfgNav5UtcEU:
		return gpsprot.GAL
	}
	return 0
}

func (raw *CfgOld) changeNav5(target *gpsprot.ConfigTarget, ver *Version) *ubxbin.CfgNav5 {
	if raw.nav5 == nil {
		return nil
	}

	nav5 := *raw.nav5
	// Note we cannot use our normal technique comparing new and old nav5 because of the Mask field.
	// Instead we set the Mask only if something has changed.
	nav5.Mask = 0
	if static := dynModelStatic(target); static != nil && ver.tmodeLevel() == 0 {
		if *static {
			nav5.DynModel = ubxbin.CfgNav5DynStationary
		} else if nav5.DynModel == ubxbin.CfgNav5DynStationary {
			nav5.DynModel = ubxbin.CfgNav5DynPortable
		}
		if nav5.DynModel != raw.nav5.DynModel {
			nav5.Mask |= ubxbin.CfgNav5MaskDyn
		}
	}

	if gnss, exists := target.Props.GetTimeGNSS(); exists {
		switch gnss {
		case gpsprot.GPS:
			nav5.UtcStandard = ubxbin.CfgNav5UtcUSNO
		case gpsprot.GLO:
			nav5.UtcStandard = ubxbin.CfgNav5UtcSU
		case gpsprot.BDS:
			nav5.UtcStandard = ubxbin.CfgNav5UtcNTSC
		case gpsprot.GAL:
			nav5.UtcStandard = ubxbin.CfgNav5UtcEU
		}
		if nav5.UtcStandard != raw.nav5.UtcStandard {
			nav5.Mask |= ubxbin.CfgNav5MaskUtc
		}
	}
	if v, ok := target.Props.GetMinElevation(); ok {
		if deg, ok := angleToInt8Degrees(v); ok {
			nav5.MinElev = deg
			if nav5.MinElev != raw.nav5.MinElev {
				nav5.Mask |= ubxbin.CfgNav5MaskMinElev
			}
		}
	}
	if nav5.Mask == 0 {
		return nil
	}
	return &nav5
}

func (raw *CfgOld) addMsgRate(msgID ubxbin.MsgID, rate [6]byte) {
	if raw.msgRate == nil {
		raw.msgRate = make(map[ubxbin.MsgID][6]byte)
	}
	raw.msgRate[msgID] = rate
}

func idToGNSS(g ubxbin.GNSSID) gpsprot.GNSS {
	switch g {
	case ubxbin.GPS:
		return gpsprot.GPS
	case ubxbin.GLO:
		return gpsprot.GLO
	case ubxbin.BDS:
		return gpsprot.BDS
	case ubxbin.GAL:
		return gpsprot.GAL
	case ubxbin.NavIC:
		return gpsprot.NAVIC
	case ubxbin.QZSS:
		return gpsprot.QZSS
	case ubxbin.SBAS:
		return gpsprot.SBAS
	}
	return 0
}

func monGNSSSet(mon ubxbin.MonGnssMajorGnss) gpsprot.GNSSSet {
	g := gpsprot.GNSSSet(0)
	if mon&ubxbin.MonGnssGPS != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.GPS)
	}
	if mon&ubxbin.MonGnssGlonass != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.GLO)
	}
	if mon&ubxbin.MonGnssBeidou != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.BDS)
	}
	if mon&ubxbin.MonGnssGalileo != 0 {
		g |= gpsprot.GNSSSetOf(gpsprot.GAL)
	}
	return g
}
