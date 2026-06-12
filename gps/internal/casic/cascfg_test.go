package casic

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/casmsg"
	"github.com/jclark/satpulse/gps/lib/latin1z"
)

// testReceiver simulates a CASIC receiver at the packet level. Requests
// are interpreted from raw packet bytes and answered with raw packet
// bytes, which the test feeds through the real PacketProcessor.
type testReceiver struct {
	t          *testing.T
	monVer     *casbin.MonVer // nil simulates V5: MON-VER poll gets NAK
	nmea       []string       // NMEA sentences received (e.g. the probe quiet command)
	rates      map[casbin.MsgID]uint16
	naks       map[casbin.MsgID]bool // requests answered with NAK
	nakTargets map[casbin.MsgID]bool // CFG-MSG set targets answered with NAK
	silent     map[casbin.MsgID]bool // requests not answered at all
	pending    [][]byte              // delivered before the next request's responses
	saves      []casbin.CfgCfg
	resets     []casbin.CfgRst
}

func (r *testReceiver) takePending() [][]byte {
	p := r.pending
	r.pending = nil
	return p
}

// respond interprets one request write (which may have NMEA sentences
// before the binary packet, as the probe does) and returns the
// receiver's response packets.
func (r *testReceiver) respond(data []byte) [][]byte {
	for len(data) > 0 && data[0] == '$' {
		i := strings.IndexByte(string(data), '\n')
		if i < 0 {
			r.t.Fatalf("unterminated NMEA sentence in request: %q", data)
		}
		r.nmea = append(r.nmea, strings.TrimRight(string(data[:i]), "\r"))
		data = data[i+1:]
	}
	if len(data) == 0 {
		return nil
	}
	m, err := casbin.ParseMsg(string(data))
	if err != nil {
		r.t.Fatalf("receiver could not parse request: %v", err)
	}
	if r.silent[m.ID()] {
		return nil
	}
	if r.naks[m.ID()] {
		return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(m.ID())})}
	}
	switch mt := m.(type) {
	case *casbin.CfgCfg:
		r.saves = append(r.saves, *mt)
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgCfgID)})}
	case *casbin.CfgRst:
		r.resets = append(r.resets, *mt)
		return nil
	case *casbin.CfgMsg:
		if mt.Rate == casbin.PollRate && mt.Target == casbin.MonVerID {
			if r.monVer == nil {
				return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgMsgID)})}
			}
			return [][]byte{
				r.pack(r.monVer),
				r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgMsgID)}),
			}
		}
		if mt.Rate != casbin.PollRate {
			if r.nakTargets[mt.Target] {
				return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgMsgID)})}
			}
			if r.rates == nil {
				r.rates = make(map[casbin.MsgID]uint16)
			}
			r.rates[mt.Target] = mt.Rate
			return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgMsgID)})}
		}
	}
	if mid := m.ID(); mid.Ackable() {
		return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(m.ID())})}
	}
	return nil
}

func (r *testReceiver) pack(m casbin.Msg) []byte {
	pkt, err := casbin.Serialize(m)
	if err != nil {
		r.t.Fatalf("serialize %T: %v", m, err)
	}
	return pkt
}

func ackOf(mid casbin.MsgID) casbin.AckPayload {
	cls, id := mid.Unpack()
	return casbin.AckPayload{ClsID: cls, MsgID: id}
}

func z32(s string) latin1z.StringZ32 {
	var z latin1z.StringZ32
	copy(z[:], s)
	return z
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

func TestProbeV6(t *testing.T) {
	rcvr := &testReceiver{
		monVer: &casbin.MonVer{
			SwVersion: z32("SW=URANUS6,V6.3.2.0"),
			HwVersion: z32("ATGM332D-AT9880-F8N-76"),
		},
	}
	cp := probe(t, rcvr)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false after MON-VER response")
	}
	if len(rcvr.nmea) != 1 || !strings.HasPrefix(rcvr.nmea[0], "$PCAS03,0") {
		t.Errorf("probe NMEA preamble = %q, want one $PCAS03,0... sentence", rcvr.nmea)
	}
	cfg, errCount := configure(t, cp, rcvr, &gpsprot.ConfigTarget{})
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	got := cfg.ReceiverInfo()
	want := &gpsprot.ReceiverInfo{
		Vendor:   Vendor,
		Firmware: "SW=URANUS6,V6.3.2.0",
		Hardware: "ATGM332D-AT9880-F8N-76",
		SupportedGNSS: gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.BDS, gpsprot.GLO,
			gpsprot.GAL, gpsprot.QZSS, gpsprot.SBAS, gpsprot.NAVIC),
	}
	if got.VendorSpecific == nil {
		t.Error("VendorSpecific = nil, want MON-VER message")
	}
	want.VendorSpecific = got.VendorSpecific
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReceiverInfo\ngot  %+v\nwant %+v", got, want)
	}
	if cfg.family != familyV6 {
		t.Errorf("family = %v, want familyV6", cfg.family)
	}
}

func TestProbeV5(t *testing.T) {
	rcvr := &testReceiver{} // no MON-VER: poll is NAKed
	cp := probe(t, rcvr)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false after MON-VER NAK")
	}
	cfg, errCount := configure(t, cp, rcvr, &gpsprot.ConfigTarget{})
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	got := cfg.ReceiverInfo()
	want := &gpsprot.ReceiverInfo{
		Vendor:        Vendor,
		SupportedGNSS: gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.BDS, gpsprot.GLO),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReceiverInfo\ngot  %+v\nwant %+v", got, want)
	}
	if cfg.family != familyV5 {
		t.Errorf("family = %v, want familyV5", cfg.family)
	}
}

func nmeaTarget(flags gpsprot.NMEAMsgFlags) *gpsprot.ConfigTarget {
	target := gpsprot.NewConfigTarget()
	target.Opts.NMEAMsg.Set(flags)
	return target
}

func TestNMEAOut(t *testing.T) {
	tests := []struct {
		name       string
		monVer     *casbin.MonVer
		flags      gpsprot.NMEAMsgFlags
		expect     map[casbin.MsgID]uint16
		expectPcas string
	}{
		{
			name:   "V6 RMC and ZDA",
			monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
			flags:  gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgZDA,
			expect: map[casbin.MsgID]uint16{
				casbin.NmeaGsvID: 0, casbin.NmeaRmcID: 1, casbin.NmeaGgaID: 0,
				casbin.NmeaGsaID: 0, casbin.NmeaZdaV6ID: 1, casbin.NmeaVtgID: 0,
				casbin.NmeaGllID: 0,
			},
			expectPcas: casmsg.OutputRates(0, 0, 0, 0, 1, 0, 1),
		},
		{
			name:  "V5 ZDA uses 0x08",
			flags: gpsprot.NMEAMsgZDA,
			expect: map[casbin.MsgID]uint16{
				casbin.NmeaGsvID: 0, casbin.NmeaRmcID: 0, casbin.NmeaGgaID: 0,
				casbin.NmeaGsaID: 0, casbin.NmeaZdaID: 1, casbin.NmeaVtgID: 0,
				casbin.NmeaGllID: 0,
			},
			expectPcas: casmsg.OutputRates(0, 0, 0, 0, 0, 0, 1),
		},
		{
			name:  "all off",
			flags: gpsprot.NMEAMsgNone,
			expect: map[casbin.MsgID]uint16{
				casbin.NmeaGsvID: 0, casbin.NmeaRmcID: 0, casbin.NmeaGgaID: 0,
				casbin.NmeaGsaID: 0, casbin.NmeaZdaID: 0, casbin.NmeaVtgID: 0,
				casbin.NmeaGllID: 0,
			},
			expectPcas: casmsg.QuietAll(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer}
			cp := probe(t, rcvr)
			_, errCount := configure(t, cp, rcvr, nmeaTarget(tc.flags))
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.rates, tc.expect) {
				t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, tc.expect)
			}
			last := rcvr.nmea[len(rcvr.nmea)-1] + "\r\n"
			if last != tc.expectPcas {
				t.Errorf("final PCAS03 = %q, want %q", last, tc.expectPcas)
			}
		})
	}
}

func TestNMEAOutNak(t *testing.T) {
	rcvr := &testReceiver{naks: map[casbin.MsgID]bool{casbin.CfgMsgID: true}}
	cp := probe(t, rcvr)
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false")
	}
	_, errCount := configure(t, cp, rcvr, nmeaTarget(gpsprot.NMEAMsgRMC))
	if errCount != 7 {
		t.Errorf("ErrorCount = %d, want 7 (every CFG-MSG set refused)", errCount)
	}
}

func TestNMEAOutNoResponse(t *testing.T) {
	rcvr := &testReceiver{monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}}
	cp := probe(t, rcvr)
	rcvr.silent = map[casbin.MsgID]bool{casbin.CfgMsgID: true}
	_, errCount := configure(t, cp, rcvr, nmeaTarget(gpsprot.NMEAMsgRMC))
	if errCount != 7 {
		t.Errorf("ErrorCount = %d, want 7 (every CFG-MSG set timed out)", errCount)
	}
}

func TestPVTOut(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name   string
		monVer *casbin.MonVer
		flags  gpsprot.PVTMsgFlags
		expect map[casbin.MsgID]uint16
	}{
		{
			name:   "V6 pos time tp",
			monVer: v6,
			flags:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse,
			expect: map[casbin.MsgID]uint16{
				casbin.Nav2PvhID: 1, casbin.Nav2TimeUTCID: 1, casbin.TimTPID: 1,
			},
		},
		{
			name:  "V5 pos time",
			flags: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
			expect: map[casbin.MsgID]uint16{
				casbin.NavPvID: 1, casbin.NavTimeUTCID: 1,
			},
		},
		{
			name:   "V6 timing PTP",
			monVer: v6,
			flags:  gpsprot.PVTMsgTimingPTP | gpsprot.PVTMsgOff,
			// tp+after+TAI: SOL for TAI time, TIM-TP for the pulse;
			// quality adds DOP; PVH and TIMEUTC turned off.
			expect: map[casbin.MsgID]uint16{
				casbin.Nav2SolID: 1, casbin.TimTPID: 1, casbin.Nav2DopID: 1,
				casbin.Nav2PvhID: 0, casbin.Nav2TimeUTCID: 0,
			},
		},
		{
			name:   "V6 ECEF pos without off is incremental",
			monVer: v6,
			flags:  gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			expect: map[casbin.MsgID]uint16{casbin.Nav2SolID: 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			target.Opts.PVTMsg = tc.flags
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
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name   string
		monVer *casbin.MonVer
		flags  gpsprot.SatsMsgFlags
		expect map[casbin.MsgID]uint16
	}{
		{
			name:   "V6 sat",
			monVer: v6,
			flags:  gpsprot.SatsMsgSat,
			expect: map[casbin.MsgID]uint16{casbin.Nav2SigID: 1},
		},
		{
			name:   "V6 none turns off",
			monVer: v6,
			flags:  gpsprot.SatsMsgNone,
			expect: map[casbin.MsgID]uint16{casbin.Nav2SigID: 0},
		},
		{
			name:  "V5 sat",
			flags: gpsprot.SatsMsgSat,
			expect: map[casbin.MsgID]uint16{
				casbin.NavGPSInfoID: 1, casbin.NavBDSInfoID: 1, casbin.NavGLNInfoID: 1,
			},
		},
		{
			name:  "V5 signal only enables nothing",
			flags: gpsprot.SatsMsgSignal,
			expect: map[casbin.MsgID]uint16{
				casbin.NavGPSInfoID: 0, casbin.NavBDSInfoID: 0, casbin.NavGLNInfoID: 0,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
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

func TestNVMOps(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name         string
		monVer       *casbin.MonVer
		nmea         gpsprot.NMEAMsgFlags
		setNMEA      bool
		save         gpsprot.SaveType
		reset        gpsprot.ResetType
		expectSaves  []casbin.CfgCfg
		expectResets []casbin.CfgRst
	}{
		{
			name:        "V5 minimal save of message changes",
			nmea:        gpsprot.NMEAMsgRMC,
			setNMEA:     true,
			save:        gpsprot.SaveMinimal,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgSectionMsg, OpMode: casbin.CfgOpSave}},
		},
		{
			name:        "V5 minimal save with no changes saves nothing",
			save:        gpsprot.SaveMinimal,
			expectSaves: nil,
		},
		{
			name:        "V5 save all",
			save:        gpsprot.SaveAll,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgSectionAll, OpMode: casbin.CfgOpSave}},
		},
		{
			name:        "V6 save always uses the all-sections mask",
			monVer:      v6,
			nmea:        gpsprot.NMEAMsgRMC,
			setNMEA:     true,
			save:        gpsprot.SaveMinimal,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgSectionAll, OpMode: casbin.CfgOpSave}},
		},
		{
			name:        "V6 reload sends load without expecting an ACK",
			monVer:      v6,
			reset:       gpsprot.ResetReload,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgSectionAll, OpMode: casbin.CfgOpLoad}},
		},
		{
			name:        "V5 reload",
			reset:       gpsprot.ResetReload,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgSectionAll, OpMode: casbin.CfgOpLoad}},
		},
		{
			name:  "V5 cold reset",
			reset: gpsprot.ResetCold,
			expectResets: []casbin.CfgRst{{NavBbrMask: bbrReset,
				ResetMode: casbin.ResetHWImmediate, StartMode: casbin.StartCold}},
		},
		{
			name:   "V6 factory reset",
			monVer: v6,
			reset:  gpsprot.ResetFactory,
			expectResets: []casbin.CfgRst{{NavBbrMask: 0,
				ResetMode: casbin.ResetHWImmediate, StartMode: casbin.StartFactory}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			if tc.setNMEA {
				target.Opts.NMEAMsg.Set(tc.nmea)
			}
			target.Opts.Save = tc.save
			target.Opts.Reset = tc.reset
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.saves, tc.expectSaves) {
				t.Errorf("saves\ngot  %+v\nwant %+v", rcvr.saves, tc.expectSaves)
			}
			if !reflect.DeepEqual(rcvr.resets, tc.expectResets) {
				t.Errorf("resets\ngot  %+v\nwant %+v", rcvr.resets, tc.expectResets)
			}
		})
	}
}

// TestSaveAfterFallback ensures the save request is generated only
// after a NAK-driven fallback request has completed, so the fallback's
// effect is included in what is saved.
func TestSaveAfterFallback(t *testing.T) {
	rcvr := &testReceiver{
		monVer:     &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		nakTargets: map[casbin.MsgID]bool{casbin.TimTPID: true},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Opts.PVTMsg = gpsprot.PVTMsgTimePulse
	target.Opts.Save = gpsprot.SaveMinimal
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.rates[casbin.Tim2TpxID] != 1 {
		t.Errorf("TIM2-TPX rate = %d, want 1 (fallback before save)", rcvr.rates[casbin.Tim2TpxID])
	}
	if len(rcvr.saves) != 1 {
		t.Fatalf("saves = %+v, want one", rcvr.saves)
	}
}

// TestTimTPFallback covers the hardware divergence found on the F8N:
// enabling TIM-TP is NAKed, and the configurator falls back to
// TIM2-TPX; if that is NAKed too, pulse-time output is simply absent.
// Neither case is an error.
func TestTimTPFallback(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name   string
		naks   []casbin.MsgID
		expect map[casbin.MsgID]uint16
	}{
		{
			name: "TIM-TP refused, TIM2-TPX accepted",
			naks: []casbin.MsgID{casbin.TimTPID},
			expect: map[casbin.MsgID]uint16{
				casbin.Nav2TimeUTCID: 1, casbin.Tim2TpxID: 1,
			},
		},
		{
			name: "both refused",
			naks: []casbin.MsgID{casbin.TimTPID, casbin.Tim2TpxID},
			expect: map[casbin.MsgID]uint16{
				casbin.Nav2TimeUTCID: 1,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: v6, nakTargets: make(map[casbin.MsgID]bool)}
			for _, mid := range tc.naks {
				rcvr.nakTargets[mid] = true
			}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			target.Opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse
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

// TestLateProbeNak reproduces the V5 hazard: gpscfg sends a second
// probe when the first answers slowly, and the second probe's NAK
// arrives only after Configure, when the configurator's first CFG-MSG
// set is already outstanding. The protocol must consume that NAK
// rather than attribute it to the configurator's request.
func TestLateProbeNak(t *testing.T) {
	rcvr := &testReceiver{t: t}
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	cp := NewConfigProtocol()
	pp.SetNativeMsgHandler(cp)
	p1 := cp.ProbePacket()
	cp.ProbePacket() // second probe, as gpscfg sends after probeRetryDelay
	for _, resp := range rcvr.respond(p1) {
		if _, err := pp.ProcessPacket(string(resp), time.Unix(1, 0)); err != nil {
			t.Fatalf("ProcessPacket: %v", err)
		}
	}
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false")
	}
	rcvr.pending = [][]byte{rcvr.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgMsgID)})}
	_, errCount := configure(t, cp, rcvr, nmeaTarget(gpsprot.NMEAMsgRMC))
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0 (late probe NAK misattributed)", errCount)
	}
	if rcvr.rates[casbin.NmeaRmcID] != 1 {
		t.Errorf("RMC rate = %d, want 1", rcvr.rates[casbin.NmeaRmcID])
	}
}

func TestProbeIgnoresUnrelatedNak(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	cp := NewConfigProtocol()
	pp.SetNativeMsgHandler(cp)
	rcvr := &testReceiver{t: t}
	nak := rcvr.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgRateID)})
	if _, err := pp.ProcessPacket(string(nak), time.Unix(1, 0)); err != nil {
		t.Fatalf("ProcessPacket: %v", err)
	}
	if cp.ProbeOK() {
		t.Error("ProbeOK = true after NAK of unrelated CFG-RATE")
	}
}
