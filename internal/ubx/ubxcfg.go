package ubx

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/ubx/bin"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

type Configurator struct {
	ver       *Version // never nil
	raw       RawConfig
	steps     []func(*Configurator) (gpsmsg.ConfigRequest, error)
	stepIndex int
	target    *gpsmsg.ConfigMap // never nil
}

const nPort = 6

type RawConfig struct {
	tmode2  *bin.CfgTmode2
	tmode3  *bin.CfgTmode3
	tp5     *bin.CfgTp5
	gnss    *bin.CfgGNSS
	rate    *bin.CfgRate
	nav5    *bin.CfgNav5
	prt     *bin.CfgPrt
	msgRate map[bin.MsgID][nPort]byte
	val     ucv.Map
}

var normalConfigSteps = []func(*Configurator) (gpsmsg.ConfigRequest, error){
	(*Configurator).pollPrt,
	(*Configurator).setPrt,              // do this ASAP, because responses can be slow (at least on 8th gen) when NMEA is enabled
	(*Configurator).enableLeapSecondMsg, // do this soon, to avoid risk of GPS being completely silent
	(*Configurator).pollGNSS,            // need this to know which will be primary GNSS
	(*Configurator).enableTimeGNSSMsg,
	(*Configurator).pollTp5,
	(*Configurator).setTp5,
	(*Configurator).enableTpMsg,
	(*Configurator).pollTmode,
	(*Configurator).pollRate,
	(*Configurator).setRate,
	(*Configurator).pollNav5,
	(*Configurator).setNav5,
	(*Configurator).pollSurvey,
}

var newConfigSteps = []func(*Configurator) (gpsmsg.ConfigRequest, error){
	(*Configurator).pollPrt,
	(*Configurator).valGet,
	(*Configurator).valSet,
}

var _ gpsmsg.Configurator = &Configurator{}

func (c *Configurator) ConfigMap() *gpsmsg.ConfigMap {
	return c.raw.Config(c.ver)
}

func (c *Configurator) NextRequest() (gpsmsg.ConfigRequest, error) {
	for c.stepIndex < len(c.steps) {
		req, err := c.steps[c.stepIndex](c)
		c.stepIndex++
		if req != nil || err != nil {
			return req, err
		}
	}
	return nil, nil
}

func (c *Configurator) valGet() (gpsmsg.ConfigRequest, error) {
	_, missing, err := configItems(c.target, c.ver.GNSS, *c.raw.valMap(), c.valPort())
	if err != nil {
		return nil, err
	}
	if len(missing) == 0 {
		return nil, nil
	}
	return msgRequest{newCfgValgetRequest(missing, bin.CfgValgetLayerRAM)}, nil
}

func (c *Configurator) valSet() (gpsmsg.ConfigRequest, error) {
	items, missing, err := configItems(c.target, c.ver.GNSS, *c.raw.valMap(), c.valPort())
	if err != nil {
		return nil, err
	}
	if len(missing) != 0 {
		return nil, fmt.Errorf("missing config items: %v", missing)
	}
	val, err := newCfgValsetRequest(items, bin.CfgValsetLayerRAM)
	if err != nil {
		return nil, err
	}
	return c.msgSetRequest(val)
}

func (c *Configurator) valPort() ucv.Port {
	if c.raw.prt != nil {
		return ucv.Port(c.raw.prt.PortID)
	}
	// XXX what to do here
	return ucv.Port(ucv.UART1)
}

func newCfgValgetRequest(keys []ucv.Key, layer bin.CfgValgetLayer) *bin.CfgValget {
	return &bin.CfgValget{
		CfgValgetFixed: bin.CfgValgetFixed{
			Layer:   layer,
			Version: bin.CfgValgetVersionRequest,
		},
		CfgData: ucv.MarshalKeys(keys),
	}
}

func newCfgValsetRequest(items []ucv.Item, layers bin.CfgValsetLayer) (*bin.CfgValset, error) {
	cfgData, err := ucv.MarshalItems(items)
	if err != nil {
		return nil, err
	}
	return &bin.CfgValset{
		CfgValsetFixed: bin.CfgValsetFixed{
			Layers:  layers,
			Version: bin.CfgValsetVersionNoTransaction,
		},
		CfgData: cfgData,
	}, nil
}

func (c *Configurator) pollPrt() (gpsmsg.ConfigRequest, error) {
	return pollRequest{bin.CfgPrtID}, nil
}

func (c *Configurator) pollGNSS() (gpsmsg.ConfigRequest, error) {
	return pollRequest{bin.CfgGNSSID}, nil
}

func (c *Configurator) pollRate() (gpsmsg.ConfigRequest, error) {
	return pollRequest{bin.CfgRateID}, nil
}

func (c *Configurator) pollNav5() (gpsmsg.ConfigRequest, error) {
	return pollRequest{bin.CfgNav5ID}, nil
}

func (c *Configurator) pollTmode() (gpsmsg.ConfigRequest, error) {
	switch c.productCategory() {
	case "FTS", "TIM":
		return pollRequest{bin.CfgTmode2ID}, nil
	case "HPG":
		return pollRequest{bin.CfgTmode3ID}, nil
	}
	return nil, nil
}

func (c *Configurator) pollTp5() (gpsmsg.ConfigRequest, error) {
	tpIdx := 0
	if c.productCategory() == "FTS" {
		tpIdx = 1
	}
	return pollTp5Request{
		pollRequest: pollRequest{bin.CfgTp5ID},
		tpIdx:       tpIdx,
	}, nil
}

func (c *Configurator) enableTpMsg() (gpsmsg.ConfigRequest, error) {
	if c.productCategory() == "FTS" {
		return nil, nil
	}
	if c.raw.tp5 == nil {
		return nil, nil
	}
	flags := c.raw.tp5.Flags
	if flags&bin.CfgTp5AlignToTow == 0 || flags&bin.CfgTp5LockGpsFreq == 0 || flags&bin.CfgTp5GridUTCGNSS == bin.CfgTp5GridUTC {
		return nil, nil
	}
	return c.enableMsgRequest(bin.TimTPID)
}

func (c *Configurator) enableTimeGNSSMsg() (gpsmsg.ConfigRequest, error) {
	if c.productCategory() == "FTS" {
		return c.enableMsgRequest(bin.TimTosID)
	} else {
		return c.enableMsgRequest(bin.NavTimeGPSID)
	}
}

func (c *Configurator) enableLeapSecondMsg() (gpsmsg.ConfigRequest, error) {
	return c.enableMsgRequest(bin.NavTimeLSID)
}

// XXX not clear what to do about waiting for response for SVIN messages
// we don't have to wait for the response (unlike with CFG messages)
// should handle this like leap second messages
func (c *Configurator) pollSurvey() (gpsmsg.ConfigRequest, error) {
	switch c.productCategory() {
	case "TIM", "FTS":
		return pollRequest{bin.TimSvinID}, nil
	case "HPG":
		return pollRequest{bin.NavSvinID}, nil
	}
	return nil, nil
}

func (c *Configurator) setPrt() (gpsmsg.ConfigRequest, error) {
	prt := c.raw.changePrt(c.target)
	if prt == nil {
		return nil, nil
	}
	return c.msgSetRequest(prt)
}

func (c *Configurator) setNav5() (gpsmsg.ConfigRequest, error) {
	nav5 := c.raw.changeNav5(c.target)
	if nav5 == nil {
		return nil, nil
	}
	// XXX this isn't quite right, because of the mask
	return c.msgSetRequest(nav5)
}

func (c *Configurator) setRate() (gpsmsg.ConfigRequest, error) {
	rate := c.raw.changeRate(c.target, c.ver)
	if rate == nil {
		return nil, nil
	}
	return c.msgSetRequest(rate)
}

func (c *Configurator) setTp5() (gpsmsg.ConfigRequest, error) {
	tp5 := c.raw.changeTp5(c.target)
	if tp5 == nil {
		return nil, nil
	}
	return c.msgSetRequest(tp5)
}

type msgRequest struct {
	msg bin.Msg
}

func (r msgRequest) Packet() []byte {
	pkt, err := bin.Serialize(r.msg)
	if err != nil {
		panic(err)
	}
	return pkt
}

func (r msgRequest) ID() string { return r.msg.ID().String() }

func (r msgRequest) Ackable() bool { return r.msg.ID().Ackable() }

func (r msgRequest) Done() {}

func (c *Configurator) msgSetRequest(msg bin.Msg) (gpsmsg.ConfigRequest, error) {
	return msgSetRequest{msgRequest{msg}, &c.raw}, nil
}

type msgSetRequest struct {
	msgRequest
	raw *RawConfig
}

func (r msgSetRequest) Done() {
	_, err := r.raw.AddMsg(r.msg)
	if err != nil {
		panic(fmt.Sprintf("cannot parse acknowledge message %s: %v", r.msg.ID(), err))
	}
}

type pollRequest struct {
	msgID bin.MsgID
}

func (r pollRequest) Packet() []byte {
	return bin.Poll(r.msgID)
}

func (r pollRequest) ID() string { return r.msgID.String() }

func (r pollRequest) Done() {}

func (r pollRequest) Ackable() bool {
	return r.msgID.Ackable()
}

type pollTp5Request struct {
	pollRequest
	tpIdx int
}

func (r pollTp5Request) Packet() []byte {
	return bin.PollCfgTp5(r.tpIdx)
}

func (c *Configurator) enableMsgRequest(msgID bin.MsgID) (gpsmsg.ConfigRequest, error) {
	return enableMsgRequest{&c.raw, msgID}, nil
}

type enableMsgRequest struct {
	raw   *RawConfig
	msgID bin.MsgID
}

func (r enableMsgRequest) Packet() []byte {
	return bin.SetCfgMsg(r.msgID, 1)
}

func (r enableMsgRequest) Done() {
	r.raw.SetMsgRate(r.msgID, 1)
}

func (r enableMsgRequest) Ackable() bool { return true }

func (r enableMsgRequest) ID() string { return bin.CfgMsgID.String() }

func (c *Configurator) productCategory() string {
	if c.ver.FW != nil {
		return c.ver.FW.ProductCategory
	}
	return ""
}

func (raw *RawConfig) Config(ver *Version) *gpsmsg.ConfigMap {
	if raw == nil {
		return nil
	}
	cm := &gpsmsg.ConfigMap{}
	raw.cookPrt(cm)
	raw.cookTmode2(cm)
	if raw.tmode2 == nil {
		raw.cookTmode3(cm)
	}
	raw.cookTp5(cm)
	raw.cookGNSS(cm)
	raw.cookRate(cm, ver)
	// must call cookNav5 after cookTp5, because we want to prefer primary GNSS from TP5
	raw.cookNav5(cm)
	return cm
}

func (raw *RawConfig) SetMsgRate(msgID bin.MsgID, rate byte) {
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

func (raw *RawConfig) cookPrt(cm *gpsmsg.ConfigMap) {
	prt := raw.prt
	if prt == nil {
		return
	}
	gpsmsg.CfgNMEAEnabled.Set(cm, prt.OutProtoMask&bin.CfgPrtProtoNMEA != 0)
	if prt.PortID == bin.PortUART1 || prt.PortID == bin.PortUART2 {
		gpsmsg.CfgBaudRate.Set(cm, prt.BaudRate)
	}
}

func (raw *RawConfig) changePrt(cm *gpsmsg.ConfigMap) *bin.CfgPrt {
	if raw.prt == nil {
		return nil
	}

	prt := *raw.prt
	if nmeaEnabled, exists := gpsmsg.CfgNMEAEnabled.Get(cm); exists {
		if nmeaEnabled {
			prt.OutProtoMask |= bin.CfgPrtProtoNMEA
		} else {
			prt.OutProtoMask &^= bin.CfgPrtProtoNMEA
		}
	}
	if baudRate, exists := gpsmsg.CfgBaudRate.Get(cm); exists {
		if prt.PortID == bin.PortUART1 || prt.PortID == bin.PortUART2 {
			prt.BaudRate = baudRate
		}
	}
	if prt == *raw.prt {
		return nil
	}
	return &prt
}

func (raw *RawConfig) cookTmode2(cm *gpsmsg.ConfigMap) {
	tm := raw.tmode2
	if tm == nil {
		return
	}
	switch tm.TimeMode {
	case bin.CfgTmode2Disabled:
		gpsmsg.CfgTimeMode.Set(cm, gpsmsg.TimeModeDisabled)
	case bin.CfgTmode2SurveyIn:
		gpsmsg.CfgTimeMode.Set(cm, gpsmsg.TimeModeSurvey)
		gpsmsg.CfgSurveyMinDur.Set(cm, time.Duration(tm.SvinMinDur)*time.Second)
		gpsmsg.CfgSurveyAccLimit.Set(cm, gpsmsg.Length(tm.SvinAccLimit)*gpsmsg.Millimeter)
	case bin.CfgTmode2FixedMode:
		gpsmsg.CfgTimeMode.Set(cm, gpsmsg.TimeModeFixed)
		if tm.Flags&bin.CfgTmode2LLA == 0 {
			gpsmsg.CfgFixedPosECEF.Set(cm, gpsmsg.Point3D{
				X: gpsmsg.Length(tm.EcefXOrLat) * gpsmsg.Centimeter,
				Y: gpsmsg.Length(tm.EcefYOrLon) * gpsmsg.Centimeter,
				Z: gpsmsg.Length(tm.EcefZOrAlt) * gpsmsg.Centimeter,
			})
		}
		gpsmsg.CfgFixedPosAcc.Set(cm, gpsmsg.Length(tm.FixedPosAcc)*gpsmsg.Millimeter)
	}
}

func (raw *RawConfig) cookTmode3(cm *gpsmsg.ConfigMap) {
	tm := raw.tmode3
	if tm == nil {
		return
	}
	switch tm.Flags & bin.CfgTmode3Mode {
	case bin.CfgTmode3Disabled:
		gpsmsg.CfgTimeMode.Set(cm, gpsmsg.TimeModeDisabled)
	case bin.CfgTmode3SurveyIn:
		gpsmsg.CfgTimeMode.Set(cm, gpsmsg.TimeModeSurvey)
		gpsmsg.CfgSurveyMinDur.Set(cm, time.Duration(tm.SvinMinDur)*time.Second)
		gpsmsg.CfgSurveyAccLimit.Set(cm, gpsmsg.Length(tm.SvinAccLimit)*(gpsmsg.Millimeter/10))
	case bin.CfgTmode3FixedMode:
		gpsmsg.CfgTimeMode.Set(cm, gpsmsg.TimeModeFixed)
		if tm.Flags&bin.CfgTmode3LLA == 0 {
			gpsmsg.CfgFixedPosECEF.Set(cm, gpsmsg.Point3D{
				X: lengthHP(tm.EcefXOrLat, tm.EcefXOrLatHP),
				Y: lengthHP(tm.EcefYOrLon, tm.EcefYOrLonHP),
				Z: lengthHP(tm.EcefZOrAlt, tm.EcefZOrAltHP),
			})
		}
		gpsmsg.CfgFixedPosAcc.Set(cm, gpsmsg.Length(tm.FixedPosAcc)*(gpsmsg.Millimeter/10))
	}
}

func lengthHP(l int32, h int8) gpsmsg.Length {
	return gpsmsg.Length(l)*gpsmsg.Centimeter + gpsmsg.Length(h)*(gpsmsg.Millimeter/10)
}

func (raw *RawConfig) cookTp5(cm *gpsmsg.ConfigMap) {
	tp := raw.tp5
	if tp == nil {
		return
	}
	gpsmsg.CfgAntennaCableDelay.Set(cm, time.Duration(tp.AntCableDelay)*time.Nanosecond)
	gpsmsg.CfgTimePulsePolarityRising.Set(cm, tp.Flags&bin.CfgTp5Polarity != 0)
	flags := tp.Flags
	gnss := tp5FlagsGNSS(flags)
	gpsmsg.CfgTimePulseAlignToGNSS.Set(cm, gnss != 0)
	if gnss != 0 {
		gpsmsg.CfgPrimaryGNSS.Set(cm, gnss)
	}
	period, width := tpPeriodWidth(tp.FreqPeriod, tp.PulseLenRatio, flags)
	onlyWhenLocked := false
	if flags&bin.CfgTp5LockedOtherSet != 0 {
		onlyWhenLocked = width == 0
		period, width = tpPeriodWidth(tp.FreqPeriodLock, tp.PulseLenRatioLock, flags)
	}
	gpsmsg.CfgTimePulsePeriod.Set(cm, period)
	// report inactive pulse as pulse width 0
	if flags&bin.CfgTp5Active == 0 {
		width = 0
	}
	gpsmsg.CfgTimePulseWidth.Set(cm, width)
	gpsmsg.CfgTimePulseOnlyWhenLocked.Set(cm, onlyWhenLocked)
}

func tp5FlagsGNSS(flags bin.CfgTp5Flags) gpsmsg.GNSS {
	if flags&bin.CfgTp5AlignToTow == 0 || flags&bin.CfgTp5LockGpsFreq == 0 {
		return 0
	}
	grid := flags & bin.CfgTp5GridUTCGNSS
	switch grid {
	case bin.CfgTp5GridGPS:
		return gpsmsg.GPS
	case bin.CfgTp5GridGLONASS:
		return gpsmsg.GLO
	case bin.CfgTp5GridBeiDou:
		return gpsmsg.BDS
	case bin.CfgTp5GridGalileo:
		return gpsmsg.GAL
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

func (raw *RawConfig) changeTp5(cm *gpsmsg.ConfigMap) *bin.CfgTp5 {
	if raw.tp5 == nil {
		return nil
	}

	// Copy the current tp5
	tp := *raw.tp5

	// Handle CfgTimePulsePolarityRising
	rising, exists := gpsmsg.CfgTimePulsePolarityRising.Get(cm)
	if exists {
		if rising {
			tp.Flags |= bin.CfgTp5Polarity
		} else {
			tp.Flags &^= bin.CfgTp5Polarity
		}
	}

	// Handle CfgTimePulseAlignGNSS
	align, exists := gpsmsg.CfgTimePulseAlignToGNSS.Get(cm)
	if exists {
		gnssFlags := bin.CfgTp5AlignToTow | bin.CfgTp5LockGpsFreq
		if align {
			tp.Flags |= gnssFlags
			gnss := raw.changeTp5GNSS(cm)
			switch gnss {
			case gpsmsg.GPS:
				tp.Flags |= bin.CfgTp5GridGPS
			case gpsmsg.GLO:
				tp.Flags |= bin.CfgTp5GridGLONASS
			case gpsmsg.BDS:
				tp.Flags |= bin.CfgTp5GridBeiDou
			case gpsmsg.GAL:
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
	onlyWhenLocked, exists := gpsmsg.CfgTimePulseOnlyWhenLocked.Get(cm)
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
	period, periodExists := gpsmsg.CfgTimePulsePeriod.Get(cm)
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
	if width, exists := gpsmsg.CfgTimePulseWidth.Get(cm); exists {
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

func (raw *RawConfig) changeTp5GNSS(cm *gpsmsg.ConfigMap) gpsmsg.GNSS {
	g, _ := gpsmsg.CfgPrimaryGNSS.Get(cm)
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
	prefer := []gpsmsg.GNSS{gpsmsg.GPS, gpsmsg.GAL, gpsmsg.BDS, gpsmsg.GLO}

	for _, g := range prefer {
		if enabled.Contains(g) {
			return g
		}
	}
	return gpsmsg.GPS
}

func (raw *RawConfig) cookGNSS(cm *gpsmsg.ConfigMap) {
	gnss := raw.gnss
	if gnss == nil {
		return
	}
	gpsmsg.CfgGNSSEnabled.Set(cm, gnssEnabledSet(gnss))
}

func gnssEnabledSet(gnss *bin.CfgGNSS) gpsmsg.GNSSSet {
	if gnss == nil {
		return 0
	}
	var enabled gpsmsg.GNSSSet
	for _, blk := range gnss.Blocks {
		if blk.Enable != 0 {
			if g := idToGNSS(blk.GNSSID); g != 0 {
				enabled |= gpsmsg.GNSSFlag(g)
			}
		}
	}
	return enabled
}

func (raw *RawConfig) cookRate(cm *gpsmsg.ConfigMap, ver *Version) {
	rate := raw.rate
	if rate == nil {
		return
	}
	gpsmsg.CfgSolutionPeriod.Set(cm, rateSolutionPeriod(rate, ver))
}

func rateSolutionPeriod(rate *bin.CfgRate, ver *Version) time.Duration {
	period := time.Duration(rate.MeasRate) * time.Millisecond
	if ver.protVerAtLeast(18, 0) && rate.NavRate != 0 {
		period /= time.Duration(rate.NavRate)
	}
	return period
}

func (raw *RawConfig) changeRate(cm *gpsmsg.ConfigMap, ver *Version) *bin.CfgRate {
	if raw.rate == nil {
		return nil
	}
	rate := *raw.rate
	if period, exists := gpsmsg.CfgSolutionPeriod.Get(cm); exists {
		setSolutionPeriod(&rate, period, ver)
	}
	if gnss, exists := gpsmsg.CfgPrimaryGNSS.Get(cm); exists {
		switch gnss {
		case gpsmsg.GPS:
			rate.TimeRef = bin.CfgRateGPS
		case gpsmsg.GLO:
			rate.TimeRef = bin.CfgRateGLONASS
		case gpsmsg.BDS:
			rate.TimeRef = bin.CfgRateBeiDou
		case gpsmsg.GAL:
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

func (raw *RawConfig) cookNav5(cm *gpsmsg.ConfigMap) {
	nav5 := raw.nav5
	if nav5 == nil {
		return
	}
	stationary := false
	if nav5.DynModel == bin.CfgNav5DynStationary {
		stationary = true
	}
	if _, exist := gpsmsg.CfgPrimaryGNSS.Get(cm); !exist {
		gnss := nav5GNSS(nav5)
		if gnss != 0 {
			gpsmsg.CfgPrimaryGNSS.Set(cm, gnss)
		}
	}
	gpsmsg.CfgStationary.Set(cm, stationary)
}

func nav5GNSS(nav5 *bin.CfgNav5) gpsmsg.GNSS {
	if nav5 == nil {
		return 0
	}
	switch nav5.UtcStandard {
	case bin.CfgNav5UtcUSNO:
		return gpsmsg.GPS
	case bin.CfgNav5UtcSU:
		return gpsmsg.GLO
	case bin.CfgNav5UtcNTSC:
		return gpsmsg.BDS
	case bin.CfgNav5UtcEU:
		return gpsmsg.GAL
	}
	return 0
}

func (raw *RawConfig) changeNav5(cm *gpsmsg.ConfigMap) *bin.CfgNav5 {
	if raw.nav5 == nil {
		return nil
	}

	nav5 := *raw.nav5
	nav5.Mask = 0
	if stationary, exists := gpsmsg.CfgStationary.Get(cm); exists {
		if stationary {
			nav5.DynModel = bin.CfgNav5DynStationary
		} else if nav5.DynModel == bin.CfgNav5DynStationary {
			nav5.DynModel = bin.CfgNav5DynPortable
		}
		nav5.Mask |= bin.CfgNav5MaskDyn
	}

	if gnss, exists := gpsmsg.CfgPrimaryGNSS.Get(cm); exists {
		nav5.Mask |= bin.CfgNav5MaskUtc
		switch gnss {
		case gpsmsg.GPS:
			nav5.UtcStandard = bin.CfgNav5UtcUSNO
		case gpsmsg.GLO:
			nav5.UtcStandard = bin.CfgNav5UtcSU
		case gpsmsg.BDS:
			nav5.UtcStandard = bin.CfgNav5UtcNTSC
		case gpsmsg.GAL:
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

func (raw *RawConfig) AddMsg(m bin.Msg) (bool, error) {
	if raw == nil {
		return false, nil
	}
	switch mt := m.(type) {
	case *bin.CfgTmode2:
		raw.tmode2 = mt
	case *bin.CfgTmode3:
		raw.tmode3 = mt
	case *bin.CfgTp5:
		raw.tp5 = mt
	case *bin.CfgGNSS:
		raw.gnss = mt
	case *bin.CfgRate:
		raw.rate = mt
	case *bin.CfgNav5:
		raw.nav5 = mt
	case *bin.CfgPrt:
		raw.prt = mt
	case *bin.CfgMsg:
		raw.addMsgRate(mt.MsgID, mt.Rate)
	case *bin.CfgValget:
		// this is a response to a poll
		if mt.Layer == 0 {
			err := raw.addVal(mt.CfgData)
			if err != nil {
				return false, err
			}
		}
	case *bin.CfgValset:
		// this is an acknowledgement of a set
		if mt.Layers&bin.CfgValsetLayerRAM != 0 {
			err := raw.addVal(mt.CfgData)
			if err != nil {
				return false, err
			}
		}
	default:
		return false, nil
	}
	return true, nil
}

func (raw *RawConfig) addVal(cfgData []byte) error {
	items, err := ucv.UnmarshalItems(cfgData)
	if err != nil {
		return err
	}
	raw.valMap().AddItems(items)
	return nil
}

func (raw *RawConfig) valMap() *ucv.Map {
	if raw.val == nil {
		raw.val = make(ucv.Map)
	}
	return &raw.val
}

func (raw *RawConfig) addMsgRate(msgID bin.MsgID, rate [6]byte) {
	if raw.msgRate == nil {
		raw.msgRate = make(map[bin.MsgID][6]byte)
	}
	raw.msgRate[msgID] = rate
}

func idToGNSS(g bin.GNSSID) gpsmsg.GNSS {
	switch g {
	case bin.GPS:
		return gpsmsg.GPS
	case bin.GLO:
		return gpsmsg.GLO
	case bin.BDS:
		return gpsmsg.BDS
	case bin.GAL:
		return gpsmsg.GAL
	case bin.NavIC:
		return gpsmsg.NAVIC
	case bin.QZSS:
		return gpsmsg.QZSS
	case bin.SBAS:
		return gpsmsg.SBAS
	}
	return 0
}
