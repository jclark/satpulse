package septentrio

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// The receiver's command line is single-flight: it has no queueing and
// answers each command with one framed reply completed by the prompt. The
// Configurator therefore keeps at most one request outstanding; correlation
// is by exact command echo, and the single-flight discipline itself
// attributes the anchor-less "$R?" refusals (they carry no echo).

const (
	// maxReplyDelay is the per-attempt reply window. Replies arrive within
	// milliseconds on USB; patience comes from the director's retry budget.
	maxReplyDelay = 1500 * time.Millisecond
)

type configPhase int

const (
	phaseInit  configPhase = iota
	phaseQuery             // reading current state for Get and read-modify-write
	phaseSet               // realizing the target's properties
	phaseFinal
)

// Configurator implements gpsprot.Configurator for Septentrio receivers.
type Configurator struct {
	caps          *rxCaps
	target        *gpsprot.ConfigTarget
	phase         configPhase
	reqs          []*sReq
	complete      bool   // no more requests will be added to reqs
	nFinished     int    // all requests with index < nFinished are in a final state
	port          string // connection descriptor from reply prompts (e.g. "USB1")
	np            nativeProps
	ident         []xml.Token // Identification file tokens, the identity source
	staticQueried bool        // the static-position follow-up query was generated
}

type sReqState int

const (
	sStateNotReady  sReqState = iota // waiting for earlier requests (single-flight)
	sStateReady                      // maps to ConfigRequestReadyToSend
	sStateAwaiting                   // sent, expecting the framed reply
	sStateMayResend                  // reply window passed; eligible for retry
	sStateSucceeded
	sStateFailed
)

func (s sReqState) isFinal() bool {
	return s == sStateSucceeded || s == sStateFailed
}

// sReq is a single command request.
type sReq struct {
	state    sReqState
	cmd      string       // command text, without CR LF
	nakOK    bool         // a "$R?" refusal is an acceptable outcome, not a failure
	optional bool         // giving up after retries is success, not failure
	onReply  func(*Reply) // records achieved values from the matching reply
	onLst    func(string) // lst commands: called with the joined block contents
	blocks   []string     // block units collected so far (lst commands)
	tBase    time.Time    // send time, for the reply deadline
	err      error
}

var _ gpsprot.Configurator = (*Configurator)(nil)
var _ gpsprot.ConfigRequest = (*sReq)(nil)

// ReceiverInfo returns static information about the GPS receiver: supported
// signals from the probe's capability reply, and identity from the fetched
// Identification file.
func (c *Configurator) ReceiverInfo() *gpsprot.ReceiverInfo {
	info := &gpsprot.ReceiverInfo{
		Vendor:        Vendor,
		SupportedGNSS: c.caps.sigSet.GNSSSet(),
	}
	if id := c.ident; id != nil {
		info.Hardware, info.Firmware = identInfo(id)
		info.VendorSpecific = id
	}
	return info
}

// ConfigSupport returns the configuration options this implementation
// supports on this receiver: the RTCMv3 output features additionally need
// the correction-generation capabilities from the probe.
func (c *Configurator) ConfigSupport() gpsprot.ConfigSupportFlags {
	flags := configSupport
	if !c.caps.rtcmV3Base() {
		flags &^= gpsprot.ConfigSupportRTCMMSM4 | gpsprot.ConfigSupportRTCMMSM7 |
			gpsprot.ConfigSupportRTCMBaseID | gpsprot.ConfigSupportRTCMQZSS
	}
	return flags
}

// ConfigProps returns the current configuration of the GPS receiver.
func (c *Configurator) ConfigProps() *gpsprot.ConfigProps {
	props := &gpsprot.ConfigProps{}
	c.np.convertToProps(props)
	if c.port != "" {
		props.SetPort(c.port)
		if strings.HasPrefix(c.port, "USB") {
			// Owner ruling, following the ubx USB model: baud rate is not
			// applicable on USB connections and reads back as 0.
			props.SetBaudRate(0)
		}
	}
	return props
}

// Request returns the ConfigRequest at the given index.
func (c *Configurator) Request(index int) gpsprot.ConfigRequest {
	return c.reqs[index]
}

// GetRequestCount returns the current number of requests and whether the set is complete.
func (c *Configurator) GetRequestCount() (int, bool) {
	return len(c.reqs), c.complete
}

// GenerateRequests advances the phase machine and promotes the next request
// under the single-flight rule.
func (c *Configurator) GenerateRequests() error {
	switch c.phase {
	case phaseInit:
		c.generateQueryReqs()
		c.phase = phaseQuery
	case phaseQuery:
		if !c.allFinal() || c.generateFollowupQueries() {
			break
		}
		c.generateSetReqs()
		c.phase = phaseSet
	case phaseSet:
		if !c.allFinal() {
			break
		}
		c.complete = true
		c.phase = phaseFinal
	case phaseFinal:
	}
	c.promote()
	return nil
}

func (c *Configurator) append(reqs ...*sReq) {
	c.reqs = append(c.reqs, reqs...)
}

// allFinal reports whether every generated request is in a final state,
// advancing nFinished.
func (c *Configurator) allFinal() bool {
	for c.nFinished < len(c.reqs) {
		if !c.reqs[c.nFinished].state.isFinal() {
			return false
		}
		c.nFinished++
	}
	return true
}

// promote moves the first non-final request from NotReady to Ready when
// everything before it is final: the single-flight rule.
func (c *Configurator) promote() {
	for i := c.nFinished; i < len(c.reqs); i++ {
		req := c.reqs[i]
		if !req.state.isFinal() {
			if req.state == sStateNotReady {
				req.state = sStateReady
			}
			return
		}
	}
}

// inflight returns the request awaiting a reply, or nil.
func (c *Configurator) inflight() *sReq {
	for i := c.nFinished; i < len(c.reqs); i++ {
		if c.reqs[i].state == sStateAwaiting {
			return c.reqs[i]
		}
	}
	return nil
}

// reply processes a framed command reply. Every reply's prompt names our own
// connection; acks correlate to the in-flight request by exact command echo,
// and anchor-less refusals attribute to it by the single-flight discipline.
func (c *Configurator) reply(r *Reply, tRead time.Time) error {
	if r.Prompt != "" && r.Prompt != "STOP" {
		// The prompt names our connection, except the "STOP>" that
		// terminates a reset command's reply.
		c.port = r.Prompt
	}
	req := c.inflight()
	if req == nil {
		return nil
	}
	switch r.Kind {
	case ReplyAck, ReplyLst:
		if r.Echo != req.cmd {
			return nil // not ours: e.g. a late reply to a repeated probe
		}
		if r.Kind == ReplyLst && req.onLst != nil {
			break // accepted; the block units follow
		}
		req.state = sStateSucceeded
		if req.onReply != nil {
			req.onReply(r)
		}
	case ReplyBlock:
		if req.onLst == nil {
			break
		}
		req.blocks = append(req.blocks, r.Block)
		if r.Prompt != "" {
			req.state = sStateSucceeded
			req.onLst(strings.Join(req.blocks, "\n"))
		}
	case ReplyNak:
		if req.nakOK {
			req.state = sStateSucceeded
			if req.onReply != nil {
				req.onReply(r)
			}
			break
		}
		req.state = sStateFailed
		req.err = fmt.Errorf("%s: receiver refused: %s", req.cmd, r.Error)
	}
	return nil
}

func (req *sReq) invalidStatePanic(method string) string {
	return fmt.Sprintf("%s called when state is %v", method, req.state)
}

// GetPacket returns the packet bytes for this request.
func (req *sReq) GetPacket() []byte {
	switch req.state {
	case sStateReady, sStateMayResend, sStateFailed:
		return append([]byte(req.cmd), '\r', '\n')
	}
	panic(req.invalidStatePanic("GetPacket"))
}

// GetSpeedChangeAfter returns 0: baud rate is not applicable on USB
// connections (owner ruling), so no request changes speed.
func (req *sReq) GetSpeedChangeAfter() int {
	return 0
}

// GetState maps the internal state to the public ConfigRequestState.
func (req *sReq) GetState() gpsprot.ConfigRequestState {
	switch req.state {
	case sStateNotReady:
		return gpsprot.ConfigRequestNotReady
	case sStateReady:
		return gpsprot.ConfigRequestReadyToSend
	case sStateAwaiting:
		return gpsprot.ConfigRequestAwaitingResponse
	case sStateMayResend:
		return gpsprot.ConfigRequestMayResend
	case sStateSucceeded:
		return gpsprot.ConfigRequestSucceeded
	case sStateFailed:
		return gpsprot.ConfigRequestFailed
	}
	panic(fmt.Sprintf("unexpected internal state: %v", req.state))
}

// GetDeadline returns the reply or absorb deadline.
func (req *sReq) GetDeadline() time.Time {
	switch req.state {
	case sStateAwaiting:
		return req.tBase.Add(maxReplyDelay)
	}
	panic(req.invalidStatePanic("GetDeadline"))
}

// GetError returns the error details for a failed request.
func (req *sReq) GetError() error {
	if req.state != sStateFailed {
		panic(req.invalidStatePanic("GetError"))
	}
	if req.err == nil {
		return errors.New(req.cmd + ": request abandoned after timeout")
	}
	return req.err
}

// SetSentTime records when the request packet was transmitted.
func (req *sReq) SetSentTime(tSent time.Time) {
	switch req.state {
	case sStateReady, sStateMayResend:
		req.state = sStateAwaiting
		req.tBase = tSent
	default:
		panic(req.invalidStatePanic("SetSentTime"))
	}
}

// SetDeadlinePassed handles a passed reply or absorb deadline.
func (req *sReq) SetDeadlinePassed() {
	switch req.state {
	case sStateAwaiting:
		req.blocks = nil // a retried lst reply arrives in full
		req.state = sStateMayResend
	default:
		panic(req.invalidStatePanic("SetDeadlinePassed"))
	}
}

// SetWontResend marks a timed-out request as permanently failed, or as
// succeeded when the request is optional (give-up is an acceptable outcome).
func (req *sReq) SetWontResend() {
	if req.state != sStateMayResend {
		panic(req.invalidStatePanic("SetWontResend"))
	}
	if req.optional {
		req.state = sStateSucceeded
		return
	}
	req.state = sStateFailed
}

// MaybeSpeedChangeSucceeded is a no-op: no request changes speed.
func (req *sReq) MaybeSpeedChangeSucceeded(validPacketTime time.Time) {
}
