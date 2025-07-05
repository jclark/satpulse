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
// Since enabledGNSS can be invalidated by changed to the GNSS configuration,
// we store the gnssChangeCount at the time monGNSS was created.
// enabledGNSS is only valid if gnssChangeCount in monGNSS is the same as raw.gnssChangeCount.
type monGNSS struct {
	maxSimultaneousMajorGNSS int
	supportedGNSS            gpsprot.GNSSSet
	enabledGNSS              gpsprot.GNSSSet
	gnssChangeCount          int
}

type ackList []*Ack

type Ack struct {
	msgID bin.MsgID
	gpsprot.Ack
}

type RawConfig struct {
	CfgOld
	CfgVals             // access with valsPtr() so it gets lazily initialized
	gnssChangeCount int // incremented each time GNSS configuration changes
}

var legacyConfigSteps = []func(*Configurator) error{
	(*Configurator).pollPrt,
	(*Configurator).setMsg1,  // do this ASAP, because responses can be slow (at least on 8th gen) when NMEA is enabled
	(*Configurator).pollGNSS, // need this to know which will be time GNSS, which we need for enabling the time message
	(*Configurator).pollMonGNSS,
	(*Configurator).setGNSS, // do this early because we may need to know enabled GNSS to deduce time GNSS
	(*Configurator).pollTp5,
	(*Configurator).setTp5,
	(*Configurator).pollTmode,
	(*Configurator).setTmode,
	(*Configurator).pollRate,
	(*Configurator).pollNav5,
	(*Configurator).setNav5,
	(*Configurator).setMsg2,
	(*Configurator).setRate, // setRate must come after all the messages have been enabled, so it can tell if rate needs setting
	(*Configurator).setBaudRate,
	(*Configurator).saveMinimal,
	(*Configurator).reloadCfg,
	(*Configurator).setCfg,
	(*Configurator).reset,
}

var newConfigSteps = []func(*Configurator) error{
	(*Configurator).pollPrt,
	(*Configurator).valPollMonGNSS,
	(*Configurator).valGetSignals,
	// XXX we have to do this early at the moment, because we may need to deduce what GNSS is time GNSS
	(*Configurator).valSetSignals,
	(*Configurator).valGetNMA,
	(*Configurator).valSetNMA,
	(*Configurator).valGet,
	(*Configurator).valSet,
	(*Configurator).timeAssist,
	(*Configurator).osnmaAssist,
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
	c.stop()
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
		c.tRead[mt.ID()] = t
		c.monGNSS = c.newMonGNSS(mt)
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

func (c *Configurator) newMonGNSS(mt *bin.MonGnss) *monGNSS {
	mg := monGNSS{
		maxSimultaneousMajorGNSS: int(mt.Simultaneous),
		supportedGNSS:            monGNSSSet(mt.Supported),
		enabledGNSS:              monGNSSSet(mt.Enabled),
		gnssChangeCount:          c.raw.gnssChangeCount,
	}
	return &mg
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
	if c.target.Props.SetsAny(gpsprot.PropIDSignalsEnabled) || c.target.Props.SetsAny(cfgOldProps.tp5...) {
		saveMask |= bin.CfgCfgRXMConf
	}
	// If any messages are enabled, then the rate is set, which is part of the Nav configuration section.
	// XXX handle survey when we fix that up ip satpulsetool gps
	if c.target.Props.SetsAny(cfgOldProps.nav5...) || c.target.Opts.EnablesMsgs() {
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
	_, missing, err := c.raw.valsPtr().Transaction(c.target, c.ver, c.raw.valPort(), c.monEnabledGNSS())
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return c.addMsgPollRequest(newCfgValgetRequest(missing, c.valGetLayer()))
}

// valPollMonGNSS polls UBX-MON-GNSS for non-legacy configuration.
func (c *Configurator) valPollMonGNSS() error {
	// ZED-X20P is version 50 and its UBX-MON-GNSS is a different version,
	// which has a completely different structure, which we don't yet support.
	if c.ver.protVerAtLeast(50, 0) {
		return nil
	}
	if !c.valNeedsMonGNSS() {
		return nil
	}
	return c.addPollRequest(bin.MonGnssID)
}

func (c *Configurator) valNeedsMonGNSS() bool {
	// At the moment we use MON-GNSS only for enabledGNSS when enabling RTCM messages.
	// XXX in the future use for maxSimultaneousMajorGNSS (needed for F10T at least)
	// XXX in the future use also when inferring time GNSS
	if !c.target.Opts.RTCMMsg.IsSet() {
		return false
	}
	// If we are already getting signals, then we don't need MON-GNSS.
	if c.target.UsesAny(gpsprot.PropIDSignalsEnabled) {
		return false
	}
	return true
}

func (c *Configurator) valGetSignals() error {
	if !c.target.UsesAny(gpsprot.PropIDSignalsEnabled) {
		return nil
	}
	keys := []ucv.Key{
		ucv.KSignalGpsEna.Key().GroupWildcard(),
		ucv.KGpsL5HealthOverride.Key().GroupWildcard(),
	}
	return c.addMsgPollRequest(newCfgValgetRequest(keys, c.valGetLayer()))
}

func (c *Configurator) valSetSignals() error {
	targetEnabled, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return nil
	}
	enabled, items := c.raw.valsPtr().EnableSignals(targetEnabled, c.ver)
	// Ensure we have one non-augmentation signal from a major GNSS
	enabled &= gpsprot.SigSetMajor
	enabled &^= gpsprot.SigSetAugment
	if enabled == 0 {
		if c.raw.valsPtr().signalsSupported(c.ver) == 0 {
			return errors.New("could not determine supported GNSS signals")
		}
		return fmt.Errorf("no suitable supported GNSS signal was enabled: %v", enabled)
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return err
	}
	return c.addMsgSetPauseRequest(val, pauseAfterGNSSReset)
}

func (c *Configurator) valGetNMA() error {
	if !c.target.UsesAny(gpsprot.PropIDNavMsgAuth) {
		return nil
	}
	keys := []ucv.Key{
		ucv.KGalUseOsnma.Key().GroupWildcard(),
	}
	return c.addMsgPollRequest(newCfgValgetRequest(keys, c.valGetLayer()))
}

func (c *Configurator) valSetNMA() error {
	items := c.raw.valsPtr().NavMsgAuth(&c.target.Props)
	if len(items) == 0 {
		return nil
	}
	val, err := newCfgValsetRequest(items, c.valSetLayer())
	if err != nil {
		return nil
	}
	return c.addMsgSetRequest(val)
}

func (c *Configurator) valSet() error {
	items, missing, err := c.raw.valsPtr().Transaction(c.target, c.ver, c.raw.valPort(), c.monEnabledGNSS())
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

func (c *Configurator) monEnabledGNSS() gpsprot.GNSSSet {
	if c.monGNSS != nil && c.monGNSS.gnssChangeCount == c.raw.gnssChangeCount {
		return c.monGNSS.enabledGNSS
	}
	return 0
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
	if c.target.Opts.BaudRate == 0 && !c.target.Opts.SetsMsgs() {
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

// This is just for legacy configuration.
// It is called after polling GNSS (if any)
func (c *Configurator) pollMonGNSS() error {
	// UBX-MON-GNSS needs at least protocol version 15.00
	if !c.ver.protVerAtLeast(15, 0) {
		return nil
	}
	if !c.needsMonGNSS() {
		return nil
	}
	return c.addPollRequest(bin.MonGnssID)
}

func (c *Configurator) needsMonGNSS() bool {
	// Need maxSimultaneousMajorGNSS in this case.
	if _, ok := c.target.Props.GetSignalsEnabled(); ok {
		return true
	}
	// Need enabledGNSS for RTCM messages.
	// But only need it if we can't get it from raw.gnss.
	if c.target.Opts.RTCMMsg.IsSet() && c.raw.gnss == nil {
		return true
	}
	return false
}

func (c *Configurator) pollRate() error {
	// XXX also handle survey message
	if _, ok := c.target.Props.GetTimeGNSS(); !ok && !c.target.Opts.EnablesMsgs() {
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
	if !c.target.UsesAny(cfgOldProps.tmode...) && !c.target.Opts.SetStatic {
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

func (c *Configurator) setMsg1() error {
	if c.raw.prt != nil {
		// save the original port configuration
		// so we can restore during recovery
		c.origPrt = new(bin.CfgPrt)
		*c.origPrt = *c.raw.prt
	}
	mc := newMsgChanges()
	mc.options1(&c.target.Opts, c.ver)
	c.setMsgChanges(mc)
	return nil
}

func (c *Configurator) setMsg2() error {
	mc := newMsgChanges()
	enabledGNSS := c.monEnabledGNSS()
	if enabledGNSS == 0 {
		enabledGNSS = gnssEnabledSet(c.raw.gnss)
	}
	err := mc.options2(&c.target.Opts, c.ver, enabledGNSS, c.survey)
	if err != nil {
		return err
	}
	// this will end up doing about CFG-PRT in the event that RTCM protocol is enable/disabled
	c.setMsgChanges(mc)
	return nil
}

func (c *Configurator) setMsgChanges(mc *msgChanges) {
	prt := c.raw.changePrtProto(mc)
	if prt != nil {
		c.addMsgSetRequest(prt)
	}
	for msgID, rate := range mc.rates() {
		c.addMsgRateRequest(msgID, rate)
	}
}

func (c *Configurator) setBaudRate() error {
	prt := c.raw.changePrtBaudRate(&c.target.Opts)
	if prt == nil {
		return nil
	}
	return c.addMsgSetSpeedRequest(prt, int(c.target.Opts.BaudRate))
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
	msg1, msg2, err := c.raw.changeTmode(c.target)
	if err != nil {
		return err
	}
	if msg1 != nil {
		c.addMsgSetRequest(msg1)
	}
	if msg2 != nil {
		c.survey = true // this is needed for enabling survey messages
		c.addMsgSetRequest(msg2)
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
	return c.addMsgSetPauseRequest(gnss, pauseAfterGNSSReset)
}

func (c *Configurator) timeAssist() error {
	mga, err := mgaTime(&c.target.Opts.TimeAssist, time.Now())
	if err != nil {
		return err
	}
	if mga == nil {
		return nil
	}
	return c.addRequest(msgRequest{mga})
}

func (c *Configurator) osnmaAssist() error {
	mga := mgaOSNMAMerkle(c.target.Opts.OSNMA.MerkleTreeRoot, false)
	if mga == nil {
		return nil
	}
	return c.addRequest(msgRequest{mga})
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

		raw.cookTmode(cm)
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
		raw.gnssChangeCount++
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
		if mt.Layer == bin.CfgValgetLayerRAM {
			_, err := raw.valsPtr().AddData(mt.CfgData)
			if err != nil {
				return false, err
			}
		}
	case *bin.CfgValset:
		// this is an acknowledgement of a set
		if mt.Layers&bin.CfgValsetLayerRAM != 0 {
			groups, err := raw.valsPtr().AddData(mt.CfgData)
			if err != nil {
				return false, err
			}
			if _, found := groups[ucv.KSignalGpsEna.Key().Group()]; found {
				raw.gnssChangeCount++
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

func (r msgRequest) Pause() time.Duration { return 0 }

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

func (c *Configurator) addMsgSetPauseRequest(msg bin.Msg, pause time.Duration) error {
	return c.addRequest(msgSetPauseRequest{msgSetRequest{msgRequest{msg}, &c.raw}, pause})
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

const pauseAfterGNSSReset = time.Second / 2

type msgSetPauseRequest struct {
	msgSetRequest
	pause time.Duration
}

func (r msgSetPauseRequest) Pause() time.Duration {
	return r.pause
}

var _ gpsprot.ConfigRequest = (*msgSetPauseRequest)(nil)

type pollRequest struct {
	tRead map[bin.MsgID]time.Time
	msgID bin.MsgID
}

func (r pollRequest) Packet() []byte {
	return bin.Poll(r.msgID)
}

func (r pollRequest) ChangeSpeed() int { return 0 }

func (r pollRequest) Pause() time.Duration { return 0 }

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
	return bin.SetCfgMsg(r.msgID, r.rate)
}

func (r msgRateRequest) ChangeSpeed() int { return 0 }

func (r msgRateRequest) Pause() time.Duration { return 0 }

func (r msgRateRequest) Done() {
	r.raw.SetMsgRate(r.msgID, r.rate)
}

func (r msgRateRequest) Ackable() bool { return true }

func (r msgRateRequest) AwaitingResponse(time.Time) bool { return false }

func (r msgRateRequest) ID() string { return bin.CfgMsgID.String() }

func lengthHP(l int32, h int8) gpsprot.Length {
	return gpsprot.Length(l)*gpsprot.Centimeter + gpsprot.Length(h)*(gpsprot.Millimeter/10)
}

func angleHP(deg int32, hp int8) gpsprot.Angle {
	return gpsprot.Angle(deg)*(gpsprot.Nanodegrees*100) + gpsprot.Angle(hp)*gpsprot.Nanodegrees
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

// splitAngle splits an Angle into int32 and int8.
// The int32 is the angle in degrees * 1e-7. The int8 is the remainder in degrees * 1e-9.
func splitAngle(a gpsprot.Angle) (int32, int8, error) {
	q, r := divModRound(int64(a), int64(gpsprot.Nanodegrees*100))
	if q < math.MinInt32 || q > math.MaxInt32 {
		return 0, 0, fmt.Errorf("angle %v is out of range", a)
	}
	deg := int32(q)
	q, _ = divModRound(r, int64(gpsprot.Nanodegrees))
	return deg, int8(q), nil
}

// divModRound returns the quotient and remainder of division of x by y, with the quotient rounded.
// If the result is (q, r), then x = q*y + r, and |r| <= y/2.
// y must be positive and either 1 or even.
func divModRound(x, y int64) (int64, int64) {
	if y == 1 {
		return x, 0
	}
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
