package septentrio

import (
	"encoding/hex"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// grcReplyPacket is a framed grc reply as captured from the mosaic-G5.
const grcReplyPacket = "$R: getReceiverCapabilities\r\n  " + grcStateLine + "\r\nUSB1>"

// receiverSetupPacket is a verbatim ReceiverSetup SBF packet from the
// committed mosaic-G5 corpus (gps/testdata/packets/septentrio/mosaic-G5).
const receiverSetupPacket = "244020a30e97a80140a9be1b79090000534550540000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000556e6b6e6f776e00000000000000000000000000556e6b6e6f776e00000000000000000000000000556e6b6e6f776e00000000000000000000000000000000000000000000000000000000000000000031303030313935373700000000000000000000004752423030363000000000000000000000000000312e312e30000000000000000000000000000000556e6b6e6f776e00000000000000000000000000556e6b6e6f776e00000000000000000000000000000000000000000000000000556e6b6e6f776e00000000000000000000000000323032362e30312e322d6738336431636136373730000000000000000000000000000000000000006d6f736169632d4735205033000000000000000000000000000000000000000000000000000000001874ef155eadce3f541c41aff51afc3faaabc2c1000000000000000000000000000000000000000000000000000000000000000000000000"

// asFoundReplies answers each query with the state line verified on the
// as-found mosaic-G5 (the SignalTracking list includes GALE5, which has no
// gpsprot analogue and must not appear in the reported SignalSet).
var asFoundReplies = map[string]string{
	"getSignalTracking":               "SignalTracking, GPSL1CA+GPSL2PY+GPSL2C+GPSL5+GLOL1CA+GLOL2CA+GALE1BC+GALE6BC+GALE5a+GALE5b+GALE5+GEOL1+BDSB1I+BDSB2I+BDSB3I+BDSB1C+BDSB2a+QZSL1CA+QZSL2C+QZSL5+NAVICL5",
	"getSignalUsage":                  "SignalUsage, GPSL1CA+GPSL2PY+GLOL1CA, GPSL1CA+GALE6BC",
	"getPPSParameters":                "PPSParameters, sec1, Low2High, 0.00, GPS, 1, 100.000000",
	"getPVTMode":                      "PVTMode, Rover, StandAlone+DGNSS+RTKFixed, auto",
	"getElevationMask, PVT":           "ElevationMask, PVT, 5",
	"getGalOSNMAUsage":                `GalOSNMAUsage, loose, "", off`,
	"getRTCMv3Formatting":             `RTCMv3Formatting, 0, GPSL1CA+GPSL2PY+GLOL1CA, L2CA, "default", 128`,
	"getCalibCommonDelay":             "CalibCommonDelay, 0.000",
	"getStaticPosGeodetic, Geodetic1": "StaticPosGeodetic, Geodetic1, 13.732000000, 100.567000000, 12.3400, WGS84",
}

// getAll requests every property with a Septentrio carrier.
const getAll = gpsprot.PropIDSignalsEnabled | gpsprot.PropIDTimeGNSS |
	gpsprot.PropIDTimePulse | gpsprot.PropIDMode | gpsprot.PropIDAntennaCableDelay |
	gpsprot.PropIDNavMsgAuth | gpsprot.PropIDRTCMBaseID | gpsprot.PropIDMinElevation |
	gpsprot.PropIDBaudRate | gpsprot.PropIDPort

// runConfig probes and configures through the real ReplyProcessor / SBF
// PacketProcessor -> ConfigProtocol -> ConfigDirector path against a fake
// receiver: the probe reply carries the capability line and our prompt, the
// escape elicits no framed reply, queries are answered from replies, and
// ReceiverSetup arrives as a normal SBF block on the block's own schedule.
func runConfig(t *testing.T, target *gpsprot.ConfigTarget, replies map[string]string) (*Configurator, []string) {
	t.Helper()
	cp := NewConfigProtocol()
	rp := NewReplyProcessor()
	rp.SetNativeMsgHandler(cp)
	sp := NewPacketProcessor(nil)
	sp.SetNativeMsgHandler(cp)
	t0 := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)

	if _, err := rp.ProcessPacket(grcReplyPacket, t0); err != nil {
		t.Fatalf("probe reply: %v", err)
	}
	if !cp.ProbeOK() {
		t.Fatalf("ProbeOK false after grc reply")
	}
	cfgIntf, err := cp.Configure(target)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg := cfgIntf.(*Configurator)
	director := gpsprot.NewConfigDirector(cfg, 2)
	var sent []string
	rxSetupFed := false
	for action := range director.Actions() {
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			t0 = t0.Add(time.Millisecond)
			cmd := strings.TrimSuffix(string(action.Packet), "\r\n")
			sent = append(sent, cmd)
			cfg.Request(action.Index).SetSentTime(t0)
			if state, ok := replies[cmd]; ok {
				t0 = t0.Add(time.Millisecond)
				pkt := "$R: " + cmd + "\r\n  " + state + "\r\nUSB1>"
				if _, err := rp.ProcessPacket(pkt, t0); err != nil {
					t.Fatalf("reply to %q: %v", cmd, err)
				}
			}
		case gpsprot.ConfigActionWaitUntil:
			t0 = action.Deadline
			if !rxSetupFed {
				// The periodic ReceiverSetup block arrives mid-run.
				rxSetupFed = true
				pkt, err := hex.DecodeString(receiverSetupPacket)
				if err != nil {
					t.Fatalf("decoding ReceiverSetup hex: %v", err)
				}
				if _, err := sp.ProcessPacket(string(pkt), t0); err != nil {
					t.Fatalf("ReceiverSetup packet: %v", err)
				}
			}
			director.AdvanceTimeTo(t0)
		case gpsprot.ConfigActionError:
			t.Fatalf("unexpected error action: %v", action.Error)
		}
	}
	return cfg, sent
}

func TestConfigureGetAll(t *testing.T) {
	cfg, sent := runConfig(t, &gpsprot.ConfigTarget{Get: getAll}, asFoundReplies)
	if sent[0] != escapeCmd {
		t.Errorf("first packet: got %q want the escape", sent[0])
	}

	expect := &gpsprot.ConfigProps{}
	expect.SetSignalsEnabled(gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2P, gpsprot.SigGPSL2C, gpsprot.SigGPSL5,
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		gpsprot.SigGALE1, gpsprot.SigGALE6, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		gpsprot.SigSBASL1CA,
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a,
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
		gpsprot.SigNAVICL5,
	))
	expect.SetTimePulse(gpsprot.TimePulse{
		Width:          100 * time.Millisecond,
		Period:         time.Second,
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
		PolarityRising: true,
	})
	expect.SetTimeGNSS(gpsprot.GPS)
	expect.SetMode(gpsprot.Mode{})
	expect.SetMinElevation(5 * gpsprot.Degrees)
	expect.SetNavMsgAuth(gpsprot.NavMsgAuthOSNMA)
	expect.SetRTCMBaseID(0)
	expect.SetAntennaCableDelay(0)
	expect.SetPort("USB1")
	expect.SetBaudRate(0)
	if got := cfg.ConfigProps(); !reflect.DeepEqual(got, expect) {
		t.Errorf("ConfigProps:\ngot  %+v\nwant %+v", got, expect)
	}

	info := cfg.ReceiverInfo()
	if info.Vendor != Vendor || info.Hardware != "mosaic-G5 P3" || info.Firmware != "1.1.0" {
		t.Errorf("ReceiverInfo: got %q %q %q", info.Vendor, info.Hardware, info.Firmware)
	}
	if info.SupportedGNSS != gpsprot.AllGNSSSet {
		t.Errorf("SupportedGNSS: got %v want %v", info.SupportedGNSS, gpsprot.AllGNSSSet)
	}
}

// TestConfigureGetStaticMode checks the lazy follow-up query: the PVTMode
// reply names Geodetic1, so the configurator must fetch that slot before it
// can report the mode.
func TestConfigureGetStaticMode(t *testing.T) {
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["getPVTMode"] = "PVTMode, Static, , Geodetic1"
	cfg, sent := runConfig(t, &gpsprot.ConfigTarget{Get: gpsprot.PropIDMode}, replies)
	if !slices.Contains(sent, "getStaticPosGeodetic, Geodetic1") {
		t.Fatalf("no static-position follow-up query in %v", sent)
	}
	mode, ok := cfg.ConfigProps().GetMode()
	if !ok {
		t.Fatalf("Mode not reported")
	}
	want := gpsprot.Mode{
		Static:      true,
		PosType:     gpsprot.PosTypeLLH,
		FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.732), gpsprot.DegreesFromFloat(100.567)},
		Height:      gpsprot.Meters(12.34),
	}
	if !reflect.DeepEqual(mode, want) {
		t.Errorf("Mode:\ngot  %+v\nwant %+v", mode, want)
	}
}

// TestConfiguratorReply exercises the reply correlation rules directly: acks
// match the in-flight request by exact echo, refusals attribute to it by the
// single-flight discipline.
func TestConfiguratorReply(t *testing.T) {
	tests := []struct {
		name        string
		req         *sReq
		reply       *Reply
		expectState sReqState
		expectErr   string // substring of GetError, "" for none
	}{
		{
			name:        "ack with matching echo succeeds",
			req:         &sReq{state: sStateAwaiting, cmd: "setElevationMask, PVT, 10"},
			reply:       &Reply{Kind: ReplyAck, Echo: "setElevationMask, PVT, 10", States: []string{"ElevationMask, PVT, 10"}, Prompt: "USB1"},
			expectState: sStateSucceeded,
		},
		{
			name:        "ack with mismatched echo is ignored",
			req:         &sReq{state: sStateAwaiting, cmd: "setElevationMask, PVT, 10"},
			reply:       &Reply{Kind: ReplyAck, Echo: "getReceiverCapabilities", States: []string{grcStateLine}, Prompt: "USB1"},
			expectState: sStateAwaiting,
		},
		{
			name:        "nak fails the in-flight request with the receiver's text",
			req:         &sReq{state: sStateAwaiting, cmd: "setSignalTracking, +FOO"},
			reply:       &Reply{Kind: ReplyNak, Error: "setSignalTracking: Argument 'Signal' is invalid!", Prompt: "USB1"},
			expectState: sStateFailed,
			expectErr:   "Argument 'Signal' is invalid!",
		},
		{
			name:        "nak succeeds a nakOK request",
			req:         &sReq{state: sStateAwaiting, cmd: "setSignalTracking, +FOO", nakOK: true},
			reply:       &Reply{Kind: ReplyNak, Error: "setSignalTracking: Argument 'Signal' is invalid!", Prompt: "USB1"},
			expectState: sStateSucceeded,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Configurator{reqs: []*sReq{tc.req}}
			var got *Reply
			tc.req.onReply = func(r *Reply) { got = r }
			if err := c.reply(tc.reply, time.Time{}); err != nil {
				t.Fatalf("reply: %v", err)
			}
			if tc.req.state != tc.expectState {
				t.Errorf("state: got %v want %v", tc.req.state, tc.expectState)
			}
			if c.port != tc.reply.Prompt {
				t.Errorf("port: got %q want %q", c.port, tc.reply.Prompt)
			}
			if tc.expectErr != "" {
				if err := tc.req.GetError(); err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Errorf("GetError: got %v want substring %q", err, tc.expectErr)
				}
			}
			if tc.expectState == sStateSucceeded && got != tc.reply {
				t.Errorf("onReply not called with the matching reply")
			}
		})
	}
}

// TestPromote checks the single-flight rule: only the first non-final
// request is ever promoted to ReadyToSend.
func TestPromote(t *testing.T) {
	c := &Configurator{reqs: []*sReq{
		{state: sStateSucceeded, cmd: "a"},
		{state: sStateNotReady, cmd: "b"},
		{state: sStateNotReady, cmd: "c"},
	}}
	c.promote()
	if got := c.reqs[1].state; got != sStateReady {
		t.Errorf("reqs[1]: got %v want %v", got, sStateReady)
	}
	if got := c.reqs[2].state; got != sStateNotReady {
		t.Errorf("reqs[2]: got %v want %v", got, sStateNotReady)
	}
}
