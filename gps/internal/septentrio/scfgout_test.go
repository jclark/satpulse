package septentrio

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// asFoundStream is the shipped daemon block list found on the test unit.
var asFoundStream = []string{"MeasEpoch", "PVTGeodetic", "EndOfPVT", "xPPSOffset", "ChannelStatus"}

func TestSBFOutputList(t *testing.T) {
	tests := []struct {
		name    string
		current []string
		opts    func(*gpsprot.ConfigOptions)
		expect  []string
	}{
		{
			name:    "pvt without Off only adds",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgLeapSecond
			},
			expect: []string{"BDSUtc", "ChannelStatus", "EndOfPVT", "GALUtc", "GPSUtc", "MeasEpoch", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "pvt with Off replaces the pvt class only",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgEpoch | gpsprot.PVTMsgOff
			},
			expect: []string{"ChannelStatus", "EndOfPVT", "MeasEpoch", "xPPSOffset"},
		},
		{
			name:    "sats off drops MeasEpoch: an explicit disable wins",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.SatsMsg.Set(gpsprot.SatsMsgNone)
			},
			expect: []string{"EndOfPVT", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "raw off with sats signal keeps MeasEpoch for sats",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.RawMsg.Set(gpsprot.RawMsgNone)
				o.SatsMsg.Set(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)
			},
			expect: []string{"ChannelStatus", "EndOfPVT", "MeasEpoch", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "raw off with sats sat only drops MeasEpoch",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.RawMsg.Set(gpsprot.RawMsgNone)
				o.SatsMsg.Set(gpsprot.SatsMsgSat)
			},
			expect: []string{"ChannelStatus", "EndOfPVT", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "unclassified user block is preserved",
			current: []string{"AttEuler", "PVTGeodetic"},
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgEpoch | gpsprot.PVTMsgOff
			},
			expect: []string{"AttEuler", "EndOfPVT"},
		},
		{
			name:    "survey and ecef pick PVTCartesian",
			current: nil,
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgPos | gpsprot.PVTMsgSurvey | gpsprot.PVTMsgECEF | gpsprot.PVTMsgOff
			},
			expect: []string{"PVTCartesian"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Configurator{target: &gpsprot.ConfigTarget{}}
			if tc.current != nil {
				c.np.sbfStream = &streamState{cd: "USB1", messages: slices.Clone(tc.current), interval: "sec1"}
			}
			tc.opts(&c.target.Opts)
			if got := c.sbfOutputList(); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
}

// TestConfigureOutputs drives the output options end to end: the owned streams
// are queried, rewritten when concrete messages are selected, and the output
// protocol mask is enabled separately.
func TestConfigureOutputs(t *testing.T) {
	sbfCmd := "setSBFOutput, Stream1, USB1, ChannelStatus+EndOfPVT+MeasEpoch+PVTGeodetic+xPPSOffset, sec1"
	nmeaCmd := "setNMEAOutput, Stream1, USB1, RMC+GGA, sec1"
	nmeaProtoCmd := "setDataInOut, USB1, , +NMEA"
	replies := map[string]string{
		"getSBFOutput, Stream1":  "SBFOutput, Stream1, USB1, MeasEpoch+PVTGeodetic+EndOfPVT+xPPSOffset+ChannelStatus, sec1",
		"getNMEAOutput, Stream1": "NMEAOutput, Stream1, USB1, none, sec1",
		sbfCmd:                   "SBFOutput, Stream1, USB1, MeasEpoch+PVTGeodetic+EndOfPVT+xPPSOffset+ChannelStatus, sec1",
		nmeaCmd:                  "NMEAOutput, Stream1, USB1, RMC+GGA, sec1",
		nmeaProtoCmd:             "DataInOut, USB1, auto, SBF+NMEA, (on)",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg = gpsprot.PVTMsgPos | gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgEpoch
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)
	target.Opts.NMEAMsg = opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
	cfg, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, cmd := range []string{"getSBFOutput, Stream1", "getNMEAOutput, Stream1", sbfCmd, nmeaCmd, nmeaProtoCmd} {
		if !slices.Contains(sent, cmd) {
			t.Errorf("command %q not sent; sent: %v", cmd, sent)
		}
	}
	if got := cfg.np.sbfStream; got == nil || !slices.Contains(got.messages, "ChannelStatus") {
		t.Errorf("acked SBF stream state not recorded: %+v", got)
	}
}

func TestConfigureNMEAOutProtocolMask(t *testing.T) {
	t.Run("none disables protocol only", func(t *testing.T) {
		cmd := "setDataInOut, USB1, , -NMEA"
		replies := map[string]string{
			"getNMEAOutput, Stream1": "NMEAOutput, Stream1, USB1, RMC, sec1",
			cmd:                      "DataInOut, USB1, auto, SBF, (on)",
		}
		target := &gpsprot.ConfigTarget{}
		target.Opts.NMEAMsg = opt.Make(gpsprot.NMEAMsgNone)
		_, sent, errs := runConfig(t, target, replies)
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if !slices.Contains(sent, cmd) {
			t.Errorf("NMEA protocol disable not sent; sent: %v", sent)
		}
		for _, cmd := range sent {
			if strings.HasPrefix(cmd, "setNMEAOutput") {
				t.Errorf("NMEA message list changed for none: %q", cmd)
			}
		}
	})
	t.Run("other enables protocol only", func(t *testing.T) {
		cmd := "setDataInOut, USB1, , +NMEA"
		replies := map[string]string{
			"getNMEAOutput, Stream1": "NMEAOutput, Stream1, USB1, GGAaux1, sec1",
			cmd:                      "DataInOut, USB1, auto, SBF+NMEA, (on)",
		}
		target := &gpsprot.ConfigTarget{}
		target.Opts.NMEAMsg = opt.Make(gpsprot.NMEAMsgOther)
		_, sent, errs := runConfig(t, target, replies)
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if !slices.Contains(sent, cmd) {
			t.Errorf("NMEA protocol enable not sent; sent: %v", sent)
		}
		for _, cmd := range sent {
			if strings.HasPrefix(cmd, "setNMEAOutput") {
				t.Errorf("NMEA message list changed for Other-only request: %q", cmd)
			}
		}
	})
	t.Run("known plus other preserves unknown sentences", func(t *testing.T) {
		nmeaCmd := "setNMEAOutput, Stream1, USB1, GGAaux1+RMC, sec1"
		protoCmd := "setDataInOut, USB1, , +NMEA"
		replies := map[string]string{
			"getNMEAOutput, Stream1": "NMEAOutput, Stream1, USB1, GGAaux1+GGA, sec1",
			nmeaCmd:                  "NMEAOutput, Stream1, USB1, GGAaux1+RMC, sec1",
			protoCmd:                 "DataInOut, USB1, auto, SBF+NMEA, (on)",
		}
		target := &gpsprot.ConfigTarget{}
		target.Opts.NMEAMsg = opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgOther)
		_, sent, errs := runConfig(t, target, replies)
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		for _, cmd := range []string{nmeaCmd, protoCmd} {
			if !slices.Contains(sent, cmd) {
				t.Errorf("command %q not sent; sent: %v", cmd, sent)
			}
		}
	})
}

func TestRTCMMessages(t *testing.T) {
	tests := []struct {
		name   string
		flags  gpsprot.RTCMMsgFlags
		expect []string
	}{
		{name: "auto", flags: gpsprot.RTCMMsgAuto, expect: []string{"MSM4", "RTCM1005"}},
		{name: "msm7 with arp", flags: gpsprot.RTCMMsgAutoMSM7, expect: []string{"MSM7", "RTCM1005"}},
		{name: "none", flags: gpsprot.RTCMMsgNone, expect: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rtcmMessages(tc.flags); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got %v want %v", got, tc.expect)
			}
		})
	}
}

// TestConfigureRTCMOut checks the RTCM message-list command and the separate
// RTCMv3 output protocol mask on this connection.
func TestConfigureRTCMOut(t *testing.T) {
	rtcmCmd := "setRTCMv3Output, USB1, MSM4+RTCM1005"
	protoOn := "setDataInOut, USB1, , +RTCMv3"
	replies := map[string]string{
		rtcmCmd: "RTCMv3Output, USB1, RTCM1005+RTCM1074+RTCM1084+RTCM1094+RTCM1104+RTCM1114+RTCM1124+RTCM1134",
		protoOn: "DataInOut, USB1, auto, RTCMv3+SBF+NMEA, (on)",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg = opt.Make(gpsprot.RTCMMsgAuto)
	_, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, cmd := range []string{rtcmCmd, protoOn} {
		if !slices.Contains(sent, cmd) {
			t.Errorf("command %q not sent; sent: %v", cmd, sent)
		}
	}

	protoOff := "setDataInOut, USB1, , -RTCMv3"
	replies = map[string]string{
		protoOff: "DataInOut, USB1, auto, SBF+NMEA, (on)",
	}
	target = &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg = opt.Make(gpsprot.RTCMMsgNone)
	_, sent, errs = runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, protoOff) {
		t.Errorf("RTCM protocol disable not sent; sent: %v", sent)
	}
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "setRTCMv3Output") {
			t.Errorf("RTCM message list changed for none: %q", cmd)
		}
	}

	replies = map[string]string{
		protoOn: "DataInOut, USB1, auto, RTCMv3+SBF+NMEA, (on)",
	}
	target = &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg = opt.Make(gpsprot.RTCMMsgOther)
	_, sent, errs = runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, protoOn) {
		t.Errorf("RTCM protocol enable not sent; sent: %v", sent)
	}
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "setRTCMv3Output") {
			t.Errorf("RTCM message list changed for Other-only request: %q", cmd)
		}
	}
}

// TestConfigureRTCMOutUnsupported checks the capability gate on RTCM output:
// without the correction-generation capabilities an enable request fails
// before the wire, and an empty selection still disables the port protocol.
func TestConfigureRTCMOutUnsupported(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg = opt.Make(gpsprot.RTCMMsgAuto)
	_, sent, errs := runConfigCaps(t, target, nil, trimmedCapsLine)
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "setRTCMv3Output") {
			t.Errorf("RTCM command sent without the capability: %q", cmd)
		}
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "RTCM message output not supported") {
		t.Errorf("errors: got %v; want one unsupported-output error", errs)
	}

	target = &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg = opt.Make(gpsprot.RTCMMsgOther)
	_, sent, errs = runConfigCaps(t, target, nil, trimmedCapsLine)
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "setRTCMv3Output") || strings.HasPrefix(cmd, "setDataInOut") {
			t.Errorf("RTCM command sent without the capability: %q", cmd)
		}
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "RTCM message output not supported") {
		t.Errorf("errors: got %v; want one unsupported-output error", errs)
	}

	protoOff := "setDataInOut, USB1, , -RTCMv3"
	target = &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg = opt.Make(gpsprot.RTCMMsgNone)
	_, sent, errs = runConfigCaps(t, target, map[string]string{
		protoOff: "DataInOut, USB1, auto, SBF+NMEA, (on)",
	}, trimmedCapsLine)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, protoOff) {
		t.Errorf("RTCM protocol disable not sent; sent: %v", sent)
	}
	for _, cmd := range sent {
		if strings.HasPrefix(cmd, "setRTCMv3Output") {
			t.Errorf("RTCM command sent for an empty selection without the capability: %q", cmd)
		}
	}
}

// TestConfigureOutputsEmptyList pins the hardware finding that an empty
// Messages argument KEEPS the current list (the omitted-argument rule): an
// empty selection must be spelled none.
func TestConfigureOutputsEmptyList(t *testing.T) {
	cmd := "setSBFOutput, Stream1, USB1, none, sec1"
	replies := map[string]string{
		"getSBFOutput, Stream1": "SBFOutput, Stream1, USB1, ChannelStatus+MeasEpoch, sec1",
		cmd:                     "SBFOutput, Stream1, USB1, none, sec1",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgNone)
	target.Opts.RawMsg.Set(gpsprot.RawMsgNone)
	_, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !slices.Contains(sent, cmd) {
		t.Errorf("empty selection not spelled none; sent: %v", sent)
	}
}
