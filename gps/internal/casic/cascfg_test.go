package casic

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/casbin"
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
	saveBaud   uint32        // newBaud at the time of the first save
	switchLag  time.Duration // simulated host-side gap after a speed change (default 200ms)
	resets     []casbin.CfgRst
	tp         *casbin.CfgTP // nil: CFG-TP unsupported (poll gets NAK)
	tm5        *casbin.CfgTMode
	tm6        *casbin.CfgTMode2
	navx       *casbin.CfgNavx
	navBand    *casbin.CfgNavBand
	sigCap     uint32 // hardware-receivable signals; clamps written SigIDMask
	ports      []casbin.CfgPrt
	rate       *casbin.CfgRate
	navLimit   *casbin.CfgNavLimit
	sw, hw     string   // PCAS06 replies, when non-empty
	textOut    []string // queued GPTXT payloads to deliver
	newBaud    uint32   // recorded from a CFG-PRT set
	silentPrt  bool     // CFG-PRT set gets no ACK (it arrives at the new speed)
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
		sentence := strings.TrimRight(string(data[:i]), "\r")
		r.nmea = append(r.nmea, sentence)
		if strings.HasPrefix(sentence, "$PCAS06,0") && r.sw != "" {
			r.textOut = append(r.textOut, "GPTXT,01,01,02,SW="+r.sw)
		}
		if strings.HasPrefix(sentence, "$PCAS06,1") && r.hw != "" {
			r.textOut = append(r.textOut, "GPTXT,01,01,02,HW="+r.hw)
		}
		data = data[i+1:]
	}
	if len(data) == 0 {
		return nil
	}
	if len(data) == casbin.PacketMinLen {
		return r.respondPoll(casbin.PacketMsgID(string(data)))
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
	case *casbin.CfgPrt:
		r.newBaud = mt.BaudRate
		if r.silentPrt {
			return nil
		}
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgPrtID)})}
	case *casbin.CfgNavBand:
		if r.navBand == nil {
			return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgNavBandID)})}
		}
		*r.navBand = *mt
		r.navBand.SigIDMask &= r.sigCap
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgNavBandID)})}
	case *casbin.CfgNavx:
		if r.navx == nil {
			return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgNavxID)})}
		}
		if mt.Mask&casbin.NavxNavSystem != 0 && mt.NavSystem != 0 {
			// Like the 5N71: an empty constellation set is acknowledged
			// but not applied.
			r.navx.NavSystem = mt.NavSystem
		}
		if mt.Mask&casbin.NavxMinElev != 0 {
			r.navx.MinElev = mt.MinElev
		}
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgNavxID)})}
	case *casbin.CfgNavLimit:
		if r.navLimit == nil {
			return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgNavLimID)})}
		}
		*r.navLimit = *mt
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgNavLimID)})}
	case *casbin.CfgTMode:
		if r.tm5 == nil {
			return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgTModeID)})}
		}
		*r.tm5 = *mt
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgTModeID)})}
	case *casbin.CfgTMode2:
		if r.tm6 == nil {
			return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgTMode2ID)})}
		}
		*r.tm6 = *mt
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgTMode2ID)})}
	case *casbin.CfgTP:
		if r.tp == nil {
			return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgTPID)})}
		}
		*r.tp = *mt
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgTPID)})}
	case *casbin.CfgCfg:
		if len(r.saves) == 0 {
			r.saveBaud = r.newBaud
		}
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

// respondPoll answers an empty-payload CFG query.
func (r *testReceiver) respondPoll(mid casbin.MsgID) [][]byte {
	if r.silent[mid] {
		return nil
	}
	if r.naks[mid] {
		return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(mid)})}
	}
	var m casbin.Msg
	switch mid {
	case casbin.CfgTPID:
		if r.tp != nil {
			m = r.tp
		}
	case casbin.CfgTModeID:
		if r.tm5 != nil {
			m = r.tm5
		}
	case casbin.CfgTMode2ID:
		if r.tm6 != nil {
			m = r.tm6
		}
	case casbin.CfgNavxID:
		if r.navx != nil {
			m = r.navx
		}
	case casbin.CfgNavBandID:
		if r.navBand != nil {
			m = r.navBand
		}
	case casbin.CfgRateID:
		if r.rate != nil {
			m = r.rate
		}
	case casbin.CfgNavLimID:
		if r.navLimit != nil {
			m = r.navLimit
		}
	case casbin.CfgPrtID:
		if len(r.ports) > 0 {
			var out [][]byte
			for i := range r.ports {
				out = append(out, r.pack(&r.ports[i]))
			}
			return append(out, r.pack(&casbin.AckAck{AckPayload: ackOf(mid)}))
		}
	}
	if m == nil {
		return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(mid)})}
	}
	return [][]byte{
		r.pack(m),
		r.pack(&casbin.AckAck{AckPayload: ackOf(mid)}),
	}
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
			if action.Speed != 0 {
				// the next responses arrive after the host switched speed
				lag := 200 * time.Millisecond
				if rcvr.switchLag != 0 {
					lag = rcvr.switchLag
				}
				t0 = t0.Add(lag)
			}
			for _, resp := range append(rcvr.takePending(), rcvr.respond(action.Packet)...) {
				t0 = t0.Add(5 * time.Millisecond)
				if _, err := pp.ProcessPacket(string(resp), t0); err != nil {
					t.Fatalf("ProcessPacket: %v", err)
				}
				director.ValidPacketReceived(t0)
			}
			for _, payload := range rcvr.textOut {
				t0 = t0.Add(5 * time.Millisecond)
				if err := cp.NativeMsg(nmea.Tag, "GPTXT", &nmea.Sentence{Payload: payload}, t0); err != nil {
					t.Fatalf("NativeMsg text: %v", err)
				}
			}
			rcvr.textOut = nil
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
	if len(rcvr.nmea) != 0 {
		t.Errorf("probe sent NMEA %q, want none: probing must not change receiver state", rcvr.nmea)
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
		name   string
		monVer *casbin.MonVer
		flags  gpsprot.NMEAMsgFlags
		expect map[casbin.MsgID]uint16
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
		},
		{
			name:  "V5 ZDA uses 0x08",
			flags: gpsprot.NMEAMsgZDA,
			expect: map[casbin.MsgID]uint16{
				casbin.NmeaGsvID: 0, casbin.NmeaRmcID: 0, casbin.NmeaGgaID: 0,
				casbin.NmeaGsaID: 0, casbin.NmeaZdaID: 1, casbin.NmeaVtgID: 0,
				casbin.NmeaGllID: 0,
			},
		},
		{
			name:  "all off",
			flags: gpsprot.NMEAMsgNone,
			expect: map[casbin.MsgID]uint16{
				casbin.NmeaGsvID: 0, casbin.NmeaRmcID: 0, casbin.NmeaGgaID: 0,
				casbin.NmeaGsaID: 0, casbin.NmeaZdaID: 0, casbin.NmeaVtgID: 0,
				casbin.NmeaGllID: 0,
			},
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
			// quality adds DOP; leap adds TIM2-LS; survey adds
			// TIM2-TIMEPOS; PVH and TIMEUTC turned off.
			expect: map[casbin.MsgID]uint16{
				casbin.Nav2SolID: 1, casbin.TimTPID: 1, casbin.Nav2DopID: 1,
				casbin.Tim2LsID: 1, casbin.Tim2TimePosID: 1,
				casbin.Nav2PvhID: 0, casbin.Nav2TimeUTCID: 0,
			},
		},
		{
			name:   "V6 ECEF pos without off is incremental",
			monVer: v6,
			flags:  gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			expect: map[casbin.MsgID]uint16{casbin.Nav2SolID: 1},
		},
		{
			name:   "V6 leap enables TIM2-LS",
			monVer: v6,
			flags:  gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgTimePulse,
			expect: map[casbin.MsgID]uint16{casbin.Tim2LsID: 1, casbin.TimTPID: 1},
		},
		{
			name:   "V5 leap enables MSG-GPSUTC",
			flags:  gpsprot.PVTMsgLeapSecond,
			expect: map[casbin.MsgID]uint16{casbin.MsgGPSUTCID: 1},
		},
		{
			name:   "V6 survey enables TIM2-TIMEPOS",
			monVer: v6,
			flags:  gpsprot.PVTMsgSurvey,
			expect: map[casbin.MsgID]uint16{casbin.Tim2TimePosID: 1},
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

func defaultTP() *casbin.CfgTP {
	return &casbin.CfgTP{Interval: 1000000, Width: 100000, PPSOutMode: 3, TBase: 1, TSrcMode: 5}
}

func TestTimePulseSet(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name     string
		monVer   *casbin.MonVer
		tp       *casbin.CfgTP
		setup    func(*gpsprot.ConfigTarget)
		expectTP casbin.CfgTP
	}{
		{
			name:   "V6 PPS with GPS time",
			monVer: v6,
			tp:     defaultTP(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetPPS(100 * time.Millisecond)
				target.Props.SetTimeGNSS(gpsprot.GPS)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 100000,
				PPSOutMode: 5, Polarity: 0, TBase: 0, TSrcMode: 0},
		},
		{
			name: "V5 PPS inverts TBase and uses fix-only mode",
			tp:   &casbin.CfgTP{Interval: 1000000, Width: 100000, PPSOutMode: 1, TBase: 0, TSrcMode: 0},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetPPS(200 * time.Millisecond)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 200000,
				PPSOutMode: 3, Polarity: 0, TBase: 1, TSrcMode: 0},
		},
		{
			name:   "disable pulse",
			monVer: v6,
			tp:     defaultTP(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetTimePulseWidth(0)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 100000,
				PPSOutMode: 0, TBase: 1, TSrcMode: 5},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer, tp: tc.tp}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			tc.setup(target)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(*rcvr.tp, tc.expectTP) {
				t.Errorf("receiver CFG-TP\ngot  %+v\nwant %+v", *rcvr.tp, tc.expectTP)
			}
			got := cfg.ConfigProps()
			gotTP, ok := got.GetTimePulse()
			if !ok {
				t.Fatal("ConfigProps has no TimePulse")
			}
			wantWidth := time.Duration(tc.expectTP.Width) * time.Microsecond
			if tc.expectTP.PPSOutMode == 0 {
				wantWidth = 0
			}
			if gotTP.Width != wantWidth {
				t.Errorf("achieved width = %v, want %v", gotTP.Width, wantWidth)
			}
		})
	}
}

func TestTimePulseGet(t *testing.T) {
	rcvr := &testReceiver{
		monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		tp:     &casbin.CfgTP{Interval: 1000000, Width: 100000, PPSOutMode: 5, TBase: 0, TSrcMode: 1},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Get = gpsprot.PropIDTimePulse | gpsprot.PropIDTimeGNSS
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	props := cfg.ConfigProps()
	tp, ok := props.GetTimePulse()
	if !ok {
		t.Fatal("ConfigProps has no TimePulse")
	}
	want := gpsprot.TimePulse{Width: 100 * time.Millisecond, Period: time.Second,
		AlignToGNSS: true, OnlyWhenLocked: true, PolarityRising: true}
	if !reflect.DeepEqual(tp, want) {
		t.Errorf("TimePulse\ngot  %+v\nwant %+v", tp, want)
	}
	if g, ok := props.GetTimeGNSS(); !ok || g != gpsprot.BDS {
		t.Errorf("TimeGNSS = %v,%v, want BDS", g, ok)
	}
}

func TestTimePulseUnsupported(t *testing.T) {
	rcvr := &testReceiver{} // V5 with no CFG-TP at all: poll gets NAK
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0 (nonexistence is not an error)", errCount)
	}
	if _, ok := cfg.ConfigProps().GetTimePulse(); ok {
		t.Error("ConfigProps reports TimePulse despite NAKed poll")
	}
}

func TestTimeMode(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name    string
		monVer  *casbin.MonVer
		tm5     *casbin.CfgTMode
		tm6     *casbin.CfgTMode2
		setup   func(*gpsprot.ConfigTarget)
		expect5 *casbin.CfgTMode
		expect6 *casbin.CfgTMode2
		static  bool
		hasMode bool
	}{
		{
			name:   "V6 setStatic starts survey",
			monVer: v6,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime, BandMode: 1, TSrcMode: 2},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Opts.SetStatic = true
				target.Opts.Survey = gpsprot.Survey{MinDur: 2000 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey, BandMode: 1, TSrcMode: 2,
				SvinMinDur: 2000, SvinPaccLim: 20000},
			static:  true,
			hasMode: true,
		},
		{
			name:   "V6 setStatic leaves running survey alone",
			monVer: v6,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey, SvinMinDur: 300},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Opts.SetStatic = true
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey, SvinMinDur: 300},
			static:  true,
			hasMode: true,
		},
		{
			name:   "V6 fixed position",
			monVer: v6,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{
					Static:  true,
					PosType: gpsprot.PosTypeECEF,
					FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(-1144700.25),
						gpsprot.Meters(6090345.5), gpsprot.Meters(1504171)},
					FixedPosAcc: 3 * gpsprot.Meter,
				})
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Fixed,
				XFixed: -114470025, YFixed: 609034550, ZFixed: 150417100, FixedPacc: 3000},
			static:  true,
			hasMode: true,
		},
		{
			name: "V5 survey uses variances",
			tm5:  &casbin.CfgTMode{Mode: casbin.TModeAuto},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			expect5: &casbin.CfgTMode{Mode: casbin.TModeSurvey, SvinMinDur: 300, SvinVarLimit: 400},
			static:  true,
			hasMode: true,
		},
		{
			name:   "V6 mobile",
			monVer: v6,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey, SvinMinDur: 300},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime},
			static:  false,
			hasMode: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer, tm5: tc.tm5, tm6: tc.tm6}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			tc.setup(target)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if tc.expect5 != nil && !reflect.DeepEqual(rcvr.tm5, tc.expect5) {
				t.Errorf("CFG-TMODE\ngot  %+v\nwant %+v", rcvr.tm5, tc.expect5)
			}
			if tc.expect6 != nil && !reflect.DeepEqual(rcvr.tm6, tc.expect6) {
				t.Errorf("CFG-TMODE2\ngot  %+v\nwant %+v", rcvr.tm6, tc.expect6)
			}
			m, ok := cfg.ConfigProps().GetMode()
			if ok != tc.hasMode || m.Static != tc.static {
				t.Errorf("Mode = %+v,%v, want static=%v,%v", m, ok, tc.static, tc.hasMode)
			}
		})
	}
}

func TestSurveyAgainRestarts(t *testing.T) {
	rcvr := &testReceiver{
		monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey, SvinMinDur: 300},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Opts.SetStatic = true
	target.Opts.Survey = gpsprot.Survey{
		Flags:    gpsprot.SurveyAgain,
		MinDur:   600 * time.Second,
		AccLimit: 10 * gpsprot.Meter,
	}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.tm6.TimFixMode != casbin.CfgTMode2Survey || rcvr.tm6.SvinMinDur != 600 {
		t.Errorf("final CFG-TMODE2 = %+v, want survey with 600 s", rcvr.tm6)
	}
	sets := 0
	for i := 0; i < len(cfg.reqs); i++ {
		if cfg.reqs[i].mid == casbin.CfgTMode2ID && len(cfg.reqs[i].packet) > casbin.PacketMinLen {
			sets++
		}
	}
	if sets != 2 {
		t.Errorf("CFG-TMODE2 sets = %d, want 2 (auto then survey)", sets)
	}
}

func TestSignalSelection(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	const f8nMask = 0x0028CDAD   // dual-band: GPS L1CA+L5, SBAS, GLO L1, GAL E1+E5a, BDS B1I+B1C+B2a, QZSS L1CA+L5
	const at632Mask = 0x00084CA9 // L1-band only (the AT632's clamped reception set)
	tests := []struct {
		name          string
		monVer        *casbin.MonVer
		navx          *casbin.CfgNavx
		navBand       *casbin.CfgNavBand
		request       gpsprot.SignalSet
		expectMaskFix uint32
		expectSys     uint8
		expectSignals gpsprot.SignalSet
	}{
		{
			name:          "V6 dual-band GPS and GAL",
			monVer:        v6,
			navBand:       &casbin.CfgNavBand{SigBandAuto: 1, SigIDMaskFix: f8nMask, SigIDMask: f8nMask},
			request:       gpsprot.SigSetGPS | gpsprot.SigSetGAL,
			expectMaskFix: 1<<casbin.SigGPSL1CA | 1<<casbin.SigGPSL5 | 1<<casbin.SigGALE1 | 1<<casbin.SigGALE5a,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL5,
				gpsprot.SigGALE1, gpsprot.SigGALE5a),
		},
		{
			// The receiver clamps the written reception list to its
			// hardware: L5-band signals drop out in the readback.
			name:          "V6 L1-only hardware clamps GPS L5 and GAL E5a away",
			monVer:        v6,
			navBand:       &casbin.CfgNavBand{SigBandAuto: 1, SigIDMaskFix: at632Mask, SigIDMask: at632Mask},
			request:       gpsprot.SigSetGPS | gpsprot.SigSetGAL,
			expectMaskFix: 1<<casbin.SigGPSL1CA | 1<<casbin.SigGPSL5 | 1<<casbin.SigGALE1 | 1<<casbin.SigGALE5a,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1),
		},
		{
			name:          "V5 constellation level",
			navx:          &casbin.CfgNavx{NavSystem: casbin.NavSysGPS | casbin.NavSysBDS | casbin.NavSysGLN},
			request:       gpsprot.SigSetGPS | gpsprot.SigSetGAL,
			expectSys:     casbin.NavSysGPS,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
		},
		{
			// An all-GAL request empties to NavSystem 0, which the
			// receiver acknowledges without applying; the readback must
			// report the set actually in force.
			name:          "V5 empty intersection ACKed but not applied",
			navx:          &casbin.CfgNavx{NavSystem: casbin.NavSysGPS | casbin.NavSysGLN},
			request:       gpsprot.SigSetGAL,
			expectSys:     casbin.NavSysGPS | casbin.NavSysGLN,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGLOL1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer, navx: tc.navx, navBand: tc.navBand}
			if tc.navBand != nil {
				rcvr.sigCap = tc.navBand.SigIDMask
			}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			target.Props.SetSignalsEnabled(tc.request)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if tc.navBand != nil {
				if rcvr.navBand.SigBandAuto != 0 || rcvr.navBand.SigIDMaskFix != tc.expectMaskFix {
					t.Errorf("NAVBAND auto=%d maskFix=%#x, want auto=0 maskFix=%#x",
						rcvr.navBand.SigBandAuto, rcvr.navBand.SigIDMaskFix, tc.expectMaskFix)
				}
			}
			if tc.navx != nil && rcvr.navx.NavSystem != tc.expectSys {
				t.Errorf("NavSystem = %#x, want %#x", rcvr.navx.NavSystem, tc.expectSys)
			}
			got, ok := cfg.ConfigProps().GetSignalsEnabled()
			if !ok || got != tc.expectSignals {
				t.Errorf("SignalsEnabled = %v,%v\nwant %v", got, ok, tc.expectSignals)
			}
		})
	}
}

func TestSignalGetWithAutoBand(t *testing.T) {
	rcvr := &testReceiver{
		monVer:  &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		navBand: &casbin.CfgNavBand{SigBandAuto: 1, SigIDMask: 1<<casbin.SigGPSL1CA | 1<<casbin.SigGLOL1},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Get = gpsprot.PropIDSignalsEnabled
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	got, ok := cfg.ConfigProps().GetSignalsEnabled()
	want := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGLOL1)
	if !ok || got != want {
		t.Errorf("SignalsEnabled = %v,%v, want %v", got, ok, want)
	}
}

func TestBaudChange(t *testing.T) {
	ports := []casbin.CfgPrt{
		{PortID: 0, ProtoMask: 0x33, Mode: 0x0003, BaudRate: 115200},
		{PortID: 1, ProtoMask: 0x33, Mode: 0x0003, BaudRate: 115200},
	}
	tests := []struct {
		name      string
		silentPrt bool
		switchLag time.Duration
	}{
		{name: "ACK arrives at new speed and is matched"},
		{name: "no ACK, confirm poll traffic confirms", silentPrt: true},
		{
			// The garbled-ACK case on a quiet line: the poll answers
			// within the unsolicited-traffic exclusion window, but it
			// was solicited at the new rate, so it must confirm.
			name:      "no ACK, poll answers inside the exclusion window",
			silentPrt: true,
			switchLag: 20 * time.Millisecond,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{
				monVer:    &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
				ports:     append([]casbin.CfgPrt{}, ports...),
				rate:      &casbin.CfgRate{FixIntervalMs: 1000, FixRateHz: 1},
				silentPrt: tc.silentPrt,
				switchLag: tc.switchLag,
			}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			target.Props.SetBaudRate(38400)
			target.Opts.Save = gpsprot.SaveAll
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if rcvr.newBaud != 38400 {
				t.Errorf("receiver baud = %d, want 38400", rcvr.newBaud)
			}
			if got, ok := cfg.ConfigProps().GetBaudRate(); !ok || got != 38400 {
				t.Errorf("achieved baud = %d,%v, want 38400", got, ok)
			}
			if len(rcvr.saves) != 1 || rcvr.saveBaud != 38400 {
				t.Errorf("saves = %d at baud %d, want 1 at 38400",
					len(rcvr.saves), rcvr.saveBaud)
			}
		})
	}
}

func TestRawOut(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name   string
		monVer *casbin.MonVer
		flags  gpsprot.RawMsgFlags
		expect map[casbin.MsgID]uint16
	}{
		{
			name:   "V6 obs and nav",
			monVer: v6,
			flags:  gpsprot.RawMsgObs | gpsprot.RawMsgNavData,
			expect: map[casbin.MsgID]uint16{casbin.Rxm2MeasxID: 1, casbin.Rxm2SfrbxID: 1},
		},
		{
			name:   "V6 obs only turns nav off",
			monVer: v6,
			flags:  gpsprot.RawMsgObs,
			expect: map[casbin.MsgID]uint16{casbin.Rxm2MeasxID: 1, casbin.Rxm2SfrbxID: 0},
		},
		{
			name:   "V5 generates nothing",
			flags:  gpsprot.RawMsgObs,
			expect: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			target.Opts.RawMsg.Set(tc.flags)
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

func TestMinElevation(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	t.Run("V6 read-modify-write of CFG-NAVLIMIT", func(t *testing.T) {
		rcvr := &testReceiver{
			monVer:   v6,
			navLimit: &casbin.CfgNavLimit{MinSVs: 4, MaxSVs: 40, MinCNO: 8, MinElev: 5},
		}
		cp := probe(t, rcvr)
		target := gpsprot.NewConfigTarget()
		target.Props.SetMinElevation(gpsprot.DegreesFromFloat(15))
		cfg, errCount := configure(t, cp, rcvr, target)
		if errCount != 0 {
			t.Errorf("ErrorCount = %d, want 0", errCount)
		}
		want := casbin.CfgNavLimit{MinSVs: 4, MaxSVs: 40, MinCNO: 8, MinElev: 15}
		if !reflect.DeepEqual(*rcvr.navLimit, want) {
			t.Errorf("CFG-NAVLIMIT\ngot  %+v\nwant %+v", *rcvr.navLimit, want)
		}
		if got, ok := cfg.ConfigProps().GetMinElevation(); !ok || got.Degrees() != 15 {
			t.Errorf("MinElevation = %v,%v, want 15", got.Degrees(), ok)
		}
	})
	t.Run("V5 mask-applied CFG-NAVX", func(t *testing.T) {
		rcvr := &testReceiver{navx: &casbin.CfgNavx{NavSystem: casbin.NavSysGPS, MinElev: 5}}
		cp := probe(t, rcvr)
		target := gpsprot.NewConfigTarget()
		target.Props.SetMinElevation(gpsprot.DegreesFromFloat(10))
		cfg, errCount := configure(t, cp, rcvr, target)
		if errCount != 0 {
			t.Errorf("ErrorCount = %d, want 0", errCount)
		}
		if rcvr.navx.MinElev != 10 {
			t.Errorf("NAVX MinElev = %d, want 10", rcvr.navx.MinElev)
		}
		if got, ok := cfg.ConfigProps().GetMinElevation(); !ok || got.Degrees() != 10 {
			t.Errorf("MinElevation = %v,%v, want 10", got.Degrees(), ok)
		}
	})
}

func TestAntennaCableDelay(t *testing.T) {
	rcvr := &testReceiver{
		monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		tp:     defaultTP(),
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Props.SetAntennaCableDelay(50 * time.Nanosecond)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.tp.UserDelay != 5e-8 {
		t.Errorf("UserDelay = %g, want 5e-08", rcvr.tp.UserDelay)
	}
	if got, ok := cfg.ConfigProps().GetAntennaCableDelay(); !ok || got != 50*time.Nanosecond {
		t.Errorf("AntennaCableDelay = %v,%v, want 50ns", got, ok)
	}
}

func TestV5VersionFromPCAS06(t *testing.T) {
	rcvr := &testReceiver{sw: "URANUS5,V5.3.0.0", hw: "AT6558D,0000000000000"}
	cp := probe(t, rcvr)
	cfg, errCount := configure(t, cp, rcvr, gpsprot.NewConfigTarget())
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	info := cfg.ReceiverInfo()
	if info.Firmware != "SW=URANUS5,V5.3.0.0" || info.Hardware != "HW=AT6558D,0000000000000" {
		t.Errorf("ReceiverInfo = %q / %q, want PCAS06 values", info.Firmware, info.Hardware)
	}
}
