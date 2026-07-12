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
	t              *testing.T
	monVer         *asbin.MonVer
	rates          map[asbin.MsgID]uint8
	msgSets        []asbin.MsgID        // CFG-MSG set targets received, in order
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
		r.msgSets = append(r.msgSets, target)
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

// tau1201Cap is the TAU1201/TAU951M hardware signal plan (mask
// 0x4108237): GPS L1CA+L5, GLO G1, BDS B1I+B2a, GAL E1+E5a, QZSS L1+L5.
const tau1201Cap = asbin.CfgNavSatMask(0x4108237)

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
		cfg := newConfigurator(&gpsprot.ConfigTarget{}, ver, newRateEstimator())
		if got := cfg.ConfigSupport(); got != tc.want {
			t.Errorf("%s: ConfigSupport = %v, want %v", tc.hw, got, tc.want)
		}
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
	if errCount != 11 {
		t.Errorf("ErrorCount = %d, want 11 (every NMEA rate request unanswered: seven vocabulary sentences plus the four out-of-vocabulary disables)", errCount)
	}
}
