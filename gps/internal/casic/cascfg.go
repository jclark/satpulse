package casic

import (
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// maxResponseDelay is how long to wait for the ACK/NAK of a request.
// On a quiet line CASIC receivers answer within tens of milliseconds;
// at 9600 with NMEA output flowing the ACK can queue behind up to a
// second or so of pending output.
const maxResponseDelay = 2 * time.Second

// Configurator implements gpsprot.Configurator for CASIC receivers.
//
// Requests are generated in one batch. A CASIC ACK/NAK identifies the
// request only by class+id, so two outstanding requests with the same
// class+id would be ambiguous; requests are created notReady and
// promoted to ready only when no earlier live request shares their
// class+id (see promote).
type Configurator struct {
	target  *gpsprot.ConfigTarget
	ver     *casbin.MonVer // nil when MON-VER is unsupported (V5)
	family  fwFamily
	reqs    []*casReq
	phase   int               // index into genPhases of the next phase to generate
	touched uint16            // CfgSection* bits of the sections set requests touched
	tp      *casbin.CfgTP     // latest CFG-TP readback; nil if never answered
	tm5     *casbin.CfgTMode  // latest V5 CFG-TMODE readback
	tm6     *casbin.CfgTMode2 // latest V6 CFG-TMODE2 readback
}

var _ gpsprot.Configurator = (*Configurator)(nil)

// casReqState is the internal request state; see mapping in GetState.
type casReqState int

const (
	reqNotReady casReqState = iota
	reqReady
	reqAwaitingAck
	reqMayResend
	reqSucceeded
	reqFailed
)

// casReq is a single configuration request: one CASIC packet expecting
// an ACK or NAK correlated by class+id. When nakOK is set a NAK is an
// acceptable outcome (the request succeeds), optionally generating a
// fallback request via onNak first; this is how NAK-driven fallback
// stays out of the error path.
type casReq struct {
	state  casReqState
	mid    casbin.MsgID // class+id, for ACK correlation
	packet []byte
	tBase  time.Time // time request was sent
	err    error
	nakOK  bool             // NAK is acceptable, not a failure
	onNak  func()           // generates the fallback request when NAKed
	noAck  bool             // no response expected (CFG-RST): sending is success
	onData func(casbin.Msg) // receives data responses (polls); ACK still completes
}

var _ gpsprot.ConfigRequest = (*casReq)(nil)

func newConfigurator(target *gpsprot.ConfigTarget, ver *casbin.MonVer) *Configurator {
	family := familyV5
	if ver != nil && !strings.Contains(ver.SwVersion.String(), "URANUS5") {
		family = familyV6
	}
	return &Configurator{target: target, ver: ver, family: family}
}

// ReceiverInfo returns static information about the GPS receiver.
// A V5 receiver does not answer MON-VER, so its firmware and hardware
// strings are unknown and reported empty.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	info := &gpsprot.ReceiverInfo{Vendor: Vendor, SupportedGNSS: c.supportedGNSS()}
	if c.ver != nil {
		info.Firmware = c.ver.SwVersion.String()
		info.Hardware = c.ver.HwVersion.String()
		info.VendorSpecific = c.ver
	}
	return info
}

// supportedGNSS returns the constellations the firmware family can use.
// V5 supports GPS/BDS/GLONASS only; the V6 supported set should be
// refined from CFG-NAVBAND query results once implemented.
func (c *Configurator) supportedGNSS() gpsprot.GNSSSet {
	set := gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.BDS, gpsprot.GLO)
	if c.family == familyV6 {
		set |= gpsprot.GNSSSetOf(gpsprot.GAL, gpsprot.QZSS, gpsprot.SBAS, gpsprot.NAVIC)
	}
	return set
}

// ConfigSupport returns the configuration options this implementation
// supports.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	return gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportFixedPos | gpsprot.ConfigSupportFixedPosAcc
}

// ConfigProps returns the current configuration of the GPS receiver,
// from the readbacks gathered while configuring. Properties that were
// never read back (unsupported or not involved) are absent.
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	props := &gpsprot.ConfigProps{}
	c.tpConfigProps(props)
	c.tmodeConfigProps(props)
	return props
}

// generateQueryReqs polls the messages whose current values the set
// phase needs (read-modify-write) or the target asks to read.
func (c *Configurator) generateQueryReqs() {
	c.generateTPQuery()
	c.generateTModeQuery()
}

// generateSetReqs generates the property set requests, computed from
// the query phase's readbacks.
func (c *Configurator) generateSetReqs() {
	c.generateTPSet()
	c.generateTModeSet()
}

// generateVerifyReqs re-polls what the set phase changed, so that
// ConfigProps reports achieved values as the receiver holds them.
func (c *Configurator) generateVerifyReqs() {
	c.generateTPVerify()
	c.generateTModeVerify()
}

// genPhases are the request generation phases, each gated on all
// earlier requests being final: property sets need the query phase's
// readback (read-modify-write), the verify phase re-polls what the
// sets changed so achieved values are truthful, and the NVM phase
// must come last so NAK-driven fallback requests are saved too.
var genPhases = []func(*Configurator){
	(*Configurator).generateQueryReqs,
	(*Configurator).generateMsgReqs,
	(*Configurator).generateSetReqs,
	(*Configurator).generateVerifyReqs,
	(*Configurator).generateNVMReqs,
}

// GenerateRequests generates configuration requests and promotes
// requests that have become unambiguous to send.
func (c *Configurator) GenerateRequests() error {
	for c.phase < len(genPhases) && c.allFinal() {
		genPhases[c.phase](c)
		c.phase++
	}
	c.promote()
	return nil
}

func (c *Configurator) allFinal() bool {
	for _, req := range c.reqs {
		if req.state != reqSucceeded && req.state != reqFailed {
			return false
		}
	}
	return true
}

// generateNVMReqs generates the save and reset requests.
func (c *Configurator) generateNVMReqs() {
	opts := &c.target.Opts
	switch opts.Save {
	case gpsprot.SaveAll:
		c.addReq(&casbin.CfgCfg{Mask: c.saveMask(casbin.CfgSectionAll), OpMode: casbin.CfgOpSave})
	case gpsprot.SaveMinimal:
		if c.touched != 0 {
			c.addReq(&casbin.CfgCfg{Mask: c.saveMask(c.touched), OpMode: casbin.CfgOpSave})
		}
	}
	switch opts.Reset {
	case gpsprot.ResetReload:
		m := &casbin.CfgCfg{Mask: c.saveMask(casbin.CfgSectionAll), OpMode: casbin.CfgOpLoad}
		if c.family == familyV6 {
			// V6 firmware restarts the receiver on load without
			// acknowledging first (observed on the F8N: boot banner,
			// no ACK).
			c.reqs = append(c.reqs, &casReq{state: reqNotReady, mid: m.ID(), packet: serialize(m), noAck: true})
		} else {
			c.addReq(m)
		}
	case gpsprot.ResetCold:
		c.addRstReq(casbin.StartCold)
	case gpsprot.ResetFactory:
		c.addRstReq(casbin.StartFactory)
	}
}

// saveMask returns the CFG-CFG section mask to send. V5 firmware
// honours the section bits. V6 documents the field as reserved but
// does NOT ignore it: mask 0 is ACKed and saves nothing (observed on
// the F8N), so V6 always gets the all-sections mask - its save
// granularity is a single group.
func (c *Configurator) saveMask(mask uint16) uint16 {
	if c.family == familyV6 {
		return casbin.CfgSectionAll
	}
	return mask
}

// bbrReset clears everything learned from satellites plus saved
// position and config, but keeps clock drift and oscillator parameters
// (learned locally, not from satellites). Matches the casictool
// reference. V6 firmware's verified reset command clears no BBR
// sections; its start mode implies the scope.
const bbrReset = casbin.BbrEphemeris | casbin.BbrAlmanac | casbin.BbrHealth |
	casbin.BbrIonosphere | casbin.BbrPosition | casbin.BbrUTCParams |
	casbin.BbrRTC | casbin.BbrConfig

// addRstReq appends a CFG-RST request. The receiver restarts without
// acknowledging, so the request succeeds when sent.
func (c *Configurator) addRstReq(startMode uint8) {
	bbr := uint16(0)
	if c.family == familyV5 {
		bbr = bbrReset
	}
	m := &casbin.CfgRst{NavBbrMask: bbr, ResetMode: casbin.ResetHWImmediate, StartMode: startMode}
	c.reqs = append(c.reqs, &casReq{state: reqNotReady, mid: m.ID(), packet: serialize(m), noAck: true})
}

// promote readies notReady requests whose class+id no earlier live
// request shares. Requests sharing a class+id thus go out one at a
// time, in order; requests with distinct ids may be pipelined.
func (c *Configurator) promote() {
	live := make(map[casbin.MsgID]bool)
	for _, req := range c.reqs {
		switch req.state {
		case reqNotReady:
			if !live[req.mid] {
				req.state = reqReady
			}
			live[req.mid] = true
		case reqReady, reqAwaitingAck, reqMayResend:
			live[req.mid] = true
		}
	}
}

// GetRequestCount returns the current number of requests and whether
// the slice is complete. The slice is complete only when every request
// is final: a NAK on a live request may still generate a fallback
// request, and later phases generate more requests.
func (c *Configurator) GetRequestCount() (int, bool) {
	return len(c.reqs), c.phase == len(genPhases) && c.allFinal()
}

// Request returns the ConfigRequest at the given index.
func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
	return c.reqs[index]
}

// addReq appends a request for the given message, to be sent once no
// earlier request with the same class+id is outstanding. A NAK fails
// the request.
func (c *Configurator) addReq(m casbin.Msg) {
	c.touched |= setSection(m)
	c.reqs = append(c.reqs, &casReq{state: reqNotReady, mid: m.ID(), packet: serialize(m)})
}

// addReqNakOK is addReq for requests where a NAK is an acceptable
// outcome; onNak (optional) generates a fallback request first.
func (c *Configurator) addReqNakOK(m casbin.Msg, onNak func()) {
	c.touched |= setSection(m)
	c.reqs = append(c.reqs, &casReq{state: reqNotReady, mid: m.ID(), packet: serialize(m), nakOK: true, onNak: onNak})
}

// setSection returns the CFG-CFG save-section bit a set request
// touches, for minimal saves. V6-only CFG messages return 0: the V6
// save command has no section mask.
func setSection(m casbin.Msg) uint16 {
	switch m.ID() {
	case casbin.CfgMsgID:
		return casbin.CfgSectionMsg
	case casbin.CfgPrtID:
		return casbin.CfgSectionPort
	case casbin.CfgTPID:
		return casbin.CfgSectionTP
	case casbin.CfgRateID, casbin.CfgTModeID, casbin.CfgNavxID:
		return casbin.CfgSectionNav
	}
	return 0
}

func serialize(m casbin.Msg) []byte {
	pkt, err := casbin.Serialize(m)
	if err != nil {
		panic(fmt.Sprintf("serializing %v: %v", m.ID(), err))
	}
	return pkt
}

// nativeMsg processes receiver messages routed from the ConfigProtocol.
// Non-ACK messages are offered to the oldest outstanding poll request
// with the same class+id (a poll's data response echoes the request's
// class+id and precedes its ACK).
func (c *Configurator) nativeMsg(m casbin.Msg, tRead time.Time) error {
	switch mt := m.(type) {
	case *casbin.AckAck:
		c.handleAck(casbin.MakeMsgID(mt.ClsID, mt.MsgID), true, tRead)
	case *casbin.AckNak:
		c.handleAck(casbin.MakeMsgID(mt.ClsID, mt.MsgID), false, tRead)
	default:
		for _, req := range c.reqs {
			if req.mid == m.ID() && req.state == reqAwaitingAck && req.onData != nil {
				req.onData(m)
				break
			}
		}
	}
	return nil
}

// addPollReq appends an empty-payload poll of the given CFG message.
// The data response is passed to onData; a NAK means the message does
// not exist in this firmware, which is acceptable (shown by absence).
func (c *Configurator) addPollReq(mid casbin.MsgID, onData func(casbin.Msg)) {
	pkt, _ := casbin.PackMsg(mid, nil)
	c.reqs = append(c.reqs, &casReq{state: reqNotReady, mid: mid, packet: pkt, nakOK: true, onData: onData})
}

// handleAck resolves an ACK/NAK against the oldest outstanding request
// with the acknowledged class+id. Responses arrive in request order on
// all tested receivers, and promote ensures at most one request per
// class+id is outstanding.
func (c *Configurator) handleAck(mid casbin.MsgID, ack bool, tRead time.Time) {
	for _, req := range c.reqs {
		if req.mid != mid || req.state != reqAwaitingAck {
			continue
		}
		delay := tRead.Sub(req.tBase)
		if delay < 0 || delay > maxResponseDelay {
			return
		}
		if ack {
			req.state = reqSucceeded
		} else if req.nakOK {
			if req.onNak != nil {
				req.onNak()
			}
			req.state = reqSucceeded
		} else {
			req.state = reqFailed
			req.err = fmt.Errorf("receiver refused %v", mid)
		}
		return
	}
}

func (req *casReq) invalidStatePanic(method string) string {
	return fmt.Sprintf("%s called when state is %v", method, req.state)
}

func (req *casReq) GetPacket() []byte {
	switch req.state {
	case reqReady, reqMayResend, reqFailed:
		return req.packet
	}
	panic(req.invalidStatePanic("GetPacket"))
}

func (req *casReq) GetSpeedChangeAfter() int {
	return 0
}

func (req *casReq) GetState() gpsprot.ConfigRequestState {
	switch req.state {
	case reqNotReady:
		return gpsprot.ConfigRequestNotReady
	case reqReady:
		return gpsprot.ConfigRequestReadyToSend
	case reqAwaitingAck:
		return gpsprot.ConfigRequestAwaitingResponse
	case reqMayResend:
		return gpsprot.ConfigRequestMayResend
	case reqSucceeded:
		return gpsprot.ConfigRequestSucceeded
	case reqFailed:
		return gpsprot.ConfigRequestFailed
	default:
		panic(fmt.Sprintf("unexpected internal state: %v", req.state))
	}
}

func (req *casReq) GetDeadline() time.Time {
	if req.state != reqAwaitingAck {
		panic(req.invalidStatePanic("GetDeadline"))
	}
	return req.tBase.Add(maxResponseDelay)
}

func (req *casReq) GetError() error {
	if req.state != reqFailed {
		panic(req.invalidStatePanic("GetError"))
	}
	return req.err
}

func (req *casReq) SetSentTime(tSent time.Time) {
	switch req.state {
	case reqReady, reqMayResend:
		if req.noAck {
			req.state = reqSucceeded
			return
		}
		req.state = reqAwaitingAck
		req.tBase = tSent
	default:
		panic(req.invalidStatePanic("SetSentTime"))
	}
}

func (req *casReq) SetDeadlinePassed() {
	if req.state != reqAwaitingAck {
		panic(req.invalidStatePanic("SetDeadlinePassed"))
	}
	req.state = reqMayResend
}

func (req *casReq) SetWontResend() {
	if req.state != reqMayResend {
		panic(req.invalidStatePanic("SetWontResend"))
	}
	req.state = reqFailed
	if req.err == nil {
		req.err = fmt.Errorf("no response to %v", req.mid)
	}
}

func (req *casReq) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
}
