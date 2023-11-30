package ubx

import (
	"fmt"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

type Configurator struct {
	ver       *Version // never nil
	raw       RawConfig
	steps     []func(*Configurator) (gpsprot.ConfigRequest, error)
	stepIndex int
	target    *gpsprot.ConfigMap // never nil
	opts      gpsprot.ConfigOptions
}

type RawConfig struct {
	CfgOld
	CfgVals // access with valsPtr() so it gets lazily initialized
}

var normalConfigSteps = []func(*Configurator) (gpsprot.ConfigRequest, error){
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
	(*Configurator).reset,
}

var newConfigSteps = []func(*Configurator) (gpsprot.ConfigRequest, error){
	(*Configurator).pollPrt,
	(*Configurator).valGet,
	(*Configurator).valPreSet,
	(*Configurator).valSet,
	(*Configurator).reset,
}

var _ gpsprot.Configurator = (*Configurator)(nil)

func (c *Configurator) ConfigMap() *gpsprot.ConfigMap {
	return c.raw.Config(c.ver)
}

func (c *Configurator) NextRequest() (gpsprot.ConfigRequest, error) {
	for c.stepIndex < len(c.steps) {
		req, err := c.steps[c.stepIndex](c)
		c.stepIndex++
		if req != nil || err != nil {
			return req, err
		}
	}
	return nil, nil
}

func (c *Configurator) reset() (gpsprot.ConfigRequest, error) {
	if !c.opts.Reset {
		return nil, nil
	}
	return msgRequest{&bin.CfgRst{
		NavBbrMask: bin.CfgRstNavBbrColdStart,
		ResetMode:  bin.CfgRstResetModeHardwareResetImmediately,
	}}, nil
}

func (c *Configurator) valGet() (gpsprot.ConfigRequest, error) {
	_, missing, err := c.raw.valsPtr().Change(c.target, c.opts, c.ver, c.raw.valPort())
	if err != nil {
		return nil, err
	}
	if len(missing) == 0 {
		return nil, nil
	}
	layer := bin.CfgValgetLayerRAM
	if c.opts.Flash {
		layer = bin.CfgValgetLayerFlash
	}
	return msgRequest{newCfgValgetRequest(missing, layer)}, nil
}

func (c *Configurator) valPreSet() (gpsprot.ConfigRequest, error) {
	items := c.raw.valsPtr().configPreSetItems(c.target, c.opts)
	// Don't think this makes sense when saving to Flash
	if len(items) == 0 || c.opts.Flash {
		return nil, nil
	}
	val, err := newCfgValsetRequest(items, bin.CfgValsetLayerRAM)
	if err != nil {
		return nil, err
	}
	return c.msgSetRequest(val)
}

func (c *Configurator) valSet() (gpsprot.ConfigRequest, error) {
	items, missing, err := c.raw.valsPtr().Change(c.target, c.opts, c.ver, c.raw.valPort())
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
	if c.opts.Flash {
		layer = bin.CfgValsetLayerFlash
	}
	val, err := newCfgValsetRequest(items, layer)
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
	return pollRequest{bin.CfgPrtID}, nil
}

func (c *Configurator) pollGNSS() (gpsprot.ConfigRequest, error) {
	return pollRequest{bin.CfgGNSSID}, nil
}

func (c *Configurator) pollRate() (gpsprot.ConfigRequest, error) {
	return pollRequest{bin.CfgRateID}, nil
}

func (c *Configurator) pollNav5() (gpsprot.ConfigRequest, error) {
	return pollRequest{bin.CfgNav5ID}, nil
}

func (c *Configurator) pollTmode() (gpsprot.ConfigRequest, error) {
	switch c.ver.ProductCategory() {
	case "FTS", "TIM":
		return pollRequest{bin.CfgTmode2ID}, nil
	case "HPG":
		return pollRequest{bin.CfgTmode3ID}, nil
	}
	return nil, nil
}

func (c *Configurator) pollTp5() (gpsprot.ConfigRequest, error) {
	tpIdx := 0
	if c.ver.ProductCategory() == "FTS" {
		tpIdx = 1
	}
	return pollTp5Request{
		pollRequest: pollRequest{bin.CfgTp5ID},
		tpIdx:       tpIdx,
	}, nil
}

func (c *Configurator) enableTpMsg() (gpsprot.ConfigRequest, error) {
	if !c.opts.EnableTimeMsg {
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
	return c.enableMsgRequest(bin.TimTPID)
}

func (c *Configurator) enableTimeGNSSMsg() (gpsprot.ConfigRequest, error) {
	if !c.opts.EnableTimeMsg {
		return nil, nil
	}
	if c.ver.ProductCategory() == "FTS" {
		return c.enableMsgRequest(bin.TimTosID)
	} else {
		return c.enableMsgRequest(bin.NavTimeGPSID)
	}
}

func (c *Configurator) enableLeapSecondMsg() (gpsprot.ConfigRequest, error) {
	if !c.opts.EnableLeapSecondMsg {
		return nil, nil
	}
	return c.enableMsgRequest(bin.NavTimeLSID)
}

// XXX not clear what to do about waiting for response for SVIN messages
// we don't have to wait for the response (unlike with CFG messages)
// should handle this like leap second messages
func (c *Configurator) pollSurvey() (gpsprot.ConfigRequest, error) {
	switch c.ver.ProductCategory() {
	case "TIM", "FTS":
		return pollRequest{bin.TimSvinID}, nil
	case "HPG":
		return pollRequest{bin.NavSvinID}, nil
	}
	return nil, nil
}

func (c *Configurator) setPrt() (gpsprot.ConfigRequest, error) {
	prt := c.raw.changePrt(c.target)
	if prt == nil {
		return nil, nil
	}
	return c.msgSetRequest(prt)
}

func (c *Configurator) setNav5() (gpsprot.ConfigRequest, error) {
	nav5 := c.raw.changeNav5(c.target)
	if nav5 == nil {
		return nil, nil
	}
	// XXX this isn't quite right, because of the mask
	return c.msgSetRequest(nav5)
}

func (c *Configurator) setRate() (gpsprot.ConfigRequest, error) {
	rate := c.raw.changeRate(c.target, c.ver)
	if rate == nil {
		return nil, nil
	}
	return c.msgSetRequest(rate)
}

func (c *Configurator) setTp5() (gpsprot.ConfigRequest, error) {
	tp5 := c.raw.changeTp5(c.target)
	if tp5 == nil {
		return nil, nil
	}
	return c.msgSetRequest(tp5)
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
		raw.cookTmode2(cm)
		if raw.tmode2 == nil {
			raw.cookTmode3(cm)
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

func (c *Configurator) msgSetRequest(msg bin.Msg) (gpsprot.ConfigRequest, error) {
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

func (c *Configurator) enableMsgRequest(msgID bin.MsgID) (gpsprot.ConfigRequest, error) {
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
