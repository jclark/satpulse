package septentrio

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
)

// TestCorReportDiffCorrInRTCM converts a real RTCMv3 DiffCorrIn block and
// checks the tag, message ID, framed length, and cross-block base ID.
func TestCorReportDiffCorrInRTCM(t *testing.T) {
	m := findBlock(t, "rtk.jsonl", "DiffCorrIn").Params.(*sbfbin.DiffCorrIn)
	if m.Mode != sbfbin.DiffCorrModeRTCMv3 {
		t.Fatalf("expected RTCMv3 DiffCorrIn, got mode %d", m.Mode)
	}
	msg := corReportDiffCorrIn(m, opt.Make(uint16(33)), true)
	if msg == nil {
		t.Fatal("corReportDiffCorrIn returned nil")
	}
	if msg.Source != gpsprot.CorReportSourceReceiver {
		t.Errorf("Source = %v, want receiver", msg.Source)
	}
	if msg.Tag != rtcm.Tag {
		t.Errorf("Tag = %q, want %q", msg.Tag, rtcm.Tag)
	}
	if msg.MsgID == "" {
		t.Error("MsgID empty")
	}
	if !msg.ChecksumOK.IsSet() || !msg.ChecksumOK.Get() {
		t.Error("ChecksumOK should be true")
	}
	if !msg.NBytes.IsSet() || msg.NBytes.Get() < 6 {
		t.Errorf("NBytes = %v, want a framed length", msg.NBytes)
	}
	if msg.NativeMsg == nil {
		t.Error("NativeMsg not parsed")
	}
	if got := msg.RTCMRefBaseID.Get(); got != 33 {
		t.Errorf("RTCMRefBaseID = %d, want 33", got)
	}
}

// TestCorReportDiffCorrInRTCMPadding checks that SBF padding is excluded from
// the packet passed to the native RTCM parser.
func TestCorReportDiffCorrInRTCMPadding(t *testing.T) {
	payload := []byte{0, 0x10, 0xAA}
	pkt, err := rtcmbin.PackMsg(payload)
	if err != nil {
		t.Fatal(err)
	}
	m := &sbfbin.DiffCorrIn{Correction: append(pkt, 0, 0, 0)}
	m.Mode = sbfbin.DiffCorrModeRTCMv3
	msg := corReportDiffCorrIn(m, opt.Val[uint16]{}, false)
	if msg == nil {
		t.Fatal("corReportDiffCorrIn returned nil")
	}
	native, ok := msg.NativeMsg.(*rtcmbin.UnknownMsg)
	if !ok {
		t.Fatalf("NativeMsg type = %T, want *rtcmbin.UnknownMsg", msg.NativeMsg)
	}
	if native.Payload != string(payload) {
		t.Errorf("NativeMsg payload = %x, want %x", native.Payload, payload)
	}
	if !msg.NBytes.IsSet() || msg.NBytes.Get() != len(pkt) {
		t.Errorf("NBytes = %v, want %d", msg.NBytes, len(pkt))
	}
}

// TestCorReportDiffCorrInInvalid returns nil for malformed embedded frames.
func TestCorReportDiffCorrInInvalid(t *testing.T) {
	tests := []struct {
		name string
		mode sbfbin.DiffCorrMode
		corr []byte
	}{
		{"RTCM", sbfbin.DiffCorrModeRTCMv3, []byte{rtcmbin.PreambleByte, 4, 0, 0, 0, 0}},
		{"SPARTN", sbfbin.DiffCorrModeSPARTN, []byte{spartnbin.Preamble}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := &sbfbin.DiffCorrIn{Correction: test.corr}
			m.Mode = test.mode
			if corReportDiffCorrIn(m, opt.Val[uint16]{}, false) != nil {
				t.Error("malformed embedded frame should map to nil")
			}
		})
	}
}

// TestCorReportDiffCorrInUnmapped returns nil for correction modes with no
// gpsprot.Tag (RTCMv2/CMR/RTCMV).
func TestCorReportDiffCorrInUnmapped(t *testing.T) {
	var m sbfbin.DiffCorrIn
	m.Mode = sbfbin.DiffCorrModeRTCMv2
	if corReportDiffCorrIn(&m, opt.Val[uint16]{}, false) != nil {
		t.Error("RTCMv2 should map to nil (no gpsprot.Tag)")
	}
	m.Mode = sbfbin.DiffCorrModeCMR
	if corReportDiffCorrIn(&m, opt.Val[uint16]{}, false) != nil {
		t.Error("CMR should map to nil")
	}
	p, c := newTestProcessor()
	pkt, err := sbfbin.Serialize(&sbfbin.Block{Params: &m})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ProcessPacket(string(pkt), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if c.natives != 1 || len(c.cors) != 0 {
		t.Errorf("NativeMsg count = %d, CorReport count = %d, want 1, 0", c.natives, len(c.cors))
	}
}

// TestCorReportDiffCorrInSPARTN checks that the inner frame length excludes
// SBF padding and a cached RTCM base ID is not attached.
func TestCorReportDiffCorrInSPARTN(t *testing.T) {
	pkt, err := spartnbin.Pack(&spartnbin.Frame{
		FrameStart:    spartnbin.FrameStart{Type: 1, CRCType: 2},
		FrameMetadata: spartnbin.FrameMetadata{Subtype: 3},
		Payload:       []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	var m sbfbin.DiffCorrIn
	m.Mode = sbfbin.DiffCorrModeSPARTN
	m.Correction = append(pkt, 0, 0, 0)
	msg := corReportDiffCorrIn(&m, opt.Make(uint16(33)), true)
	if msg == nil {
		t.Fatal("corReportDiffCorrIn returned nil")
	}
	if !msg.NBytes.IsSet() || msg.NBytes.Get() != len(pkt) {
		t.Errorf("NBytes = %v, want %d", msg.NBytes, len(pkt))
	}
	if msg.MsgID != "1.3" {
		t.Errorf("MsgID = %q, want %q", msg.MsgID, "1.3")
	}
	if msg.NativeMsg == nil {
		t.Error("NativeMsg not parsed")
	}
	if msg.RTCMRefBaseID.IsSet() {
		t.Errorf("RTCMRefBaseID = %d, want unset", msg.RTCMRefBaseID.Get())
	}
}
