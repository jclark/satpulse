package rnxsbf

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

type testSink struct {
	obs []rinex.SignalObservation
}

func (s *testSink) Metadata(rinex.Metadata) error { return nil }

func (s *testSink) Observation(obs rinex.SignalObservation) error {
	s.obs = append(s.obs, obs)
	return nil
}

func (s *testSink) Flush() error { return nil }

var testTS = sbfbin.TimeStamp{TOW: 300000000, WNc: 2400}
var testT = rinex.TimeFromGPSWeekMillis(2400, 300000000)

func mustFreq(sys string, sig rinex.SignalID, frq int8, useFrq bool) float64 {
	var p *int8
	if useFrq {
		p = &frq
	}
	f, ok := rinex.SignalFrequencyHz(sys, sig, p)
	if !ok {
		panic("unknown frequency in test")
	}
	return f
}

// Master measurement values shared by the test blocks below.
var (
	testPR    = float64(5*4294967296+123456789) * 0.001
	testDo    = float64(-12345678) * 0.0001
	testCPOff = float64(2*65536+1500) * 0.001
)

// masterCh returns a Type1 sub-block with valid measurements for every field.
func masterCh(svid uint8, sig sbfbin.MeasType) sbfbin.MeasEpochChannelType1 {
	return sbfbin.MeasEpochChannelType1{
		RxChannel: 1, Type: sig, SVID: svid, Misc: 5, CodeLSB: 123456789,
		Doppler: -12345678, CarrierMSB: 2, CarrierLSB: 1500, CN0: 120, LockTime: 100,
	}
}

func measEpoch(flags sbfbin.CommonFlags, t1 []sbfbin.MeasEpochChannelType1, t2 [][]sbfbin.MeasEpochChannelType2) *sbfbin.MeasEpoch {
	m := &sbfbin.MeasEpoch{Type1: t1, Type2: t2}
	m.CommonFlags = flags
	if t2 == nil {
		m.Type2 = make([][]sbfbin.MeasEpochChannelType2, len(t1))
	}
	return m
}

func convert(t *testing.T, ts sbfbin.TimeStamp, m *sbfbin.MeasEpoch, extra *sbfbin.MeasExtra) []rinex.SignalObservation {
	t.Helper()
	s := &testSink{}
	if err := New(s).ConvertMeasEpoch(ts, m, extra); err != nil {
		t.Fatalf("ConvertMeasEpoch: %v", err)
	}
	return s.obs
}

func TestConvertMaster(t *testing.T) {
	glo := masterCh(40, 8)
	glo.ObsInfo = 1 << 3 // frequency number -7
	ext := masterCh(181, 31)
	ext.ObsInfo = 0 << 3 // extended signal number 32 (QZSS L1L)
	prDNU := masterCh(7, 0)
	prDNU.Misc = 0
	prDNU.CodeLSB = 0
	allDNU := sbfbin.MeasEpochChannelType1{
		RxChannel: 1, SVID: 7, Doppler: sbfbin.MeasType1DopplerDNU,
		CarrierMSB: -128, CN0: sbfbin.MeasType1CN0DNU, LockTime: sbfbin.MeasType1LockTimeDNU,
	}
	tests := []struct {
		name   string
		ts     sbfbin.TimeStamp
		flags  sbfbin.CommonFlags
		t1     sbfbin.MeasEpochChannelType1
		expect []rinex.SignalObservation
	}{
		{
			name: "GPS L1CA all fields",
			t1:   masterCh(7, 0),
			expect: []rinex.SignalObservation{{T: testT, Sat: "G07", Sig: "1C", SignalValues: rinex.SignalValues{
				PR: opt.Make(testPR), CP: opt.Make(testPR/(299792458.0/mustFreq("G", "1C", 0, false)) + testCPOff),
				Do: opt.Make(testDo), CN0: opt.Make[float32](40),
			}}},
		},
		{
			name: "timestamp DNU",
			ts:   sbfbin.TimeStamp{TOW: sbfbin.TOWDNU, WNc: 2400},
			t1:   masterCh(7, 0),
		},
		{name: "SVID DNU", t1: masterCh(0, 0)},
		{name: "SVID GLONASS unknown slot", t1: masterCh(62, 8)},
		{name: "SVID L-band", t1: masterCh(110, 23)},
		{name: "reserved signal number", t1: masterCh(7, 16)},
		{name: "signal system mismatch", t1: masterCh(7, 8)},
		{
			name: "pseudorange DNU drops carrier phase",
			t1:   prDNU,
			expect: []rinex.SignalObservation{{T: testT, Sat: "G07", Sig: "1C", SignalValues: rinex.SignalValues{
				Do: opt.Make(testDo), CN0: opt.Make[float32](40),
			}}},
		},
		{name: "no values at all", t1: allDNU},
		{
			name:  "Galileo E6C",
			t1:    masterCh(71, 19),
			flags: 0,
			expect: []rinex.SignalObservation{{T: testT, Sat: "E01", Sig: "6C", SignalValues: rinex.SignalValues{
				PR: opt.Make(testPR), CP: opt.Make(testPR/(299792458.0/mustFreq("E", "6C", 0, false)) + testCPOff),
				Do: opt.Make(testDo), CN0: opt.Make[float32](40),
			}}},
		},
		{
			name:  "Galileo E6B",
			t1:    masterCh(71, 19),
			flags: sbfbin.CommonFlagsE6BUsed,
			expect: []rinex.SignalObservation{{T: testT, Sat: "E01", Sig: "6B", SignalValues: rinex.SignalValues{
				PR: opt.Make(testPR), CP: opt.Make(testPR/(299792458.0/mustFreq("E", "6B", 0, false)) + testCPOff),
				Do: opt.Make(testDo), CN0: opt.Make[float32](40),
			}}},
		},
		{
			name: "GLONASS frequency number",
			t1:   glo,
			expect: []rinex.SignalObservation{{T: testT, Sat: "R03", Sig: "1C", SignalValues: rinex.SignalValues{
				Frq: opt.Make[int8](-7),
				PR:  opt.Make(testPR), CP: opt.Make(testPR/(299792458.0/mustFreq("R", "1C", -7, true)) + testCPOff),
				Do: opt.Make(testDo), CN0: opt.Make[float32](40),
			}}},
		},
		{
			name: "extended signal number",
			t1:   ext,
			expect: []rinex.SignalObservation{{T: testT, Sat: "J01", Sig: "1L", SignalValues: rinex.SignalValues{
				PR: opt.Make(testPR), CP: opt.Make(testPR/(299792458.0/mustFreq("J", "1L", 0, false)) + testCPOff),
				Do: opt.Make(testDo), CN0: opt.Make[float32](40),
			}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := tc.ts
			if ts == (sbfbin.TimeStamp{}) {
				ts = testTS
			}
			got := convert(t, ts, measEpoch(tc.flags, []sbfbin.MeasEpochChannelType1{tc.t1}, nil), nil)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestConvertSlave(t *testing.T) {
	slave := sbfbin.MeasEpochChannelType2{
		Type: 2, OffsetsMSB: 7 | 30<<3, CodeOffsetLSB: 500, DopplerOffsetLSB: 400,
		CarrierMSB: -1, CarrierLSB: 200, CN0: 100, LockTime: 50,
	}
	slaveDNU := sbfbin.MeasEpochChannelType2{
		Type: 2, OffsetsMSB: 4 | 16<<3, CarrierMSB: -128, CN0: 100, LockTime: 50,
	}
	gloSlave := sbfbin.MeasEpochChannelType2{
		Type: 11, OffsetsMSB: 3, CodeOffsetLSB: 500, DopplerOffsetLSB: 400,
		CarrierMSB: 1, CarrierLSB: 200, CN0: 100, LockTime: 50,
	}
	f1 := mustFreq("G", "1C", 0, false)
	f2 := mustFreq("G", "2W", 0, false)
	prOff := float64(-1*65536+500) * 0.001
	doOff := float64(-2*65536+400) * 0.0001
	cpOff := float64(-1*65536+200) * 0.001
	gloMaster := masterCh(40, 8)
	gloMaster.ObsInfo = 10 << 3 // frequency number 2
	rf1 := mustFreq("R", "1C", 2, true)
	rf2 := mustFreq("R", "2C", 2, true)
	gloPR := testPR + float64(3*65536+500)*0.001
	tests := []struct {
		name   string
		master sbfbin.MeasEpochChannelType1
		slave  sbfbin.MeasEpochChannelType2
		expect rinex.SignalObservation
	}{
		{
			name:   "GPS L2W reconstruction",
			master: masterCh(7, 0),
			slave:  slave,
			expect: rinex.SignalObservation{T: testT, Sat: "G07", Sig: "2W", SignalValues: rinex.SignalValues{
				PR: opt.Make(testPR + prOff),
				CP: opt.Make((testPR+prOff)/(299792458.0/f2) + cpOff),
				Do: opt.Make(testDo*(f2/f1) + doOff), CN0: opt.Make[float32](25),
			}},
		},
		{
			name:   "slave offsets DNU",
			master: masterCh(7, 0),
			slave:  slaveDNU,
			expect: rinex.SignalObservation{T: testT, Sat: "G07", Sig: "2W", SignalValues: rinex.SignalValues{
				CN0: opt.Make[float32](25),
			}},
		},
		{
			name: "master pseudorange DNU disables slave PR and CP",
			master: func() sbfbin.MeasEpochChannelType1 {
				m := masterCh(7, 0)
				m.Misc = 0
				m.CodeLSB = 0
				return m
			}(),
			slave: slave,
			expect: rinex.SignalObservation{T: testT, Sat: "G07", Sig: "2W", SignalValues: rinex.SignalValues{
				Do: opt.Make(testDo*(f2/f1) + doOff), CN0: opt.Make[float32](25),
			}},
		},
		{
			name:   "GLONASS slave inherits master frequency number",
			master: gloMaster,
			slave:  gloSlave,
			expect: rinex.SignalObservation{T: testT, Sat: "R03", Sig: "2C", SignalValues: rinex.SignalValues{
				Frq: opt.Make[int8](2),
				PR:  opt.Make(gloPR),
				CP:  opt.Make(gloPR/(299792458.0/rf2) + float64(1*65536+200)*0.001),
				Do:  opt.Make(testDo*(rf2/rf1) + float64(0*65536+400)*0.0001),
				CN0: opt.Make[float32](35),
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := measEpoch(0, []sbfbin.MeasEpochChannelType1{tc.master},
				[][]sbfbin.MeasEpochChannelType2{{tc.slave}})
			got := convert(t, testTS, m, nil)
			var want []rinex.SignalObservation
			if masterObs := convert(t, testTS, measEpoch(0, []sbfbin.MeasEpochChannelType1{tc.master}, nil), nil); len(masterObs) > 0 {
				want = append(want, masterObs...)
			}
			want = append(want, tc.expect)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestSlaveEmittedWithoutMaster(t *testing.T) {
	master := masterCh(7, 16) // reserved signal number: no master observation
	slave := sbfbin.MeasEpochChannelType2{
		Type: 2, OffsetsMSB: 7 | 30<<3, CodeOffsetLSB: 500, DopplerOffsetLSB: 400,
		CarrierMSB: -1, CarrierLSB: 200, CN0: 100, LockTime: 50,
	}
	m := measEpoch(0, []sbfbin.MeasEpochChannelType1{master},
		[][]sbfbin.MeasEpochChannelType2{{slave}})
	got := convert(t, testTS, m, nil)
	pr := testPR + float64(-1*65536+500)*0.001
	f2 := mustFreq("G", "2W", 0, false)
	// The master carrier frequency is unknown, so the slave Doppler cannot be
	// reconstructed; PR and CP only need the master pseudorange.
	want := []rinex.SignalObservation{{T: testT, Sat: "G07", Sig: "2W", SignalValues: rinex.SignalValues{
		PR: opt.Make(pr), CP: opt.Make(pr/(299792458.0/f2) + float64(-1*65536+200)*0.001),
		CN0: opt.Make[float32](25),
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestMainAntennaOnly(t *testing.T) {
	main := masterCh(7, 0)
	aux := main
	aux.Type |= 1 << 5
	slave := sbfbin.MeasEpochChannelType2{Type: 2, CN0: 100}
	auxSlave := slave
	auxSlave.Type |= 1 << 5
	var mainExtra, auxExtra sbfbin.MeasExtraChannelSub
	mainExtra.RxChannel = 1
	mainExtra.Misc = 3
	auxExtra = mainExtra
	auxExtra.Type = 1 << 5
	auxExtra.Misc = 6
	extra := &sbfbin.MeasExtra{Channels: []sbfbin.MeasExtraChannelSub{mainExtra, auxExtra}}
	extra.SBLength = 16
	m := measEpoch(0, []sbfbin.MeasEpochChannelType1{main, aux},
		[][]sbfbin.MeasEpochChannelType2{{slave, auxSlave}, nil})
	got := convert(t, testTS, m, extra)
	if len(got) != 2 || got[0].Sig != "1C" || got[1].Sig != "2W" {
		t.Fatalf("got %+v, want main-antenna 1C and 2W", got)
	}
	wantCN0 := float32(40 + 3*0.03125)
	if !got[0].CN0.IsSet() || got[0].CN0.Get() != wantCN0 {
		t.Errorf("main CN0 = %v, want %v", got[0].CN0, wantCN0)
	}
}

func TestCN0Refinement(t *testing.T) {
	extra := func(sbLength, rxChannel, sig, misc uint8) *sbfbin.MeasExtra {
		var e sbfbin.MeasExtra
		e.SBLength = sbLength
		var s sbfbin.MeasExtraChannelSub
		s.RxChannel = rxChannel
		s.Type = sbfbin.MeasType(sig)
		s.Misc = misc
		e.Channels = []sbfbin.MeasExtraChannelSub{s}
		return &e
	}
	tests := []struct {
		name   string
		extra  *sbfbin.MeasExtra
		expect float32
	}{
		{name: "no MeasExtra", extra: nil, expect: 40},
		{name: "matching refinement", extra: extra(16, 1, 0, 5), expect: 40 + 5*0.03125},
		{name: "pre-revision-3 block ignored", extra: extra(15, 1, 0, 5), expect: 40},
		{name: "channel mismatch", extra: extra(16, 2, 0, 5), expect: 40},
		{name: "signal mismatch", extra: extra(16, 1, 3, 5), expect: 40},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(t, testTS, measEpoch(0, []sbfbin.MeasEpochChannelType1{masterCh(7, 0)}, nil), tc.extra)
			if len(got) != 1 || !got[0].CN0.IsSet() || got[0].CN0.Get() != tc.expect {
				t.Errorf("got %+v, want CN0 %v", got, tc.expect)
			}
		})
	}
}

func TestArcHC(t *testing.T) {
	// Each step feeds one GPS L1CA master through the same converter and
	// checks the loss-of-lock outputs across lock-time resets, an epoch with
	// no carrier phase, and Do-Not-Use lock-times.
	steps := []struct {
		name      string
		lockTime  uint16
		noPhase   bool
		halfCycle bool
		expectArc uint32
		expectHC  bool
	}{
		{name: "initial lock", lockTime: 10, expectArc: 0},
		{name: "reset with phase", lockTime: 0, expectArc: 1},
		{name: "reset without phase stays pending", lockTime: 0, noPhase: true, expectArc: 1},
		{name: "phase returns", lockTime: 3, expectArc: 2},
		{name: "DNU lock-time leaves state alone", lockTime: sbfbin.MeasType1LockTimeDNU, noPhase: true, expectArc: 2},
		{name: "lock grows", lockTime: 5, expectArc: 2},
		{name: "half-cycle with phase", lockTime: 6, halfCycle: true, expectArc: 2, expectHC: true},
		{name: "decrease marks slip", lockTime: 2, expectArc: 3},
		{name: "clipped lock-time is valid", lockTime: sbfbin.MeasType1LockTimeClipped, expectArc: 3},
	}
	s := &testSink{}
	c := New(s)
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			t1 := masterCh(7, 0)
			t1.LockTime = st.lockTime
			if st.noPhase {
				t1.CarrierMSB = -128
				t1.CarrierLSB = 0
			}
			if st.halfCycle {
				t1.ObsInfo = 1 << 2
			}
			s.obs = nil
			if err := c.ConvertMeasEpoch(testTS, measEpoch(0, []sbfbin.MeasEpochChannelType1{t1}, nil), nil); err != nil {
				t.Fatalf("ConvertMeasEpoch: %v", err)
			}
			if len(s.obs) != 1 {
				t.Fatalf("len observations = %d, want 1", len(s.obs))
			}
			if s.obs[0].Arc != st.expectArc || s.obs[0].HC != st.expectHC {
				t.Errorf("Arc/HC = %d/%v, want %d/%v", s.obs[0].Arc, s.obs[0].HC, st.expectArc, st.expectHC)
			}
		})
	}
}

func TestArcAcrossMasterSlaveSwitch(t *testing.T) {
	// The receiver can re-select which signal of a satellite is the Type1
	// master. A signal moving from master (lock 800, clipped at 65534) to
	// slave (lock clipped at 254) must not read as a lock-time decrease, but
	// a genuine slave reset must still count.
	s := &testSink{}
	c := New(s)
	asMaster := func(lock uint16) *sbfbin.MeasEpoch {
		t1 := masterCh(181, 7) // QZSS L2C master
		t1.LockTime = lock
		return measEpoch(0, []sbfbin.MeasEpochChannelType1{t1}, nil)
	}
	asSlave := func(lock uint8) *sbfbin.MeasEpoch {
		return measEpoch(0, []sbfbin.MeasEpochChannelType1{masterCh(181, 26)},
			[][]sbfbin.MeasEpochChannelType2{{{
				Type: 7, LockTime: lock, CN0: 100,
				OffsetsMSB: 3, CodeOffsetLSB: 500, DopplerOffsetLSB: 400,
				CarrierMSB: 1, CarrierLSB: 200,
			}}})
	}
	steps := []struct {
		name      string
		m         *sbfbin.MeasEpoch
		expectArc uint32
	}{
		{name: "slave clipped", m: asSlave(254), expectArc: 0},
		{name: "becomes master above slave ceiling", m: asMaster(800), expectArc: 0},
		{name: "back to clipped slave", m: asSlave(254), expectArc: 0},
		{name: "genuine slave reset", m: asSlave(3), expectArc: 1},
		{name: "becomes master again", m: asMaster(4), expectArc: 1},
		{name: "master reset", m: asMaster(0), expectArc: 2},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			s.obs = nil
			if err := c.ConvertMeasEpoch(testTS, st.m, nil); err != nil {
				t.Fatalf("ConvertMeasEpoch: %v", err)
			}
			for _, o := range s.obs {
				if o.Sig != "2L" {
					continue
				}
				if o.Arc != st.expectArc {
					t.Errorf("Arc = %d, want %d", o.Arc, st.expectArc)
				}
				return
			}
			t.Fatalf("no 2L observation in %+v", s.obs)
		})
	}
}

func TestConvertBlock(t *testing.T) {
	extra := func() *sbfbin.MeasExtra {
		var e sbfbin.MeasExtra
		e.SBLength = 16
		var s sbfbin.MeasExtraChannelSub
		s.RxChannel = 1
		s.Misc = 5
		e.Channels = []sbfbin.MeasExtraChannelSub{s}
		return &e
	}
	ts2 := sbfbin.TimeStamp{TOW: testTS.TOW + 1000, WNc: testTS.WNc}
	ts3 := sbfbin.TimeStamp{TOW: testTS.TOW + 2000, WNc: testTS.WNc}
	epoch := func() *sbfbin.MeasEpoch {
		return measEpoch(0, []sbfbin.MeasEpochChannelType1{masterCh(7, 0)}, nil)
	}
	steps := []struct {
		name      string
		b         sbfbin.Block
		expectOK  bool
		expectObs int // cumulative observation count after the call
	}{
		{name: "other block type ignored", b: sbfbin.Block{Params: &sbfbin.EndOfMeas{}}},
		{name: "MeasEpoch held", b: sbfbin.Block{TimeStamp: testTS, Params: epoch()}, expectOK: true},
		{name: "other block type leaves it held", b: sbfbin.Block{TimeStamp: testTS, Params: &sbfbin.EndOfMeas{}}},
		{name: "matching MeasExtra converts the pair", b: sbfbin.Block{TimeStamp: testTS, Params: extra()}, expectObs: 1},
		{name: "next MeasEpoch held", b: sbfbin.Block{TimeStamp: ts2, Params: epoch()}, expectOK: true, expectObs: 1},
		{name: "mismatched MeasExtra converts it alone", b: sbfbin.Block{TimeStamp: ts3, Params: extra()}, expectObs: 2},
		{name: "last MeasEpoch held", b: sbfbin.Block{TimeStamp: ts3, Params: epoch()}, expectOK: true, expectObs: 2},
	}
	s := &testSink{}
	c := New(s)
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			ok, err := c.ConvertBlock(&st.b)
			if err != nil {
				t.Fatalf("ConvertBlock: %v", err)
			}
			if ok != st.expectOK || len(s.obs) != st.expectObs {
				t.Errorf("ok %v obs %d, want %v %d", ok, len(s.obs), st.expectOK, st.expectObs)
			}
		})
	}
	if err := c.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := c.Flush(); err != nil {
		t.Fatalf("second Flush: %v", err)
	}
	if len(s.obs) != 3 {
		t.Fatalf("len observations after Flush = %d, want 3", len(s.obs))
	}
	// The paired epoch gets the CN0 refinement, the unpaired ones do not.
	for i, want := range []float32{40 + 5*0.03125, 40, 40} {
		if got := s.obs[i].CN0.Get(); got != want {
			t.Errorf("observation %d CN0 = %v, want %v", i, got, want)
		}
	}
	for i, want := range []rinex.Time{testT, rinex.TimeFromGPSWeekMillis(2400, ts2.TOW), rinex.TimeFromGPSWeekMillis(2400, ts3.TOW)} {
		if got := s.obs[i].T; got != want {
			t.Errorf("observation %d T = %v, want %v", i, got, want)
		}
	}
}

func TestArcFromCumLossCont(t *testing.T) {
	// CumLossCont changes mark a slip even when the lock time gives no hint
	// (e.g. re-clipped after an outage); an absent MeasExtra epoch in between
	// leaves the counter state alone.
	extra := func(cumLossCont uint8) *sbfbin.MeasExtra {
		var e sbfbin.MeasExtra
		e.SBLength = 16
		var s sbfbin.MeasExtraChannelSub
		s.RxChannel = 1
		s.CumLossCont = cumLossCont
		e.Channels = []sbfbin.MeasExtraChannelSub{s}
		return &e
	}
	steps := []struct {
		name      string
		extra     *sbfbin.MeasExtra
		expectArc uint32
	}{
		{name: "first epoch", extra: extra(3), expectArc: 0},
		{name: "steady counter", extra: extra(3), expectArc: 0},
		{name: "counter increments", extra: extra(4), expectArc: 1},
		{name: "no MeasExtra leaves counter state alone", extra: nil, expectArc: 1},
		{name: "still 4 after gap", extra: extra(4), expectArc: 1},
		{name: "wraps modulo 256", extra: extra(0), expectArc: 2},
	}
	s := &testSink{}
	c := New(s)
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			t1 := masterCh(7, 0)
			t1.LockTime = sbfbin.MeasType1LockTimeClipped
			s.obs = nil
			if err := c.ConvertMeasEpoch(testTS, measEpoch(0, []sbfbin.MeasEpochChannelType1{t1}, nil), st.extra); err != nil {
				t.Fatalf("ConvertMeasEpoch: %v", err)
			}
			if len(s.obs) != 1 {
				t.Fatalf("len observations = %d, want 1", len(s.obs))
			}
			if s.obs[0].Arc != st.expectArc {
				t.Errorf("Arc = %d, want %d", s.obs[0].Arc, st.expectArc)
			}
		})
	}
}

func TestFirstObservationWithZeroLockTime(t *testing.T) {
	t1 := masterCh(7, 0)
	t1.LockTime = 0
	got := convert(t, testTS, measEpoch(0, []sbfbin.MeasEpochChannelType1{t1}, nil), nil)
	if len(got) != 1 || got[0].Arc != 0 {
		t.Fatalf("got %+v, want one observation with Arc 0", got)
	}
}
