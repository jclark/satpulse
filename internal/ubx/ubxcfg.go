package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
	"github.com/jclark/gps4ptp/internal/ubx/cfg"
)

type Configurator struct {
	ver    *Version // never nil
	raw    RawConfig
	step   int
	target *gpsmsg.Config // never nil
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
}

var configSteps = []func(*Configurator) gpsmsg.ConfigRequest{
	(*Configurator).pollPrt,
	(*Configurator).setPrtUBXOnly, // do this ASAP, because responses can be slow (at least on 8th gen) when NMEA is enabled
	(*Configurator).pollGNSS,      // need this to know which will be primary GNSS
	(*Configurator).enableTpMsg,   // do this soon, to avoid risk of GPS being completely silent
	(*Configurator).pollTp5,
	(*Configurator).setTp5,
	(*Configurator).pollTmode,
	(*Configurator).pollRate,
	(*Configurator).pollNav5,
	(*Configurator).pollSurvey,
	(*Configurator).pollLeapSecond,
	(*Configurator).enableTimeGNSSMsg,
}

var _ gpsmsg.Configurator = &Configurator{}

func (c *Configurator) Config() *gpsmsg.Config {
	return c.raw.Config(c.ver)
}

func (c *Configurator) NextRequest() gpsmsg.ConfigRequest {
	for c.step < len(configSteps) {
		req := configSteps[c.step](c)
		c.step++
		if req != nil {
			return req
		}
	}
	return nil
}

// XXX this is old code; needs to be generalized into support for new style UBX config
func (c *Configurator) TPTimegridGPS() []byte {
	cfgMap := map[string]map[string]any{
		"TP": {
			"TIMEGRID_TP1": "GPS",
		},
	}
	u := bin.CfgValset{
		CfgValsetFixed: bin.CfgValsetFixed{
			Layers: bin.CfgValsetLayerRAM,
		},
		CfgData: cfg.GetSchema().MustMarshal(cfgMap),
	}
	bytes, err := bin.Serialize(&u)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (c *Configurator) setPrtUBXOnly() gpsmsg.ConfigRequest {
	prtMsg := c.raw.ReqSetPrtUBXOnly()
	if prtMsg == nil {
		return nil
	}
	return msgRequest{&c.raw, prtMsg}
}

func (c *Configurator) pollPrt() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgPrtID}
}

func (c *Configurator) pollGNSS() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgGNSSID}
}

func (c *Configurator) pollRate() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgRateID}
}

func (c *Configurator) pollNav5() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgNav5ID}
}

func (c *Configurator) pollTmode() gpsmsg.ConfigRequest {
	switch c.productCategory() {
	case "FTS", "TIM":
		return pollRequest{bin.CfgTmode2ID}
	case "HPG":
		return pollRequest{bin.CfgTmode3ID}
	}
	return nil
}

func (c *Configurator) pollTp5() gpsmsg.ConfigRequest {
	tpIdx := 0
	if c.productCategory() == "FTS" {
		tpIdx = 1
	}
	return pollTp5Request{
		pollRequest: pollRequest{bin.CfgTp5ID},
		tpIdx:       tpIdx,
	}
}

func (c *Configurator) enableTpMsg() gpsmsg.ConfigRequest {
	if c.productCategory() == "FTS" {
		return c.enableMsgRequest(bin.TimTosID)
	} else {
		return c.enableMsgRequest(bin.TimTPID)
	}
}

func (c *Configurator) enableTimeGNSSMsg() gpsmsg.ConfigRequest {
	if c.productCategory() == "FTS" {
		return nil
	}
	return c.enableMsgRequest(bin.NavTimeGPSID)
}

// XXX not clear what to do about waiting for response NAV-TIMELS response
// we don't have to wait for the response (unlike with CFG messages)
func (c *Configurator) pollLeapSecond() gpsmsg.ConfigRequest {
	return pollRequest{bin.NavTimeLSID}
}

// XXX same problem as with pollLeapSecond
func (c *Configurator) pollSurvey() gpsmsg.ConfigRequest {
	switch c.productCategory() {
	case "TIM", "FTS":
		return pollRequest{bin.TimSvinID}
	case "HPG":
		return pollRequest{bin.NavSvinID}
	}
	return nil
}

func (c *Configurator) setTp5() gpsmsg.ConfigRequest {
	tp5 := c.raw.changeTp5(c.target)
	if tp5 == nil {
		return nil
	}
	return msgRequest{&c.raw, tp5}
}

type msgRequest struct {
	raw *RawConfig
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

func (r msgRequest) Done() {
	r.raw.AddMsg(r.msg)
}

func (r msgRequest) Ackable() bool { return r.msg.ID().Ackable() }

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

func (c *Configurator) enableMsgRequest(msgID bin.MsgID) gpsmsg.ConfigRequest {
	return enableMsgRequest{&c.raw, msgID}
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

func (raw *RawConfig) Config(ver *Version) *gpsmsg.Config {
	if raw == nil {
		return nil
	}
	cfg := &gpsmsg.Config{}
	raw.cookTmode2(cfg)
	if raw.tmode2 == nil {
		raw.cookTmode3(cfg)
	}
	raw.cookTp5(cfg)
	raw.cookGNSS(cfg)
	raw.cookRate(cfg, ver)
	raw.cookNav5(cfg)
	return cfg
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

func (raw *RawConfig) cookTmode2(cfg *gpsmsg.Config) {
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

func (raw *RawConfig) cookTmode3(cfg *gpsmsg.Config) {
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

func (raw *RawConfig) cookTp5(cfg *gpsmsg.Config) {
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
		onlyWhenLocked = width == 0
		period, width = tpPeriodWidth(tp.FreqPeriodLock, tp.PulseLenRatioLock, flags)
	}
	gpsmsg.CfgTimePulsePeriod.Set(cfg, period)
	// report inactive pulse as pulse width 0
	if flags&bin.CfgTp5Active == 0 {
		width = 0
	}
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

func (raw *RawConfig) changeTp5(cfg *gpsmsg.Config) *bin.CfgTp5 {
	if raw.tp5 == nil {
		return nil
	}

	// Copy the current tp5
	tp := *raw.tp5

	// Handle CfgTimePulsePolarityRising
	rising, exists := gpsmsg.CfgTimePulsePolarityRising.Get(cfg)
	if exists {
		if rising {
			tp.Flags |= bin.CfgTp5Polarity
		} else {
			tp.Flags &^= bin.CfgTp5Polarity
		}
	}

	// Handle CfgTimePulseGNSS
	gnss, exists := gpsmsg.CfgTimePulseGNSS.Get(cfg)
	if exists {
		gnssFlags := bin.CfgTp5AlignToTow | bin.CfgTp5LockGpsFreq
		// XXX need to check whether the GNSS is enabled
		switch gnss {
		case gpsmsg.GPS:
			tp.Flags |= bin.CfgTp5GridGPS | gnssFlags
		case gpsmsg.GLONASS:
			tp.Flags |= bin.CfgTp5GridGLONASS | gnssFlags
		case gpsmsg.BeiDou:
			tp.Flags |= bin.CfgTp5GridBeiDou | gnssFlags
		case gpsmsg.Galileo:
			tp.Flags |= bin.CfgTp5GridGalileo | gnssFlags
		default:
			tp.Flags &^= gnssFlags
		}
	}

	// Handle CfgTimePulseOnlyWhenLocked
	// Also set up where to write the perioad and width if we change them
	lenRatioPtr := &tp.PulseLenRatio
	freqPeriodPtr := &tp.FreqPeriod
	onlyWhenLocked, exists := gpsmsg.CfgTimePulseOnlyWhenLocked.Get(cfg)
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
	period, periodExists := gpsmsg.CfgTimePulsePeriod.Get(cfg)
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
	if width, exists := gpsmsg.CfgTimePulseWidth.Get(cfg); exists {
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

func (raw *RawConfig) cookGNSS(cfg *gpsmsg.Config) {
	gnss := raw.gnss
	if gnss == nil {
		return
	}
	var enabled gpsmsg.MajorGNSSSet
	for _, blk := range gnss.Blocks {
		if blk.Enable != 0 {
			if g, ok := majorGNSS(blk.GNSSID); ok {
				enabled |= gpsmsg.MajorGNSSFlag(g)
			}
		}
	}
	gpsmsg.CfgEnabledGNSS.Set(cfg, enabled)
}

func (raw *RawConfig) cookRate(cfg *gpsmsg.Config, ver *Version) {
	rate := raw.rate
	if rate == nil {
		return
	}
	period := time.Duration(raw.rate.MeasRate) * time.Millisecond
	if ver.protVerAtLeast(18, 0) && period != 0 {
		period /= time.Duration(raw.rate.NavRate)
	}
	gpsmsg.CfgSolutionPeriod.Set(cfg, period)
}

func (raw *RawConfig) cookNav5(cfg *gpsmsg.Config) {
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

func (cfg *RawConfig) ReqSetPrtUBXOnly() *bin.CfgPrt {
	if cfg == nil || cfg.prt == nil {
		return nil
	}
	if cfg.prt.OutProtoMask == bin.CfgPrtProtoUBX {
		return nil
	}
	prt := *cfg.prt
	prt.OutProtoMask = bin.CfgPrtProtoUBX
	return &prt
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
