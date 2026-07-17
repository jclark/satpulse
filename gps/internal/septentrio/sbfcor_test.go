package septentrio

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/lib/opt"
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
// SBF padding.
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
	msg := corReportDiffCorrIn(&m, opt.Val[uint16]{}, false)
	if msg == nil {
		t.Fatal("corReportDiffCorrIn returned nil")
	}
	if !msg.NBytes.IsSet() || msg.NBytes.Get() != len(pkt) {
		t.Errorf("NBytes = %v, want %d", msg.NBytes, len(pkt))
	}
}
