package ubx

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

type Configurator struct {
	ver       *Version // never nil
	acks      ackList
	tRead     map[bin.MsgID]time.Time
	raw       RawConfig
	origPrt   *bin.CfgPrt
	steps     []func(*Configurator) (gpsprot.ConfigRequest, error)
	stepIndex int                   // -1 says to perform recovery
	target    *gpsprot.ConfigTarget // never nil
	survey    bool                  // start a survey
}

type ackList []*Ack

type Ack struct {
	msgID bin.MsgID
	gpsprot.Ack
}

type RawConfig struct {
	CfgOld
	CfgVals // access with valsPtr() so it gets lazily initialized
}

var legacyConfigSteps = []func(*Configurator) (gpsprot.ConfigRequest, error){
	(*Configurator).pollPrt,
	(*Configurator).setPrt,            // do this ASAP, because responses can be slow (at least on 8th gen) when NMEA is enabled
	(*Configurator).pollGNSS,          // need this to know which will be primary GNSS, which we need for enabling the time message
	(*Configurator).enableTimeGNSSMsg, // do this early to minimize likelihood of leaving GPS in unuseable state with no time messages being output
	(*Configurator).pollTp5,
	(*Configurator).setTp5,
	(*Configurator).enableTpMsg,
	(*Configurator).enableLeapSecondMsg,
	(*Configurator).pollTmode,
	(*Configurator).setTmode,
	(*Configurator).reqSurvey,
	(*Configurator).enableSurveyMsg,
	(*Configurator).pollRate,
	(*Configurator).setRate,
	(*Configurator).pollNav5,
	(*Configurator).setNav5,
	(*Configurator).reset,
}

var newConfigSteps = []func(*Configurator) (gpsprot.ConfigRequest, error){
	(*Configurator).pollPrt,
	(*Configurator).valGet,
	(*Configurator).valSet,
	(*Configurator).valSurvey,
	(*Configurator).reset,
}

var _ gpsprot.Configurator = (*Configurator)(nil)

func newConfigurator(target *gpsprot.ConfigTarget, ver *Version) *Configurator {
	steps := legacyConfigSteps
	if ver.protVerGreater(23, 1) {
		steps = newConfigSteps
	}
	return &Configurator{
		ver:    ver,
		target: target,
		steps:  steps,
		tRead:  make(map[bin.MsgID]time.Time),
	}
}

func (c *Configurator) ConfigMap() *gpsprot.ConfigMap {
	return c.raw.Config(c.ver)
}

func (c *Configurator) NextRequest() (gpsprot.ConfigRequest, error) {
	if c.stepIndex < 0 {
		c.stepIndex = len(c.steps)
		return c.recover()
	}
	for c.stepIndex < len(c.steps) {
		req, err := c.steps[c.stepIndex](c)
		c.stepIndex++
		if req != nil || err != nil {
			if err != nil {
				c.stepIndex = -1
			}
			return req, err
		}
	}
	return nil, nil
}

func (c *Configurator) FindAck(packet []byte, tSent time.Time) *gpsprot.Ack {
	a := c.acks.findAckByMsgId(bin.PacketMsgId(packet), tSent)
	if a == nil {
		return nil
	}
	if !a.OK {
		c.stepIndex = -1
	}
	return &a.Ack
}

func (c *Configurator) recover() (gpsprot.ConfigRequest, error) {
	// only need to do recovery for legacy configuration
	if &c.steps[0] != &legacyConfigSteps[0] {
		return nil, nil
	}
	// if we didn't cause NMEA output to be disabled, then we don't need to perform recovery
	if !c.raw.prtNMEAOutDisabled(c.origPrt) {
		return nil, nil
	}
	// if we got far enough to enable the time GNSS message, then do not need to switch back to NMEA
	if c.timeGNSSMsgEnabled() {
		return nil, nil
	}
	// we disabled NMEA output, but didn't get far enough to enable the time GNSS message
	// we had better reenable NMEA output, or the GPS will be silent
	return c.msgSetRequest(c.origPrt)
}

func (c *Configurator) processMsg(msg bin.Msg, t time.Time) (bool, error) {
	switch mt := msg.(type) {
	case *bin.AckAck:
		c.acks.ack(mt.MsgID, true, t)
		return true, nil
	case *bin.AckNak:
		c.acks.ack(mt.MsgID, false, t)
		return true, nil
	}
	mid := msg.ID()
	if mid.CfgClass() {
		c.tRead[mid] = t
		_, err := c.raw.AddMsg(msg)
		return true, err
	}
	return false, nil
}

func (c *Configurator) reset() (gpsprot.ConfigRequest, error) {
	if !c.target.Opts.Reset {
		return nil, nil
	}
	return msgRequest{&bin.CfgRst{
		NavBbrMask: bin.CfgRstNavBbrColdStart,
		ResetMode:  bin.CfgRstResetModeHardwareResetImmediately,
	}}, nil
}

func (c *Configurator) valGet() (gpsprot.ConfigRequest, error) {
	_, missing, _, err := c.raw.valsPtr().Transaction(c.target, c.ver, c.raw.valPort())
	if err != nil {
		return nil, err
	}
	if len(missing) == 0 {
		return nil, nil
	}
	layer := bin.CfgValgetLayerRAM
	if c.target.Opts.Flash {
		layer = bin.CfgValgetLayerFlash
	}
	return c.msgPollRequest(newCfgValgetRequest(missing, layer)), nil
}

func (c *Configurator) valSet() (gpsprot.ConfigRequest, error) {
	items, missing, survey, err := c.raw.valsPtr().Transaction(c.target, c.ver, c.raw.valPort())
	if err != nil {
		return nil, err
	}
	if len(missing) != 0 {
		return nil, fmt.Errorf("missing config items: %v", missing)
	}
	if len(items) == 0 {
		return nil, nil
	}
	layer := bin.CfgValsetLayerRAM
	if c.target.Opts.Flash {
		layer = bin.CfgValsetLayerFlash
	}
	val, err := newCfgValsetRequest(items, layer)
	if err != nil {
		return nil, err
	}
	c.survey = survey
	return c.msgSetRequest(val)
}

func (c *Configurator) valSurvey() (gpsprot.ConfigRequest, error) {
	if !c.survey {
		return nil, nil
	}
	items := c.raw.valsPtr().Survey(c.target.Opts)
	if len(items) == 0 {
		return nil, nil
	}
	val, err := newCfgValsetRequest(items, bin.CfgValsetLayerRAM)
	if err != nil {
		return nil, err
	}
	return c.msgSetRequest(val)
}

func (raw *RawConfig) valPort() ucv.Port {
	if raw.prt != nil {
		return ucv.Port(raw.prt.PortID)
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

func (c *Configurator) pollPrt() (gpsprot.ConfigRequest, error) {
	// This is used both by old and new.
	if !c.target.UsesAny(cfgOldKeys.prt...) && !c.target.Opts.EnableTimeMsg && !c.target.Opts.EnableLeapSecondMsg &&
		c.target.Opts.Survey.When == 0 {
		return nil, nil
	}
	return c.pollRequest(bin.CfgPrtID), nil
}

func (c *Configurator) pollGNSS() (gpsprot.ConfigRequest, error) {
	// UBX-CFG-GNSS needs at least protocol version 14.00
	if !c.target.UsesAny(cfgOldKeys.gnss...) || !c.ver.protVerAtLeast(14, 0) {
		return nil, nil
	}
	return c.pollRequest(bin.CfgGNSSID), nil
}

func (c *Configurator) pollRate() (gpsprot.ConfigRequest, error) {
	if !c.target.UsesAny(cfgOldKeys.rate...) {
		return nil, nil
	}
	return c.pollRequest(bin.CfgRateID), nil
}

func (c *Configurator) pollNav5() (gpsprot.ConfigRequest, error) {
	if !c.target.UsesAny(cfgOldKeys.nav5...) {
		return nil, nil
	}
	return c.pollRequest(bin.CfgNav5ID), nil
}

func (c *Configurator) pollTmode() (gpsprot.ConfigRequest, error) {
	if !c.target.UsesAny(cfgOldKeys.tmode...) && c.target.Opts.Survey.When == 0 {
		return nil, nil
	}
	switch c.ver.tmodeLevel() {
	case 1:
		return c.pollRequest(bin.CfgTmodeID), nil
	case 2:
		return c.pollRequest(bin.CfgTmode2ID), nil
	case 3:
		return c.pollRequest(bin.CfgTmode3ID), nil
	}
	return nil, nil
}

func (c *Configurator) pollTp5() (gpsprot.ConfigRequest, error) {
	if !c.target.UsesAny(cfgOldKeys.tp5...) {
		return nil, nil
	}
	tpIdx := 0
	if c.ver.ProductCategory() == "FTS" {
		tpIdx = 1
	}
	return c.pollTp5Request(tpIdx), nil
}

func (c *Configurator) enableTpMsg() (gpsprot.ConfigRequest, error) {
	if !c.target.Opts.EnableTimeMsg {
		return nil, nil
	}
	if c.ver.ProductCategory() == "FTS" {
		return nil, nil
	}
	if c.raw.tp5 == nil {
		return nil, nil
	}
	flags := c.raw.tp5.Flags
	if flags&bin.CfgTp5AlignToTow == 0 || flags&bin.CfgTp5LockGpsFreq == 0 || flags&bin.CfgTp5GridUTCGNSS == bin.CfgTp5GridUTC {
		return nil, nil
	}
	return c.enableMsgRequest(bin.TimTPID, true)
}

func (c *Configurator) enableTimeGNSSMsg() (gpsprot.ConfigRequest, error) {
	if !c.target.Opts.EnableTimeMsg {
		return nil, nil
	}
	if c.ver.ProductCategory() == "FTS" {
		return c.enableMsgRequest(bin.TimTosID, true)
	} else {
		return c.enableMsgRequest(bin.NavTimeGPSID, true)
	}
}

func (c *Configurator) timeGNSSMsgEnabled() bool {
	if c.ver.ProductCategory() == "FTS" {
		return c.raw.msgEnabled(bin.TimTosID)
	}
	return c.raw.msgEnabled(bin.NavTimeGPSID)
}

func (c *Configurator) enableLeapSecondMsg() (gpsprot.ConfigRequest, error) {
	if c.target.Opts.EnableLeapSecondMsg && c.ver.protVerAtLeast(18, 0) {
		return c.enableMsgRequest(bin.NavTimeLSID, true)
	}
	return nil, nil
}

func (c *Configurator) enableSurveyMsg() (gpsprot.ConfigRequest, error) {
	msgID := bin.TimSvinID
	surveyMode := false
	// note there is no survey progress message in early models that do not yet support tmode2
	if !c.ver.protVerAtLeast(18, 0) {
		return nil, nil
	}
	switch c.ver.tmodeLevel() {
	case 2:
		surveyMode = c.raw.tmode2 != nil && c.raw.tmode2.TimeMode == bin.CfgTmode2SurveyIn
	case 3:
		msgID = bin.NavSvinID
		surveyMode = c.raw.tmode3 != nil && c.raw.tmode3.Flags&bin.CfgTmode3Mode == bin.CfgTmode3SurveyIn
	default:
		return nil, nil
	}
	// XXX this is not right: we should not change anything unless Target sets time mode
	if surveyMode {
		return c.enableMsgRequest(msgID, true)
	}
	if _, exists := gpsprot.CfgTimeMode.Get(&c.target.Map); exists {
		return c.enableMsgRequest(msgID, false)
	}
	return nil, nil
}

func (c *Configurator) setPrt() (gpsprot.ConfigRequest, error) {
	prt := c.raw.changePrt(&c.target.Map)
	if prt == nil {
		return nil, nil
	}
	c.origPrt = new(bin.CfgPrt)
	*c.origPrt = *c.raw.prt
	return c.msgSetRequest(prt)
}

func (c *Configurator) setNav5() (gpsprot.ConfigRequest, error) {
	nav5 := c.raw.changeNav5(&c.target.Map)
	if nav5 == nil {
		return nil, nil
	}
	// XXX this isn't quite right, because of the mask
	return c.msgSetRequest(nav5)
}

func (c *Configurator) setRate() (gpsprot.ConfigRequest, error) {
	rate := c.raw.changeRate(&c.target.Map, c.ver)
	if rate == nil {
		return nil, nil
	}
	return c.msgSetRequest(rate)
}

func (c *Configurator) setTmode() (gpsprot.ConfigRequest, error) {
	switch c.ver.tmodeLevel() {
	case 1:
		var tm *bin.CfgTmode
		tm, c.survey = c.raw.changeTmode(c.target)
		if tm != nil {
			return c.msgSetRequest(tm)
		}
	case 2:
		var tm *bin.CfgTmode2
		tm, c.survey = c.raw.changeTmode2(c.target)
		if tm != nil {
			return c.msgSetRequest(tm)
		}
	case 3:
		var tm *bin.CfgTmode3
		tm, c.survey = c.raw.changeTmode3(c.target)
		if tm != nil {
			return c.msgSetRequest(tm)
		}
	}
	return nil, nil
}

func (c *Configurator) reqSurvey() (gpsprot.ConfigRequest, error) {
	if !c.survey {
		return nil, nil
	}
	switch c.ver.tmodeLevel() {
	case 1:
		tm := c.raw.surveyTmode(c.target.Opts)
		if tm != nil {
			return c.msgSetRequest(tm)
		}
	case 2:
		tm := c.raw.surveyTmode2(c.target.Opts)
		if tm != nil {
			return c.msgSetRequest(tm)
		}
	case 3:
		tm := c.raw.surveyTmode3(c.target.Opts)
		if tm != nil {
			return c.msgSetRequest(tm)
		}
	}
	return nil, nil
}

func (c *Configurator) setTp5() (gpsprot.ConfigRequest, error) {
	tp5 := c.raw.changeTp5(&c.target.Map)
	if tp5 == nil {
		return nil, nil
	}
	return c.msgSetRequest(tp5)
}

func (acks *ackList) ack(msgID bin.MsgID, ok bool, t time.Time) {
	*acks = append(*acks, &Ack{msgID, gpsprot.Ack{OK: ok, TRead: t}})
}

func (acks *ackList) findAckByMsgId(msgID bin.MsgID, tSent time.Time) (ack *Ack) {
	stale := 0
	for i, a := range *acks {
		if !a.TRead.After(tSent) {
			stale = i + 1
		} else if a.msgID == msgID {
			ack = a
			break
		}
	}
	if stale > 0 {
		*acks = (*acks)[stale:]
	}
	return
}

func (raw *RawConfig) Config(ver *Version) *gpsprot.ConfigMap {
	if raw == nil {
		return nil
	}
	cm := &gpsprot.ConfigMap{}
	raw.cookPrt(cm)
	if !raw.CfgVals.isNil() {
		raw.CfgVals.Cook(ver, raw.valPort(), cm)
	} else {
		if raw.tmode2 != nil {
			raw.cookTmode2(cm)
		} else if raw.tmode3 != nil {
			raw.cookTmode3(cm)
		} else if raw.tmode != nil {
			raw.cookTmode(cm)
		}
		raw.cookTp5(cm)
		raw.cookGNSS(cm)
		raw.cookRate(cm, ver)
		// must call cookNav5 after cookTp5, because we want to prefer primary GNSS from TP5
		raw.cookNav5(cm)
	}
	return cm
}

func (raw *RawConfig) AddMsg(m bin.Msg) (bool, error) {
	if raw == nil {
		return false, nil
	}
	switch mt := m.(type) {
	case *bin.CfgTmode:
		raw.tmode = mt
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
			err := raw.valsPtr().AddData(mt.CfgData)
			if err != nil {
				return false, err
			}
		}
	case *bin.CfgValset:
		// this is an acknowledgement of a set
		if mt.Layers&bin.CfgValsetLayerRAM != 0 {
			err := raw.valsPtr().AddData(mt.CfgData)
			if err != nil {
				return false, err
			}
		}
	default:
		return false, nil
	}
	return true, nil
}

type msgRequest struct {
	msg bin.Msg
}

var _ gpsprot.ConfigRequest = (*msgRequest)(nil)

func (r msgRequest) Packet() []byte {
	pkt, err := bin.Serialize(r.msg)
	if err != nil {
		panic(err)
	}
	return pkt
}

func (r msgRequest) ID() string { return r.msg.ID().String() }

func (r msgRequest) Ackable() bool { return r.msg.ID().Ackable() }

func (r msgRequest) AwaitingResponse(time.Time) bool { return false }

func (r msgRequest) Done() {}

func (c *Configurator) msgSetRequest(msg bin.Msg) (gpsprot.ConfigRequest, error) {
	return msgSetRequest{msgRequest{msg}, &c.raw}, nil
}

func (c *Configurator) msgPollRequest(msg bin.Msg) gpsprot.ConfigRequest {
	return msgPollRequest{msgRequest: msgRequest{msg}, tRead: c.tRead}
}

func (c *Configurator) pollRequest(mid bin.MsgID) gpsprot.ConfigRequest {
	return pollRequest{c.tRead, mid}
}

func (c *Configurator) pollTp5Request(tpIdx int) gpsprot.ConfigRequest {
	return pollTp5Request{
		pollRequest: pollRequest{c.tRead, bin.CfgTp5ID},
		tpIdx:       tpIdx,
	}
}

type msgPollRequest struct {
	msgRequest
	tRead map[bin.MsgID]time.Time
}

func (r msgPollRequest) AwaitingResponse(tSent time.Time) bool {
	return r.tRead[r.msg.ID()].Before(tSent)
}

var _ gpsprot.ConfigRequest = (*msgPollRequest)(nil)

type msgSetRequest struct {
	msgRequest
	raw *RawConfig
}

var _ gpsprot.ConfigRequest = (*msgSetRequest)(nil)

func (r msgSetRequest) Done() {
	_, err := r.raw.AddMsg(r.msg)
	if err != nil {
		panic(fmt.Sprintf("cannot parse acknowledge message %s: %v", r.msg.ID(), err))
	}
}

type pollRequest struct {
	tRead map[bin.MsgID]time.Time
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

func (r pollRequest) AwaitingResponse(tSent time.Time) bool {
	return r.tRead[r.msgID].Before(tSent)
}

type pollTp5Request struct {
	pollRequest
	tpIdx int
}

func (r pollTp5Request) Packet() []byte {
	return bin.PollCfgTp5(r.tpIdx)
}

func (c *Configurator) enableMsgRequest(msgID bin.MsgID, enabled bool) (gpsprot.ConfigRequest, error) {
	rate := byte(0)
	if enabled {
		rate = 1
	}
	return msgRateRequest{&c.raw, msgID, rate}, nil
}

type msgRateRequest struct {
	raw   *RawConfig
	msgID bin.MsgID
	rate  byte
}

func (r msgRateRequest) Packet() []byte {
	return bin.SetCfgMsg(r.msgID, 1)
}

func (r msgRateRequest) Done() {
	r.raw.SetMsgRate(r.msgID, r.rate)
}

func (r msgRateRequest) Ackable() bool { return true }

func (r msgRateRequest) AwaitingResponse(time.Time) bool { return false }

func (r msgRateRequest) ID() string { return bin.CfgMsgID.String() }

func lengthHP(l int32, h int8) gpsprot.Length {
	return gpsprot.Length(l)*gpsprot.Centimeter + gpsprot.Length(h)*(gpsprot.Millimeter/10)
}

// splitLength splits a Length into a int32 and int8.
// The int32 is the length in centimeters. The int8 is the remainder in units of 0.1mm.
func splitLength(n gpsprot.Length) (int32, int8, error) {
	q, r := divModRound(int64(n), int64(gpsprot.Centimeter))
	if q < math.MinInt32 || q > math.MaxInt32 {
		return 0, 0, fmt.Errorf("length %v is out of range", n)
	}
	cm := int32(q)
	q, _ = divModRound(r, int64(gpsprot.Millimeter/10))
	return cm, int8(q), nil
}

// divModRound returns the quotient and remainder of division of x by y, with the quotient rounded.
// If the result is (q, r), then x = q*y + r, and |r| <= y/2.
// y is assumed to be positive and even.
func divModRound(x, y int64) (int64, int64) {
	if y <= 0 || y%2 != 0 {
		panic("divisor y must be positive and even")
	}
	xRound := x
	if x >= 0 {
		xRound += y / 2
	} else {
		xRound -= y / 2
	}
	quotient := xRound / y
	return quotient, x - quotient*y
}
