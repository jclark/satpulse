package as

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestNMEAOut(t *testing.T) {
	tests := []struct {
		name   string
		flags  gpsprot.NMEAMsgFlags
		expect map[asbin.MsgID]uint8
	}{
		{
			// without NMEAMsgOther the request is complete: the
			// out-of-vocabulary sentences (GST, GRS, DTM, JAM) are turned
			// off along with the unnamed vocabulary ones
			name:  "rmc_gga",
			flags: gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgGGA,
			expect: map[asbin.MsgID]uint8{
				asbin.NmeaGsvID: 0, asbin.NmeaRmcID: 1, asbin.NmeaGgaID: 1,
				asbin.NmeaGsaID: 0, asbin.NmeaZdaID: 0, asbin.NmeaVtgID: 0,
				asbin.NmeaGllID: 0,
				asbin.NmeaGstID: 0, asbin.NmeaGrsID: 0, asbin.NmeaDtmID: 0,
				asbin.NmeaJamID: 0,
			},
		},
		{
			name:  "none",
			flags: gpsprot.NMEAMsgNone,
			expect: map[asbin.MsgID]uint8{
				asbin.NmeaGsvID: 0, asbin.NmeaRmcID: 0, asbin.NmeaGgaID: 0,
				asbin.NmeaGsaID: 0, asbin.NmeaZdaID: 0, asbin.NmeaVtgID: 0,
				asbin.NmeaGllID: 0,
				asbin.NmeaGstID: 0, asbin.NmeaGrsID: 0, asbin.NmeaDtmID: 0,
				asbin.NmeaJamID: 0,
			},
		},
		{
			// with NMEAMsgOther the named types are still controlled
			// exactly, but the out-of-vocabulary sentences are left alone
			name:  "rmc_other",
			flags: gpsprot.NMEAMsgRMC | gpsprot.NMEAMsgOther,
			expect: map[asbin.MsgID]uint8{
				asbin.NmeaGsvID: 0, asbin.NmeaRmcID: 1, asbin.NmeaGgaID: 0,
				asbin.NmeaGsaID: 0, asbin.NmeaZdaID: 0, asbin.NmeaVtgID: 0,
				asbin.NmeaGllID: 0,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.NMEAMsg.Set(tc.flags)
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

func TestPVTOut(t *testing.T) {
	tests := []struct {
		name   string
		flags  gpsprot.PVTMsgFlags
		expect map[asbin.MsgID]uint8
	}{
		{
			name:  "pos_time_incremental",
			flags: gpsprot.PVTMsgPos | gpsprot.PVTMsgTime,
			expect: map[asbin.MsgID]uint8{
				asbin.NavPosLlhID: 1, asbin.NavTimeUtcID: 1,
			},
		},
		{
			name:  "pos_vel_ecef_off",
			flags: gpsprot.PVTMsgPos | gpsprot.PVTMsgVel | gpsprot.PVTMsgECEF | gpsprot.PVTMsgOff,
			expect: map[asbin.MsgID]uint8{
				asbin.NavPosEcefID: 1, asbin.NavVelEcefID: 1,
				asbin.NavPosLlhID: 0, asbin.NavVelNedID: 0,
				asbin.NavTimeUtcID: 0, asbin.NavTimeID: 0,
				asbin.NavDopID: 0, asbin.NavAutoID: 0, asbin.NavSvinID: 0,
			},
		},
		{
			name:  "tai_time",
			flags: gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI,
			expect: map[asbin.MsgID]uint8{
				asbin.NavTimeID: 1,
			},
		},
		{
			name:  "leap_and_qual",
			flags: gpsprot.PVTMsgLeapSecond | gpsprot.PVTMsgQuality,
			expect: map[asbin.MsgID]uint8{
				asbin.NavTimeID: 1, asbin.NavDopID: 1, asbin.NavAutoID: 1,
			},
		},
		{
			// tp and epoch have no carrier: nothing is enabled and the
			// absence in the output is the statement.
			name:   "tp_epoch_absent",
			flags:  gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgEpoch,
			expect: nil,
		},
		{
			// the PTP timing preset: the after option makes the GNSS
			// time message stand in for the absent pulse-time message
			name:  "ptp_preset",
			flags: gpsprot.PVTMsgTimingPTP,
			expect: map[asbin.MsgID]uint8{
				asbin.NavTimeID: 1, asbin.NavDopID: 1, asbin.NavAutoID: 1,
				asbin.NavSvinID: 1,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.PVTMsg.Set(tc.flags)
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
	tests := []struct {
		name   string
		flags  gpsprot.SatsMsgFlags
		expect map[asbin.MsgID]uint8
	}{
		{
			name:   "sat",
			flags:  gpsprot.SatsMsgSat,
			expect: map[asbin.MsgID]uint8{asbin.NavSvInfoID: 1},
		},
		{
			// per-signal information has no carrier; a signal-only
			// request is complete, so satellite info is turned off
			name:   "sig_only",
			flags:  gpsprot.SatsMsgSignal,
			expect: map[asbin.MsgID]uint8{asbin.NavSvInfoID: 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
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

func TestPVTOutMissingMessage(t *testing.T) {
	// A firmware lacking a PVT carrier NAKs its enable (the TAU951M
	// NAKs NAV-SVSTATE targets, for example); the information shows as
	// absence, never as an error.
	rcvr := &testReceiver{
		monVer:     tau1201Ver(),
		nakTargets: map[asbin.MsgID]bool{asbin.NavSvinID: true},
	}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgSurvey | gpsprot.PVTMsgTime)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0: a missing carrier is absence, not an error", errCount)
	}
	if rcvr.rates[asbin.NavTimeUtcID] != 1 {
		t.Errorf("TIMEUTC rate = %d, want 1", rcvr.rates[asbin.NavTimeUtcID])
	}
}

func TestRTCMOut(t *testing.T) {
	tests := []struct {
		name  string
		flags gpsprot.RTCMMsgFlags
		other bool // expect eph and proprietary targets turned off
	}{
		{name: "msm4_arp", flags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP, other: true},
		{name: "msm4_arp_other", flags: gpsprot.RTCMMsgMSM4 | gpsprot.RTCMMsgARP | gpsprot.RTCMMsgOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.RTCMMsg.Set(tc.flags)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			expect := map[asbin.MsgID]uint8{asbin.RtcmArpID: 1}
			for _, mid := range rtcmMSM4IDs {
				expect[mid] = 1
			}
			for _, mid := range rtcmMSM7IDs {
				expect[mid] = 0
			}
			if tc.other {
				for _, mid := range rtcmEphIDs {
					expect[mid] = 0
				}
				for _, mid := range rtcmPropIDs {
					expect[mid] = 0
				}
			}
			if !reflect.DeepEqual(rcvr.rates, expect) {
				t.Errorf("rates\ngot  %v\nwant %v", rcvr.rates, expect)
			}
		})
	}
}

func TestRTCMOutAbsent(t *testing.T) {
	// The TAU1201 NAKs every 0xF8 target: an RTCM request achieves
	// nothing and reports no error - absence, not failure.
	nakAll := make(map[asbin.MsgID]bool)
	for _, mid := range rtcmMSM4IDs {
		nakAll[mid] = true
	}
	for _, mid := range rtcmMSM7IDs {
		nakAll[mid] = true
	}
	for _, mid := range rtcmEphIDs {
		nakAll[mid] = true
	}
	for _, mid := range rtcmPropIDs {
		nakAll[mid] = true
	}
	nakAll[asbin.RtcmArpID] = true
	rcvr := &testReceiver{monVer: tau1201Ver(), nakTargets: nakAll}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.RTCMMsg.Set(gpsprot.RTCMMsgAuto)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0: no RTCM output shows as absence", errCount)
	}
	if len(rcvr.rates) != 0 {
		t.Errorf("rates = %v, want none applied", rcvr.rates)
	}
}

func TestRawOut(t *testing.T) {
	tests := []struct {
		name      string
		flags     gpsprot.RawMsgFlags
		nakRawOff bool
		expect    uint8
	}{
		{name: "obs", flags: gpsprot.RawMsgObs, expect: 1},
		{name: "nav", flags: gpsprot.RawMsgNavData, expect: 1},
		{name: "off", flags: gpsprot.RawMsgNone, expect: 0},
		{
			// TAU951M quirk: the disable is NAKed yet applied; the NAK
			// must not surface as an error
			name: "off_nak_quirk", flags: gpsprot.RawMsgNone,
			nakRawOff: true, expect: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver(), rawOn: 1, nakRawOff: tc.nakRawOff}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Opts.RawMsg.Set(tc.flags)
			_, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if rcvr.rawOn != tc.expect {
				t.Errorf("rawOn = %d, want %d", rcvr.rawOn, tc.expect)
			}
		})
	}
}

func TestNMEARefused(t *testing.T) {
	// A refused NMEA rate is a genuine failure: it must surface as an
	// error while the remaining requests still complete.
	rcvr := &testReceiver{
		monVer:     tau1201Ver(),
		nakTargets: map[asbin.MsgID]bool{asbin.NmeaGsvID: true},
	}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgRMC)
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", errCount)
	}
	if rcvr.rates[asbin.NmeaRmcID] != 1 {
		t.Errorf("RMC rate = %d, want 1: later requests must proceed past a refusal", rcvr.rates[asbin.NmeaRmcID])
	}
}
