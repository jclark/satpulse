package ubx

import (
	"errors"
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
	monGNSS   *monGNSS
	steps     []func(*Configurator) error
	stepIndex int
	reqs      []gpsprot.ConfigRequest
	target    *gpsprot.ConfigTarget // never nil
	survey    bool                  // start a survey
}

// monGNSS records information from UBX-MON-GNSS
// We need to be a little bit careful here, and should not include anything that might be
// invalidated by a configuration change by by UBX-CFG-GNSS.
// We could add supported GNSS signals here.
type monGNSS struct {
	maxSimultaneousMajorGNSS int
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

var legacyConfigSteps = []func(*Configurator) error{
	(*Configurator).pollPrt,
	(*Configurator).setMsg1,  // do this ASAP, because responses can be slow (at least on 8th gen) when NMEA is enabled
	(*Configurator).pollGNSS, // need this to know which will be primary GNSS, which we need for enabling the time message
	(*Configurator).pollMonGNSS,
	(*Configurator).setGNSS, // do this early because we may need to know enabled GNSS to deduce primary GNSS
	(*Configurator).pollTp5,
	(*Configurator).setTp5,
	(*Configurator).pollTmode,
	(*Configurator).setTmode,
	(*Configurator).reqSurvey,
	(*Configurator).pollRate,
	(*Configurator).pollNav5,
	(*Configurator).setNav5,
	(*Configurator).enableSurveyMsg,
	(*Configurator).setMsg2,
	(*Configurator).setRate, // setRate must come after all the messages have been enabled, so it can tell if rate needs setting
	(*Configurator).saveMinimal,
	(*Configurator).reloadCfg,
	(*Configurator).setCfg,
	(*Configurator).reset,
}

var newConfigSteps = []func(*Configurator) error{
	(*Configurator).pollPrt,
	(*Configurator).valGetSignals,
	// XXX we have to do this early at the moment, because we may need to deduce what GNSS is primary
	(*Configurator).valSetSignals,
	(*Configurator).valGet,
	(*Configurator).valSet,
	(*Configurator).valSurvey,
	(*Configurator).valBaudRate,
	(*Configurator).reloadCfg,
	(*Configurator).setCfg,
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

func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	return c.raw.Config(c.ver)
}

func (c *Configurator) NextRequest() (gpsprot.ConfigRequest, error) {
	for {
		if len(c.reqs) > 0 {
			req := c.reqs[0]
			c.reqs = c.reqs[1:]
			return req, nil
		}
		if c.stepIndex >= len(c.steps) {
			break
		}
		err := c.steps[c.stepIndex](c)
		c.stepIndex++
		if err != nil {
			c.stop()
			return nil, err
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
		c.stop()
	}
	return &a.Ack
}

func (c *Configurator) Abort() {
	c.stepIndex = -1
}

func (c *Configurator) stop() {
	c.stepIndex = len(c.steps) // don't do any more steps
	c.reqs = nil

	// consider whether we need to perform recovery

	// only need to do recovery for legacy configuration
	if &c.steps[0] != &legacyConfigSteps[0] {
		return
	}
	// if we didn't cause NMEA output to be disabled, then we don't need to perform recovery
	if !c.raw.prtNMEAOutDisabled(c.origPrt) {
		return
	}
	// if we got far enough to enable a message, then do not need to switch back to NMEA
	if c.raw.anyMsgEnabled() {
		return
	}
	// we disabled NMEA output, but didn't get far enough to enable the time GNSS message
	// we had better reenable NMEA output, or the GPS will be silent
	_ = c.addMsgSetRequest(c.origPrt)
}

func (c *Configurator) addRequest(req gpsprot.ConfigRequest) error {
	c.reqs = append(c.reqs, req)
	return nil
}

func (c *Configurator) processMsg(msg bin.Msg, t time.Time) (bool, error) {
	switch mt := msg.(type) {
	case *bin.AckAck:
		c.acks.ack(mt.MsgID, true, t)
		return true, nil
	case *bin.AckNak:
		c.acks.ack(mt.MsgID, false, t)
		return true, nil
	case *bin.MonGnss:
		mg := monGNSS{maxSimultaneousMajorGNSS: int(mt.Simultaneous)}
		c.tRead[mt.ID()] = t
		c.monGNSS = &mg
		return true, nil
	}
	mid := msg.ID()
	if mid.CfgClass() {
		c.tRead[mid] = t
		_, err := c.raw.AddMsg(msg)
		return err == nil, err
	}
	return false, nil
}

func (c *Configurator) saveMinimal() error {
	// This is just for legacy configuration.
	// For new configuration, we use CFG-VALSET to save to the right layer.
	var saveMask bin.CfgCfgSectionMask
	if c.target.Opts.Save != gpsprot.SaveMinimal {
		return nil
	}
	// Port has bits for wther NMEA/RTCM output is enabled at all on the port.
	if c.target.Opts.BaudRate != 0 || c.target.Opts.NMEAMsg.IsSet() || c.target.Opts.RTCMMsg.IsSet() {
		saveMask |= bin.CfgCfgIOPort
	}
	if c.target.Opts.SetsMsgs() {
		saveMask |= bin.CfgCfgMsgConf
	}
	if c.target.UsesAny(gpsprot.PropIDSignalsEnabled) || c.target.UsesAny(cfgOldProps.tp5...) {
		saveMask |= bin.CfgCfgRXMConf
	}
	// If any messages are enabled, then the rate is set, which is part of the Nav configuration section.
	// XXX handle survey when we fix that up ip satpulsetool gps
	if c.target.UsesAny(cfgOldProps.nav5...) || c.target.Opts.EnablesMsgs() {
		saveMask |= bin.CfgCfgNavConf
	}
	if saveMask == 0 {
		return nil
	}
	return c.addRequest(msgRequest{c.newCfgCfgRequest(0, saveMask, 0, bin.CfgCfgDevFlash|bin.CfgCfgDevBBR)})
}

func (c *Configurator) setCfg() error {
	var saveMask, clearMask bin.CfgCfgSectionMask

	if c.target.Opts.Save == gpsprot.SaveAll {
		saveMask = bin.CfgCfgSectionMaskAll
	}
	if c.target.Opts.Reset == gpsprot.ResetFactory {
		clearMask = bin.CfgCfgSectionMaskAll
	}
	if clearMask == 0 && saveMask == 0 {
		return nil
	}
	return c.addRequest(msgRequest{c.newCfgCfgRequest(clearMask, saveMask, 0, bin.CfgCfgDevFlash|bin.CfgCfgDevBBR)})
}

func (c *Configurator) reloadCfg() error {
	if c.target.Opts.Reset != gpsprot.ResetReload {
		return nil
	}
	return c.addRequest(msgRequest{c.newCfgCfgRequest(0, 0, bin.CfgCfgSectionMaskAll, 0)})
}

func (*Configurator) newCfgCfgRequest(clearMask, saveMask, loadMask bin.CfgCfgSectionMask, deviceMask bin.CfgCfgDeviceMask) *bin.CfgCfg {
	return &bin.CfgCfg{
		CfgCfgFixed: bin.CfgCfgFixed{
			ClearMask: clearMask,
			SaveMask:  saveMask,
			LoadMask:  loadMask,
		},
		DeviceMask: []bin.CfgCfgDeviceMask{deviceMask},
	}
}

func (c *Configurator) reset() error {
	if c.target.Opts.Reset <= gpsprot.ResetReload {
		return nil
	}
	return c.addRequest(msgRequest{&bin.CfgRst{
		NavBbrMask: bin.CfgRstNavBbrColdStart,
		ResetMode:  bin.CfgRstResetModeHardwareResetImmediately,
	}})
}

func (c *Configurator) valGet() error {
	_, missing, _, err := c.raw.valsPtr().Transaction(c.target, c.ver, c.raw.valPort())
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return c.addMsgPollRequest(newCfgValgetRequest(missing, c.valGetLayer()))
}

func (c *Configurator) valGetSignals() error {
	if _, ok := c.target.Props.GetSignalsEnabled(); !ok && c.target.Get&gpsprot.PropIDSignalsEnabled == 0 {
		return nil
	}
	keys := []ucv.Key{ucv.KSignalGpsEna.Key().GroupWildcard()}
	return c.addMsgPollRequest(newCfgValgetRequest(keys, c.valGetLayer()))
}

func (c *Configurator) valSetSignals() error {
	targetEnabled, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return nil
	}
	enabled, items := c.raw.valsPtr().EnableSignals(targetEnabled)
	// Ensure we have one non-augmentation signal from a major GNSS
	enabled &= gpsprot.SigSetMajor
	enabled &^= gpsprot.SigSetAugment
	if enabled == 0 {
		if c.raw.valsPtr().signalsSupported() == 0 {
			return errors.New("could not determine supported GNSS signals")
		}
		return fmt.Errorf("no suitable supported GNSS signal was enabled: %v", enabled)
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return err
	}
	// XXX we need to pause 0.5s after sending this
	return c.addMsgSetRequest(val)
}

func (c *Configurator) valSet() error {
	items, missing, survey, err := c.raw.valsPtr().Transaction(c.target, c.ver, c.raw.valPort())
	if err != nil {
		return err
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing config items: %v", missing)
	}
	if len(items) == 0 {
		return nil
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return err
	}
	c.survey = survey
	return c.addMsgSetRequest(val)
}

func (c *Configurator) valGetLayer() bin.CfgValgetLayer {
	return bin.CfgValgetLayerRAM
}

func (c *Configurator) valSetLayer() bin.CfgValsetLayer {
	if c.target.Opts.Save == gpsprot.SaveMinimal {
		// SaveMinimal is implemented by writing to non-volatile layers.
		// SaveAll is implemented using UBX-CFG-CFG to save everything
		return bin.CfgValsetLayerFlash | bin.CfgValsetLayerBBR | bin.CfgValsetLayerRAM
	}
	return bin.CfgValsetLayerRAM
}

func (c *Configurator) valSurvey() error {
	if !c.survey {
		return nil
	}
	items := c.raw.valsPtr().Survey(c.target.Opts)
	if len(items) == 0 {
		return nil
	}
	// XXX how does this work with the Flash option?
	// XXX this is disabled for now, because Transaction doesn't set c.survey
	val, err := newCfgValsetRequest(items, bin.CfgValsetLayerRAM)
	if err != nil {
		return err
	}
	return c.addMsgSetRequest(val)
}

func (c *Configurator) valBaudRate() error {
	items := c.raw.valsPtr().BaudRate(c.target, c.raw.valPort())
	if len(items) == 0 {
		return nil
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return nil
	}
	return c.addMsgSetSpeedRequest(val, int(items[0].Value))
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

func (c *Configurator) pollPrt() error {
	// This is used both by old and new.
	if c.target.Opts.BaudRate == 0 && !c.target.Opts.SetsMsgs() && c.target.Opts.Survey.When == 0 {
		return nil
	}
	return c.addPollRequest(bin.CfgPrtID)
}

func (c *Configurator) pollGNSS() error {
	// UBX-CFG-GNSS needs at least protocol version 14.00
	if !c.target.UsesAny(cfgOldProps.gnss...) || !c.ver.protVerAtLeast(14, 0) {
		return nil
	}
	return c.addPollRequest(bin.CfgGNSSID)
}

func (c *Configurator) pollMonGNSS() error {
	if _, ok := c.target.Props.GetSignalsEnabled(); !ok {
		return nil
	}
	// UBX-MON-GNSS needs at least protocol version 15.00
	if !c.ver.protVerAtLeast(15, 0) {
		return nil
	}
	return c.addPollRequest(bin.MonGnssID)
}

func (c *Configurator) pollRate() error {
	// XXX also handle survey message
	if _, ok := c.target.Props.GetPrimaryGNSS(); !ok && !c.target.Opts.EnablesMsgs() {
		return nil
	}
	return c.addPollRequest(bin.CfgRateID)
}

func (c *Configurator) pollNav5() error {
	if !c.target.UsesAny(cfgOldProps.nav5...) {
		return nil
	}
	return c.addPollRequest(bin.CfgNav5ID)
}

func (c *Configurator) pollTmode() error {
	if !c.target.UsesAny(cfgOldProps.tmode...) && c.target.Opts.Survey.When == 0 {
		return nil
	}
	switch c.ver.tmodeLevel() {
	case 1:
		return c.addPollRequest(bin.CfgTmodeID)
	case 2:
		return c.addPollRequest(bin.CfgTmode2ID)
	case 3:
		return c.addPollRequest(bin.CfgTmode3ID)
	}
	return nil
}

func (c *Configurator) pollTp5() error {
	if !c.target.UsesAny(cfgOldProps.tp5...) {
		return nil
	}
	tpIdx := 0
	if c.ver.ProductCategory() == "FTS" {
		tpIdx = 1
	}
	return c.addPollTp5Request(tpIdx)
}

func (c *Configurator) enableSurveyMsg() error {
	msgID := bin.TimSvinID
	surveyMode := false
	// note there is no survey progress message in early models that do not yet support tmode2
	if !c.ver.protVerAtLeast(18, 0) {
		return nil
	}
	switch c.ver.tmodeLevel() {
	case 2:
		surveyMode = c.raw.tmode2 != nil && c.raw.tmode2.TimeMode == bin.CfgTmode2SurveyIn
	case 3:
		msgID = bin.NavSvinID
		surveyMode = c.raw.tmode3 != nil && c.raw.tmode3.Flags&bin.CfgTmode3Mode == bin.CfgTmode3SurveyIn
	default:
		return nil
	}
	// XXX this is not right: we should not change anything unless Target sets time mode
	if surveyMode {
		return c.addMsgRateRequest(msgID, 1)
	}
	if _, exists := c.target.Props.GetTimeMode(); exists {
		return c.addMsgRateRequest(msgID, 0)
	}
	return nil
}

func (c *Configurator) setMsg1() error {
	c.origPrt = new(bin.CfgPrt)
	*c.origPrt = *c.raw.prt
	mc := newMsgChanges()
	mc.options1(&c.target.Opts, c.ver)
	c.setMsgChanges(mc)
	return nil
}

func (c *Configurator) setMsg2() error {
	mc := newMsgChanges()
	// XXX need to use enabled GNSS not supported GNSS from c.ver
	mc.options2(&c.target.Opts, c.ver, c.ver.GNSS)
	// this will end up doing about CFG-PRT in the event that RTCM protocol is enable/disabled
	c.setMsgChanges(mc)
	return nil
}

func (c *Configurator) setMsgChanges(mc *msgChanges) {
	prt := c.raw.changePrt(&c.target.Opts, mc)
	if prt != nil {
		c.addMsgSetRequest(prt)
	}
	for msgID, rate := range mc.rates() {
		c.addMsgRateRequest(msgID, rate)
	}
}

func (c *Configurator) setNav5() error {
	nav5 := c.raw.changeNav5(&c.target.Props)
	if nav5 == nil {
		return nil
	}
	// XXX this isn't quite right, because of the mask
	return c.addMsgSetRequest(nav5)
}

func (c *Configurator) setRate() error {
	rate := c.raw.changeRate(&c.target.Props)
	if rate == nil {
		return nil
	}
	return c.addMsgSetRequest(rate)
}

func (c *Configurator) setTmode() error {
	switch c.ver.tmodeLevel() {
	case 1:
		var tm *bin.CfgTmode
		tm, c.survey = c.raw.changeTmode(c.target)
		if tm != nil {
			return c.addMsgSetRequest(tm)
		}
	case 2:
		var tm *bin.CfgTmode2
		tm, c.survey = c.raw.changeTmode2(c.target)
		if tm != nil {
			return c.addMsgSetRequest(tm)
		}
	case 3:
		var tm *bin.CfgTmode3
		tm, c.survey = c.raw.changeTmode3(c.target)
		if tm != nil {
			return c.addMsgSetRequest(tm)
		}
	}
	return nil
}

func (c *Configurator) reqSurvey() error {
	if !c.survey {
		return nil
	}
	switch c.ver.tmodeLevel() {
	case 1:
		tm := c.raw.surveyTmode(c.target.Opts)
		if tm != nil {
			return c.addMsgSetRequest(tm)
		}
	case 2:
		tm := c.raw.surveyTmode2(c.target.Opts)
		if tm != nil {
			return c.addMsgSetRequest(tm)
		}
	case 3:
		tm := c.raw.surveyTmode3(c.target.Opts)
		if tm != nil {
			return c.addMsgSetRequest(tm)
		}
	}
	return nil
}

func (c *Configurator) setTp5() error {
	tp5 := c.raw.changeTp5(&c.target.Props)
	if tp5 == nil {
		return nil
	}
	return c.addMsgSetRequest(tp5)
}

func (c *Configurator) setGNSS() error {
	gnss, err := c.raw.changeGNSS(&c.target.Props, c.ver, c.monGNSS)
	if gnss == nil || err != nil {
		return err
	}
	return c.addMsgSetRequest(gnss)
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

func (raw *RawConfig) Config(ver *Version) *gpsprot.ConfigProps {
	if raw == nil {
		return nil
	}
	cm := &gpsprot.ConfigProps{}
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

func (r msgRequest) ChangeSpeed() int { return 0 }

func (r msgRequest) ID() string { return r.msg.ID().String() }

func (r msgRequest) Ackable() bool { return r.msg.ID().Ackable() }

func (r msgRequest) AwaitingResponse(time.Time) bool { return false }

func (r msgRequest) Done() {}

func (c *Configurator) addMsgSetRequest(msg bin.Msg) error {
	return c.addRequest(msgSetRequest{msgRequest{msg}, &c.raw})
}

func (c *Configurator) addMsgSetSpeedRequest(msg bin.Msg, speed int) error {
	return c.addRequest(msgSetSpeedRequest{msgSetRequest{msgRequest{msg}, &c.raw}, speed})
}

func (c *Configurator) addMsgPollRequest(msg bin.Msg) error {
	return c.addRequest(msgPollRequest{msgRequest: msgRequest{msg}, tRead: c.tRead})
}

func (c *Configurator) addPollRequest(mid bin.MsgID) error {
	return c.addRequest(pollRequest{c.tRead, mid})
}

func (c *Configurator) addPollTp5Request(tpIdx int) error {
	return c.addRequest(pollTp5Request{
		pollRequest: pollRequest{c.tRead, bin.CfgTp5ID},
		tpIdx:       tpIdx,
	})
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

type msgSetSpeedRequest struct {
	msgSetRequest
	speed int
}

func (r msgSetSpeedRequest) ChangeSpeed() int {
	return r.speed
}

var _ gpsprot.ConfigRequest = (*msgSetSpeedRequest)(nil)

type pollRequest struct {
	tRead map[bin.MsgID]time.Time
	msgID bin.MsgID
}

func (r pollRequest) Packet() []byte {
	return bin.Poll(r.msgID)
}

func (r pollRequest) ChangeSpeed() int { return 0 }

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

func (c *Configurator) addMsgRateRequest(msgID bin.MsgID, rate MsgRate) error {
	return c.addRequest(msgRateRequest{&c.raw, msgID, uint8(rate)})
}

type msgRateRequest struct {
	raw   *RawConfig
	msgID bin.MsgID
	rate  byte
}

func (r msgRateRequest) Packet() []byte {
	return bin.SetCfgMsg(r.msgID, 1)
}

func (r msgRateRequest) ChangeSpeed() int { return 0 }

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
