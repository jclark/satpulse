package ubx

import (
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestConfigurationGet_Legacy(t *testing.T) {
	testConfigurationGet(t, newLegacyReceiver())
}

var m10Version = Version{
	HW:   "000S0000",
	SW:   "ROM SPG 5.10 (c00d69)",
	Prot: &ProtVer{Major: 34, Minor: 10},
	FW:   &FWVer{ProductCategory: "SPG", Major: 5, Minor: 10},
	GNSS: gpsprot.MajorGNSSSet,
}

var m8tVersion = Version{
	Prot: &ProtVer{Major: 22, Minor: 00},
	Mod:  "LEA-M8T-0",
	FW:   &FWVer{ProductCategory: "TIM", Major: 8, Minor: 01},
}

func TestConfigurationEmpty_New(t *testing.T) {
	testConfigurationEmpty(t, &m10Version)
}

func TestConfigurationEmpty_Legacy(t *testing.T) {
	testConfigurationEmpty(t, &m8tVersion)
}

func testConfigurationEmpty(t *testing.T, ver *Version) {
	target := gpsprot.NewConfigTarget()
	rcvr := newGpsReceiver(ver)
	rcvr.raw.tmode2 = &ubxbin.CfgTmode2{
		TimeMode: ubxbin.CfgTmode2SurveyIn,
	}
	_, _, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Errorf("unexpected error from runConfiguration: %v", err)
	}
	if rcvr.nSent != 0 {
		t.Errorf("expected no messages to be sent, got %d", rcvr.nSent)
	}
}

func TestConfigurationGet_New(t *testing.T) {
	testConfigurationGet(t, &gpsReceiver{
		version: &m10Version,
		raw:     newDefaultRawConfig(),
	})
}

func newLegacyReceiver() *gpsReceiver {
	return newGpsReceiver(new(Version))
}

func newGpsReceiver(ver *Version) *gpsReceiver {
	return &gpsReceiver{
		version: ver,
		raw:     newDefaultRawConfig(),
	}
}

func testConfigurationGet(t *testing.T, rcvr *gpsReceiver) {
	target := gpsprot.NewConfigTarget()
	target.Get |= gpsprot.PropIDTimePulseWidth
	c, naks, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Fatalf("unexpected error from runConfiguration: %v", err)
	}
	if len(naks) != 0 {
		t.Errorf("expected no naks, got %v", naks)
	}
	if w, ok := c.ConfigProps().GetTimePulseWidth(); !ok || w != time.Second/10 {
		t.Errorf("expected pulse width to be 0.1s, got %v", w)
	}
	if rcvr.nSent != 1 {
		t.Errorf("expected 1 message to be sent, got %d", rcvr.nSent)
	}
}

type gpsReceiver struct {
	version      *Version
	raw          *RawConfig
	nakPollMsgID ubxbin.MsgID
	abortMsgID   ubxbin.MsgID // simulate abort (timeout/corruption) when sending this message
	nSent        int
}

func runConfiguration(rcvr *gpsReceiver, target *gpsprot.ConfigTarget) (*Configurator, []string, error) {
	cp := NewConfigProtocol()
	cp.ver = rcvr.version
	var naks []string

	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetNativeMsgHandler(cp)

	c, err := cp.Configure(target)
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error from Configure: %w", err)
	}

	// Use ConfigDirector to drive the configuration
	director := gpsprot.NewConfigDirector(c, 3) // 3 max retries
	tm := time.Now()

	for action := range director.Actions() {
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			req := c.Request(action.Index)
			sendTm := tm
			pkt := req.GetPacket()
			msgID := ubxbin.PacketMsgId(pkt)

			// Simulate abort scenario (timeout/corruption)
			if msgID == rcvr.abortMsgID {
				// Simulate timeout
				req.SetSentTime(sendTm)
				tm = tm.Add(2 * time.Second)
				req.SetDeadlinePassed()
				req.SetWontResend()
				continue
			}

			// Send the request
			req.SetSentTime(sendTm)
			resps := rcvr.sendReceive(pkt)

			// Process responses
			for _, resp := range resps {
				tm = tm.Add(time.Second / 10)
				_, err = pp.ProcessPacket(string(resp), tm)
				if err != nil {
					return nil, nil, fmt.Errorf("unexpected error processing response packet: %w", err)
				}
			}

			// Check if this was a NACK
			if len(resps) > 0 {
				for _, resp := range resps {
					msg, _ := ubxbin.ParseMsg(string(resp))
					if nakMsg, ok := msg.(*ubxbin.AckNak); ok && nakMsg.MsgID == msgID {
						naks = append(naks, msgID.String())
					}
				}
			}

		case gpsprot.ConfigActionWaitUntil:
			// Simulate waiting
			if action.Deadline.After(tm) {
				tm = action.Deadline
			}
			// Advance time past deadline to trigger timeout processing
			director.AdvanceTimeTo(action.Deadline.Add(time.Millisecond))

		case gpsprot.ConfigActionError:
			// Configuration error - record but continue to allow recovery
			// Only fail if recovery also fails
			continue
		}
	}
	uc := c.(*Configurator)
	return uc, naks, nil
}

func (r *gpsReceiver) sendReceive(pkt []byte) [][]byte {
	r.nSent++
	msgID := ubxbin.PacketMsgId(pkt)
	const pollMaxLen = 9
	var respPkts [][]byte
	var respMsg ubxbin.Msg
	nak := false

	if msgID == ubxbin.CfgValgetID {
		respMsg = r.valgetResp(pkt)
		if respMsg == nil {
			nak = true
		}
	} else if len(pkt) <= pollMaxLen {
		switch msgID {
		case ubxbin.CfgPrtID:
			respMsg = r.raw.prt
		case ubxbin.CfgTp5ID:
			respMsg = r.raw.tp5
		case ubxbin.CfgNav5ID:
			respMsg = r.raw.nav5
		case ubxbin.CfgRateID:
			respMsg = r.raw.rate
		case ubxbin.CfgGNSSID:
			respMsg = r.raw.gnss
		case ubxbin.CfgTmode2ID:
			respMsg = r.raw.tmode2
		case ubxbin.CfgTmode3ID:
			respMsg = r.raw.tmode3
		}
	}
	if msgID == r.nakPollMsgID {
		nak = true
		respMsg = nil
	}
	if respMsg != nil {
		if respPkt, err := ubxbin.Serialize(respMsg); err == nil {
			respPkts = append(respPkts, respPkt)
		}
	}
	// If it's not a poll (i.e., a set command), update the receiver's state
	if len(pkt) > pollMaxLen {
		switch msgID {
		case ubxbin.CfgPrtID:
			// Parse and apply the CFG-PRT set command
			if msg, err := ubxbin.ParseMsg(string(pkt)); err == nil {
				if prtMsg, ok := msg.(*ubxbin.CfgPrt); ok {
					r.raw.prt = prtMsg
				}
			}
			// Add other message types here as needed
		}
	}

	if msgID.Ackable() {
		var ackMsg ubxbin.Msg
		if nak {
			ackMsg = &ubxbin.AckNak{MsgID: msgID}
		} else {
			ackMsg = &ubxbin.AckAck{MsgID: msgID}
		}
		if resp, err := ubxbin.Serialize(ackMsg); err == nil {
			respPkts = append(respPkts, resp)
		}
	}
	return respPkts
}

func newDefaultRawConfig() *RawConfig {
	raw := RawConfig{}
	raw.prt = &ubxbin.CfgPrt{
		PortID:       ubxbin.PortUART1,
		InProtoMask:  ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA,
		OutProtoMask: ubxbin.CfgPrtProtoUBX | ubxbin.CfgPrtProtoNMEA,
	}
	raw.tp5 = &ubxbin.CfgTp5{
		Flags:             ubxbin.CfgTp5IsLength | ubxbin.CfgTp5Active | ubxbin.CfgTp5LockGpsFreq | ubxbin.CfgTp5Polarity | ubxbin.CfgTp5AlignToTow | ubxbin.CfgTp5LockedOtherSet,
		Version:           1,
		PulseLenRatio:     0,
		PulseLenRatioLock: 100 * 1000,
		FreqPeriod:        1000 * 1000,
		FreqPeriodLock:    1000 * 1000,
		AntCableDelay:     50,
	}
	raw.nav5 = &ubxbin.CfgNav5{
		DynModel:    ubxbin.CfgNav5DynPortable,
		UtcStandard: ubxbin.CfgNav5UtcAuto,
	}
	raw.rate = &ubxbin.CfgRate{
		MeasRate: 1000,
		NavRate:  1,
		TimeRef:  ubxbin.CfgRateUTC,
	}
	raw.gnss = &ubxbin.CfgGNSS{
		CfgGNSSFixed: ubxbin.CfgGNSSFixed{NumConfigBlocks: 1},
		Blocks:       []ubxbin.CfgGNSSBlock{{GNSSID: ubxbin.GPS}},
	}
	cfgValsInit(raw.valsPtr())
	return &raw
}

// Tests for sequence-based response/ACK ownership in the configurator.
// See plan/ubx-config-response-sequence.md. The previous implementation used
// timestamp comparisons (`tRead.Before(sentTime)`, `!t.Before(sentTime)`),
// which meant that two events at the same simulated instant could be claimed
// by the wrong request. The replacement uses a per-event sequence counter,
// and these tests pin down its behaviour at the boundaries.

// TestSeqOwnership_StaleResponseAtEqualTimestamp covers the central bug:
// repeated polls of the same MsgID must not be advanced by the prior poll's
// response when both events share a simulated timestamp.
func TestSeqOwnership_StaleResponseAtEqualTimestamp(t *testing.T) {
	c := newConfigurator(gpsprot.NewConfigTarget(), &m10Version)
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})

	T0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c.reqs[0].state = stateReadyToSend
	c.reqs[0].SetSentTime(T0.Add(-2 * time.Millisecond))
	if got := c.reqs[0].state; got != stateAwaitingAckAndResponse {
		t.Fatalf("reqs[0] after send: got %v, want stateAwaitingAckAndResponse", got)
	}

	if _, err := c.processMsg(&ubxbin.CfgPrt{PortID: ubxbin.PortUART1}, T0); err != nil {
		t.Fatalf("processMsg response: %v", err)
	}
	if got := c.reqs[0].state; got != stateAwaitingAck {
		t.Fatalf("reqs[0] after response: got %v, want stateAwaitingAck", got)
	}

	if _, err := c.processMsg(&ubxbin.AckAck{MsgID: ubxbin.CfgPrtID}, T0); err != nil {
		t.Fatalf("processMsg ack: %v", err)
	}
	if got := c.reqs[0].state; got != stateSucceeded {
		t.Fatalf("reqs[0] after ack: got %v, want stateSucceeded", got)
	}

	// Send the second poll at the same simulated instant as the prior
	// response and ACK. Sequence ownership must keep it awaiting.
	c.reqs[1].state = stateReadyToSend
	c.reqs[1].SetSentTime(T0)
	if got := c.reqs[1].state; got != stateAwaitingAckAndResponse {
		t.Fatalf("reqs[1] after send at equal timestamp: got %v, want stateAwaitingAckAndResponse "+
			"(sentSeq=%d, msgSeq[CFG-PRT]=%d, eventSeq=%d)",
			got, c.reqs[1].sentSeq, c.msgSeq[ubxbin.CfgPrtID], c.eventSeq)
	}

	// A fresh response and ACK must drive it to completion.
	if _, err := c.processMsg(&ubxbin.CfgPrt{PortID: ubxbin.PortUART1}, T0.Add(4*time.Millisecond)); err != nil {
		t.Fatalf("processMsg fresh response: %v", err)
	}
	if got := c.reqs[1].state; got != stateAwaitingAck {
		t.Errorf("reqs[1] after fresh response: got %v, want stateAwaitingAck", got)
	}
	if _, err := c.processMsg(&ubxbin.AckAck{MsgID: ubxbin.CfgPrtID}, T0.Add(4*time.Millisecond)); err != nil {
		t.Fatalf("processMsg fresh ack: %v", err)
	}
	if got := c.reqs[1].state; got != stateSucceeded {
		t.Errorf("reqs[1] final: got %v, want stateSucceeded", got)
	}
}

// TestSeqOwnership_ResponseAloneDoesNotCompleteRequest pins down the
// specific bug in the previous implementation where receiving the response
// for a poll request that needed both response and ACK would prematurely
// transition the request all the way to stateSucceeded (and call ops.Done())
// without ever observing the ACK. The director would then advance to the
// next request even though the receiver had not yet acknowledged the prior
// one.
func TestSeqOwnership_ResponseAloneDoesNotCompleteRequest(t *testing.T) {
	c := newConfigurator(gpsprot.NewConfigTarget(), &m10Version)
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})

	T0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.reqs[0].state = stateReadyToSend
	c.reqs[0].SetSentTime(T0)
	if got := c.reqs[0].state; got != stateAwaitingAckAndResponse {
		t.Fatalf("after send: got %v, want stateAwaitingAckAndResponse", got)
	}

	// Response only — no ACK yet.
	if _, err := c.processMsg(&ubxbin.CfgPrt{PortID: ubxbin.PortUART1}, T0.Add(time.Millisecond)); err != nil {
		t.Fatalf("processMsg response: %v", err)
	}

	switch c.reqs[0].state {
	case stateSucceeded:
		t.Fatal("response alone marked the request stateSucceeded - the recursive checkComplete bug is back")
	case statePausing:
		t.Fatal("response alone advanced the request to statePausing - the recursive checkComplete bug is back")
	}
	if got := c.reqs[0].state; got != stateAwaitingAck {
		t.Errorf("after response only: got %v, want stateAwaitingAck", got)
	}
	// From the director's perspective the request must still be in an
	// awaiting state, not anything terminal.
	if got := c.reqs[0].GetState(); got != gpsprot.ConfigRequestAwaitingResponse {
		t.Errorf("GetState after response only: got %v, want ConfigRequestAwaitingResponse", got)
	}
}

// TestSeqOwnership_ResponseBeforeAckPreservesAwaitingAck guards against the
// previous recursive checkComplete that prematurely succeeded a request as
// soon as the response arrived. With the fix, only the ACK can complete a
// request that needed both. It also exercises the related fix in
// checkPollResponses, which must not run a stateAwaitingAck request through
// checkComplete on every CFG message.
func TestSeqOwnership_ResponseBeforeAckPreservesAwaitingAck(t *testing.T) {
	c := newConfigurator(gpsprot.NewConfigTarget(), &m10Version)
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})

	T0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.reqs[0].state = stateReadyToSend
	c.reqs[0].SetSentTime(T0)

	if _, err := c.processMsg(&ubxbin.CfgPrt{PortID: ubxbin.PortUART1}, T0.Add(time.Millisecond)); err != nil {
		t.Fatalf("processMsg response: %v", err)
	}
	if got := c.reqs[0].state; got != stateAwaitingAck {
		t.Fatalf("after response only: got %v, want stateAwaitingAck", got)
	}

	// An unrelated CFG message must not falsely succeed an ACK-only-waiter.
	if _, err := c.processMsg(&ubxbin.CfgRate{}, T0.Add(2*time.Millisecond)); err != nil {
		t.Fatalf("processMsg unrelated CFG: %v", err)
	}
	if got := c.reqs[0].state; got != stateAwaitingAck {
		t.Errorf("after unrelated CFG: got %v, want stateAwaitingAck", got)
	}

	if _, err := c.processMsg(&ubxbin.AckAck{MsgID: ubxbin.CfgPrtID}, T0.Add(3*time.Millisecond)); err != nil {
		t.Fatalf("processMsg ack: %v", err)
	}
	if got := c.reqs[0].state; got != stateSucceeded {
		t.Errorf("after ack: got %v, want stateSucceeded", got)
	}
}

// TestSeqOwnership_WindowsConfigBugScenario reproduces the field-observed
// failure that motivated plan/ubx-config-response-sequence.md. On Windows
// the desktop app reported a spurious "missing config items" error during a
// CFG-VALGET sequence. Three CFG-VALGET requests (#0, #1, #2) were in
// flight; the response for #0, the send of #1, and the ACK for #0 all
// carried the same wall-clock timestamp T0. Under the old timestamp-based
// ownership, request #1 was misclassified as not needing a response and
// was prematurely succeeded by the ACK that actually belonged to request
// #0. The director then sent request #2, whose response was expected to
// carry items that should have come from request #1 — and so the
// configuration failed before request #1's real response arrived.
//
// Timeline:
//
//	send #0 at T0 - 2ms
//	receive response #0 at T0
//	send #1 at T0
//	receive ACK for #0 at T0
//	[invariant: send #2 must not happen yet]
//	receive response #1 at T0 + 4ms
//	receive ACK for #1 at T0 + 4ms
func TestSeqOwnership_WindowsConfigBugScenario(t *testing.T) {
	c := newConfigurator(gpsprot.NewConfigTarget(), &m10Version)

	mkValget := func() *ubxbin.CfgValget {
		return &ubxbin.CfgValget{
			CfgValgetFixed: ubxbin.CfgValgetFixed{
				Layer:   ubxbin.CfgValgetLayerRAM,
				Version: ubxbin.CfgValgetVersionRequest,
			},
		}
	}
	for range 3 {
		c.addRequest(msgPollRequest{msgRequest: msgRequest{mkValget()}, msgSeq: c.msgSeq})
	}

	valgetResp := &ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{
			Layer:   ubxbin.CfgValgetLayerRAM,
			Version: ubxbin.CfgValgetVersionResponse,
		},
	}
	valgetAck := &ubxbin.AckAck{MsgID: ubxbin.CfgValgetID}

	// Timestamps from the original observed timeline.
	T0 := time.Date(2026, 1, 1, 0, 58, 6, 875667000, time.UTC)

	// send #0 at T0 - 2ms.
	c.advanceSequentialProgress() // moves req[0] from stateNotReady to stateReadyToSend
	c.reqs[0].SetSentTime(T0.Add(-2 * time.Millisecond))

	// receive response #0 at T0.
	if _, err := c.processMsg(valgetResp, T0); err != nil {
		t.Fatalf("processMsg response#0: %v", err)
	}
	if got := c.reqs[0].state; got != stateAwaitingAck {
		t.Fatalf("reqs[0] after response: got %v, want stateAwaitingAck "+
			"(the recursive checkComplete must not have run)", got)
	}

	// send #1 at T0 — at the same simulated instant as response #0.
	// In production this happens because the driver wakes after response #0
	// updates state, then sees the prior request is no longer awaiting under
	// the old buggy code and sends the next one. We force the situation by
	// manually marking ready and calling SetSentTime, which is what the
	// director does in the action loop.
	c.reqs[1].state = stateReadyToSend
	c.reqs[1].SetSentTime(T0)

	// CRITICAL: under the old timestamp ownership, msgPollRequest's
	// AwaitingResponse(T0) compared tRead[CFG-VALGET]=T0 with sent T0 via
	// `Before`, which returns false — making the configurator think a
	// response had already arrived and putting the request in stateAwaitingAck
	// instead of stateAwaitingAckAndResponse. That is the bug.
	if got := c.reqs[1].state; got != stateAwaitingAckAndResponse {
		t.Fatalf("reqs[1] after equal-timestamp send: got %v, want stateAwaitingAckAndResponse "+
			"(sentSeq=%d, msgSeq=%d) — sequence ownership is not protecting against the stale response",
			got, c.reqs[1].sentSeq, c.msgSeq[ubxbin.CfgValgetID])
	}

	// receive ACK for #0 at T0 — same timestamp as request #1's send.
	if _, err := c.processMsg(valgetAck, T0); err != nil {
		t.Fatalf("processMsg ack#0: %v", err)
	}

	// The ACK belongs to #0. Iteration order plus the sentSeq predicate must
	// route it to req[0], not the still-awaiting req[1].
	if got := c.reqs[0].state; got != stateSucceeded {
		t.Errorf("reqs[0]: got %v, want stateSucceeded", got)
	}
	if got := c.reqs[1].state; got != stateAwaitingAckAndResponse {
		t.Errorf("reqs[1] after stale-instant ACK: got %v, want stateAwaitingAckAndResponse "+
			"(sentSeq=%d, eventSeq=%d) — the ACK was misrouted to a later request",
			got, c.reqs[1].sentSeq, c.eventSeq)
	}

	// Plan invariant: request #2 must not have been advanced. The driver
	// advances the next-pending request only when the previous succeeds; with
	// req[1] still awaiting, req[2] must remain stateNotReady.
	if got := c.reqs[2].state; got != stateNotReady {
		t.Errorf("reqs[2]: got %v, want stateNotReady — director would have prematurely sent #2", got)
	}

	// receive response #1 at T0 + 4ms.
	if _, err := c.processMsg(valgetResp, T0.Add(4*time.Millisecond)); err != nil {
		t.Fatalf("processMsg response#1: %v", err)
	}
	if got := c.reqs[1].state; got != stateAwaitingAck {
		t.Errorf("reqs[1] after fresh response: got %v, want stateAwaitingAck", got)
	}

	// receive ACK for #1 at T0 + 4ms.
	if _, err := c.processMsg(valgetAck, T0.Add(4*time.Millisecond)); err != nil {
		t.Fatalf("processMsg ack#1: %v", err)
	}
	if got := c.reqs[1].state; got != stateSucceeded {
		t.Errorf("reqs[1]: got %v, want stateSucceeded", got)
	}

	// req[2] must have been untouched throughout the entire bug scenario.
	// The whole point of the regression test is that no event between T0-2ms
	// and T0+4ms should have advanced req[2] off stateNotReady.
	if got := c.reqs[2].state; got != stateNotReady {
		t.Errorf("reqs[2] must remain stateNotReady throughout: got %v", got)
	}
}

// TestSeqOwnership_NackIterationOwnership documents the iteration-order
// half of NACK ownership: when two same-MsgID requests are simultaneously
// awaiting, processAckNak iterates and breaks on the first match, so a
// NACK is consumed by the earliest still-awaiting request. Both requests
// are sent before any inbound event here, so the sentSeq predicate is
// not what protects reqs[1] — iteration order is. The strict-greater-than
// boundary itself is covered by TestSeqOwnership_AckNackBoundaryPredicate.
func TestSeqOwnership_NackIterationOwnership(t *testing.T) {
	c := newConfigurator(gpsprot.NewConfigTarget(), &m10Version)
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})

	T0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c.reqs[0].state = stateReadyToSend
	c.reqs[0].SetSentTime(T0.Add(-time.Millisecond))
	c.reqs[1].state = stateReadyToSend
	c.reqs[1].SetSentTime(T0)
	if got := c.reqs[1].state; got != stateAwaitingAckAndResponse {
		t.Fatalf("reqs[1] after send: got %v, want stateAwaitingAckAndResponse", got)
	}

	if _, err := c.processMsg(&ubxbin.AckNak{MsgID: ubxbin.CfgPrtID}, T0); err != nil {
		t.Fatalf("processMsg nak: %v", err)
	}

	if got := c.reqs[0].state; got != stateFailed {
		t.Errorf("reqs[0] after NACK: got %v, want stateFailed", got)
	}
	if got := c.reqs[1].state; got != stateAwaitingAckAndResponse {
		t.Errorf("reqs[1] must not be failed by the NACK consumed by reqs[0]: "+
			"got %v, want stateAwaitingAckAndResponse",
			got)
	}
}

// TestSeqOwnership_AckNackBoundaryPredicate exercises the strict-
// greater-than predicate in processAckNak directly, per the plan: "An
// ACK/NACK with sequence equal to the request's send boundary does not
// satisfy or fail the request." This is the line-490 fix in isolation.
// The natural event flow guarantees eventSeq > sentSeq for any new ACK
// (since nextEventSeq increments before passing the value), but the
// predicate is the documented contract; we pin it down by calling
// processAckNak directly with eventSeq < sentSeq, eventSeq == sentSeq,
// and eventSeq > sentSeq.
//
// We seed eventSeq with two unrelated CFG-class events before sending, so
// the request's sentSeq is non-zero and the eventSeq < sentSeq branch is
// genuinely exercised rather than vacuous.
func TestSeqOwnership_AckNackBoundaryPredicate(t *testing.T) {
	c := newConfigurator(gpsprot.NewConfigTarget(), &m10Version)
	c.addRequest(pollRequest{c.msgSeq, ubxbin.CfgPrtID})

	T0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Drive eventSeq forward so the request's sentSeq lands above 0.
	if _, err := c.processMsg(&ubxbin.CfgRate{}, T0); err != nil {
		t.Fatalf("seed processMsg: %v", err)
	}
	if _, err := c.processMsg(&ubxbin.CfgRate{}, T0); err != nil {
		t.Fatalf("seed processMsg: %v", err)
	}

	c.reqs[0].state = stateReadyToSend
	c.reqs[0].SetSentTime(T0)
	sentSeq := c.reqs[0].sentSeq
	if sentSeq < 2 {
		t.Fatalf("seed did not advance sentSeq: got %d, want >= 2", sentSeq)
	}

	// eventSeq < sentSeq must not match (NACK).
	c.processAckNak(ubxbin.CfgPrtID, false, T0, sentSeq-1)
	if got := c.reqs[0].state; got == stateFailed {
		t.Errorf("NACK with eventSeq<sentSeq matched: state=%v", got)
	}

	// eventSeq == sentSeq must not match (NACK).
	c.processAckNak(ubxbin.CfgPrtID, false, T0, sentSeq)
	if got := c.reqs[0].state; got == stateFailed {
		t.Errorf("NACK with eventSeq==sentSeq matched: state=%v", got)
	}

	// eventSeq == sentSeq must not match (ACK).
	c.processAckNak(ubxbin.CfgPrtID, true, T0, sentSeq)
	if got := c.reqs[0].state; got != stateAwaitingAckAndResponse {
		t.Errorf("ACK with eventSeq==sentSeq advanced state: got %v, want stateAwaitingAckAndResponse", got)
	}

	// eventSeq == sentSeq+1 (strictly greater) must match the NACK and fail
	// the request — confirming the predicate uses '>', not '>='.
	c.processAckNak(ubxbin.CfgPrtID, false, T0, sentSeq+1)
	if got := c.reqs[0].state; got != stateFailed {
		t.Errorf("NACK with strictly-greater eventSeq did not fail the request: got %v", got)
	}
}

func TestMonCommsPort(t *testing.T) {
	tests := []struct {
		name   string
		mc     ubxbin.MonComms
		want   ubxbin.PortID
		wantOK bool
	}{
		{
			name: "output port set",
			mc: ubxbin.MonComms{
				MonCommsFixed: ubxbin.MonCommsFixed{
					TxErrors: ubxbin.MonCommsTxErrors(ubxbin.PortUART2+1) << 2,
					NPorts:   4,
				},
				Ports: []ubxbin.MonCommsPort{
					{PortID: ubxbin.MonCommsPortIDI2C},
					{PortID: ubxbin.MonCommsPortIDUART1},
					{PortID: ubxbin.MonCommsPortIDUART2},
					{PortID: ubxbin.MonCommsPortIDUSB},
				},
			},
			want:   ubxbin.PortUART2,
			wantOK: true,
		},
		{
			name: "single port",
			mc: ubxbin.MonComms{
				MonCommsFixed: ubxbin.MonCommsFixed{
					NPorts: 1,
				},
				Ports: []ubxbin.MonCommsPort{
					{PortID: ubxbin.MonCommsPortIDUART1},
				},
			},
			want:   ubxbin.PortUART1,
			wantOK: true,
		},
		{
			name: "multiple ports no output port",
			mc: ubxbin.MonComms{
				MonCommsFixed: ubxbin.MonCommsFixed{
					NPorts: 2,
				},
				Ports: []ubxbin.MonCommsPort{
					{PortID: ubxbin.MonCommsPortIDUART1},
					{PortID: ubxbin.MonCommsPortIDUSB},
				},
			},
			wantOK: false,
		},
		{
			name:   "no ports",
			mc:     ubxbin.MonComms{},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := monCommsPort(&tt.mc)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("monCommsPort() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
