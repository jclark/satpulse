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

const (
	msgOff      = casbin.CfgMsgRateOff
	msgEveryFix = casbin.CfgMsgRateEveryFix
)

// testReceiver simulates a CASIC receiver at the packet level. Requests
// are interpreted from raw packet bytes and answered with raw packet
// bytes, which the test feeds through the real PacketProcessor.
type testReceiver struct {
	t          *testing.T
	monVer     *casbin.MonVer // nil simulates V5: MON-VER poll gets NAK
	nmea       []string       // NMEA sentences received (e.g. the probe quiet command)
	rates      map[casbin.MsgID]casbin.CfgMsgRate
	naks       map[casbin.MsgID]bool // requests answered with NAK
	nakTargets map[casbin.MsgID]bool // CFG-MSG set targets answered with NAK
	silent     map[casbin.MsgID]bool // requests not answered at all
	pending    [][]byte              // delivered before the next request's responses
	staleNak   bool                  // deliver a stale CFG-MSG NAK before the next set's ACK
	saves      []casbin.CfgCfg
	saveBaud   uint32        // newBaud at the time of the first save
	switchLag  time.Duration // simulated host-side gap after a speed change (default 200ms)
	resets     []casbin.CfgRst
	tp         *casbin.CfgTP // nil: CFG-TP unsupported (poll gets NAK)
	tm5        *casbin.CfgTMode
	tm6        *casbin.CfgTMode2
	navx       *casbin.CfgNavx
	navBand    *casbin.CfgNavBand
	sigCap     casbin.CfgNavBandSigIDMask // hardware-receivable signals; clamps written SigIDMask
	ports      []casbin.CfgPrt
	rate       *casbin.CfgRate  // poll response for CFG-RATE
	rateSets   []casbin.CfgRate // CFG-RATE set payloads received
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
		if mt.Mask&casbin.CfgNavxApplyNavSystem != 0 && mt.NavSystem != 0 {
			// Like the 5N71: an empty constellation set is acknowledged
			// but not applied.
			r.navx.NavSystem = mt.NavSystem
		}
		if mt.Mask&casbin.CfgNavxApplyMinElev != 0 {
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
	case *casbin.CfgRate:
		r.rateSets = append(r.rateSets, *mt)
		return [][]byte{r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgRateID)})}
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
		if mt.Rate == casbin.CfgMsgRatePoll && mt.Target == casbin.MonVerID {
			if r.monVer == nil {
				return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgMsgID)})}
			}
			return [][]byte{
				r.pack(r.monVer),
				r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgMsgID)}),
			}
		}
		if mt.Rate != casbin.CfgMsgRatePoll {
			if r.nakTargets[mt.Target] {
				return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgMsgID)})}
			}
			if r.rates == nil {
				r.rates = make(map[casbin.MsgID]casbin.CfgMsgRate)
			}
			r.rates[mt.Target] = mt.Rate
			ack := r.pack(&casbin.AckAck{AckPayload: ackOf(casbin.CfgMsgID)})
			if r.staleNak {
				// A late probe NAK arriving while this set awaits its ACK.
				r.staleNak = false
				return [][]byte{r.pack(&casbin.AckNak{AckPayload: ackOf(casbin.CfgMsgID)}), ack}
			}
			return [][]byte{ack}
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
	packets, _ := cp.ProbePackets()
	for _, p := range packets {
		for _, resp := range rcvr.respond(p) {
			if _, err := pp.ProcessPacket(string(resp), t0); err != nil {
				t.Fatalf("ProcessPacket: %v", err)
			}
			t0 = t0.Add(5 * time.Millisecond)
		}
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

// countSetRequests counts payload-bearing requests for mid. The
// configurator's state queries use empty payloads, so this distinguishes
// property writes from their query and verification polls.
func countSetRequests(cfg *Configurator, mid casbin.MsgID) int {
	n := 0
	for _, req := range cfg.reqs {
		if req.mid == mid && len(req.packet) > casbin.PacketMinLen {
			n++
		}
	}
	return n
}

func countPollRequests(cfg *Configurator, mid casbin.MsgID) int {
	n := 0
	for _, req := range cfg.reqs {
		if req.mid == mid && len(req.packet) == casbin.PacketMinLen {
			n++
		}
	}
	return n
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
		Firmware: "URANUS6,V6.3.2.0",
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

func TestConfigSupport(t *testing.T) {
	v6cfg := func(hw string) *Configurator {
		return newConfigurator(gpsprot.NewConfigTarget(),
			&casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0"), HwVersion: z32(hw)})
	}
	v5 := newConfigurator(gpsprot.NewConfigTarget(), nil)
	base := gpsprot.ConfigSupportSpeed |
		gpsprot.ConfigSupportSurvey | gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportFixedPos | gpsprot.ConfigSupportFixedPosAcc
	v6base := base | gpsprot.ConfigSupportSignal
	tests := []struct {
		name string
		cfg  *Configurator
		want gpsprot.ConfigSupportFlags
	}{
		{"V5", v5, base | gpsprot.ConfigSupportReload},
		{"V6 unclassified", v6cfg(""), v6base | gpsprot.ConfigSupportRaw |
			gpsprot.ConfigSupportSurveyMsg},
		{"V6 navigation", v6cfg("ATGM332D-AT9880-F8N-76"), v6base},
		{"V6 positioning", v6cfg("AT372-AT6668-6P-34"), v6base | gpsprot.ConfigSupportRaw},
		{"V6 timing", v6cfg("AT362-AT6668-6T-30"), v6base | gpsprot.ConfigSupportRaw |
			gpsprot.ConfigSupportSurveyMsg},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.ConfigSupport(); got != tc.want {
				t.Errorf("ConfigSupport() = %v, want %v", got.Items(), tc.want.Items())
			}
		})
	}
}

func TestReceiverClass(t *testing.T) {
	tests := []struct {
		hw   string
		want string
	}{
		{"ATGM332D-AT9880-F8N-76", "N"},
		{"AT372-AT6668-6P-34", "P"},
		{"AT362-AT6668-6T-30", "T"},
		{"ATGM332D-5N71", "N"},
		{"AT999-AT9999-6TS-10", "TS"},
		{"AT999-AT9999-F7F-10", "F"},
		{"AT6558D,0000000000000", ""},
		{"ATGM336H-5", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := receiverClass(tc.hw); got != tc.want {
			t.Errorf("receiverClass(%q) = %q, want %q", tc.hw, got, tc.want)
		}
	}
}

// TestUndeclaredNotAttempted: a capability the receiver's class does
// not declare generates no requests (the flag layer's warning is the
// user-visible outcome).
func TestUndeclaredNotAttempted(t *testing.T) {
	rcvr := &testReceiver{monVer: &casbin.MonVer{
		SwVersion: z32("SW=URANUS6,V6.3.2.0"),
		HwVersion: z32("ATGM332D-AT9880-F8N-76"),
	}}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Opts.RawMsg.Set(gpsprot.RawMsgObs | gpsprot.RawMsgNavData)
	target.Opts.PVTMsg = gpsprot.PVTMsgSurvey | gpsprot.PVTMsgOff
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	for _, mid := range []casbin.MsgID{casbin.Rxm2MeasxID, casbin.Rxm2SfrbxID, casbin.Tim2TimePosID} {
		if _, ok := rcvr.rates[mid]; ok {
			t.Errorf("undeclared message %v was touched", mid)
		}
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
		expect map[casbin.MsgID]casbin.CfgMsgRate
	}{
		{
			name:   "V6 RMC and ZDA",
			monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
			flags:  gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgZDA,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NmeaGsvID: msgOff, casbin.NmeaRmcID: msgEveryFix, casbin.NmeaGgaID: msgOff,
				casbin.NmeaGsaID: msgOff, casbin.NmeaZdaV6ID: msgEveryFix, casbin.NmeaVtgID: msgOff,
				casbin.NmeaGllID:    msgOff,
				casbin.NmeaTxtAntID: msgOff, casbin.NmeaDhvV6ID: msgOff, casbin.NmeaTxtLpsID: msgOff,
				casbin.NmeaTxtInsID: msgOff, casbin.NmeaUtcV6ID: msgOff, casbin.NmeaGstV6ID: msgOff,
				casbin.NmeaTxtRfeID: msgOff,
			},
		},
		{
			// V5 ZDA is 0x08 (V6 uses 0x06); the extra sentences are the
			// V5 id set.
			name:  "V5 ZDA uses 0x08",
			flags: gpsprot.NMEAMsgZDA,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NmeaGsvID: msgOff, casbin.NmeaRmcID: msgOff, casbin.NmeaGgaID: msgOff,
				casbin.NmeaGsaID: msgOff, casbin.NmeaZdaID: msgEveryFix, casbin.NmeaVtgID: msgOff,
				casbin.NmeaGllID: msgOff,
				casbin.NmeaGstID: msgOff, casbin.NmeaAntID: msgOff, casbin.NmeaLpsID: msgOff,
				casbin.NmeaDhvID: msgOff, casbin.NmeaUtcID: msgOff,
			},
		},
		{
			name:  "all off",
			flags: gpsprot.NMEAMsgNone,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NmeaGsvID: msgOff, casbin.NmeaRmcID: msgOff, casbin.NmeaGgaID: msgOff,
				casbin.NmeaGsaID: msgOff, casbin.NmeaZdaID: msgOff, casbin.NmeaVtgID: msgOff,
				casbin.NmeaGllID: msgOff,
				casbin.NmeaGstID: msgOff, casbin.NmeaAntID: msgOff, casbin.NmeaLpsID: msgOff,
				casbin.NmeaDhvID: msgOff, casbin.NmeaUtcID: msgOff,
			},
		},
		{
			// Other means the unmodeled sentences keep their current
			// selection: only the standard seven are touched.
			name:  "Other leaves extra sentences alone",
			flags: gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgOther,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NmeaGsvID: msgOff, casbin.NmeaRmcID: msgEveryFix, casbin.NmeaGgaID: msgOff,
				casbin.NmeaGsaID: msgOff, casbin.NmeaZdaID: msgOff, casbin.NmeaVtgID: msgOff,
				casbin.NmeaGllID: msgOff,
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
	if errCount != 14 {
		t.Errorf("ErrorCount = %d, want 14 (every CFG-MSG set timed out, extras included)", errCount)
	}
}

func TestPVTOut(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name   string
		monVer *casbin.MonVer
		flags  gpsprot.PVTMsgFlags
		tm6    *casbin.CfgTMode2
		survey bool // request a survey, so the TIM2-TIMEPOS gate opens
		expect map[casbin.MsgID]casbin.CfgMsgRate
	}{
		{
			name:   "V6 pos time tp",
			monVer: v6,
			flags:  gpsprot.PVTMsgPos | gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.Nav2PvhID: msgEveryFix, casbin.Nav2TimeUTCID: msgEveryFix,
				casbin.Tim2TpxID: msgEveryFix,
			},
		},
		{
			name:  "V5 pos time",
			flags: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NavPvID: msgEveryFix, casbin.NavTimeUTCID: msgEveryFix,
			},
		},
		{
			name:   "V6 timing PTP",
			monVer: v6,
			flags:  gpsprot.PVTMsgTimingPTP | gpsprot.PVTMsgOff,
			// tp+after+TAI: SOL for TAI time, TIM2-TPX for the pulse;
			// quality adds DOP; leap adds TIM2-LS; no survey was
			// requested, so TIM2-TIMEPOS is turned off with PVH and
			// TIMEUTC.
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.Nav2SolID: msgEveryFix, casbin.Tim2TpxID: msgEveryFix,
				casbin.Nav2DopID: msgEveryFix, casbin.Tim2LsID: msgEveryFix,
				casbin.Tim2TimePosID: msgOff, casbin.Nav2PvhID: msgOff,
				casbin.Nav2TimeUTCID: msgOff,
			},
		},
		{
			name:   "V6 ECEF pos without off is incremental",
			monVer: v6,
			flags:  gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{casbin.Nav2SolID: casbin.CfgMsgRateEveryFix},
		},
		{
			name:   "V6 leap enables TIM2-LS",
			monVer: v6,
			flags:  gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgTimePulse,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.Tim2LsID: casbin.CfgMsgRateEveryFix, casbin.Tim2TpxID: casbin.CfgMsgRateEveryFix},
		},
		{
			name:   "V5 leap enables MSG-GPSUTC",
			flags:  gpsprot.PVTMsgLeapSecond,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{casbin.MsgGPSUTCID: casbin.CfgMsgRateEveryFix},
		},
		{
			// The survey flag alone declares interest (satpulsed sets it
			// unconditionally); without a survey request nothing is sent.
			name:   "V6 survey flag without a survey enables nothing",
			monVer: v6,
			flags:  gpsprot.PVTMsgSurvey,
		},
		{
			name:   "V6 survey request enables TIM2-TIMEPOS",
			monVer: v6,
			flags:  gpsprot.PVTMsgSurvey,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime},
			survey: true,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{casbin.Tim2TimePosID: casbin.CfgMsgRateEveryFix},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer, tm6: tc.tm6}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			target.Opts.PVTMsg = tc.flags
			if tc.survey {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			}
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
		expect map[casbin.MsgID]casbin.CfgMsgRate
	}{
		{
			name:   "V6 sat",
			monVer: v6,
			flags:  gpsprot.SatsMsgSat,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{casbin.Nav2SigID: casbin.CfgMsgRateEveryFix},
		},
		{
			name:   "V6 none turns off",
			monVer: v6,
			flags:  gpsprot.SatsMsgNone,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{casbin.Nav2SigID: casbin.CfgMsgRateOff},
		},
		{
			name:  "V5 sat",
			flags: gpsprot.SatsMsgSat,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NavGPSInfoID: msgEveryFix, casbin.NavBDSInfoID: msgEveryFix,
				casbin.NavGLNInfoID: msgEveryFix,
			},
		},
		{
			name:  "V5 signal only enables nothing",
			flags: gpsprot.SatsMsgSignal,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.NavGPSInfoID: msgOff, casbin.NavBDSInfoID: msgOff,
				casbin.NavGLNInfoID: msgOff,
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

// TestMsgRate verifies that an accepted enable forces the positioning
// interval to 1000 ms via CFG-RATE (a CFG-MSG rate is a per-fix divisor,
// not a frequency, so enabled output runs at 1 Hz only when the interval
// is 1000 ms), while an invocation that enables nothing - only disables,
// or has every enable refused - leaves the interval alone. On V6 the
// accompanying FixRateHz is set to 1; on V5 the trailing bytes are
// reserved and stay zero.
func TestMsgRate(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name       string
		monVer     *casbin.MonVer
		nakTargets []casbin.MsgID
		setup      func(*gpsprot.ConfigTarget)
		expect     []casbin.CfgRate
	}{
		{
			name:  "V5 enable forces 1 Hz, FixRateHz reserved",
			setup: func(tg *gpsprot.ConfigTarget) { tg.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC) },
			expect: []casbin.CfgRate{{FixIntervalMs: casbin.CfgRateFixInterval1Hz,
				FixRateHz: casbin.CfgRateFixRateV5Reserved}},
		},
		{
			name:   "V6 enable forces 1 Hz with FixRateHz 1",
			monVer: v6,
			setup:  func(tg *gpsprot.ConfigTarget) { tg.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC) },
			expect: []casbin.CfgRate{{FixIntervalMs: casbin.CfgRateFixInterval1Hz,
				FixRateHz: casbin.CfgRateFixRate1Hz}},
		},
		{
			name:  "disable only sends no CFG-RATE",
			setup: func(tg *gpsprot.ConfigTarget) { tg.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone) },
		},
		{
			// The only enable is TIM2-TPX and it is refused, so nothing
			// was enabled and the interval must be left alone.
			name:       "all enables refused sends no CFG-RATE",
			monVer:     v6,
			nakTargets: []casbin.MsgID{casbin.Tim2TpxID},
			setup:      func(tg *gpsprot.ConfigTarget) { tg.Opts.PVTMsg = gpsprot.PVTMsgTimePulse },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer}
			if len(tc.nakTargets) > 0 {
				rcvr.nakTargets = make(map[casbin.MsgID]bool)
				for _, mid := range tc.nakTargets {
					rcvr.nakTargets[mid] = true
				}
			}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			tc.setup(target)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(rcvr.rateSets, tc.expect) {
				t.Errorf("CFG-RATE sets\ngot  %+v\nwant %+v", rcvr.rateSets, tc.expect)
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
		tm6          *casbin.CfgTMode2
		setStatic    bool
		save         gpsprot.SaveType
		reset        gpsprot.ResetType
		nakSave      bool // the receiver refuses the CFG-CFG save
		nakRMC       bool // the receiver refuses the NMEA RMC enable
		expectSaves  []casbin.CfgCfg
		expectResets []casbin.CfgRst
		wantErr      int
	}{
		{
			// Enabling a message forces CFG-RATE (a Nav-section set) so
			// the 1 Hz interval persists with the message, so the minimal
			// save covers both the Msg and Nav sections.
			name:        "V5 minimal save of message changes",
			nmea:        gpsprot.NMEAMsgRMC,
			setNMEA:     true,
			save:        gpsprot.SaveMinimal,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionMsg | casbin.CfgCfgSectionNav, OpMode: casbin.CfgCfgOpSave}},
		},
		{
			name:        "V5 minimal save with no changes saves nothing",
			save:        gpsprot.SaveMinimal,
			expectSaves: nil,
		},
		{
			name:        "V5 save all",
			save:        gpsprot.SaveAll,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpSave}},
		},
		{
			name:        "V6 save always uses the all-sections mask",
			monVer:      v6,
			nmea:        gpsprot.NMEAMsgRMC,
			setNMEA:     true,
			save:        gpsprot.SaveMinimal,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpSave}},
		},
		{
			// A V6 set that is not a message change (here a time-mode
			// set) must still make the minimal save fire; the section
			// mask V6 ignores becomes the all-sections mask.
			name:        "V6 minimal save of a time-mode change",
			monVer:      v6,
			tm6:         &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime},
			setStatic:   true,
			save:        gpsprot.SaveMinimal,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpSave}},
		},
		{
			name:   "V6 reload is unsupported and sends nothing",
			monVer: v6,
			reset:  gpsprot.ResetReload,
		},
		{
			name:        "V5 reload",
			reset:       gpsprot.ResetReload,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpLoad}},
		},
		{
			name:  "V5 cold reset",
			reset: gpsprot.ResetCold,
			expectResets: []casbin.CfgRst{{NavBbrMask: bbrReset,
				ResetMode: casbin.CfgRstResetHardwareImmediate, StartMode: casbin.CfgRstStartCold}},
		},
		{
			name:   "V6 factory reset",
			monVer: v6,
			reset:  gpsprot.ResetFactory,
			expectResets: []casbin.CfgRst{{NavBbrMask: casbin.CfgRstNavBbrV6Reserved,
				ResetMode: casbin.CfgRstResetModeV6Reserved, StartMode: casbin.CfgRstStartFactory}},
		},
		{
			// Save and reset together: the reset is generated in a later
			// phase, only after the save is final, so both go out.
			name:        "V5 save all then cold reset",
			save:        gpsprot.SaveAll,
			reset:       gpsprot.ResetCold,
			expectSaves: []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpSave}},
			expectResets: []casbin.CfgRst{{NavBbrMask: bbrReset,
				ResetMode: casbin.CfgRstResetHardwareImmediate, StartMode: casbin.CfgRstStartCold}},
		},
		{
			// Save and reload share a class+id, so this pair crosses the
			// save/reset phase split as two CFG-CFG messages: the save
			// must complete first so the reload restores what was saved.
			name:  "V5 save all then reload",
			save:  gpsprot.SaveAll,
			reset: gpsprot.ResetReload,
			expectSaves: []casbin.CfgCfg{
				{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpSave},
				{Mask: casbin.CfgCfgSectionAll, OpMode: casbin.CfgCfgOpLoad},
			},
		},
		{
			// A refused save gates its paired reload just like a reset:
			// the reload must not discard the changes the save failed to
			// persist.
			name:         "V5 refused save blocks reload",
			save:         gpsprot.SaveAll,
			reset:        gpsprot.ResetReload,
			nakSave:      true,
			expectSaves:  nil,
			expectResets: nil,
			wantErr:      1,
		},
		{
			// A refused save gates its paired reset: the save is the
			// reported error and no reset is generated, so the reset
			// cannot discard the unsaved running changes.
			name:         "V5 refused save blocks cold reset",
			save:         gpsprot.SaveAll,
			reset:        gpsprot.ResetCold,
			nakSave:      true,
			expectSaves:  nil,
			expectResets: nil,
			wantErr:      1,
		},
		{
			// NVM is written only by an invocation in which everything
			// succeeded: the ACKed disables would have made the minimal
			// save fire, but the refused enable prevents it. The failure
			// is the invocation's only reported error.
			name:        "V5 refused enable blocks minimal save",
			nmea:        gpsprot.NMEAMsgRMC,
			setNMEA:     true,
			nakRMC:      true,
			save:        gpsprot.SaveMinimal,
			expectSaves: nil,
			wantErr:     1,
		},
		{
			// The skipped save gates its paired reset exactly as a
			// refused save does.
			name:         "V6 refused enable blocks save all and cold reset",
			monVer:       v6,
			nmea:         gpsprot.NMEAMsgRMC,
			setNMEA:      true,
			nakRMC:       true,
			save:         gpsprot.SaveAll,
			reset:        gpsprot.ResetCold,
			expectSaves:  nil,
			expectResets: nil,
			wantErr:      1,
		},
		{
			// The failure gate is the save's: a reset with no save
			// requested proceeds after a failure (a reset discards
			// running changes by design and writes nothing to NVM).
			name:    "V5 refused enable does not block a lone cold reset",
			nmea:    gpsprot.NMEAMsgRMC,
			setNMEA: true,
			nakRMC:  true,
			reset:   gpsprot.ResetCold,
			expectResets: []casbin.CfgRst{{NavBbrMask: bbrReset,
				ResetMode: casbin.CfgRstResetHardwareImmediate, StartMode: casbin.CfgRstStartCold}},
			wantErr: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tc.monVer, tm6: tc.tm6}
			if tc.nakSave {
				rcvr.naks = map[casbin.MsgID]bool{casbin.CfgCfgID: true}
			}
			if tc.nakRMC {
				rcvr.nakTargets = map[casbin.MsgID]bool{casbin.NmeaRmcID: true}
			}
			cp := probe(t, rcvr)
			target := gpsprot.NewConfigTarget()
			if tc.setNMEA {
				target.Opts.NMEAMsg.Set(tc.nmea)
			}
			target.Opts.SetStatic = tc.setStatic
			target.Opts.Save = tc.save
			target.Opts.Reset = tc.reset
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != tc.wantErr {
				t.Errorf("ErrorCount = %d, want %d", errCount, tc.wantErr)
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

// TestTimTPRefused covers a receiver that refuses the time-of-pulse
// enable: pulse-time information is not deliverable there, which
// shows as absence, not an error; the requested time message is
// unaffected.
func TestTimTPRefused(t *testing.T) {
	rcvr := &testReceiver{
		monVer:     &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		nakTargets: map[casbin.MsgID]bool{casbin.Tim2TpxID: true},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Opts.PVTMsg = gpsprot.PVTMsgTime | gpsprot.PVTMsgTimePulse
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	expect := map[casbin.MsgID]casbin.CfgMsgRate{casbin.Nav2TimeUTCID: casbin.CfgMsgRateEveryFix}
	if !reflect.DeepEqual(rcvr.rates, expect) {
		t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, expect)
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
	packets, _ := cp.ProbePackets()
	cp.ProbePackets() // second probe, as gpscfg sends after probeRetryDelay
	for _, resp := range rcvr.respond(packets[0]) {
		if _, err := pp.ProcessPacket(string(resp), time.Unix(1, 0)); err != nil {
			t.Fatalf("ProcessPacket: %v", err)
		}
	}
	if !cp.ProbeOK() {
		t.Fatal("ProbeOK = false")
	}
	// The second probe's NAK arrives only while the configurator's
	// first CFG-MSG set is awaiting its ACK; the protocol must consume
	// it (pollsPending) rather than let it fail that set.
	rcvr.staleNak = true
	_, errCount := configure(t, cp, rcvr, nmeaTarget(gpsprot.NMEAMsgRMC))
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0 (late probe NAK misattributed)", errCount)
	}
	if rcvr.rates[casbin.NmeaRmcID] != 1 {
		t.Errorf("RMC rate = %d, want 1", rcvr.rates[casbin.NmeaRmcID])
	}
}

// TestLateAckRejected pins the handleAck response window: an ACK read
// outside maxResponseDelay of the send (a response to an earlier send
// of a since-resent request) or before the send must not complete the
// request; a timely ACK still does.
func TestLateAckRejected(t *testing.T) {
	rcvr := &testReceiver{monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}}
	cp := probe(t, rcvr)
	cfgI, err := cp.Configure(nmeaTarget(gpsprot.NMEAMsgRMC))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg := cfgI.(*Configurator)
	if err := cfg.GenerateRequests(); err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	req := cfg.reqs[0]
	t0 := time.Unix(100, 0)
	req.SetSentTime(t0)
	ack := &casbin.AckAck{AckPayload: ackOf(casbin.CfgMsgID)}
	cfg.nativeMsg(ack, t0.Add(maxResponseDelay+time.Second))
	if req.state != reqAwaitingAck {
		t.Fatalf("state = %v after out-of-window ACK, want awaiting", req.state)
	}
	cfg.nativeMsg(ack, t0.Add(-time.Second))
	if req.state != reqAwaitingAck {
		t.Fatalf("state = %v after before-send ACK, want awaiting", req.state)
	}
	cfg.nativeMsg(ack, t0.Add(50*time.Millisecond))
	if req.state != reqSucceeded {
		t.Fatalf("state = %v after timely ACK, want succeeded", req.state)
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

func defaultTPV6() *casbin.CfgTP {
	return &casbin.CfgTP{Interval: 1000000, Width: 100000,
		PPSOutMode: casbin.CfgTPPPSOutV6PositionTimeValid,
		TBase:      casbin.CfgTPTBaseV6UTC,
		TSrcMode:   casbin.CfgTPTSrcV6PrimaryGPS}
}

func TestPropertySetComparison(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	const signalMask = casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigGLOL1
	signals := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGLOL1)
	type testCase struct {
		name           string
		rcvr           *testReceiver
		setup          func(*gpsprot.ConfigTarget)
		mid            casbin.MsgID
		wantSets       int
		wantPolls      int
		extraPollMid   casbin.MsgID
		wantExtraPolls int
		check          func(*testing.T, *Configurator, *testReceiver)
	}
	tests := []testCase{
		{
			name: "time pulse encoded no-op",
			rcvr: func() *testReceiver {
				tp := defaultTPV6()
				tp.Width = 100001
				tp.UserDelay = casbin.CfgTPUserDelaySeconds(50 * time.Nanosecond)
				return &testReceiver{monVer: v6, tp: tp}
			}(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetTimePulseWidth(100*time.Millisecond + 750*time.Nanosecond)
				target.Props.SetAntennaCableDelay(50 * time.Nanosecond)
			},
			mid: casbin.CfgTPID, wantPolls: 1,
		},
		{
			name: "time pulse change",
			rcvr: &testReceiver{monVer: v6, tp: defaultTPV6()},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetTimePulseWidth(100*time.Millisecond + 750*time.Nanosecond)
			},
			mid: casbin.CfgTPID, wantSets: 1, wantPolls: 1,
		},
		{
			name: "V6 time mode no-op",
			rcvr: &testReceiver{monVer: v6, tm6: &casbin.CfgTMode2{
				TimFixMode: casbin.CfgTMode2Survey, BandMode: casbin.CfgTMode2BandL1,
				TSrcMode: casbin.CfgTMode2TSrcForceGLN, SvinMinDur: 300, SvinPaccLim: 20000,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			mid: casbin.CfgTMode2ID, wantPolls: 1,
		},
		{
			name: "V6 time mode change",
			rcvr: &testReceiver{monVer: v6, tm6: &casbin.CfgTMode2{
				TimFixMode: casbin.CfgTMode2Survey, SvinMinDur: 299, SvinPaccLim: 20000,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			mid: casbin.CfgTMode2ID, wantSets: 1, wantPolls: 1,
		},
		{
			name: "V5 time mode ignores upper mode garbage",
			rcvr: &testReceiver{tm5: &casbin.CfgTMode{
				Mode: casbin.CfgTModeSurvey, Res: 0xA5A5, SvinMinDur: 300, SvinVarLimit: 400,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			mid: casbin.CfgTModeID, wantPolls: 1,
		},
		{
			name: "V5 time mode change",
			rcvr: &testReceiver{tm5: &casbin.CfgTMode{
				Mode: casbin.CfgTModeSurvey, Res: 0xA5A5, SvinMinDur: 299, SvinVarLimit: 400,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			mid: casbin.CfgTModeID, wantSets: 1, wantPolls: 1,
		},
		{
			name: "V6 signal no-op skips verification poll",
			rcvr: &testReceiver{monVer: v6, sigCap: signalMask, navBand: &casbin.CfgNavBand{
				SigBandAuto: casbin.CfgNavBandManual, Res1: 0xA5, Res2: 0x5A5A,
				SigIDMaskFix: signalMask, SigIDMask: signalMask,
			}},
			setup: func(target *gpsprot.ConfigTarget) { target.Props.SetSignalsEnabled(signals) },
			mid:   casbin.CfgNavBandID, wantPolls: 1,
		},
		{
			name: "V6 signal change keeps verification poll",
			rcvr: &testReceiver{monVer: v6, sigCap: signalMask, navBand: &casbin.CfgNavBand{
				SigBandAuto:  casbin.CfgNavBandManual,
				SigIDMaskFix: casbin.CfgNavBandSigGPSL1CA, SigIDMask: casbin.CfgNavBandSigGPSL1CA,
			}},
			setup: func(target *gpsprot.ConfigTarget) { target.Props.SetSignalsEnabled(signals) },
			mid:   casbin.CfgNavBandID, wantSets: 1, wantPolls: 2,
		},
		{
			name: "V5 signal no-op ignores mask and reserved fields",
			rcvr: &testReceiver{navx: &casbin.CfgNavx{
				Mask: casbin.CfgNavxApplyMinElev, Res1: 0xA5,
				NavSystem: casbin.CfgNavxNavSystemGPS | casbin.CfgNavxNavSystemGLN,
			}},
			setup: func(target *gpsprot.ConfigTarget) { target.Props.SetSignalsEnabled(signals) },
			mid:   casbin.CfgNavxID, wantPolls: 1,
		},
		{
			name: "V5 signal change",
			rcvr: &testReceiver{navx: &casbin.CfgNavx{
				NavSystem: casbin.CfgNavxNavSystemGPS | casbin.CfgNavxNavSystemGLN,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA))
			},
			mid: casbin.CfgNavxID, wantSets: 1, wantPolls: 1,
		},
		{
			name: "V6 minimum elevation encoded no-op",
			rcvr: &testReceiver{monVer: v6, navLimit: &casbin.CfgNavLimit{
				MinSVs: 4, MaxSVs: 40, MinCNO: 8, MinElev: 15, Res: 0xA5A55A5A,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMinElevation(gpsprot.DegreesFromFloat(14.2))
			},
			mid: casbin.CfgNavLimID, wantPolls: 1,
		},
		{
			name: "V6 minimum elevation change",
			rcvr: &testReceiver{monVer: v6, navLimit: &casbin.CfgNavLimit{MinElev: 14}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMinElevation(gpsprot.DegreesFromFloat(14.2))
			},
			mid: casbin.CfgNavLimID, wantSets: 1, wantPolls: 1,
		},
		{
			name: "V5 minimum elevation no-op ignores mask and reserved fields",
			rcvr: &testReceiver{navx: &casbin.CfgNavx{
				Mask: casbin.CfgNavxApplyNavSystem, Res1: 0xA5, MinElev: 15,
			}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMinElevation(gpsprot.DegreesFromFloat(14.2))
			},
			mid: casbin.CfgNavxID, wantPolls: 1,
		},
		{
			name: "V5 minimum elevation change",
			rcvr: &testReceiver{navx: &casbin.CfgNavx{MinElev: 14}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMinElevation(gpsprot.DegreesFromFloat(14.2))
			},
			mid: casbin.CfgNavxID, wantSets: 1, wantPolls: 1,
		},
		{
			name: "baud no-op skips write confirmation and minimal save",
			rcvr: &testReceiver{monVer: v6, ports: []casbin.CfgPrt{{
				PortID: casbin.CfgPrtPortUART0, ProtoMask: casbin.CfgPrtProtoBinaryIn,
				Mode: casbin.CfgPrtModeCharLen8, BaudRate: 115200,
			}}},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetBaudRate(115200)
				target.Opts.Save = gpsprot.SaveMinimal
			},
			mid: casbin.CfgPrtID, wantPolls: 1,
			extraPollMid: casbin.CfgRateID,
			check: func(t *testing.T, cfg *Configurator, rcvr *testReceiver) {
				if len(rcvr.saves) != 0 {
					t.Errorf("saves = %+v, want none", rcvr.saves)
				}
				if got, ok := cfg.ConfigProps().GetBaudRate(); !ok || got != 115200 {
					t.Errorf("BaudRate = %d,%v, want 115200,true", got, ok)
				}
			},
		},
		{
			name: "baud change keeps confirmation poll",
			rcvr: &testReceiver{monVer: v6, ports: []casbin.CfgPrt{{
				PortID: casbin.CfgPrtPortUART0, ProtoMask: casbin.CfgPrtProtoBinaryIn,
				Mode: casbin.CfgPrtModeCharLen8, BaudRate: 115200,
			}}, rate: &casbin.CfgRate{FixIntervalMs: casbin.CfgRateFixInterval1Hz,
				FixRateHz: casbin.CfgRateFixRate1Hz}},
			setup: func(target *gpsprot.ConfigTarget) { target.Props.SetBaudRate(38400) },
			mid:   casbin.CfgPrtID, wantSets: 1, wantPolls: 1,
			extraPollMid: casbin.CfgRateID, wantExtraPolls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cp := probe(t, tc.rcvr)
			target := gpsprot.NewConfigTarget()
			tc.setup(target)
			cfg, errCount := configure(t, cp, tc.rcvr, target)
			if errCount != 0 {
				t.Fatalf("ErrorCount = %d, want 0", errCount)
			}
			if got := countSetRequests(cfg, tc.mid); got != tc.wantSets {
				t.Errorf("%v sets = %d, want %d", tc.mid, got, tc.wantSets)
			}
			if got := countPollRequests(cfg, tc.mid); got != tc.wantPolls {
				t.Errorf("%v polls = %d, want %d", tc.mid, got, tc.wantPolls)
			}
			if tc.extraPollMid != 0 {
				if got := countPollRequests(cfg, tc.extraPollMid); got != tc.wantExtraPolls {
					t.Errorf("%v polls = %d, want %d", tc.extraPollMid, got, tc.wantExtraPolls)
				}
			}
			if tc.check != nil {
				tc.check(t, cfg, tc.rcvr)
			}
		})
	}
}

func TestMixedPropertyTargetsSuppressOnlyNoOps(t *testing.T) {
	rcvr := &testReceiver{
		tp: &casbin.CfgTP{Interval: 1000000, Width: 100000,
			PPSOutMode: casbin.CfgTPPPSOutV5On, TBase: casbin.CfgTPTBaseV5UTC,
			TSrcMode: casbin.CfgTPTSrcV5ForceGPS},
		navx: &casbin.CfgNavx{Mask: casbin.CfgNavxApplyDynModel, Res1: 0xA5,
			NavSystem: casbin.CfgNavxNavSystemGPS, MinElev: 5},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Props.SetTimePulseWidth(100 * time.Millisecond)
	target.Props.SetSignalsEnabled(gpsprot.SignalSetOf(gpsprot.SigGPSL1CA))
	target.Props.SetMinElevation(gpsprot.DegreesFromFloat(10))
	target.Opts.Save = gpsprot.SaveMinimal
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Fatalf("ErrorCount = %d, want 0", errCount)
	}
	if got := countSetRequests(cfg, casbin.CfgTPID); got != 0 {
		t.Errorf("CFG-TP sets = %d, want 0", got)
	}
	if got := countSetRequests(cfg, casbin.CfgNavxID); got != 1 {
		t.Fatalf("CFG-NAVX sets = %d, want 1", got)
	}
	if rcvr.navx.NavSystem != casbin.CfgNavxNavSystemGPS || rcvr.navx.MinElev != 10 {
		t.Errorf("CFG-NAVX = %+v, want GPS unchanged and MinElev 10", rcvr.navx)
	}
	wantSaves := []casbin.CfgCfg{{Mask: casbin.CfgCfgSectionNav, OpMode: casbin.CfgCfgOpSave}}
	if !reflect.DeepEqual(rcvr.saves, wantSaves) {
		t.Errorf("saves = %+v, want %+v", rcvr.saves, wantSaves)
	}
}

func TestTimePulseSet(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name      string
		monVer    *casbin.MonVer
		tp        *casbin.CfgTP
		setup     func(*gpsprot.ConfigTarget)
		expectTP  casbin.CfgTP
		expectOff bool
	}{
		{
			name:   "V6 PPS with GPS time",
			monVer: v6,
			tp:     defaultTPV6(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetPPS(100 * time.Millisecond)
				target.Props.SetTimeGNSS(gpsprot.GPS)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 100000,
				PPSOutMode: casbin.CfgTPPPSOutV6PositionTimeReliable,
				Polarity:   casbin.CfgTPPolarityRising,
				TBase:      casbin.CfgTPTBaseV6GNSS,
				TSrcMode:   casbin.CfgTPTSrcV6ForceGPS},
		},
		{
			name: "V5 PPS inverts TBase and uses fix-only mode",
			tp: &casbin.CfgTP{Interval: 1000000, Width: 100000,
				PPSOutMode: casbin.CfgTPPPSOutV5On,
				TBase:      casbin.CfgTPTBaseV5UTC,
				TSrcMode:   casbin.CfgTPTSrcV5ForceGPS},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetPPS(200 * time.Millisecond)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 200000,
				PPSOutMode: casbin.CfgTPPPSOutV5FixOnly,
				Polarity:   casbin.CfgTPPolarityRising,
				TBase:      casbin.CfgTPTBaseV5Satellite,
				TSrcMode:   casbin.CfgTPTSrcV5ForceGPS},
		},
		{
			name:   "disable pulse",
			monVer: v6,
			tp:     defaultTPV6(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetTimePulseWidth(0)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 100000,
				PPSOutMode: casbin.CfgTPPPSOutV6Off,
				TBase:      casbin.CfgTPTBaseV6UTC,
				TSrcMode:   casbin.CfgTPTSrcV6PrimaryGPS},
			expectOff: true,
		},
		{
			name:   "width and period round to nearest microsecond",
			monVer: v6,
			tp:     defaultTPV6(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetTimePulseWidth(100*time.Millisecond + 750*time.Nanosecond)
				target.Props.SetTimePulsePeriod(time.Second - 750*time.Nanosecond)
			},
			expectTP: casbin.CfgTP{Interval: 999999, Width: 100001,
				PPSOutMode: casbin.CfgTPPPSOutV6PositionTimeValid,
				TBase:      casbin.CfgTPTBaseV6UTC,
				TSrcMode:   casbin.CfgTPTSrcV6PrimaryGPS},
		},
		{
			name:   "falling polarity",
			monVer: v6,
			tp:     defaultTPV6(),
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetTimePulseWidth(100 * time.Millisecond)
				target.Props.SetTimePulsePolarityRising(false)
			},
			expectTP: casbin.CfgTP{Interval: 1000000, Width: 100000,
				PPSOutMode: casbin.CfgTPPPSOutV6PositionTimeValid,
				Polarity:   casbin.CfgTPPolarityFalling,
				TBase:      casbin.CfgTPTBaseV6UTC,
				TSrcMode:   casbin.CfgTPTSrcV6PrimaryGPS},
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
			wantWidth := casbin.CfgTPDuration(tc.expectTP.Width)
			if tc.expectOff {
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
		tp: &casbin.CfgTP{Interval: 1000000, Width: 100000,
			PPSOutMode: casbin.CfgTPPPSOutV6PositionTimeReliable,
			TBase:      casbin.CfgTPTBaseV6GNSS,
			TSrcMode:   casbin.CfgTPTSrcV6ForceBDS},
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
			tm6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime,
				BandMode: casbin.CfgTMode2BandL1, TSrcMode: casbin.CfgTMode2TSrcForceGLN},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Opts.SetStatic = true
				target.Opts.Survey = gpsprot.Survey{MinDur: 2000 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey,
				BandMode: casbin.CfgTMode2BandL1, TSrcMode: casbin.CfgTMode2TSrcForceGLN,
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
			// LLH is converted to ECEF: lat 0, lon 0, height 0 is
			// (a, 0, 0) with the WGS84 semi-major axis.
			name:   "V6 fixed position from LLH",
			monVer: v6,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{
					Static:      true,
					PosType:     gpsprot.PosTypeLLH,
					FixedPosAcc: 5 * gpsprot.Meter,
				})
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Fixed,
				XFixed: 637813700, YFixed: 0, ZFixed: 0, FixedPacc: 5000},
			static:  true,
			hasMode: true,
		},
		{
			// A mobile Mode alongside SetStatic is overridden to
			// static: with no position, a survey.
			name:   "V6 mobile overridden by setStatic",
			monVer: v6,
			tm6:    &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Realtime},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: false})
				target.Opts.SetStatic = true
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			expect6: &casbin.CfgTMode2{TimFixMode: casbin.CfgTMode2Survey,
				SvinMinDur: 300, SvinPaccLim: 20000},
			static:  true,
			hasMode: true,
		},
		{
			name: "V5 survey uses variances",
			tm5:  &casbin.CfgTMode{Mode: casbin.CfgTModeAuto},
			setup: func(target *gpsprot.ConfigTarget) {
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{MinDur: 300 * time.Second, AccLimit: 20 * gpsprot.Meter}
			},
			expect5: &casbin.CfgTMode{Mode: casbin.CfgTModeSurvey, SvinMinDur: 300, SvinVarLimit: 400},
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

func TestSurveyAgainRestartsV5(t *testing.T) {
	rcvr := &testReceiver{tm5: &casbin.CfgTMode{
		Mode: casbin.CfgTModeSurvey, Res: 0xA5A5, SvinMinDur: 300, SvinVarLimit: 400,
	}}
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
	if rcvr.tm5.Mode != casbin.CfgTModeSurvey || rcvr.tm5.SvinMinDur != 600 {
		t.Errorf("final CFG-TMODE = %+v, want survey with 600 s", rcvr.tm5)
	}
	if sets := countSetRequests(cfg, casbin.CfgTModeID); sets != 2 {
		t.Errorf("CFG-TMODE sets = %d, want 2 (auto then survey)", sets)
	}
}

func TestSignalSelection(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	const f8nMask = casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigGPSL5 |
		casbin.CfgNavBandSigSBASL1 | casbin.CfgNavBandSigSBASL5 |
		casbin.CfgNavBandSigGLOL1 | casbin.CfgNavBandSigGALE1 |
		casbin.CfgNavBandSigGALE5a | casbin.CfgNavBandSigBDSB1IGEO |
		casbin.CfgNavBandSigBDSB1IMEO | casbin.CfgNavBandSigBDSB1C |
		casbin.CfgNavBandSigBDSB2a | casbin.CfgNavBandSigQZSSL1CA |
		casbin.CfgNavBandSigQZSSL5
	const at632Mask = casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigSBASL1 |
		casbin.CfgNavBandSigGLOL1 | casbin.CfgNavBandSigGALE1 |
		casbin.CfgNavBandSigBDSB1IGEO | casbin.CfgNavBandSigBDSB1IMEO |
		casbin.CfgNavBandSigBDSB1C | casbin.CfgNavBandSigQZSSL1CA
	tests := []struct {
		name          string
		monVer        *casbin.MonVer
		navx          *casbin.CfgNavx
		navBand       *casbin.CfgNavBand
		request       gpsprot.SignalSet
		expectMaskFix casbin.CfgNavBandSigIDMask
		expectSys     casbin.CfgNavxNavSystem
		expectSignals gpsprot.SignalSet
	}{
		{
			name:    "V6 dual-band GPS and GAL",
			monVer:  v6,
			navBand: &casbin.CfgNavBand{SigBandAuto: casbin.CfgNavBandAutomatic, SigIDMaskFix: f8nMask, SigIDMask: f8nMask},
			request: gpsprot.SigSetGPS | gpsprot.SigSetGAL,
			expectMaskFix: casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigGPSL5 |
				casbin.CfgNavBandSigGALE1 | casbin.CfgNavBandSigGALE5a,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL5,
				gpsprot.SigGALE1, gpsprot.SigGALE5a),
		},
		{
			// The receiver clamps the written reception list to its
			// hardware: L5-band signals drop out in the readback.
			name:    "V6 L1-only hardware clamps GPS L5 and GAL E5a away",
			monVer:  v6,
			navBand: &casbin.CfgNavBand{SigBandAuto: casbin.CfgNavBandAutomatic, SigIDMaskFix: at632Mask, SigIDMask: at632Mask},
			request: gpsprot.SigSetGPS | gpsprot.SigSetGAL,
			expectMaskFix: casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigGPSL5 |
				casbin.CfgNavBandSigGALE1 | casbin.CfgNavBandSigGALE5a,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1),
		},
		{
			name: "V5 constellation level",
			navx: &casbin.CfgNavx{NavSystem: casbin.CfgNavxNavSystemGPS |
				casbin.CfgNavxNavSystemBDS | casbin.CfgNavxNavSystemGLN},
			request:       gpsprot.SigSetGPS | gpsprot.SigSetGAL,
			expectSys:     casbin.CfgNavxNavSystemGPS,
			expectSignals: gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
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
				if rcvr.navBand.SigBandAuto != casbin.CfgNavBandManual || rcvr.navBand.SigIDMaskFix != tc.expectMaskFix {
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

// TestSignalRequestUnsupported checks the guard on requests whose
// intersection with what the backend can express keeps no major-GNSS
// signal: the request fails instead of disabling every constellation,
// nothing is written, and the reported set is the current one from the
// query readback.
func TestSignalRequestUnsupported(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	const curMask = casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigGLOL1
	tests := []struct {
		name    string
		monVer  *casbin.MonVer
		navx    *casbin.CfgNavx
		navBand *casbin.CfgNavBand
		request gpsprot.SignalSet
	}{
		{
			name: "V5 all-GAL request empties the intersection",
			navx: &casbin.CfgNavx{NavSystem: casbin.CfgNavxNavSystemGPS |
				casbin.CfgNavxNavSystemGLN},
			request: gpsprot.SigSetGAL,
		},
		{
			name:    "V6 signal outside the CASIC universe",
			monVer:  v6,
			navBand: &casbin.CfgNavBand{SigBandAuto: casbin.CfgNavBandAutomatic, SigIDMaskFix: curMask, SigIDMask: curMask},
			request: gpsprot.SignalSetOf(gpsprot.SigGPSL2C),
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
			if errCount != 1 {
				t.Errorf("ErrorCount = %d, want 1", errCount)
			}
			if tc.navx != nil && rcvr.navx.NavSystem != tc.navx.NavSystem {
				t.Errorf("NavSystem = %#x, want unchanged %#x", rcvr.navx.NavSystem, tc.navx.NavSystem)
			}
			if tc.navBand != nil && (rcvr.navBand.SigBandAuto != casbin.CfgNavBandAutomatic || rcvr.navBand.SigIDMaskFix != curMask) {
				t.Errorf("NAVBAND changed: %+v", rcvr.navBand)
			}
			want := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGLOL1)
			if got, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || got != want {
				t.Errorf("SignalsEnabled = %v,%v, want %v", got, ok, want)
			}
		})
	}
}

func TestSignalGetWithAutoBand(t *testing.T) {
	rcvr := &testReceiver{
		monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		navBand: &casbin.CfgNavBand{SigBandAuto: casbin.CfgNavBandAutomatic,
			SigIDMask: casbin.CfgNavBandSigGPSL1CA | casbin.CfgNavBandSigGLOL1},
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

// TestPromoteSerializesCFG verifies the spec's one-CFG-at-a-time rule
// (casic2.md 2.5 / zkw3.md 3.5): with several distinct-id CFG requests
// pending, promote readies only one at a time, and they proceed in
// slice order as each is resolved.
func TestPromoteSerializesCFG(t *testing.T) {
	mids := []casbin.MsgID{casbin.CfgTPID, casbin.CfgRateID, casbin.CfgTMode2ID}
	c := &Configurator{}
	for _, mid := range mids {
		c.add(&casReq{mid: mid})
	}
	for i := range mids {
		c.promote()
		for j, req := range c.reqs {
			want := reqNotReady
			switch {
			case j < i:
				want = reqSucceeded
			case j == i:
				want = reqReady
			}
			if req.state != want {
				t.Fatalf("with %d resolved, req %d state = %v, want %v", i, j, req.state, want)
			}
		}
		c.reqs[i].state = reqSucceeded
	}
}

// TestBaudChange also covers the speed-change carve-out in promote: the
// silentPrt cases leave the sent CFG-PRT baud change awaiting an ACK
// that never arrives (it would come garbled at the new speed), so the
// following CFG-RATE confirmation poll - itself a CFG message - must
// still be promoted and transmitted at the new rate; without the
// carve-out it would be blocked and the change would deadlock into
// timeout, failing the test.
func TestBaudChange(t *testing.T) {
	ports := []casbin.CfgPrt{
		{PortID: casbin.CfgPrtPortUART0,
			ProtoMask: casbin.CfgPrtProtoBinaryIn | casbin.CfgPrtProtoTextIn |
				casbin.CfgPrtProtoBinaryOut | casbin.CfgPrtProtoTextOut,
			Mode: casbin.CfgPrtMode(0x0003), BaudRate: 115200},
		{PortID: casbin.CfgPrtPortUART1,
			ProtoMask: casbin.CfgPrtProtoBinaryIn | casbin.CfgPrtProtoTextIn |
				casbin.CfgPrtProtoBinaryOut | casbin.CfgPrtProtoTextOut,
			Mode: casbin.CfgPrtMode(0x0003), BaudRate: 115200},
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
				monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
				ports:  append([]casbin.CfgPrt{}, ports...),
				rate: &casbin.CfgRate{FixIntervalMs: casbin.CfgRateFixInterval1Hz,
					FixRateHz: casbin.CfgRateFixRate1Hz},
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

func TestShowPort(t *testing.T) {
	rcvr := &testReceiver{
		monVer: &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")},
		ports: []casbin.CfgPrt{
			{PortID: casbin.CfgPrtPortUART0,
				ProtoMask: casbin.CfgPrtProtoBinaryIn | casbin.CfgPrtProtoTextIn |
					casbin.CfgPrtProtoBinaryOut | casbin.CfgPrtProtoTextOut,
				Mode: casbin.CfgPrtMode(0x0003), BaudRate: 115200},
			{PortID: casbin.CfgPrtPortUART1,
				ProtoMask: casbin.CfgPrtProtoBinaryIn | casbin.CfgPrtProtoTextIn |
					casbin.CfgPrtProtoBinaryOut | casbin.CfgPrtProtoTextOut,
				Mode: casbin.CfgPrtMode(0x0003), BaudRate: 9600},
		},
	}
	cp := probe(t, rcvr)
	target := gpsprot.NewConfigTarget()
	target.Get = gpsprot.PropIDPort | gpsprot.PropIDBaudRate
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	// The serial speed is reported from the wired UART (port 0).
	if got, ok := cfg.ConfigProps().GetBaudRate(); !ok || got != 115200 {
		t.Errorf("baud = %d,%v, want 115200,true", got, ok)
	}
	// CASIC cannot identify the active port, so it reports no port name
	// and does not advertise ConfigSupportPort.
	if _, ok := cfg.ConfigProps().GetPort(); ok {
		t.Error("ConfigProps reports a port despite CASIC being unable to identify it")
	}
	if cfg.ConfigSupport()&gpsprot.ConfigSupportPort != 0 {
		t.Error("CASIC must not advertise ConfigSupportPort")
	}
}

func TestRawOut(t *testing.T) {
	v6 := &casbin.MonVer{SwVersion: z32("SW=URANUS6,V6.3.2.0")}
	tests := []struct {
		name   string
		monVer *casbin.MonVer
		flags  gpsprot.RawMsgFlags
		expect map[casbin.MsgID]casbin.CfgMsgRate
	}{
		{
			name:   "V6 obs and nav",
			monVer: v6,
			flags:  gpsprot.RawMsgObs | gpsprot.RawMsgNavData,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.Rxm2MeasxID: casbin.CfgMsgRateEveryFix, casbin.Rxm2SfrbxID: casbin.CfgMsgRateEveryFix},
		},
		{
			name:   "V6 obs only turns nav off",
			monVer: v6,
			flags:  gpsprot.RawMsgObs,
			expect: map[casbin.MsgID]casbin.CfgMsgRate{
				casbin.Rxm2MeasxID: casbin.CfgMsgRateEveryFix, casbin.Rxm2SfrbxID: casbin.CfgMsgRateOff},
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
		target.Props.SetMinElevation(gpsprot.DegreesFromFloat(14.2)) // Ceil to 15
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
		rcvr := &testReceiver{navx: &casbin.CfgNavx{NavSystem: casbin.CfgNavxNavSystemGPS, MinElev: 5}}
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
		tp:     defaultTPV6(),
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
	if info.Firmware != "URANUS5,V5.3.0.0" || info.Hardware != "AT6558D,0000000000000" {
		t.Errorf("ReceiverInfo = %q / %q, want PCAS06 values", info.Firmware, info.Hardware)
	}
}
