package casic

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/latin1z"
)

// testReceiver simulates a CASIC receiver at the packet level. Requests
// are interpreted from raw packet bytes and answered with raw packet
// bytes, which the test feeds through the real PacketProcessor.
type testReceiver struct {
	t      *testing.T
	monVer *casbin.MonVer // nil simulates V5: MON-VER poll gets NAK
	nmea   []string       // NMEA sentences received (e.g. the probe quiet command)
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
	switch mt := m.(type) {
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
			for _, resp := range rcvr.respond(action.Packet) {
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
