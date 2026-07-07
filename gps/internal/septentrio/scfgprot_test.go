package septentrio

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// grcStateLine is the verbatim ReceiverCapabilities state line captured from
// a mosaic-G5 (v1.1.0).
const grcStateLine = "ReceiverCapabilities, Main, GPSL1CA+GPSL2PY+GPSL2C+GPSL5+GPSL1C+GLOL1CA+GLOL2P+GLOL2CA+GLOL3+GALE1BC+GALE6BC+GALE5a+GALE5b+GEOL1+GEOL5+BDSB1I+BDSB2I+BDSB3I+BDSB1C+BDSB2a+BDSB2b+QZSL1CA+QZSL2C+QZSL5+QZSL6+QZSL1C+QZSL1S+QZSL1CB+NAVICL5, DSK1+COM1+COM2+USB1+USB2, SBAS+DGNSSRover+RTKRover+RTKBase+RTCMv3x+xPPSOutput+TimedEvent+InternalLogging+APME+RAIM+MovingBase+MeasAv+IM+DSK1+GalOSNMA+UnrestrictedMotion+PPPGalileoHAS-SIS, 50, 50"

// g5SignalSet is the device-independent signal set for the G5's 29 supported
// signals: all 27 signals of the coarse table (GLOL2P and QZSL1CB have no
// gpsprot analogue).
var g5SignalSet = gpsprot.SignalSetOf(
	gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2P, gpsprot.SigGPSL2C, gpsprot.SigGPSL5,
	gpsprot.SigGLOL1, gpsprot.SigGLOL2, gpsprot.SigGLOL3,
	gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b, gpsprot.SigGALE6,
	gpsprot.SigBDSB1I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2I, gpsprot.SigBDSB2b, gpsprot.SigBDSB2a, gpsprot.SigBDSB3I,
	gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL1C, gpsprot.SigQZSSL1S, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5, gpsprot.SigQZSSL6,
	gpsprot.SigNAVICL5,
	gpsprot.SigSBASL1CA, gpsprot.SigSBASL5,
)

func TestParseCaps(t *testing.T) {
	tests := []struct {
		name         string
		states       []string
		expectPorts  []string
		expectCaps   []string
		expectNoCaps []string
		expectSigSet gpsprot.SignalSet
		expectErr    bool
	}{
		{
			name:         "mosaic-G5 verbatim",
			states:       []string{grcStateLine},
			expectPorts:  []string{"DSK1", "COM1", "COM2", "USB1", "USB2"},
			expectCaps:   []string{"GalOSNMA", "PPPGalileoHAS-SIS", "xPPSOutput", "RTKRover"},
			expectNoCaps: []string{"PPPBeiDouB2b", ""},
			expectSigSet: g5SignalSet,
		},
		{
			name:      "no capabilities state line",
			states:    []string{"SomethingElse, Main, x"},
			expectErr: true,
		},
		{
			name:      "empty reply",
			states:    nil,
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCaps(&Reply{Kind: ReplyAck, Echo: grcCmd, States: tc.states, Prompt: "USB1"})
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got.ports, tc.expectPorts) {
				t.Errorf("ports: got %v want %v", got.ports, tc.expectPorts)
			}
			for _, name := range tc.expectCaps {
				if !got.caps[name] {
					t.Errorf("capability %q missing", name)
				}
			}
			for _, name := range tc.expectNoCaps {
				if got.caps[name] {
					t.Errorf("capability %q unexpectedly present", name)
				}
			}
			if got.sigSet != tc.expectSigSet {
				t.Errorf("sigSet: got %v want %v", got.sigSet, tc.expectSigSet)
			}
		})
	}
}

func TestProbe(t *testing.T) {
	cp := NewConfigProtocol()
	packets, delay := cp.ProbePackets()
	if delay != probePacketDelay {
		t.Errorf("ProbePackets delay: got %v want %v", delay, probePacketDelay)
	}
	if len(packets) != 2 {
		t.Fatalf("ProbePackets len: got %d want 2", len(packets))
	}
	if got, want := string(packets[0]), "SSSSSSSSSS\r\n"; got != want {
		t.Errorf("ProbePackets[0]: got %q want %q", got, want)
	}
	if got, want := string(packets[1]), "getReceiverCapabilities\r\n"; got != want {
		t.Errorf("ProbePackets[1]: got %q want %q", got, want)
	}
	if cp.ProbeOK() {
		t.Fatalf("ProbeOK true before any reply")
	}
	// Unrelated replies and non-reply messages must not satisfy the probe.
	if err := cp.NativeMsg(TagReply, "ack", &Reply{Kind: ReplyAck, Echo: "getPPSParameters", States: []string{"PPSParameters, sec1"}, Prompt: "USB1"}, time.Time{}); err != nil {
		t.Fatalf("NativeMsg: %v", err)
	}
	if err := cp.NativeMsg(Tag, "MeasEpoch", "not a reply", time.Time{}); err != nil {
		t.Fatalf("NativeMsg: %v", err)
	}
	if cp.ProbeOK() {
		t.Fatalf("ProbeOK true after unrelated messages")
	}
	reply := &Reply{Kind: ReplyAck, Echo: grcCmd, States: []string{grcStateLine}, Prompt: "USB1"}
	if err := cp.NativeMsg(TagReply, "ack", reply, time.Time{}); err != nil {
		t.Fatalf("NativeMsg: %v", err)
	}
	if !cp.ProbeOK() {
		t.Fatalf("ProbeOK false after grc reply")
	}
	if cp.caps.sigSet != g5SignalSet {
		t.Errorf("sigSet: got %v want %v", cp.caps.sigSet, g5SignalSet)
	}
	if got, want := cp.caps.sigSet.GNSSSet(), gpsprot.AllGNSSSet; got != want {
		t.Errorf("GNSSSet: got %v want %v", got, want)
	}
}
