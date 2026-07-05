package as

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/latin1z"
)

// testReceiver simulates an Allystar receiver at the packet level.
// Requests are interpreted from raw packet bytes and answered with raw
// packet bytes, which the test feeds through the real PacketProcessor.
// It mimics DISCOVERED hardware behavior: polls are answered by the
// message itself with no ACK, unknown CFG ids are SILENT to polls,
// unknown CFG-MSG targets NAK both poll and set forms, and sets are
// acknowledged with ACK-ACK/ACK-NAK naming class+id.
type testReceiver struct {
	t              *testing.T
	monVer         *asbin.MonVer
	rates          map[asbin.MsgID]uint8
	naks           map[asbin.MsgID]bool // set requests answered with NAK
	nakTargets     map[asbin.MsgID]bool // CFG-MSG targets answered with NAK
	silent         map[asbin.MsgID]bool // requests not answered at all
	deferIDs       map[asbin.MsgID]bool // ids whose responses are deferred (see pending)
	pending        [][]byte             // deferred responses, delivered after a later request's responses
	rawOn          uint8                // RXM-DUMPRAW state
	nakRawOff      bool                 // TAU951M quirk: NAK the raw disable yet apply it
	saves          []asbin.CfgCfg       // CFG-CFG save/load requests received (ACKed)
	clears         []asbin.CfgCfg       // CFG-CFG clear requests received (silent, like hardware)
	resets         []asbin.CfgSimpleRst // SIMPLERST requests received (silent)
	pps            *asbin.CfgPps        // nil: CFG-PPS unsupported (poll is silent)
	fixedEcef      *asbin.CfgFixedECEF  // nil: unsupported (poll is silent)
	survey         *asbin.CfgSurvey     // nil: unsupported (poll is silent)
	navSat         *asbin.CfgNavSat     // nil: unsupported (poll is silent)
	sigCap         asbin.CfgNavSatMask  // hardware capability; clamps written masks
	prt            [2]uint32            // stored CFG-PRT records
	liveRate       uint32               // the live rate of the arriving port
	elev           asbin.CfgElev        // CFG-ELEV store (always present: every unit has it)
	ignoreZeroDuty bool                 // TAU1302 quirk: ACK a zero duty cycle without applying it
}

func (r *testReceiver) takePending() [][]byte {
	p := r.pending
	r.pending = nil
	return p
}

// respond interprets one request write and returns the receiver's
// response packets. Responses to deferred ids are stashed and
// delivered after a later request's responses (see configure),
// simulating the cross-id response reordering the hardware does.
func (r *testReceiver) respond(data []byte) [][]byte {
	out := r.respondOne(data)
	if r.deferIDs[asbin.PacketMsgId(data)] {
		r.pending = append(r.pending, out...)
		return nil
	}
	return out
}

func (r *testReceiver) respondOne(data []byte) [][]byte {
	mid := asbin.PacketMsgId(data)
	if r.silent[mid] {
		return nil
	}
	if len(data) == asbin.PacketMinLen+asbin.PollPayloadLen(mid) {
		return r.respondPoll(mid, data)
	}
	m, err := asbin.ParseMsg(string(data))
	if err != nil {
		r.t.Fatalf("receiver could not parse request: %v", err)
	}
	if r.naks[mid] {
		return [][]byte{r.nak(mid)}
	}
	switch mt := m.(type) {
	case *asbin.CfgMsg:
		target := asbin.MakeMsgID(mt.MsgClass, mt.MsgID)
		if r.nakTargets[target] {
			return [][]byte{r.nak(asbin.CfgMsgID)}
		}
		if r.rates == nil {
			r.rates = make(map[asbin.MsgID]uint8)
		}
		r.rates[target] = mt.Rate
		return [][]byte{r.ack(asbin.CfgMsgID)}
	case *asbin.RxmDumpRaw:
		r.rawOn = mt.Enable
		if mt.Enable == 0 && r.nakRawOff {
			return [][]byte{r.nak(asbin.RxmDumpRawID)}
		}
		return [][]byte{r.ack(asbin.RxmDumpRawID)}
	case *asbin.CfgCfg:
		if mt.Action == asbin.CfgCfgActionClear {
			r.clears = append(r.clears, *mt)
			return nil
		}
		r.saves = append(r.saves, *mt)
		return [][]byte{r.ack(asbin.CfgCfgID)}
	case *asbin.CfgSimpleRst:
		r.resets = append(r.resets, *mt)
		return nil
	case *asbin.CfgPps:
		if r.pps == nil {
			return [][]byte{r.nak(asbin.CfgPpsID)}
		}
		if mt.DutyCycle == 0 && r.ignoreZeroDuty {
			// TAU1302 quirk: a zero duty cycle is ACKed and ignored
			return [][]byte{r.ack(asbin.CfgPpsID)}
		}
		*r.pps = *mt
		return [][]byte{r.ack(asbin.CfgPpsID)}
	case *asbin.CfgFixedECEF:
		if r.fixedEcef == nil {
			return [][]byte{r.nak(asbin.CfgFixedECEFID)}
		}
		*r.fixedEcef = *mt
		return [][]byte{r.ack(asbin.CfgFixedECEFID)}
	case *asbin.CfgSurvey:
		if r.survey == nil {
			return [][]byte{r.nak(asbin.CfgSurveyID)}
		}
		*r.survey = *mt
		return [][]byte{r.ack(asbin.CfgSurveyID)}
	case *asbin.CfgNavSat:
		if r.navSat == nil {
			return [][]byte{r.nak(asbin.CfgNavSatID)}
		}
		// the silicon ACKs and clamps to the hardware capability
		r.navSat.EnableMask = mt.EnableMask & r.sigCap
		return [][]byte{r.ack(asbin.CfgNavSatID)}
	case *asbin.CfgElev:
		r.elev = *mt
		return [][]byte{r.ack(asbin.CfgElevID)}
	case *asbin.CfgPrt:
		r.prt[mt.PortID&1] = mt.Baudrate
		if mt.Baudrate != r.liveRate {
			// the arriving port switches immediately; the ACK is
			// destroyed by the transition (hardware behavior)
			r.liveRate = mt.Baudrate
			return nil
		}
		return [][]byte{r.ack(asbin.CfgPrtID)}
	}
	if mid.Ackable() {
		return [][]byte{r.ack(mid)}
	}
	return nil
}

// respondPoll answers a poll: the data message echoing the id, no ACK.
// An id the firmware lacks is silent; a CFG-MSG poll of an unknown
// target NAKs.
func (r *testReceiver) respondPoll(mid asbin.MsgID, data []byte) [][]byte {
	switch mid {
	case asbin.MonVerID:
		if r.monVer == nil {
			return nil
		}
		return [][]byte{r.pack(r.monVer)}
	case asbin.CfgMsgID:
		target := asbin.MakeMsgID(data[asbin.HeaderLen], data[asbin.HeaderLen+1])
		if r.nakTargets[target] {
			return [][]byte{r.nak(asbin.CfgMsgID)}
		}
		pkt, _ := asbin.PackMsg(asbin.CfgMsgID,
			[]byte{data[asbin.HeaderLen], data[asbin.HeaderLen+1], r.rates[target]})
		return [][]byte{pkt}
	case asbin.CfgPpsID:
		if r.pps == nil {
			return nil // unknown CFG ids are silent to polls
		}
		return [][]byte{r.pack(r.pps)}
	case asbin.CfgFixedECEFID:
		if r.fixedEcef == nil {
			return nil
		}
		return [][]byte{r.pack(r.fixedEcef)}
	case asbin.CfgSurveyID:
		if r.survey == nil {
			return nil
		}
		return [][]byte{r.pack(r.survey)}
	case asbin.CfgNavSatID:
		if r.navSat == nil {
			return nil
		}
		return [][]byte{r.pack(r.navSat)}
	case asbin.CfgPrtID:
		port := data[asbin.HeaderLen] & 1
		return [][]byte{r.pack(&asbin.CfgPrt{PortID: port, Baudrate: r.prt[port]})}
	case asbin.CfgElevID:
		return [][]byte{r.pack(&r.elev)}
	}
	return nil
}

func (r *testReceiver) pack(m asbin.Msg) []byte {
	pkt, err := asbin.Serialize(m)
	if err != nil {
		r.t.Fatalf("serialize %T: %v", m, err)
	}
	return pkt
}

func (r *testReceiver) ack(mid asbin.MsgID) []byte {
	cls, id := mid.Unpack()
	return r.pack(&asbin.AckAck{MsgClass: cls, MsgID: id})
}

func (r *testReceiver) nak(mid asbin.MsgID) []byte {
	cls, id := mid.Unpack()
	return r.pack(&asbin.AckNak{MsgClass: cls, MsgID: id})
}

func z16(s string) latin1z.StringZ16 {
	var z latin1z.StringZ16
	copy(z[:], s)
	return z
}

func tau1201Ver() *asbin.MonVer {
	return &asbin.MonVer{
		SwVersion: z16("3.018.aab95e7"),
		HwVersion: z16("HD8040D.9529b663"),
	}
}

// probe sends the probe packet to the receiver and feeds its responses
// through the real PacketProcessor, returning the ConfigProtocol.
func probe(t *testing.T, rcvr *testReceiver) *ConfigProtocol {
	rcvr.t = t
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	cp := NewConfigProtocol()
	pp.SetNativeMsgHandler(cp)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, resp := range rcvr.respond(cp.ProbePacket()) {
		if _, err := pp.ProcessPacket(string(resp), t0); err != nil {
			t.Fatalf("ProcessPacket: %v", err)
		}
		t0 = t0.Add(5 * time.Millisecond)
	}
	return cp
}

// configure runs the full ConfigDirector loop against the test
// receiver, feeding all responses through the real PacketProcessor.
func configure(t *testing.T, cp *ConfigProtocol, rcvr *testReceiver, target *gpsprot.ConfigTarget) (*Configurator, int) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetNativeMsgHandler(cp)
	cfgI, err := cp.Configure(target)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg := cfgI.(*Configurator)
	director := gpsprot.NewConfigDirector(cfg, 2)
	t0 := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	for action := range director.Actions() {
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			t0 = t0.Add(10 * time.Millisecond)
			cfg.Request(action.Index).SetSentTime(t0)
			for _, resp := range append(rcvr.respond(action.Packet), rcvr.takePending()...) {
				t0 = t0.Add(5 * time.Millisecond)
				if _, err := pp.ProcessPacket(string(resp), t0); err != nil {
					t.Fatalf("ProcessPacket: %v", err)
				}
				director.ValidPacketReceived(t0)
			}
		case gpsprot.ConfigActionWaitUntil:
			if action.Deadline.After(t0) {
				t0 = action.Deadline.Add(time.Millisecond)
			}
			director.AdvanceTimeTo(t0)
		}
	}
	return cfg, director.ErrorCount
}

func TestProbe(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false after MON-VER response")
	}
	cfg, errCount := configure(t, cp, rcvr, &gpsprot.ConfigTarget{})
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	got := cfg.ReceiverInfo()
	want := &gpsprot.ReceiverInfo{
		Vendor:        Vendor,
		Firmware:      "3.018.aab95e7",
		Hardware:      "HD8040D.9529b663",
		SupportedGNSS: navSatToSignals(tau1201Cap).GNSSSet(),
	}
	if got.VendorSpecific == nil {
		t.Error("VendorSpecific = nil, want MON-VER message")
	}
	want.VendorSpecific = got.VendorSpecific
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReceiverInfo\ngot  %+v\nwant %+v", got, want)
	}
}

func TestProbeSilentReceiver(t *testing.T) {
	rcvr := &testReceiver{} // no MON-VER answer: not an Allystar receiver
	cp := probe(t, rcvr)
	if cp.ProbeOK() {
		t.Fatal("ProbeOK = true with no MON-VER response")
	}
}

func TestNMEAOut(t *testing.T) {
	tests := []struct {
		name   string
		flags  gpsprot.NMEAMsgFlags
		expect map[asbin.MsgID]uint8
	}{
		{
			name:  "rmc_gga",
			flags: gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA,
			expect: map[asbin.MsgID]uint8{
				asbin.NmeaGsvID: 0, asbin.NmeaRmcID: 1, asbin.NmeaGgaID: 1,
				asbin.NmeaGsaID: 0, asbin.NmeaZdaID: 0, asbin.NmeaVtgID: 0,
				asbin.NmeaGllID: 0,
			},
		},
		{
			name:  "none",
			flags: gpsprot.NMEAMsgNone,
			expect: map[asbin.MsgID]uint8{
				asbin.NmeaGsvID: 0, asbin.NmeaRmcID: 0, asbin.NmeaGgaID: 0,
				asbin.NmeaGsaID: 0, asbin.NmeaZdaID: 0, asbin.NmeaVtgID: 0,
				asbin.NmeaGllID: 0,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.NMEAMsg.Set(tc.flags)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.rates, tc.expect) {
				t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, tc.expect)
			}
		})
	}
}

func TestPVTOut(t *testing.T) {
	tests := []struct {
		name   string
		flags  gpsprot.PVTMsgFlags
		expect map[asbin.MsgID]uint8
	}{
		{
			name:  "pos_time_incremental",
			flags: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
			expect: map[asbin.MsgID]uint8{
				asbin.NavPosLlhID: 1, asbin.NavTimeUtcID: 1,
			},
		},
		{
			name:  "pos_vel_ecef_off",
			flags: gpsprot.PVTMsgPos | gpsprot.PVTMsgVel | gpsprot.PVTMsgECEF | gpsprot.PVTMsgOff,
			expect: map[asbin.MsgID]uint8{
				asbin.NavPosEcefID: 1, asbin.NavVelEcefID: 1,
				asbin.NavPosLlhID: 0, asbin.NavVelNedID: 0,
				asbin.NavTimeUtcID: 0, asbin.NavTimeID: 0,
				asbin.NavDopID: 0, asbin.NavAutoID: 0, asbin.NavSvinID: 0,
			},
		},
		{
			name:  "tai_time",
			flags: gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			expect: map[asbin.MsgID]uint8{
				asbin.NavTimeID: 1,
			},
		},
		{
			name:  "leap_and_qual",
			flags: gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgQuality,
			expect: map[asbin.MsgID]uint8{
				asbin.NavTimeID: 1, asbin.NavDopID: 1, asbin.NavAutoID: 1,
			},
		},
		{
			// tp and epoch have no carrier: nothing is enabled and the
			// absence in the output is the statement.
			name:   "tp_epoch_absent",
			flags:  gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgEpoch,
			expect: nil,
		},
		{
			// the PTP timing preset: the after option makes the GNSS
			// time message stand in for the absent pulse-time message
			name:  "ptp_preset",
			flags: gpsprot.PVTMsgTimingPTP,
			expect: map[asbin.MsgID]uint8{
				asbin.NavTimeID: 1, asbin.NavDopID: 1, asbin.NavAutoID: 1,
				asbin.NavSvinID: 1,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.PVTMsg.Set(tc.flags)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.rates, tc.expect) {
				t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, tc.expect)
			}
		})
	}
}

func TestSatsOut(t *testing.T) {
	tests := []struct {
		name   string
		flags  gpsprot.SatsMsgFlags
		expect map[asbin.MsgID]uint8
	}{
		{
			name:   "sat",
			flags:  gpsprot.SatsMsgSat,
			expect: map[asbin.MsgID]uint8{asbin.NavSvInfoID: 1},
		},
		{
			// per-signal information has no carrier; a signal-only
			// request is complete, so satellite info is turned off
			name:   "sig_only",
			flags:  gpsprot.SatsMsgSignal,
			expect: map[asbin.MsgID]uint8{asbin.NavSvInfoID: 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.SatsMsg.Set(tc.flags)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.rates, tc.expect) {
				t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, tc.expect)
			}
		})
	}
}

func TestPVTOutMissingMessage(t *testing.T) {
	// A firmware lacking a PVT carrier NAKs its enable (the TAU951M
	// NAKs NAV-SVSTATE targets, for example); the information shows as
	// absence, never as an error.
	rcvr := &testReceiver{
		monVer:     tau1201Ver(),
		nakTargets: map[asbin.MsgID]bool{asbin.NavSvinID: true},
	}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgSurvey | gpsprot.PVTMsgTime)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0: a missing carrier is absence, not an error", errCount)
	}
	if rcvr.rates[asbin.NavTimeUtcID] != 1 {
		t.Errorf("TIMEUTC rate = %d, want 1", rcvr.rates[asbin.NavTimeUtcID])
	}
}

func TestRTCMOut(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	expect := map[asbin.MsgID]uint8{asbin.RtcmArpID: 1}
	for _, mid := range rtcmMSM4IDs {
		expect[mid] = 1
	}
	for _, mid := range rtcmMSM7IDs {
		expect[mid] = 0
	}
	if !reflect.DeepEqual(rcvr.rates, expect) {
		t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, expect)
	}
}

func TestRTCMOutAbsent(t *testing.T) {
	// The TAU1201 NAKs every 0xF8 target: an RTCM request achieves
	// nothing and reports no error - absence, not failure.
	nakAll := make(map[asbin.MsgID]bool)
	for _, mid := range rtcmMSM4IDs {
		nakAll[mid] = true
	}
	for _, mid := range rtcmMSM7IDs {
		nakAll[mid] = true
	}
	nakAll[asbin.RtcmArpID] = true
	rcvr := &testReceiver{monVer: tau1201Ver(), nakTargets: nakAll}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgAuto)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0: no RTCM output shows as absence", errCount)
	}
	if len(rcvr.rates) != 0 {
		t.Errorf("rates = %v, want none applied", rcvr.rates)
	}
}

func TestRawOut(t *testing.T) {
	tests := []struct {
		name      string
		flags     gpsprot.RawMsgFlags
		nakRawOff bool
		expect    uint8
	}{
		{name: "obs", flags: gpsprot.RawMsgObs, expect: 1},
		{name: "nav", flags: gpsprot.RawMsgNavData, expect: 1},
		{name: "off", flags: gpsprot.RawMsgNone, expect: 0},
		{
			// TAU951M quirk: the disable is NAKed yet applied; the NAK
			// must not surface as an error
			name: "off_nak_quirk", flags: gpsprot.RawMsgNone,
			nakRawOff: true, expect: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver(), rawOn: 1, nakRawOff: tc.nakRawOff}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.RawMsg.Set(tc.flags)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if rcvr.rawOn != tc.expect {
				t.Errorf("rawOn = %d, want %d", rcvr.rawOn, tc.expect)
			}
		})
	}
}

// tau951mPps returns the TAU951M's as-found CFG-PPS values, factory
// 530 ns offset included.
func tau951mPps() *asbin.CfgPps {
	return &asbin.CfgPps{
		Period:    1000000,
		Offset:    530,
		DutyCycle: 10000,
		Polarity:  asbin.CfgPpsPolarityFallingEdge,
		GPIO:      13,
		Sync:      asbin.CfgPpsSyncOnlyWithFix,
	}
}

func TestTimePulseSet(t *testing.T) {
	tests := []struct {
		name       string
		width      time.Duration
		expectPps  asbin.CfgPps
		expectWant gpsprot.TimePulse
	}{
		{
			// setting the width merges into the readback: period, GPIO,
			// and the factory offset are preserved
			name:  "width_100ms",
			width: 100 * time.Millisecond,
			expectPps: asbin.CfgPps{Period: 1000000, Offset: 530, DutyCycle: 100000,
				Polarity: asbin.CfgPpsPolarityFallingEdge, GPIO: 13,
				Sync: asbin.CfgPpsSyncOnlyWithFix},
		},
		{
			name:  "width_zero_disables",
			width: 0,
			expectPps: asbin.CfgPps{Period: 1000000, Offset: 530, DutyCycle: 0,
				Polarity: asbin.CfgPpsPolarityFallingEdge, GPIO: 13,
				Sync: asbin.CfgPpsSyncOnlyWithFix},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver(), pps: tau951mPps()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Props.SetTimePulseWidth(tc.width)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(*rcvr.pps, tc.expectPps) {
				t.Errorf("receiver PPS\ngot  %+v\nwant %+v", *rcvr.pps, tc.expectPps)
			}
			props := cfg.ConfigProps()
			w, ok := props.GetTimePulseWidth()
			if !ok || w != tc.width {
				t.Errorf("achieved width = %v/%v, want %v", w, ok, tc.width)
			}
		})
	}
}

func TestTimePulseZeroDutyIgnored(t *testing.T) {
	// TAU1302 defect: a zero duty cycle can be acknowledged without
	// being stored. Per the semantics the achieved value is what the
	// receiver ACCEPTED - the set exchange reports width 0, with no
	// verify roundtrip to work around the defect. The discrepancy is
	// receiver characterization, visible on a later readback.
	rcvr := &testReceiver{monVer: tau1201Ver(), pps: tau951mPps(), ignoreZeroDuty: true}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulseWidth(0)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if w, ok := cfg.ConfigProps().GetTimePulseWidth(); !ok || w != 0 {
		t.Errorf("achieved width = %v/%v, want the accepted 0", w, ok)
	}
	if rcvr.pps.DutyCycle == 0 {
		t.Error("fake stored the zero duty; it must model the defect")
	}
}

func TestTimePulseReadback(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver(), pps: tau951mPps()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: tpProps}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	props := cfg.ConfigProps()
	if w, ok := props.GetTimePulseWidth(); !ok || w != 10*time.Millisecond {
		t.Errorf("width = %v/%v, want 10ms", w, ok)
	}
	if p, ok := props.GetTimePulsePeriod(); !ok || p != time.Second {
		t.Errorf("period = %v/%v, want 1s", p, ok)
	}
	if r, ok := props.GetTimePulsePolarityRising(); !ok || r {
		t.Errorf("polarityRising = %v/%v, want false", r, ok)
	}
	if l, ok := props.GetTimePulseOnlyWhenLocked(); !ok || !l {
		t.Errorf("onlyWhenLocked = %v/%v, want true", l, ok)
	}
	if a, ok := props.GetTimePulseAlignToGNSS(); !ok || !a {
		t.Errorf("alignToGNSS = %v/%v, want true (the pulse is always GNSS-aligned)", a, ok)
	}
}

func TestTimePulseAbsent(t *testing.T) {
	// A receiver without CFG-PPS is silent to the poll: the property
	// shows as absence - nothing set, nothing reported, no error.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulseWidth(100 * time.Millisecond)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if _, ok := cfg.ConfigProps().GetTimePulseWidth(); ok {
		t.Error("width reported for a receiver without CFG-PPS")
	}
}

func TestTimeMode(t *testing.T) {
	surveyed := asbin.CfgFixedECEF{X: -114469630, Y: 609033320, Z: 150417057}
	surveyedMode := gpsprot.Mode{Static: true, PosType: gpsprot.PosTypeECEF,
		FixedPosECEF: gpsprot.Point3D{
			gpsprot.Meters(-1144696.30), gpsprot.Meters(6090333.20), gpsprot.Meters(1504170.57)}}
	tests := []struct {
		name       string
		mode       *gpsprot.Mode
		setStatic  bool
		surveyOpts gpsprot.Survey
		asFoundPos asbin.CfgFixedECEF
		asFoundSvy asbin.CfgSurvey
		expectPos  asbin.CfgFixedECEF
		expectSvy  asbin.CfgSurvey
		expectMode gpsprot.Mode
	}{
		{
			name:       "mobile_from_fixed",
			mode:       &gpsprot.Mode{Static: false},
			asFoundPos: surveyed,
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectMode: gpsprot.Mode{Static: false},
		},
		{
			name:       "fixed_pos_ecef",
			mode:       &surveyedMode,
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectPos:  surveyed,
			expectMode: surveyedMode,
		},
		{
			name:       "survey_in",
			mode:       &gpsprot.Mode{Static: true},
			surveyOpts: gpsprot.Survey{MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundPos: surveyed,
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			name:       "setstatic_preserves_fixed",
			setStatic:  true,
			asFoundPos: surveyed,
			expectPos:  surveyed,
			expectMode: surveyedMode,
		},
		{
			name:       "setstatic_preserves_running_survey",
			setStatic:  true,
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectSvy:  asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			name:       "setstatic_idle_starts_survey",
			setStatic:  true,
			surveyOpts: gpsprot.Survey{MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
		{
			name:       "setstatic_survey_again_restarts",
			setStatic:  true,
			surveyOpts: gpsprot.Survey{Flags: gpsprot.SurveyAgain, MinDur: 5 * time.Minute, AccLimit: gpsprot.Meters(10)},
			asFoundSvy: asbin.CfgSurvey{MinDur: 40, AccLimit: 100000},
			expectSvy:  asbin.CfgSurvey{MinDur: 300, AccLimit: 10000},
			expectMode: gpsprot.Mode{Static: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pos, svy := tc.asFoundPos, tc.asFoundSvy
			rcvr := &testReceiver{monVer: tau1201Ver(), fixedEcef: &pos, survey: &svy}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			if tc.mode != nil {
				target.Props.SetMode(*tc.mode)
			}
			target.Opts.SetStatic = tc.setStatic
			target.Opts.Survey = tc.surveyOpts
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(*rcvr.fixedEcef, tc.expectPos) {
				t.Errorf("FIXEDECEF\ngot  %+v\nwant %+v", *rcvr.fixedEcef, tc.expectPos)
			}
			if !reflect.DeepEqual(*rcvr.survey, tc.expectSvy) {
				t.Errorf("SURVEY\ngot  %+v\nwant %+v", *rcvr.survey, tc.expectSvy)
			}
			got, ok := cfg.ConfigProps().GetMode()
			if !ok || !reflect.DeepEqual(got, tc.expectMode) {
				t.Errorf("mode = %+v/%v, want %+v", got, ok, tc.expectMode)
			}
		})
	}
}

// tau1201Cap is the TAU1201/TAU951M hardware signal plan (mask
// 0x4108237): GPS L1CA+L5, GLO G1, BDS B1I+B2a, GAL E1+E5a, QZSS L1+L5.
const tau1201Cap = asbin.CfgNavSatMask(0x4108237)

func TestTimeModeAbsent(t *testing.T) {
	// A receiver without the registers is silent to both polls: the
	// mode property shows as absence, nothing is written, no error.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: true})
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if _, ok := cfg.ConfigProps().GetMode(); ok {
		t.Error("mode reported for a receiver without the registers")
	}
}

func TestReorderedResponses(t *testing.T) {
	// Cross-id response reordering (the hardware does it): the time-mode
	// readback polls two distinct ids, and the CFG-FIXEDECEF answer is
	// deferred until after the CFG-SURVEY answer. Correlation by
	// class+id must attribute both correctly.
	rcvr := &testReceiver{monVer: tau1201Ver(),
		deferIDs:  map[asbin.MsgID]bool{asbin.CfgFixedECEFID: true},
		fixedEcef: &asbin.CfgFixedECEF{X: -114469630, Y: 609033320, Z: 150417057},
		survey:    &asbin.CfgSurvey{}}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: gpsprot.PropIDMode}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if m, ok := cfg.ConfigProps().GetMode(); !ok || !m.Static || m.PosType != gpsprot.PosTypeECEF {
		t.Errorf("mode = %+v/%v, want static with a fixed ECEF position", m, ok)
	}
}

func TestSignals(t *testing.T) {
	tests := []struct {
		name       string
		requested  gpsprot.SignalSet
		expectMask asbin.CfgNavSatMask
	}{
		{
			// the L2C/L1C bits are written but the silicon clamps them
			// away; the verify poll reveals the achieved set
			name:       "gps_only",
			requested:  gpsprot.SigSetGPS,
			expectMask: 0x201, // GPS L1CA + L5
		},
		{
			name:       "everything",
			requested:  gpsprot.SigSetAll,
			expectMask: tau1201Cap,
		},
		{
			name:       "gps_l1ca_alone",
			requested:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
			expectMask: 0x1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver(),
				navSat: &asbin.CfgNavSat{EnableMask: tau1201Cap}, sigCap: tau1201Cap}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Props.SetSignalsEnabled(tc.requested)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if rcvr.navSat.EnableMask != tc.expectMask {
				t.Errorf("receiver mask = %#x, want %#x", rcvr.navSat.EnableMask, tc.expectMask)
			}
			got, ok := cfg.ConfigProps().GetSignalsEnabled()
			if !ok || got != navSatToSignals(tc.expectMask) {
				t.Errorf("achieved = %v/%v, want %v", got, ok, navSatToSignals(tc.expectMask))
			}
		})
	}
}

func TestSignalsUnknownHardware(t *testing.T) {
	// An unknown model has no identity-deduced plan: the full request
	// goes to the wire and the silicon's clamp, revealed by the verify
	// poll, supplies the achieved set.
	ver := &asbin.MonVer{SwVersion: z16("9.999"), HwVersion: z16("HD9999.0")}
	rcvr := &testReceiver{monVer: ver,
		navSat: &asbin.CfgNavSat{EnableMask: tau1201Cap}, sigCap: tau1201Cap}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(gpsprot.SigSetGPS)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.navSat.EnableMask != 0x201 {
		t.Errorf("receiver mask = %#x, want 0x201 (clamped by the silicon)", rcvr.navSat.EnableMask)
	}
	want := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL5)
	if got, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || got != want {
		t.Errorf("achieved = %v/%v, want %v", got, ok, want)
	}
}

func TestSignalsReadback(t *testing.T) {
	// TAU1302 as-found: a selection narrower than its capability
	rcvr := &testReceiver{monVer: tau1201Ver(),
		navSat: &asbin.CfgNavSat{EnableMask: 0x40415}, sigCap: 0x8042437}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: gpsprot.PropIDSignalsEnabled}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	want := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C,
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigGALE1)
	if got, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || got != want {
		t.Errorf("signals = %v/%v, want %v", got, ok, want)
	}
}

func TestConfigSupport(t *testing.T) {
	base := gpsprot.ConfigSupportRaw |
		gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportSurveyMsg | gpsprot.ConfigSupportFixedPos |
		gpsprot.ConfigSupportSignal | gpsprot.ConfigSupportSpeed
	rtcm := gpsprot.ConfigSupportRTCMMSM4 | gpsprot.ConfigSupportRTCMMSM7 |
		gpsprot.ConfigSupportRTCMQZSS
	for _, tc := range []struct {
		hw   string
		want gpsprot.ConfigSupportFlags
	}{
		{"HD8040D.9529b663", base}, // TAU1201: no RTCM
		{"HD8041.0", base},         // any HD80xx: no RTCM
		{"HD9510.4740d9ec2", base | rtcm},
		{"HD9310.92257eed4", base | rtcm},
		{"HD9300.0", base | rtcm}, // all HD93xx do RTCM
		{"HD9999.0", base | rtcm}, // unknown chip number: optimistic
		{"XYZ12.34", base | rtcm}, // no HDxxxx at all
		{"HD.n0dig", base | rtcm}, // HD without digits
	} {
		ver := &asbin.MonVer{SwVersion: z16("3.018"), HwVersion: z16(tc.hw)}
		cfg := newConfigurator(&gpsprot.ConfigTarget{}, ver)
		if got := cfg.ConfigSupport(); got != tc.want {
			t.Errorf("%s: ConfigSupport = %v, want %v", tc.hw, got, tc.want)
		}
	}
}

func TestMinElev(t *testing.T) {
	// as-found: trk 1 deg, navi 5 deg (all three units)
	asFound := asbin.CfgElev{TrkMask: 0.017453, NaviMask: 0.087266}
	rcvr := &testReceiver{monVer: tau1201Ver(), elev: asFound}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMinElevation(gpsprot.DegreesFromFloat(10))
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.elev.TrkMask != asFound.TrkMask {
		t.Errorf("TrkMask = %v, want preserved %v", rcvr.elev.TrkMask, asFound.TrkMask)
	}
	if deg := float64(rcvr.elev.NaviMask) * 180 / math.Pi; deg < 9.99 || deg > 10.01 {
		t.Errorf("NaviMask = %v deg, want 10", deg)
	}
	if e, ok := cfg.ConfigProps().GetMinElevation(); !ok || e.Degrees() < 9.99 || e.Degrees() > 10.01 {
		t.Errorf("achieved minElev = %v/%v, want ~10 deg", e, ok)
	}
}

func TestSpeedChange(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver(),
		prt: [2]uint32{115200, 115200}, liveRate: 115200}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(230400)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.prt != [2]uint32{230400, 230400} {
		t.Errorf("PRT records = %v, want both 230400", rcvr.prt)
	}
	if rcvr.liveRate != 230400 {
		t.Errorf("live rate = %d, want 230400", rcvr.liveRate)
	}
	if b, ok := cfg.ConfigProps().GetBaudRate(); !ok || b != 230400 {
		t.Errorf("achieved baud = %d/%v, want 230400", b, ok)
	}
}

func TestSpeedChangeThenSave(t *testing.T) {
	// A combined --speed --save persists the NEW rate: the save runs
	// after the confirmed switch and its minimal mask covers baud.
	rcvr := &testReceiver{monVer: tau1201Ver(),
		prt: [2]uint32{115200, 115200}, liveRate: 115200}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(230400)
	target.Opts.Save = gpsprot.SaveMinimal
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if len(rcvr.saves) != 1 || rcvr.saves[0].Mask&asbin.CfgCfgMaskBaudrate == 0 {
		t.Errorf("saves = %v, want one save covering baud", rcvr.saves)
	}
	if rcvr.liveRate != 230400 {
		t.Errorf("live rate = %d, want 230400 before the save", rcvr.liveRate)
	}
}

func TestShowPort(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver(),
		prt: [2]uint32{115200, 115200}, liveRate: 115200}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: gpsprot.PropIDBaudRate}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	props := cfg.ConfigProps()
	if b, ok := props.GetBaudRate(); !ok || b != 115200 {
		t.Errorf("baud = %d/%v, want 115200", b, ok)
	}
	if _, ok := props.GetPort(); ok {
		t.Error("port name reported; the active UART is not identifiable")
	}
}

func TestNVMOps(t *testing.T) {
	tests := []struct {
		name         string
		nmea         bool // request an NMEA change so minimal has something to save
		pvt          bool // request a binary-only change (not persistable)
		rtcm         bool // request an RTCM change (persists with NMEA rates)
		save         gpsprot.SaveType
		reset        gpsprot.ResetType
		expectSaves  []asbin.CfgCfg
		expectClears []asbin.CfgCfg
		expectResets []asbin.CfgSimpleRst
	}{
		{
			name: "save_minimal_nmea",
			nmea: true,
			save: gpsprot.SaveMinimal,
			expectSaves: []asbin.CfgCfg{
				{Action: asbin.CfgCfgActionSave, Mask: asbin.CfgCfgMaskNmeaMsgRate},
			},
		},
		{
			// binary message rates are not persistable, so a minimal
			// save with nothing persistable to save sends nothing
			name: "save_minimal_binary_only",
			pvt:  true,
			save: gpsprot.SaveMinimal,
		},
		{
			// RTCM rates persist under the same section as NMEA rates
			name: "save_minimal_rtcm",
			rtcm: true,
			save: gpsprot.SaveMinimal,
			expectSaves: []asbin.CfgCfg{
				{Action: asbin.CfgCfgActionSave, Mask: asbin.CfgCfgMaskNmeaMsgRate},
			},
		},
		{
			name: "save_all",
			save: gpsprot.SaveAll,
			expectSaves: []asbin.CfgCfg{
				{Action: asbin.CfgCfgActionSave, Mask: cfgCfgMaskAll},
			},
		},
		{
			name:         "reload",
			reset:        gpsprot.ResetReload,
			expectResets: []asbin.CfgSimpleRst{{Mode: asbin.CfgSimpleRstModeReset}},
		},
		{
			name:         "reset_cold",
			reset:        gpsprot.ResetCold,
			expectResets: []asbin.CfgSimpleRst{{Mode: asbin.CfgSimpleRstModeColdStart}},
		},
		{
			name:  "factory",
			reset: gpsprot.ResetFactory,
			expectClears: []asbin.CfgCfg{
				{Action: asbin.CfgCfgActionClear, Mask: asbin.CfgCfgMaskFactoryReset},
			},
			expectResets: []asbin.CfgSimpleRst{{Mode: asbin.CfgSimpleRstModeColdStart}},
		},
		{
			name:  "save_all_then_reload",
			save:  gpsprot.SaveAll,
			reset: gpsprot.ResetReload,
			expectSaves: []asbin.CfgCfg{
				{Action: asbin.CfgCfgActionSave, Mask: cfgCfgMaskAll},
			},
			expectResets: []asbin.CfgSimpleRst{{Mode: asbin.CfgSimpleRstModeReset}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.Save = tc.save
			target.Opts.Reset = tc.reset
			if tc.nmea {
				target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC)
			}
			if tc.pvt {
				target.Opts.PVTMsg.Set(gpsprot.PVTMsgPos)
			}
			if tc.rtcm {
				target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgMSM4)
			}
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.saves, tc.expectSaves) {
				t.Errorf("saves\ngot  %v\nwant %v", rcvr.saves, tc.expectSaves)
			}
			if !reflect.DeepEqual(rcvr.clears, tc.expectClears) {
				t.Errorf("clears\ngot  %v\nwant %v", rcvr.clears, tc.expectClears)
			}
			if !reflect.DeepEqual(rcvr.resets, tc.expectResets) {
				t.Errorf("resets\ngot  %v\nwant %v", rcvr.resets, tc.expectResets)
			}
		})
	}
}

func TestNMEARefused(t *testing.T) {
	// A refused NMEA rate is a genuine failure: it must surface as an
	// error while the remaining requests still complete.
	rcvr := &testReceiver{
		monVer:     tau1201Ver(),
		nakTargets: map[asbin.MsgID]bool{asbin.NmeaGsvID: true},
	}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", errCount)
	}
	if rcvr.rates[asbin.NmeaRmcID] != 1 {
		t.Errorf("RMC rate = %d, want 1: later requests must proceed past a refusal", rcvr.rates[asbin.NmeaRmcID])
	}
}

func TestSilentRequestFails(t *testing.T) {
	// A receiver that never answers a non-optional request exhausts the
	// director's retries and fails, without hanging.
	rcvr := &testReceiver{
		monVer: tau1201Ver(),
		silent: map[asbin.MsgID]bool{asbin.CfgMsgID: true},
	}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 7 {
		t.Errorf("ErrorCount = %d, want 7 (every NMEA rate request unanswered)", errCount)
	}
}
