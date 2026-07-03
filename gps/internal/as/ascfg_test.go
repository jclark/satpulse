package as

import (
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
	t          *testing.T
	monVer     *asbin.MonVer
	rates      map[asbin.MsgID]uint8
	naks       map[asbin.MsgID]bool // set requests answered with NAK
	nakTargets map[asbin.MsgID]bool // CFG-MSG targets answered with NAK
	silent     map[asbin.MsgID]bool // requests not answered at all
	reorder    bool                 // deliver each burst's responses in reverse
	pending    [][]byte             // delivered before the next request's responses
}

func (r *testReceiver) takePending() [][]byte {
	p := r.pending
	r.pending = nil
	return p
}

// respond interprets one request write and returns the receiver's
// response packets.
func (r *testReceiver) respond(data []byte) [][]byte {
	out := r.respondOne(data)
	if r.reorder {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
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
			for _, resp := range append(rcvr.takePending(), rcvr.respond(action.Packet)...) {
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
		SupportedGNSS: supportedGNSS,
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
