package rnxunc

import (
	"math"
	"testing"

	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

type testSink struct {
	obs []rinex.SignalObservation
}

func (s *testSink) Metadata(rinex.Metadata) error {
	return nil
}

func (s *testSink) Observation(obs rinex.SignalObservation) error {
	s.obs = append(s.obs, obs)
	return nil
}

func (s *testSink) Flush() error {
	return nil
}

func TestConvertObsVMObservation(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	ok, err := c.ConvertObsVM(hdr(1, 1000), obsvm(uncmsg.ObsVMObs{
		PRN:        7,
		PSR:        23956830.530,
		ADR:        -125893980.172,
		Dopp:       -12.5,
		CN0:        4534,
		LockTime:   100,
		ChTrStatus: tracking(uncmsg.SysGPS, uncmsg.FreqGPSL1CA, false, true, true),
	}))
	if err != nil {
		t.Fatalf("ConvertObsVM: %v", err)
	}
	if !ok {
		t.Fatalf("ConvertObsVM ok = false, want true")
	}
	if len(s.obs) != 1 {
		t.Fatalf("observation count = %d, want 1", len(s.obs))
	}
	obs := s.obs[0]
	if obs.T != rinex.TimeFromGPSWeekMillis(1, 1000) || obs.Sat != "G07" || obs.Sig != "1C" {
		t.Fatalf("observation ID = (%v, %q, %q), want (%v, G07, 1C)", obs.T, obs.Sat, obs.Sig, rinex.TimeFromGPSWeekMillis(1, 1000))
	}
	if !obs.PR.IsSet() || obs.PR.Get() != 23956830.530 {
		t.Errorf("PR = %v, want 23956830.530", obs.PR)
	}
	if !obs.CP.IsSet() || obs.CP.Get() != 125893980.172 {
		t.Errorf("CP = %v, want 125893980.172", obs.CP)
	}
	if !obs.Do.IsSet() || obs.Do.Get() != -12.5 {
		t.Errorf("Do = %v, want -12.5", obs.Do)
	}
	if !obs.CN0.IsSet() || obs.CN0.Get() != 45.34 {
		t.Errorf("CN0 = %v, want 45.34", obs.CN0)
	}
	if obs.Arc != 0 || obs.HC || obs.BT {
		t.Errorf("Arc/HC/BT = %v/%v/%v, want 0/false/false", obs.Arc, obs.HC, obs.BT)
	}
}

func TestConvertObsVMGLONASSFrequency(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	_, err := c.ConvertObsVM(hdr(1, 1000), obsvm(
		uncmsg.ObsVMObs{
			SystemFreq: 0,
			PRN:        38,
			PSR:        1,
			ADR:        -2,
			LockTime:   100,
			ChTrStatus: tracking(uncmsg.SysGLO, uncmsg.FreqGLOL1CA, false, true, true),
		},
		uncmsg.ObsVMObs{
			SystemFreq: 12,
			PRN:        39,
			PSR:        3,
			ADR:        -4,
			LockTime:   100,
			ChTrStatus: tracking(uncmsg.SysGLO, uncmsg.FreqGLOL2CA, false, true, true),
		},
	))
	if err != nil {
		t.Fatalf("ConvertObsVM: %v", err)
	}
	if len(s.obs) != 2 {
		t.Fatalf("observation count = %d, want 2", len(s.obs))
	}
	if s.obs[0].Sat != "R01" || !s.obs[0].Frq.IsSet() || s.obs[0].Frq.Get() != -7 {
		t.Fatalf("observation = %#v, want R01 with GLONASS frequency -7", s.obs[0])
	}
	if s.obs[1].Sat != "R02" || !s.obs[1].Frq.IsSet() || s.obs[1].Frq.Get() != 5 {
		t.Fatalf("observation = %#v, want R02 with GLONASS frequency 5", s.obs[1])
	}
}

func TestConvertObsVMQZSSL2CM(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	_, err := c.ConvertObsVM(hdr(1, 1000), obsvm(uncmsg.ObsVMObs{
		PRN:        193,
		PSR:        23000000,
		ADR:        -120000000,
		LockTime:   100,
		ChTrStatus: tracking(uncmsg.SysQZSS, uncmsg.FreqQZSSL2CM, true, true, true),
	}))
	if err != nil {
		t.Fatalf("ConvertObsVM: %v", err)
	}
	if len(s.obs) != 1 {
		t.Fatalf("observation count = %d, want 1", len(s.obs))
	}
	if s.obs[0].Sat != "J01" || s.obs[0].Sig != "2S" {
		t.Fatalf("observation = (%q, %q), want (J01, 2S)", s.obs[0].Sat, s.obs[0].Sig)
	}
}

func TestConvertObsVMSkipsUnmappedAndEmpty(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	ok, err := c.ConvertObsVM(hdr(1, 1000), obsvm(
		uncmsg.ObsVMObs{
			PRN:        7,
			PSR:        1,
			ADR:        -2,
			Dopp:       3,
			CN0:        4000,
			ChTrStatus: tracking(uncmsg.SysQZSS, uncmsg.FreqQZSSL2CM, false, true, true),
		},
		uncmsg.ObsVMObs{
			PRN:        7,
			Dopp:       float32(math.NaN()),
			ChTrStatus: tracking(uncmsg.SysGPS, uncmsg.FreqGPSL1CA, false, false, false),
		},
	))
	if err != nil {
		t.Fatalf("ConvertObsVM: %v", err)
	}
	if ok {
		t.Fatalf("ConvertObsVM ok = true, want false")
	}
	if len(s.obs) != 0 {
		t.Fatalf("observation count = %d, want 0", len(s.obs))
	}
}

func TestConvertObsVMCarriesLostLockUntilPhase(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	if _, err := c.ConvertObsVM(hdr(1, 1000), obsvm(uncmsg.ObsVMObs{
		PRN:        7,
		PSR:        1,
		ADR:        -2,
		LockTime:   100,
		ChTrStatus: tracking(uncmsg.SysGPS, uncmsg.FreqGPSL1CA, false, true, true),
	})); err != nil {
		t.Fatalf("ConvertObsVM first: %v", err)
	}
	if _, err := c.ConvertObsVM(hdr(1, 2000), obsvm(uncmsg.ObsVMObs{
		PRN:        7,
		PSR:        3,
		ADR:        -4,
		LockTime:   0,
		ChTrStatus: tracking(uncmsg.SysGPS, uncmsg.FreqGPSL1CA, false, true, false),
	})); err != nil {
		t.Fatalf("ConvertObsVM second: %v", err)
	}
	if _, err := c.ConvertObsVM(hdr(1, 3000), obsvm(uncmsg.ObsVMObs{
		PRN:        7,
		PSR:        5,
		ADR:        -6,
		LockTime:   1,
		ChTrStatus: tracking(uncmsg.SysGPS, uncmsg.FreqGPSL1CA, false, true, true),
	})); err != nil {
		t.Fatalf("ConvertObsVM third: %v", err)
	}
	if len(s.obs) != 3 {
		t.Fatalf("observation count = %d, want 3", len(s.obs))
	}
	if s.obs[0].Arc != 0 || s.obs[0].HC || s.obs[0].BT {
		t.Fatalf("first Arc/HC/BT = %v/%v/%v, want 0/false/false", s.obs[0].Arc, s.obs[0].HC, s.obs[0].BT)
	}
	if s.obs[1].CP.IsSet() || s.obs[1].Arc != 0 || s.obs[1].HC || s.obs[1].BT {
		t.Fatalf("second observation = %#v, want no phase and no arc", s.obs[1])
	}
	if !s.obs[2].CP.IsSet() || s.obs[2].Arc != 1 || s.obs[2].HC || s.obs[2].BT {
		t.Fatalf("third observation = %#v, want carried lost lock", s.obs[2])
	}
}

func hdr(week uint16, ms uint32) *uncmsg.MsgHdr {
	return &uncmsg.MsgHdr{TimingHdr: uncmsg.TimingHdr{Week: week, MillisecondsOfWeek: ms}}
}

func obsvm(obs ...uncmsg.ObsVMObs) *uncmsg.ObsVM {
	return &uncmsg.ObsVM{
		ObsVMFixed: uncmsg.ObsVMFixed{ObsNumber: uint32(len(obs))},
		Obs:        obs,
	}
}

func tracking(sys uncmsg.SysID, sig uncmsg.FreqID, l2c, pr, cp bool) uncmsg.TrackingStatus {
	s := uint32(sys)<<16 | uint32(sig)<<21
	if cp {
		s |= 1 << 10
	}
	if pr {
		s |= 1 << 12
	}
	if l2c {
		s |= 1 << 26
	}
	return uncmsg.TrackingStatus(s)
}
