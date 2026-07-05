package septentrio

import (
	"reflect"
	"slices"
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
			expect: []string{"ReceiverSetup", "BDSUtc", "ChannelStatus", "EndOfPVT", "GALUtc", "GPSUtc", "MeasEpoch", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "pvt with Off replaces the pvt class only",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgEpoch | gpsprot.PVTMsgOff
			},
			expect: []string{"ReceiverSetup", "ChannelStatus", "EndOfPVT", "MeasEpoch", "xPPSOffset"},
		},
		{
			name:    "sats off keeps MeasEpoch for the untouched raw class",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.SatsMsg.Set(gpsprot.SatsMsgNone)
			},
			expect: []string{"ReceiverSetup", "EndOfPVT", "MeasEpoch", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "raw off with sats signal keeps MeasEpoch for sats",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.RawMsg.Set(gpsprot.RawMsgNone)
				o.SatsMsg.Set(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)
			},
			expect: []string{"ReceiverSetup", "ChannelStatus", "EndOfPVT", "MeasEpoch", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "raw off with sats sat only drops MeasEpoch",
			current: asFoundStream,
			opts: func(o *gpsprot.ConfigOptions) {
				o.RawMsg.Set(gpsprot.RawMsgNone)
				o.SatsMsg.Set(gpsprot.SatsMsgSat)
			},
			expect: []string{"ReceiverSetup", "ChannelStatus", "EndOfPVT", "PVTGeodetic", "xPPSOffset"},
		},
		{
			name:    "unclassified user block is preserved",
			current: []string{"AttEuler", "PVTGeodetic"},
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgEpoch | gpsprot.PVTMsgOff
			},
			expect: []string{"ReceiverSetup", "AttEuler", "EndOfPVT"},
		},
		{
			name:    "survey and ecef pick PVTCartesian",
			current: nil,
			opts: func(o *gpsprot.ConfigOptions) {
				o.PVTMsg = gpsprot.PVTMsgPos | gpsprot.PVTMsgSurvey | gpsprot.PVTMsgECEF | gpsprot.PVTMsgOff
			},
			expect: []string{"ReceiverSetup", "PVTCartesian"},
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

// TestConfigureOutputs drives the output options end to end: the owned
// streams are queried, rewritten as one self-contained command each, and the
// acked state lines are the readback.
func TestConfigureOutputs(t *testing.T) {
	sbfCmd := "setSBFOutput, Stream1, USB1, ReceiverSetup+ChannelStatus+EndOfPVT+MeasEpoch+PVTGeodetic+xPPSOffset, sec1"
	nmeaCmd := "setNMEAOutput, Stream1, USB1, RMC+GGA, sec1"
	replies := map[string]string{
		"getSBFOutput, Stream1":  "SBFOutput, Stream1, USB1, MeasEpoch+PVTGeodetic+EndOfPVT+xPPSOffset+ChannelStatus, sec1",
		"getNMEAOutput, Stream1": "NMEAOutput, Stream1, USB1, none, sec1",
		sbfCmd:                   "SBFOutput, Stream1, USB1, MeasEpoch+PVTGeodetic+EndOfPVT+xPPSOffset+ChannelStatus+ReceiverSetup, sec1",
		nmeaCmd:                  "NMEAOutput, Stream1, USB1, RMC+GGA, sec1",
	}
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg = gpsprot.PVTMsgPos | gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgEpoch
	target.Opts.SatsMsg.Set(gpsprot.SatsMsgSat | gpsprot.SatsMsgSignal)
	target.Opts.NMEAMsg = opt.Make(gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA)
	cfg, sent, errs := runConfig(t, target, replies)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, cmd := range []string{"getSBFOutput, Stream1", "getNMEAOutput, Stream1", sbfCmd, nmeaCmd} {
		if !slices.Contains(sent, cmd) {
			t.Errorf("command %q not sent; sent: %v", cmd, sent)
		}
	}
	if got := cfg.np.sbfStream; got == nil || !slices.Contains(got.messages, "ReceiverSetup") {
		t.Errorf("acked SBF stream state not recorded: %+v", got)
	}
}
