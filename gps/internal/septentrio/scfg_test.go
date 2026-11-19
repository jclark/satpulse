package septentrio

import (
	"encoding/xml"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// grcReply frames a grc reply carrying the given capability state line.
func grcReply(capsLine string) string {
	return "$R: getReceiverCapabilities\r\n  " + capsLine + "\r\nUSB1>"
}

// trimmedCapsLine models a reduced receiver: no GPSL5, GALE6BC, or NAVIC
// signals, and no correction-generation capabilities (RTKBase, DGNSSBase,
// RTCMv3x all absent).
const trimmedCapsLine = "ReceiverCapabilities, Main, GPSL1CA+GPSL2C+GLOL1CA+GALE1BC+GALE5a+GEOL1+BDSB1I+QZSL1CA, COM1+COM2+USB1+USB2, SBAS+DGNSSRover+RTKRover+xPPSOutput+APME+RAIM+GalOSNMA, 50, 50"

// allSatsTracking is the as-found mosaic-G5's SatelliteTracking state line:
// every satellite allowed, read back per satellite.
const allSatsTracking = "SatelliteTracking, G01+G02+G03+G04+G05+G06+G07+G08+G09+G10+G11+G12+G13+G14+G15+G16+G17+G18+G19+G20+G21+G22+G23+G24+G25+G26+G27+G28+G29+G30+G31+G32+G33+G34+G35+G36+G38+G39+R01+R02+R03+R04+R05+R06+R07+R08+R09+R10+R11+R12+R13+R14+R15+R16+R17+R18+R19+R20+R21+R22+R23+R24+R25+R26+R27+R28+R29+R30+E01+E02+E03+E04+E05+E06+E07+E08+E09+E10+E11+E12+E13+E14+E15+E16+E17+E18+E19+E20+E21+E22+E23+E24+E25+E26+E27+E28+E29+E30+E31+E32+E33+E34+E35+E36+S120+S121+S122+S123+S124+S125+S126+S127+S128+S129+S130+S131+S132+S133+S134+S135+S136+S137+S138+S139+S140+S141+S142+S143+S144+S145+S146+S147+S148+S149+S150+S151+S152+S153+S154+S155+S156+S157+S158+C01+C02+C03+C04+C05+C06+C07+C08+C09+C10+C11+C12+C13+C14+C15+C16+C17+C18+C19+C20+C21+C22+C23+C24+C25+C26+C27+C28+C29+C30+C31+C32+C33+C34+C35+C36+C37+C38+C39+C40+C41+C42+C43+C44+C45+C46+C47+C48+C49+C50+C51+C52+C53+C54+C55+C56+C57+C58+C59+C60+C61+C62+C63+J01+J02+J03+J04+J05+J06+J07+J08+J09+J10+I01+I02+I03+I04+I05+I06+I07+I08+I09+I10+I11+I12+I13+I14"

// asFoundReplies answers each query with the state line verified on the
// as-found mosaic-G5 (the SignalTracking list includes GALE5, which has no
// gpsprot analogue and must not appear in the reported SignalSet).
var asFoundReplies = map[string]string{
	"getSatelliteTracking":            allSatsTracking,
	"getSignalTracking":               "SignalTracking, GPSL1CA+GPSL2PY+GPSL2C+GPSL5+GLOL1CA+GLOL2CA+GALE1BC+GALE6BC+GALE5a+GALE5b+GALE5+GEOL1+BDSB1I+BDSB2I+BDSB3I+BDSB1C+BDSB2a+QZSL1CA+QZSL2C+QZSL5+NAVICL5",
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

// runConfig probes and configures through the real ReplyProcessor ->
// ConfigProtocol -> ConfigDirector path against a fake receiver behaving as
// the mosaic-G5 (its verbatim capability line).
func runConfig(t *testing.T, target *gpsprot.ConfigTarget, replies map[string]string) (*Configurator, []string, []error) {
	t.Helper()
	return runConfigCaps(t, target, replies, grcStateLine)
}

// runConfigCaps is runConfig with the fake receiver advertising the given
// capability state line: the probe reply carries it and our prompt, the
// escape elicits no framed reply, queries are answered from replies, and the
// identity fetch is answered with the captured Identification file.
func runConfigCaps(t *testing.T, target *gpsprot.ConfigTarget, replies map[string]string, capsLine string) (*Configurator, []string, []error) {
	t.Helper()
	cp := NewConfigProtocol()
	rp := NewReplyProcessor()
	rp.SetNativeMsgHandler(cp)
	t0 := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)

	if _, err := rp.ProcessPacket(grcReply(capsLine), t0); err != nil {
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
	var errs []error
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
				if nak, found := strings.CutPrefix(state, "!"); found {
					pkt = "$R? " + nak + "\r\nUSB1>"
				}
				if stop, found := strings.CutPrefix(state, "@STOP "); found {
					pkt = "$R: " + cmd + "\r\n  " + stop + "\r\nSTOP>"
				}
				if _, err := rp.ProcessPacket(pkt, t0); err != nil {
					t.Fatalf("reply to %q: %v", cmd, err)
				}
			} else if cmd == "lstInternalFile, Identification" {
				for _, pkt := range lifIdentPackets {
					t0 = t0.Add(time.Millisecond)
					if _, err := rp.ProcessPacket(pkt, t0); err != nil {
						t.Fatalf("lst packet: %v", err)
					}
				}
			}
		case gpsprot.ConfigActionWaitUntil:
			t0 = action.Deadline
			director.AdvanceTimeTo(t0)
		case gpsprot.ConfigActionError:
			errs = append(errs, action.Error)
		}
	}
	return cfg, sent, errs
}

// lifIdentPackets are the verbatim framed packets of a "lstInternalFile,
// Identification" reply captured from the mosaic-G5 (log 09).
var lifIdentPackets = []string{
	"$R; lstInternalFile, Identification\r\n---->",
	"$-- BLOCK 1 / 5\r\n<?xml version=\"1.0\" encoding=\"ISO-8859-1\" ?>\r\n<rxproduct xmlns=\"http://septentrio.com/ns/ProductDescription/2.9\">\r\n\r\n---->",
	"$-- BLOCK 2 / 5\r\n    <hwplatform product=\"mosaic-G5 P3\"\r\n                name=\"GRB0060\" serialnr=\"0100019577\"\r\n                rxfullid=\"SN25510100019577\">\r\n        <mainboard type=\"GRB00601000AA0401\"/>\r\n    </hwplatform>\r\n\r\n---->",
	"$-- BLOCK 3 / 5\r\n    <firmware version=\"1.1.0\" date=\"260507\" rev=\"ge4d576\">\r\n        <bootloader version=\"2025.04.1-g2b4d387\" date=\"250730\"/>\r\n        <gnssfw version=\"2026.01.2-g83d1ca6770\" date=\"260507\" type=\"std\">\r\n        </gnssfw>\r\n    </firmware>\r\n\r\n---->",
	"$-- BLOCK 4 / 5\r\n    <files>\r\n        <permfile permid=\"20251217b-100019577-1\"/>\r\n    </files>\r\n\r\n---->",
	"$-- BLOCK 5 / 5\r\n</rxproduct>\r\n\r\nUSB1>",
}

func TestConfigureGetAll(t *testing.T) {
	cfg, sent, errs := runConfig(t, &gpsprot.ConfigTarget{Get: getAll}, asFoundReplies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if sent[0] != "getSatelliteTracking" {
		t.Errorf("first configuration packet: got %q want getSatelliteTracking", sent[0])
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

// TestConfigSupportRTCMGate checks that the RTCMv3 output features are
// declared and realized only on receivers with the correction-generation
// capabilities: without them the support flags are cleared and the
// RTCMBaseID property shows absence (no query, no set, no error).
func TestConfigSupportRTCMGate(t *testing.T) {
	target := &gpsprot.ConfigTarget{Get: gpsprot.PropIDRTCMBaseID}
	target.Props.SetRTCMBaseID(5)
	cfg, sent, errs := runConfigCaps(t, target, nil, trimmedCapsLine)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, cmd := range sent {
		if strings.Contains(cmd, "RTCMv3Formatting") {
			t.Errorf("RTCM base ID command sent without the capability: %q", cmd)
		}
	}
	if v, ok := cfg.ConfigProps().GetRTCMBaseID(); ok {
		t.Errorf("RTCMBaseID: got %v; want absence", v)
	}
	rtcmFlags := gpsprot.ConfigSupportRTCMMSM4 | gpsprot.ConfigSupportRTCMMSM7 |
		gpsprot.ConfigSupportRTCMBaseID | gpsprot.ConfigSupportRTCMQZSS
	if got := cfg.ConfigSupport(); got&rtcmFlags != 0 {
		t.Errorf("ConfigSupport: got %v; want no RTCM flags", got)
	}

	cfg, _, errs = runConfig(t, &gpsprot.ConfigTarget{}, nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := cfg.ConfigSupport(); got&rtcmFlags != rtcmFlags {
		t.Errorf("ConfigSupport: got %v; want all RTCM flags on the full capability line", got)
	}
}

// TestConfigureGetStaticMode checks the lazy follow-up query: the PVTMode
// reply names Geodetic1, so the configurator must fetch that slot before it
// can report the mode.
func TestConfigureGetStaticMode(t *testing.T) {
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["getPVTMode"] = "PVTMode, Static, , Geodetic1"
	cfg, sent, errs := runConfig(t, &gpsprot.ConfigTarget{Get: gpsprot.PropIDMode}, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
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
		{
			name:        "nak naming the configuration item fails the request",
			req:         &sReq{state: sStateAwaiting, cmd: "setSBFOutput, Stream1, USB1, MeasEpoch, sec1"},
			reply:       &Reply{Kind: ReplyNak, Error: "SBFOutput: Not authorized!", Prompt: "USB1"},
			expectState: sStateFailed,
			expectErr:   "Not authorized!",
		},
		{
			name:        "nak naming a foreign command is ignored",
			req:         &sReq{state: sStateAwaiting, cmd: "getSignalTracking"},
			reply:       &Reply{Kind: ReplyNak, Error: "VERSIONB: Invalid command!", Prompt: "USB1"},
			expectState: sStateAwaiting,
		},
		{
			name:        "nak naming another of our commands is ignored",
			req:         &sReq{state: sStateAwaiting, cmd: "setPPSParameters, sec1"},
			reply:       &Reply{Kind: ReplyNak, Error: "setElevationMask: Argument 'Elevation' is invalid!", Prompt: "USB1"},
			expectState: sStateAwaiting,
		},
		{
			name:        "nak without a recognizable name keeps the single-flight attribution",
			req:         &sReq{state: sStateAwaiting, cmd: "setPPSParameters, sec1"},
			reply:       &Reply{Kind: ReplyNak, Error: "something went wrong", Prompt: "USB1"},
			expectState: sStateFailed,
			expectErr:   "something went wrong",
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

// TestLstBlockContinuity checks that stale tail blocks of an abandoned lst
// attempt cannot complete a retried request with truncated content: block
// numbering must be continuous, and a fresh block 1 restarts collection.
func TestLstBlockContinuity(t *testing.T) {
	var got string
	req := &sReq{state: sStateAwaiting, cmd: "lstInternalFile, Identification",
		onLst: func(s string) { got = s }}
	c := &Configurator{reqs: []*sReq{req}}
	feed := func(num, total int, block, prompt string) {
		if err := c.reply(&Reply{Kind: ReplyBlock, BlockNum: num, BlockTotal: total,
			Block: block, Prompt: prompt}, time.Time{}); err != nil {
			t.Fatalf("reply: %v", err)
		}
	}
	// Stale continuation of the abandoned first attempt: ignored, even the
	// prompt-carrying final block.
	feed(2, 3, "stale-2", "")
	feed(3, 3, "stale-3", "USB1")
	if req.state != sStateAwaiting {
		t.Fatalf("state after stale blocks: got %v want %v", req.state, sStateAwaiting)
	}
	// The retried reply arrives in full and completes the request.
	feed(1, 3, "fresh-1", "")
	feed(2, 3, "fresh-2", "")
	feed(3, 3, "fresh-3", "USB1")
	if req.state != sStateSucceeded {
		t.Fatalf("state: got %v want %v", req.state, sStateSucceeded)
	}
	if want := "fresh-1\nfresh-2\nfresh-3"; got != want {
		t.Errorf("content: got %q want %q", got, want)
	}
}

// TestLstDeadlineExtends checks that a multi-block reply restarts the
// reply window with each accepted unit, so a reply taking longer than
// maxReplyDelay in total (a slow serial link) still completes, while a
// stale block does not extend it.
func TestLstDeadlineExtends(t *testing.T) {
	cmd := "lstInternalFile, Identification"
	req := &sReq{state: sStateReady, cmd: cmd, onLst: func(string) {}}
	c := &Configurator{reqs: []*sReq{req}}
	t0 := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	req.SetSentTime(t0)
	tests := []struct {
		name   string
		reply  *Reply
		at     time.Duration
		expect time.Duration
	}{
		{"opener", &Reply{Kind: ReplyLst, Echo: cmd}, 1 * time.Second, 1*time.Second + maxReplyDelay},
		{"stale block", &Reply{Kind: ReplyBlock, BlockNum: 3, Block: "stale"}, 1200 * time.Millisecond, 1*time.Second + maxReplyDelay},
		{"first block", &Reply{Kind: ReplyBlock, BlockNum: 1, Block: "b1"}, 2 * time.Second, 2*time.Second + maxReplyDelay},
		{"second block", &Reply{Kind: ReplyBlock, BlockNum: 2, Block: "b2"}, 3 * time.Second, 3*time.Second + maxReplyDelay},
	}
	for _, tc := range tests {
		if err := c.reply(tc.reply, t0.Add(tc.at)); err != nil {
			t.Fatalf("%s: reply: %v", tc.name, err)
		}
		if got, want := req.GetDeadline(), t0.Add(tc.expect); !got.Equal(want) {
			t.Errorf("%s: deadline got %v want %v", tc.name, got.Sub(t0), want.Sub(t0))
		}
	}
	if err := c.reply(&Reply{Kind: ReplyBlock, BlockNum: 3, Block: "b3", Prompt: "COM1"}, t0.Add(4*time.Second)); err != nil {
		t.Fatalf("final block: %v", err)
	}
	if req.state != sStateSucceeded {
		t.Errorf("state: got %v want %v", req.state, sStateSucceeded)
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

// TestConfigureSet drives property sets end to end: each set command is
// built from the target, and the ACKED state line (not the request) is what
// ConfigProps reports - including the receiver adjusting a value (the
// elevation mask reply answers 8 for a requested 10).
func TestConfigureSet(t *testing.T) {
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["setElevationMask, PVT, 10"] = "ElevationMask, PVT, 8"
	replies["setPPSParameters, msec100, High2Low, , Galileo, 0, 0.2"] = "PPSParameters, msec100, High2Low, 0.00, Galileo, 0, 0.200000"
	replies["setCalibCommonDelay, 51"] = "CalibCommonDelay, 51.000"
	replies["setRTCMv3Formatting, 4095"] = `RTCMv3Formatting, 4095, GPSL1CA+GPSL2PY+GLOL1CA, L2CA, "default", 128`
	replies["setGalOSNMAUsage, off, , off"] = `GalOSNMAUsage, off, "", off`

	target := &gpsprot.ConfigTarget{}
	target.Props.SetMinElevation(10 * gpsprot.Degrees)
	target.Props.SetTimePulse(gpsprot.TimePulse{
		Width:          200 * time.Microsecond,
		Period:         100 * time.Millisecond,
		AlignToGNSS:    true,
		OnlyWhenLocked: false,
		PolarityRising: false,
	})
	target.Props.SetTimeGNSS(gpsprot.GAL)
	target.Props.SetAntennaCableDelay(51 * time.Nanosecond)
	target.Props.SetRTCMBaseID(9999) // clamped to 4095 before the wire
	target.Props.SetNavMsgAuth(gpsprot.NavMsgAuthNone)

	cfg, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for cmd := range replies {
		if strings.HasPrefix(cmd, "set") && !slices.Contains(sent, cmd) {
			t.Errorf("set command %q not sent; sent: %v", cmd, sent)
		}
	}

	props := cfg.ConfigProps()
	expect := &gpsprot.ConfigProps{}
	expect.SetMinElevation(8 * gpsprot.Degrees) // the acked value, not the requested 10
	expect.SetTimePulse(gpsprot.TimePulse{
		Width:          200 * time.Microsecond,
		Period:         100 * time.Millisecond,
		AlignToGNSS:    true,
		OnlyWhenLocked: false,
		PolarityRising: false,
	})
	expect.SetTimeGNSS(gpsprot.GAL)
	expect.SetAntennaCableDelay(51 * time.Nanosecond)
	expect.SetRTCMBaseID(4095)
	expect.SetNavMsgAuth(gpsprot.NavMsgAuthNone)
	expect.SetPort("USB1")
	expect.SetBaudRate(0)
	if !reflect.DeepEqual(props, expect) {
		t.Errorf("ConfigProps:\ngot  %+v\nwant %+v", props, expect)
	}
}

// TestConfigureGetSatelliteTracking checks that the enabled signals leave
// out the signals of constellations with no satellite allowed to be tracked:
// SignalTracking alone allows Galileo and the others, but SatelliteTracking
// allows only GPS satellites.
func TestConfigureGetSatelliteTracking(t *testing.T) {
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["getSatelliteTracking"] = "SatelliteTracking, G01+G02+G03"
	cfg, _, errs := runConfig(t, &gpsprot.ConfigTarget{Get: gpsprot.PropIDSignalsEnabled}, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	expect := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL2P, gpsprot.SigGPSL2C, gpsprot.SigGPSL5)
	if ss, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || ss != expect {
		t.Errorf("SignalsEnabled: got %v, %v; want %v", ss, ok, expect)
	}
}

// TestConfigureSetRefused checks that a refusal surfaces the receiver's
// error text and leaves the queried value standing.
func TestConfigureSetRefused(t *testing.T) {
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["setCalibCommonDelay, 42"] = "!setCalibCommonDelay: Argument 'CommonDelay' is invalid!"

	target := &gpsprot.ConfigTarget{}
	target.Props.SetAntennaCableDelay(42 * time.Nanosecond)
	cfg, _, errs := runConfig(t, target, replies)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Argument 'CommonDelay' is invalid!") {
		t.Fatalf("errors: got %v want one containing the receiver's text", errs)
	}
	if d, ok := cfg.ConfigProps().GetAntennaCableDelay(); !ok || d != 0 {
		t.Errorf("AntennaCableDelay: got %v, %v; want the queried 0 (refusal leaves config unchanged)", d, ok)
	}
}

// TestConfigureSignalsAndMode drives the SignalsEnabled two-command
// realization (explicit tracking list preserving the receiver-only GALE5,
// then the constellation gate) and a fixed-position static mode.
func TestConfigureSignalsAndMode(t *testing.T) {
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	sntCmd := "setSignalTracking, GALE1BC+GALE6BC+GPSL1CA+GPSL5+GALE5"
	replies["setSatelliteTracking, GPS+GALILEO"] = "SatelliteTracking, G01+G02+E01+E02"
	replies[sntCmd] = "SignalTracking, GPSL1CA+GPSL5+GALE1BC+GALE6BC+GALE5"
	replies["setStaticPosGeodetic, Geodetic1, 13.732000000, 100.567000000, 12.3400"] = "StaticPosGeodetic, Geodetic1, 13.732000000, 100.567000000, 12.3400, WGS84"
	replies["setPVTMode, Static, , Geodetic1"] = "PVTMode, Static, , Geodetic1"

	requested := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL5, gpsprot.SigGALE1, gpsprot.SigGALE6)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetSignalsEnabled(requested)
	target.Props.SetMode(gpsprot.Mode{
		Static:      true,
		PosType:     gpsprot.PosTypeLLH,
		FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.732), gpsprot.DegreesFromFloat(100.567)},
		Height:      gpsprot.Meters(12.34),
	})
	cfg, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantOrder := []string{
		sntCmd,
		"setSatelliteTracking, GPS+GALILEO",
		"setStaticPosGeodetic, Geodetic1, 13.732000000, 100.567000000, 12.3400",
		"setPVTMode, Static, , Geodetic1",
	}
	pos := -1
	for _, cmd := range wantOrder {
		i := slices.Index(sent, cmd)
		if i < 0 {
			t.Fatalf("command %q not sent; sent: %v", cmd, sent)
		}
		if i < pos {
			t.Errorf("command %q sent out of order; sent: %v", cmd, sent)
		}
		pos = i
	}

	props := cfg.ConfigProps()
	if ss, ok := props.GetSignalsEnabled(); !ok || ss != requested {
		t.Errorf("SignalsEnabled: got %v, %v; want %v", ss, ok, requested)
	}
	mode, ok := props.GetMode()
	if !ok || !mode.Static || mode.PosType != gpsprot.PosTypeLLH {
		t.Errorf("Mode: got %+v, %v", mode, ok)
	}
}

// TestConfigureSaveReset checks the NVM operations: save before reset, both
// last, and the reset's STOP>-terminated reply completing the request
// without disturbing the Port property.
func TestConfigureSaveReset(t *testing.T) {
	replies := map[string]string{
		"exeCopyConfigFile, Current, Boot":        "CopyConfigFile, Current, Boot",
		"exeResetReceiver, Hard, PVTData+SatData": "@STOP ResetReceiver, Hard, PVTData+SatData",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.Save = gpsprot.SaveAll
	target.Opts.Reset = gpsprot.ResetCold
	cfg, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	iSave := slices.Index(sent, "exeCopyConfigFile, Current, Boot")
	iReset := slices.Index(sent, "exeResetReceiver, Hard, PVTData+SatData")
	if iSave < 0 || iReset < 0 || iSave > iReset {
		t.Errorf("save/reset order wrong; sent: %v", sent)
	}
	if port, ok := cfg.ConfigProps().GetPort(); !ok || port != "USB1" {
		t.Errorf("Port: got %q, %v; the STOP prompt must not overwrite it", port, ok)
	}
}

// TestConfigureSaveResetGate checks the SEMANTICS.md gate: a failed save
// suppresses the paired reset, so a reset never discards running changes the
// save failed to persist.
func TestConfigureSaveResetGate(t *testing.T) {
	replies := map[string]string{
		"exeCopyConfigFile, Current, Boot": "!CopyConfigFile: Not authorized!",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.Save = gpsprot.SaveAll
	target.Opts.Reset = gpsprot.ResetCold
	_, sent, errs := runConfig(t, target, replies)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Not authorized!") {
		t.Fatalf("errors: got %v; want the save refusal alone", errs)
	}
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "exeResetReceiver") {
			t.Errorf("reset sent after a failed save; sent: %v", sent)
		}
	}
}

// TestConfigureIdentityRefused checks that a refused identity fetch is not a
// failure: identity is best-effort, so the invocation reports no error and a
// requested save still runs.
func TestConfigureIdentityRefused(t *testing.T) {
	replies := map[string]string{
		"lstInternalFile, Identification":  "!InternalFile: Not authorized!",
		"exeCopyConfigFile, Current, Boot": "CopyConfigFile, Current, Boot",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.Save = gpsprot.SaveAll
	cfg, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, "exeCopyConfigFile, Current, Boot") {
		t.Errorf("save not sent after a refused identity fetch; sent: %v", sent)
	}
	if info := cfg.ReceiverInfo(); info.Hardware != "" || info.Firmware != "" {
		t.Errorf("identity reported after a refusal: %+v", info)
	}
}

// TestConfigureFailureGatesSave checks the SEMANTICS.md rule that NVM is
// written only by an invocation in which everything succeeded: after a
// refused set, a requested save is skipped and so is its paired reset, while
// a reset with no save requested still runs.
func TestConfigureFailureGatesSave(t *testing.T) {
	tests := []struct {
		name       string
		save       gpsprot.SaveType
		expectSent []string
	}{
		{"save and reload", gpsprot.SaveAll, nil},
		{"reload alone", gpsprot.SaveNone, []string{"exeCopyConfigFile, Boot, Current"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			replies := make(map[string]string, len(asFoundReplies))
			maps.Copy(replies, asFoundReplies)
			replies["setCalibCommonDelay, 42"] = "!setCalibCommonDelay: Argument 'CommonDelay' is invalid!"
			replies["exeCopyConfigFile, Current, Boot"] = "CopyConfigFile, Current, Boot"
			replies["exeCopyConfigFile, Boot, Current"] = "CopyConfigFile, Boot, Current"
			target := &gpsprot.ConfigTarget{}
			target.Props.SetAntennaCableDelay(42 * time.Nanosecond)
			target.Opts.Save = tc.save
			target.Opts.Reset = gpsprot.ResetReload
			_, sent, errs := runConfig(t, target, replies)
			if len(errs) != 1 {
				t.Fatalf("errors: got %v; want the refusal alone", errs)
			}
			var got []string
			for _, cmd := range sent {
				if strings.HasPrefix(cmd, "exe") {
					got = append(got, cmd)
				}
			}
			if !reflect.DeepEqual(got, tc.expectSent) {
				t.Errorf("NVM commands sent: got %v want %v", got, tc.expectSent)
			}
		})
	}
}

// TestConfigureStaticPosRefused checks that a refused static-position write
// skips the mode switch that would reference its slot: the slot still holds
// its previous position, which the switch would silently adopt.
func TestConfigureStaticPosRefused(t *testing.T) {
	tests := []struct {
		name    string
		mode    gpsprot.Mode
		posCmd  string
		nak     string
		modeCmd string
	}{
		{
			name: "geodetic",
			mode: gpsprot.Mode{
				Static:      true,
				PosType:     gpsprot.PosTypeLLH,
				FixedPosLLH: [2]gpsprot.Angle{gpsprot.DegreesFromFloat(13.732), gpsprot.DegreesFromFloat(100.567)},
				Height:      gpsprot.Meters(12.34),
			},
			posCmd:  "setStaticPosGeodetic, Geodetic1, 13.732000000, 100.567000000, 12.3400",
			nak:     "!StaticPosGeodetic: Argument 'Latitude' is invalid!",
			modeCmd: "setPVTMode, Static, , Geodetic1",
		},
		{
			name: "cartesian",
			mode: gpsprot.Mode{
				Static:       true,
				PosType:      gpsprot.PosTypeECEF,
				FixedPosECEF: gpsprot.Point3D{gpsprot.Meters(-1132914.5), gpsprot.Meters(6092528.25), gpsprot.Meters(1504633.0)},
			},
			posCmd:  "setStaticPosCartesian, Cartesian1, -1132914.5000, 6092528.2500, 1504633.0000",
			nak:     "!StaticPosCartesian: Argument 'X' is invalid!",
			modeCmd: "setPVTMode, Static, , Cartesian1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			replies := make(map[string]string, len(asFoundReplies))
			maps.Copy(replies, asFoundReplies)
			replies[tc.posCmd] = tc.nak
			target := &gpsprot.ConfigTarget{}
			target.Props.SetMode(tc.mode)
			_, sent, errs := runConfig(t, target, replies)
			if len(errs) != 1 {
				t.Fatalf("errors: got %v; want the refusal alone", errs)
			}
			if !slices.Contains(sent, tc.posCmd) {
				t.Fatalf("position command %q not sent; sent: %v", tc.posCmd, sent)
			}
			if slices.Contains(sent, tc.modeCmd) {
				t.Errorf("mode command %q sent after a refused position write", tc.modeCmd)
			}
		})
	}
}

// TestConfigureQueryFailureSkipsSets checks that a failed query skips the
// whole set phase: realizing the target without the current state would be
// guesswork (an add-only PVT request would wipe the stream's other blocks).
// A reply whose state line cannot be read is a failed query too.
func TestConfigureQueryFailureSkipsSets(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		expectErr string
	}{
		{
			name:      "refused",
			state:     "!SBFOutput: Not authorized!",
			expectErr: "Not authorized!",
		},
		{
			name:      "state line of another stream",
			state:     "SBFOutput, Stream2, USB1, PVTGeodetic, sec1",
			expectErr: "no usable state line",
		},
		{
			name:      "truncated state line",
			state:     "SBFOutput, Stream1",
			expectErr: "no usable state line",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := &gpsprot.ConfigTarget{}
			target.Opts.PVTMsg = gpsprot.PVTMsgPos | gpsprot.PVTMsgTime
			_, sent, errs := runConfig(t, target, map[string]string{"getSBFOutput, Stream1": tc.state})
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), tc.expectErr) {
				t.Fatalf("errors: got %v; want one containing %q", errs, tc.expectErr)
			}
			for _, cmd := range sent {
				if strings.HasPrefix(cmd, "set") || strings.HasPrefix(cmd, "exe") {
					t.Errorf("set command sent after a failed query: %q", cmd)
				}
			}
		})
	}
}

// TestConfigureStaticAsIs checks the SEMANTICS.md static-mode rules: SetStatic
// leaves a receiver already static as it is, a static request without a
// position leaves an existing self-determined position untouched, and only an
// explicit fresh survey (SurveyAgain) re-issues the auto determination.
func TestConfigureStaticAsIs(t *testing.T) {
	autoReplies := map[string]string{"getPVTMode": "PVTMode, Static, , auto"}
	modeCmds := func(sent []string) []string {
		var cmds []string
		for _, cmd := range sent {
			if strings.HasPrefix(cmd, "setPVTMode") {
				cmds = append(cmds, cmd)
			}
		}
		return cmds
	}

	// SetStatic on a rover switches to the self-determined static mode.
	replies := make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["setPVTMode, Static, , auto"] = "PVTMode, Static, , auto"
	target := &gpsprot.ConfigTarget{}
	target.Opts.SetStatic = true
	_, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if want := []string{"setPVTMode, Static, , auto"}; !reflect.DeepEqual(modeCmds(sent), want) {
		t.Errorf("SetStatic on rover: got %v want %v", modeCmds(sent), want)
	}

	// SetStatic leaves an existing fixed position untouched.
	replies = make(map[string]string, len(asFoundReplies))
	maps.Copy(replies, asFoundReplies)
	replies["getPVTMode"] = "PVTMode, Static, , Geodetic1"
	target = &gpsprot.ConfigTarget{}
	target.Opts.SetStatic = true
	_, sent, errs = runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cmds := modeCmds(sent); cmds != nil {
		t.Errorf("SetStatic on fixed static: got %v want none", cmds)
	}

	// A static request without a position leaves an existing self-determined
	// position alone unless a fresh survey is requested.
	target = &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: true})
	_, sent, errs = runConfig(t, target, autoReplies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cmds := modeCmds(sent); cmds != nil {
		t.Errorf("static-auto re-request: got %v want none", cmds)
	}

	target = &gpsprot.ConfigTarget{}
	target.Props.SetMode(gpsprot.Mode{Static: true})
	target.Opts.Survey.Flags |= gpsprot.SurveyAgain
	replies = map[string]string{
		"getPVTMode":                 "PVTMode, Static, , auto",
		"setPVTMode, Static, , auto": "PVTMode, Static, , auto",
	}
	_, sent, errs = runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if want := []string{"setPVTMode, Static, , auto"}; !reflect.DeepEqual(modeCmds(sent), want) {
		t.Errorf("SurveyAgain: got %v want %v", modeCmds(sent), want)
	}
}

// TestConfigureReloadAndFactory checks the remaining reset mappings.
func TestConfigureReloadAndFactory(t *testing.T) {
	replies := map[string]string{
		"exeCopyConfigFile, Boot, Current": "CopyConfigFile, Boot, Current",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.Reset = gpsprot.ResetReload
	_, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, "exeCopyConfigFile, Boot, Current") {
		t.Errorf("reload not sent; sent: %v", sent)
	}

	replies = map[string]string{
		"exeResetReceiver, Hard, all": "@STOP ResetReceiver, Hard, all",
	}
	target = &gpsprot.ConfigTarget{}
	target.Opts.Reset = gpsprot.ResetFactory
	_, sent, errs = runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, "exeResetReceiver, Hard, all") {
		t.Errorf("factory reset not sent; sent: %v", sent)
	}
}

// TestConfigureIdentity checks the identity fetch: the configurator requests
// the Identification internal file (no configuration change, best-effort) and
// parses its XML block units.
func TestConfigureIdentity(t *testing.T) {
	cfg, sent, errs := runConfig(t, &gpsprot.ConfigTarget{}, nil)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, "lstInternalFile, Identification") {
		t.Fatalf("identity fetch not sent; sent: %v", sent)
	}
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "set") || strings.HasPrefix(cmd, "exe") {
			t.Errorf("identity fetch must not change configuration; sent %q", cmd)
		}
	}
	info := cfg.ReceiverInfo()
	if info.Hardware != "mosaic-G5 P3" || info.Firmware != "1.1.0" {
		t.Errorf("identity: got %q %q", info.Hardware, info.Firmware)
	}
	// VendorSpecific carries the whole token stream, so fields absent from the
	// typed struct (here the serial number) remain recoverable.
	toks, ok := info.VendorSpecific.([]xml.Token)
	if !ok || len(toks) == 0 {
		t.Fatalf("VendorSpecific: got %T, want non-empty []xml.Token", info.VendorSpecific)
	}
	var serial string
	for _, tok := range toks {
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "hwplatform" {
			serial, _ = attr(se, "", "serialnr")
		}
	}
	if serial != "0100019577" {
		t.Errorf("VendorSpecific serialnr: got %q want %q", serial, "0100019577")
	}
}
