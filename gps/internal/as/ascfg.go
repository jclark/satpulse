package as

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// maxResponseDelay is how long one attempt waits for the response to a
// request. Allystar receivers on a healthy 115200 line answer within
// tens of milliseconds; patience beyond this comes from the director's
// retries, and a response outside the window must not confirm a
// resent request.
const maxResponseDelay = 2 * time.Second

// Configurator implements gpsprot.Configurator for Allystar receivers.
//
// An ACK/NAK identifies the request only by class+id, and responses to
// distinct ids can arrive out of request order, so requests are
// correlated strictly per class+id: they are created notReady and
// promoted to ready only when no earlier live request shares their
// class+id (see promote). A poll is answered by the message itself
// echoing the id - no ACK follows - so poll requests complete on
// their data response.
type Configurator struct {
	target    *gpsprot.ConfigTarget
	ver       *asbin.MonVer
	reqs      []*asReq
	phase     int                 // index into genPhases of the next phase to generate
	touched   asbin.CfgCfgMask    // sections set requests touched, for minimal saves
	pps       *asbin.CfgPps       // latest CFG-PPS readback; nil if never answered
	fixedEcef *asbin.CfgFixedECEF // latest CFG-FIXEDECEF readback
	survey    *asbin.CfgSurvey    // latest CFG-SURVEY readback
	navSat    *asbin.CfgNavSat    // latest CFG-NAVSAT readback
	speedReq  *asReq              // the baud change request, when one was generated
	prt0      *asbin.CfgPrt       // CFG-PRT record 0 readback (show-port)
	elev      *asbin.CfgElev      // latest CFG-ELEV readback
}

var _ gpsprot.Configurator = (*Configurator)(nil)

// asReqState is the internal request state; see mapping in GetState.
type asReqState int

const (
	reqNotReady asReqState = iota
	reqReady
	reqAwaitingAck
	reqMayResend
	reqSucceeded
	reqFailed
)

// asReq is a single configuration request: one Allystar packet
// expecting an ACK/NAK correlated by class+id, or (for polls) the data
// message echoing the id. When nakOK is set a NAK is an acceptable
// outcome (the request succeeds), optionally generating a fallback
// request via onNak first; NAK-driven absence stays out of the error
// path this way.
type asReq struct {
	state      asReqState
	mid        asbin.MsgID // class+id, for response correlation
	packet     []byte
	tBase      time.Time // time request was sent
	err        error
	nakOK      bool            // NAK is acceptable, not a failure
	onAck      func()          // records the accepted values on ACK
	onNak      func()          // generates the fallback request when NAKed
	onData     func(asbin.Msg) // receives the data response, which completes a poll
	noAck      bool            // no response expected (resets): sending is success
	optional   bool            // a timed-out request succeeds rather than fails
	speedAfter int             // new baud rate to switch to after sending
}

var _ gpsprot.ConfigRequest = (*asReq)(nil)

func newConfigurator(target *gpsprot.ConfigTarget, ver *asbin.MonVer) *Configurator {
	return &Configurator{target: target, ver: ver}
}

// ReceiverInfo returns static information about the GPS receiver, from
// the probe's MON-VER answer.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	return &gpsprot.ReceiverInfo{
		Vendor:         Vendor,
		Firmware:       c.ver.SwVersion.String(),
		Hardware:       c.ver.HwVersion.String(),
		SupportedGNSS:  c.supportedGNSS(),
		VendorSpecific: c.ver,
	}
}

// supportedGNSS returns the constellations of the identity-deduced
// signal plan. For unknown hardware it is the full set the CFG-NAVSAT
// mask can express; what such a unit can actually receive shows in the
// achieved signal set (the silicon clamps the mask).
func (c *Configurator) supportedGNSS() gpsprot.GNSSSet {
	return navSatToSignals(c.signalPlan()).GNSSSet()
}

// ConfigSupport returns the configuration options this implementation
// supports. The RTCM flags are declared for the whole vendor even
// though the TAU1201 has no RTCM output: capability differences show
// per-unit as NAK-driven absence in the achieved output, which the
// flags bound but do not gate.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	return gpsprot.ConfigSupportRaw | gpsprot.ConfigSupportRTCMMSM4 |
		gpsprot.ConfigSupportRTCMMSM7 | gpsprot.ConfigSupportRTCMQZSS |
		gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportSurveyMsg | gpsprot.ConfigSupportFixedPos |
		gpsprot.ConfigSupportSignal | gpsprot.ConfigSupportSpeed
}

// ConfigProps returns the current configuration of the GPS receiver,
// from the readbacks gathered while configuring. Properties that were
// never read back (unsupported or not involved) are absent.
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	props := &gpsprot.ConfigProps{}
	c.tpConfigProps(props)
	c.tmodeConfigProps(props)
	c.signalConfigProps(props)
	c.minElevConfigProps(props)
	c.speedConfigProps(props)
	return props
}

// genPhases are the request generation phases, each gated on all
// earlier requests being final. Message enabling runs before the baud
// change and the NVM save so that a save persists the message rates
// and the new rate on the confirmed line; the NVM phase is last so
// that it persists everything, fallback outcomes included.
var genPhases = []func(*Configurator){
	(*Configurator).generateQueryReqs,
	(*Configurator).generateSetReqs,
	(*Configurator).generateMsgReqs,
	(*Configurator).generateSpeedReqs,
	(*Configurator).generateNVMReqs,
}

// generateQueryReqs polls the messages whose current values the set
// phase needs (read-modify-write) or the target asks to read.
func (c *Configurator) generateQueryReqs() {
	c.generateTPQuery()
	c.generateTModeQuery()
	c.generateSignalQuery()
	c.generateMinElevQuery()
	c.generatePrtQuery()
}

// generateSetReqs generates the property set requests, computed from
// the query phase's readbacks.
func (c *Configurator) generateSetReqs() {
	c.generateTPSet()
	c.generateTModeSet()
	c.generateSignalSet()
	c.generateMinElevSet()
}

// generateNVMReqs generates the save and reset requests. Reload is
// realized as a soft reset: CFG-CFG load is acknowledged by every
// tested unit but actually loads on only one of three, while SIMPLERST
// mode 0 reloads NVM everywhere, keeps satellite data, and restarts
// within a second. Neither the resets nor the factory clear are
// acknowledged (the clear was observed silent for 12 s), so all
// succeed on send.
func (c *Configurator) generateNVMReqs() {
	opts := &c.target.Opts
	switch opts.Save {
	case gpsprot.SaveAll:
		c.addReq(&asbin.CfgCfg{Action: asbin.CfgCfgActionSave, Mask: cfgCfgMaskAll})
	case gpsprot.SaveMinimal:
		if c.touched != 0 {
			c.addReq(&asbin.CfgCfg{Action: asbin.CfgCfgActionSave, Mask: c.touched})
		}
	}
	switch opts.Reset {
	case gpsprot.ResetReload:
		c.addRstReq(asbin.CfgSimpleRstModeReset)
	case gpsprot.ResetCold:
		c.addRstReq(asbin.CfgSimpleRstModeColdStart)
	case gpsprot.ResetFactory:
		m := &asbin.CfgCfg{Action: asbin.CfgCfgActionClear, Mask: asbin.CfgCfgMaskFactoryReset}
		c.addMsg(m, asReq{noAck: true})
		c.addRstReq(asbin.CfgSimpleRstModeColdStart)
	}
}

// cfgCfgMaskAll covers every section the save mask selects: baud, NMEA
// message rates, and navigation settings. Binary message rates have no
// section and are not persistable (hardware finding).
const cfgCfgMaskAll = asbin.CfgCfgMaskBaudrate | asbin.CfgCfgMaskNmeaMsgRate |
	asbin.CfgCfgMaskNavSettings

// addRstReq appends a SIMPLERST request. Modes 0-3 restart the
// receiver without acknowledging (documented and observed), so the
// request succeeds when sent; a restart banner follows within a second.
func (c *Configurator) addRstReq(mode asbin.CfgSimpleRstMode) {
	c.addMsg(&asbin.CfgSimpleRst{Mode: mode}, asReq{noAck: true})
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

// promote readies notReady requests whose class+id no earlier live
// request shares. Requests sharing a class+id thus go out one at a
// time, in order; requests with distinct ids may be pipelined (the
// TAU951M silently drops requests beyond ~12 outstanding, a depth the
// director's window keeps us well under).
func (c *Configurator) promote() {
	live := make(map[asbin.MsgID]bool)
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

// add appends a request, to be sent once no earlier request with the
// same class+id is outstanding (see promote). All requests enter the
// slice through here.
func (c *Configurator) add(req *asReq) *asReq {
	req.state = reqNotReady
	c.reqs = append(c.reqs, req)
	return req
}

// addMsg is add for a request carrying a serialized message, tracking
// the configuration section it touches for minimal saves.
func (c *Configurator) addMsg(m asbin.Msg, req asReq) *asReq {
	c.touched |= setSection(m)
	req.mid = m.ID()
	req.packet = serialize(m)
	return c.add(&req)
}

// addReq appends a request for the given message. A NAK fails it.
func (c *Configurator) addReq(m asbin.Msg) {
	c.addMsg(m, asReq{})
}

// addReqNakOK is addReq for requests where a NAK is an acceptable
// outcome; onNak (optional) generates a fallback request first.
func (c *Configurator) addReqNakOK(m asbin.Msg, onNak func()) {
	c.addMsg(m, asReq{nakOK: true, onNak: onNak})
}

// addSetReq appends a property set request. When the receiver
// acknowledges it, onAck records the accepted values as the receiver's
// assumed configuration - per the semantics, the achieved value a set
// reports is what the receiver accepted, and a refusal (NAK) leaves
// the assumed configuration unchanged.
func (c *Configurator) addSetReq(m asbin.Msg, onAck func()) {
	c.addMsg(m, asReq{onAck: onAck})
}

// addPollReq appends an empty-payload poll of the given CFG message.
// The data response completes the request; an id this firmware lacks
// answers with SILENCE (not a NAK), so polls are optional and a
// timeout means the message does not exist, which shows as absence.
func (c *Configurator) addPollReq(mid asbin.MsgID, onData func(asbin.Msg)) {
	pkt, _ := asbin.PackMsg(mid, nil)
	c.add(&asReq{mid: mid, packet: pkt, onData: onData, optional: true})
}

// setSection returns the CFG-CFG save-section bit a set request
// touches, for minimal saves. The "NMEA message rate" section covers
// the NMEA and RTCM rate tables (hardware-verified: an F8 rate saved
// under it survives a reset) but NOT binary NAV rates, which the
// hardware does not persist at all.
func setSection(m asbin.Msg) asbin.CfgCfgMask {
	switch mt := m.(type) {
	case *asbin.CfgMsg:
		if mt.MsgClass == 0xF0 || mt.MsgClass == 0xF8 {
			return asbin.CfgCfgMaskNmeaMsgRate
		}
	case *asbin.CfgPrt:
		return asbin.CfgCfgMaskBaudrate
	case *asbin.CfgElev, *asbin.CfgNavSat, *asbin.CfgNmeaVer,
		*asbin.CfgSurvey, *asbin.CfgFixedLLA, *asbin.CfgFixedECEF, *asbin.CfgPps:
		return asbin.CfgCfgMaskNavSettings
	}
	return 0
}

func serialize(m asbin.Msg) []byte {
	pkt, err := asbin.Serialize(m)
	if err != nil {
		panic(fmt.Sprintf("serializing %v: %v", m.ID(), err))
	}
	return pkt
}

// nativeMsg processes receiver messages routed from the ConfigProtocol.
// ACKs and NAKs resolve the oldest outstanding request with the named
// class+id; any other message completes the oldest outstanding poll
// with its id (a poll is answered by the message itself, with no ACK).
func (c *Configurator) nativeMsg(m asbin.Msg, tRead time.Time) error {
	switch mt := m.(type) {
	case *asbin.AckAck:
		c.handleAck(asbin.MakeMsgID(mt.MsgClass, mt.MsgID), true, tRead)
	case *asbin.AckNak:
		c.handleAck(asbin.MakeMsgID(mt.MsgClass, mt.MsgID), false, tRead)
	default:
		for _, req := range c.reqs {
			if req.mid == m.ID() && req.state == reqAwaitingAck && req.onData != nil {
				if !req.inWindow(tRead) {
					return nil
				}
				req.onData(m)
				req.state = reqSucceeded
				break
			}
		}
	}
	return nil
}

// handleAck resolves an ACK/NAK against the oldest outstanding request
// with the acknowledged class+id. promote ensures at most one request
// per class+id is outstanding, so cross-id response reordering (which
// the hardware does) cannot misattribute a response.
func (c *Configurator) handleAck(mid asbin.MsgID, ack bool, tRead time.Time) {
	for _, req := range c.reqs {
		if req.mid != mid || req.state != reqAwaitingAck {
			continue
		}
		if !req.inWindow(tRead) {
			// A response outside the window belongs to an earlier send
			// of this (since resent) request; it must not confirm the
			// resend.
			return
		}
		if ack {
			if req.onAck != nil {
				req.onAck()
			}
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

// inWindow reports whether a response at tRead can belong to the
// current send of this request.
func (req *asReq) inWindow(tRead time.Time) bool {
	delay := tRead.Sub(req.tBase)
	return delay >= 0 && delay <= maxResponseDelay
}

func (req *asReq) invalidStateMsg(method string) string {
	return fmt.Sprintf("%s called when state is %v", method, req.state)
}

// GetPacket returns the packet bytes for this request.
func (req *asReq) GetPacket() []byte {
	switch req.state {
	case reqReady, reqMayResend, reqFailed:
		return req.packet
	}
	panic(req.invalidStateMsg("GetPacket"))
}

// GetSpeedChangeAfter returns the baud rate the host must switch to
// after sending this request, or 0.
func (req *asReq) GetSpeedChangeAfter() int {
	switch req.state {
	case reqReady, reqMayResend, reqFailed:
		return req.speedAfter
	}
	panic(req.invalidStateMsg("GetSpeedChangeAfter"))
}

// GetState reports the request state to the director.
func (req *asReq) GetState() gpsprot.ConfigRequestState {
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

// GetDeadline returns the absolute time by which a response is expected.
func (req *asReq) GetDeadline() time.Time {
	if req.state != reqAwaitingAck {
		panic(req.invalidStateMsg("GetDeadline"))
	}
	return req.tBase.Add(maxResponseDelay)
}

// GetError returns why the request failed.
func (req *asReq) GetError() error {
	if req.state != reqFailed {
		panic(req.invalidStateMsg("GetError"))
	}
	return req.err
}

// SetSentTime records when the request packet was transmitted. A
// request that expects no acknowledgement succeeds at this point.
func (req *asReq) SetSentTime(tSent time.Time) {
	switch req.state {
	case reqReady, reqMayResend:
		if req.noAck {
			req.state = reqSucceeded
			return
		}
		req.state = reqAwaitingAck
		req.tBase = tSent
	default:
		panic(req.invalidStateMsg("SetSentTime"))
	}
}

// SetDeadlinePassed marks the response window as expired.
func (req *asReq) SetDeadlinePassed() {
	if req.state != reqAwaitingAck {
		panic(req.invalidStateMsg("SetDeadlinePassed"))
	}
	req.state = reqMayResend
}

// SetWontResend gives up on the request: failure, unless the request
// was optional (an unanswered discovery poll shows as absence).
func (req *asReq) SetWontResend() {
	if req.state != reqMayResend {
		panic(req.invalidStateMsg("SetWontResend"))
	}
	if req.optional {
		req.state = reqSucceeded
		return
	}
	req.state = reqFailed
	if req.err == nil {
		req.err = fmt.Errorf("no response to %v", req.mid)
	}
}

// MaybeSpeedChangeSucceeded confirms a baud change from incidental
// valid traffic: once the host has switched speed, a valid packet read
// after the exclusion delay can only have arrived at the new speed.
func (req *asReq) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
	if req.state != reqAwaitingAck || req.speedAfter == 0 {
		return
	}
	if validPacketTime.Sub(req.tBase) > speedChangeDelay {
		req.state = reqSucceeded
	}
}

// speedChangeDelay is how long after sending a baud change until a
// valid packet counts as confirmation. The receiver switches the
// arriving port's rate immediately and the host follows right after
// the write, so the delay only excludes packets buffered before the
// switch.
const speedChangeDelay = 150 * time.Millisecond
