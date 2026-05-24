package rtcm

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
)

type testSink struct {
	meta []rinex.Metadata
	obs  []rinex.SignalObservation
}

func (s *testSink) Metadata(meta rinex.Metadata) error {
	s.meta = append(s.meta, meta)
	return nil
}

func (s *testSink) Observation(obs rinex.SignalObservation) error {
	s.obs = append(s.obs, obs)
	return nil
}

func (s *testSink) Flush() error {
	return nil
}

func TestNewNilSinkPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New with nil sink did not panic")
		}
	}()
	_ = New(nil, Options{})
}

func TestConvertMetadata(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	station := rtcmbin.MsgHdrStationID{StationID: 1234}
	ok, err := c.ConvertMsg(&rtcmbin.MT1006{
		MT1005: rtcmbin.MT1005{
			MsgHdrStationID: station,
			ECEFX:           111 * rtcmbin.Meter,
			ECEFY:           -222 * rtcmbin.Meter,
			ECEFZ:           333 * rtcmbin.Meter,
		},
		AntennaHeight: 123,
	}, TimeInterval{})
	if err != nil {
		t.Fatalf("ConvertMsg MT1006: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMsg MT1006 ok = false, want true")
	}
	ok, err = c.ConvertMsg(&rtcmbin.MT1033{
		MT1008: rtcmbin.MT1008{
			MT1007: rtcmbin.MT1007{
				MsgHdrStationID:   station,
				AntennaDescriptor: rtcmbin.ASCIIString("ANT TYPE   "),
			},
			AntennaSerialNumber: rtcmbin.ASCIIString("ANT123"),
		},
		ReceiverTypeDescriptor:  rtcmbin.ASCIIString("RX TYPE"),
		ReceiverFirmwareVersion: rtcmbin.ASCIIString("FW1"),
		ReceiverSerialNumber:    rtcmbin.ASCIIString("RX123"),
	}, TimeInterval{})
	if err != nil {
		t.Fatalf("ConvertMsg MT1033: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMsg MT1033 ok = false, want true")
	}
	ok, err = c.ConvertMsg(&rtcmbin.MT1013{
		MsgHdrStationID: station,
		LeapSeconds:     17,
	}, TimeInterval{})
	if err != nil {
		t.Fatalf("ConvertMsg MT1013: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMsg MT1013 ok = false, want true")
	}
	if len(s.meta) != 3 {
		t.Fatalf("metadata count = %d, want 3", len(s.meta))
	}
	if s.meta[0].Marker.Number != "1234" || s.meta[0].ApproxPosition == nil || *s.meta[0].ApproxPosition != [3]float64{111, -222, 333} || s.meta[0].AntennaDelta == nil || !near(s.meta[0].AntennaDelta[0], 0.0123, 1e-12) {
		t.Fatalf("MT1006 metadata = %#v", s.meta[0])
	}
	if s.meta[1].Antenna.Type != "ANT TYPE" || s.meta[1].Antenna.Number != "ANT123" || s.meta[1].Receiver.Type != "RX TYPE" || s.meta[1].Receiver.Version != "FW1" || s.meta[1].Receiver.Number != "RX123" {
		t.Fatalf("MT1033 metadata = %#v", s.meta[1])
	}
	if s.meta[2].LeapSeconds == nil || *s.meta[2].LeapSeconds != 17 {
		t.Fatalf("MT1013 metadata = %#v", s.meta[2])
	}
}

func TestConvertMSM7GPS(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	ok, err := c.ConvertMSM7(msm7GPS(tow), weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMSM7 ok = false, want true")
	}
	if len(s.obs) != 1 {
		t.Fatalf("observation count = %d, want 1", len(s.obs))
	}
	obs := s.obs[0]
	if obs.T != wantT || obs.Sat != "G03" || obs.Sig != "1C" {
		t.Fatalf("observation identity = %#v", obs)
	}
	pr := speedOfLight / 1000 * 70.5
	wl := speedOfLight / (1575.420 * 1e6)
	if !near(obs.PR.Get(), pr, 1e-6) {
		t.Fatalf("PR = %.9f, want %.9f", obs.PR.Get(), pr)
	}
	if !near(obs.CP.Get(), pr/wl, 1e-6) {
		t.Fatalf("CP = %.9f, want %.9f", obs.CP.Get(), pr/wl)
	}
	if !near(obs.Do.Get(), 99.8/wl, 3e-5) {
		t.Fatalf("Do = %.9f, want %.9f", obs.Do.Get(), 99.8/wl)
	}
	if obs.CN0.Get() != 45 {
		t.Fatalf("CN0 = %.3f, want 45", obs.CN0.Get())
	}
	if !obs.LLI.IsSet() || obs.LLI.Get() != rinex.LLIHalfCycleAmbiguity {
		t.Fatalf("LLI = %v, want half cycle", obs.LLI)
	}
}

func TestConvertMSM7StrictPhaseRangeRateSign(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{UseSpecPhaseRangeRateSign: true})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	ok, err := c.ConvertMSM7(msm7GPS(tow), weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7: %v", err)
	}
	if !ok || len(s.obs) != 1 {
		t.Fatalf("observations = %#v ok=%v, want one", s.obs, ok)
	}
	wl := speedOfLight / (1575.420 * 1e6)
	if !near(s.obs[0].Do.Get(), -99.8/wl, 3e-5) {
		t.Fatalf("Do = %.9f, want %.9f", s.obs[0].Do.Get(), -99.8/wl)
	}
}

func TestConvertMSM7GPSMultiCell(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	m := &rtcmbin.MSMHiRes{
		MSMHeader: rtcmbin.MSMHeader{
			MsgHdrStationID: rtcmbin.MsgHdrStationID{
				MsgHdr:    rtcmbin.MsgHdr{MsgNum: 1077},
				StationID: 100,
			},
			EpochTime: tow,
			SatMask:   satMask(3, 4),
			SigMask:   sigMask(2, 22),
		},
		// Satellite 4 carries two signals, so cellIndex diverges from satIndex.
		CellMask: 0b0111,
		Sat: rtcmbin.MSMSatData{
			RangeInt:  rtcmbin.Uint8Slice{70, 80},
			RangeMod:  []uint16{0, 512},
			PhaseRate: []int16{10, 20},
		},
		Sig: rtcmbin.MSMHiResSigData{
			Pseudorange: []int32{0, 0, 0},
			PhaseRange:  []int32{0, 0, 0},
			LockTime:    []uint16{10, 10, 10},
			HalfCycle:   []bool{false, false, false},
			CNR:         []uint16{720, 640, 800},
			PhaseRate:   []int16{-1000, -2000, -3000},
		},
	}
	ok, err := c.ConvertMSM7(m, weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMSM7 ok = false, want true")
	}
	if len(s.obs) != 3 {
		t.Fatalf("observation count = %d, want 3", len(s.obs))
	}
	wants := []struct {
		sat rinex.SatelliteID
		sig rinex.SignalID
		rng float64
		f   float64
		dop float64
		cn0 float32
	}{
		{sat: "G03", sig: "5I", rng: 70, f: 1176.450, dop: 9.9, cn0: 45},
		{sat: "G04", sig: "1C", rng: 80.5, f: 1575.420, dop: 19.8, cn0: 40},
		{sat: "G04", sig: "5I", rng: 80.5, f: 1176.450, dop: 19.7, cn0: 50},
	}
	for i, want := range wants {
		obs := s.obs[i]
		if obs.T != wantT || obs.Sat != want.sat || obs.Sig != want.sig {
			t.Fatalf("observation %d identity = %#v", i, obs)
		}
		pr := speedOfLight / 1000 * want.rng
		wl := speedOfLight / (want.f * 1e6)
		if !near(obs.PR.Get(), pr, 1e-6) {
			t.Fatalf("observation %d PR = %.9f, want %.9f", i, obs.PR.Get(), pr)
		}
		if !near(obs.CP.Get(), pr/wl, 1e-6) {
			t.Fatalf("observation %d CP = %.9f, want %.9f", i, obs.CP.Get(), pr/wl)
		}
		if !near(obs.Do.Get(), want.dop/wl, 3e-5) {
			t.Fatalf("observation %d Do = %.9f, want %.9f", i, obs.Do.Get(), want.dop/wl)
		}
		if obs.CN0.Get() != want.cn0 {
			t.Fatalf("observation %d CN0 = %.3f, want %.3f", i, obs.CN0.Get(), want.cn0)
		}
	}
}

func TestConvertMSM7SkipsUnknownIDs(t *testing.T) {
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	for _, tt := range []struct {
		name  string
		satID uint8
		sigID uint8
	}{
		{name: "satellite", satID: 64, sigID: 2},
		{name: "signal", satID: 3, sigID: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &testSink{}
			c := New(s, Options{})
			ok, err := c.ConvertMSM7(msm7(1077, tow, tt.satID, tt.sigID, 0), weekFor(wantT))
			if err != nil {
				t.Fatalf("ConvertMSM7: %v", err)
			}
			if ok || len(s.obs) != 0 {
				t.Fatalf("observations = %#v ok=%v, want none", s.obs, ok)
			}
		})
	}
}

func TestConvertMSM7InvalidFieldsUnset(t *testing.T) {
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	for _, tt := range []struct {
		name                            string
		mutate                          func(*rtcmbin.MSMHiRes)
		wantPR, wantCP, wantDo, wantCN0 bool
	}{
		{
			name: "rough range",
			mutate: func(m *rtcmbin.MSMHiRes) {
				m.Sat.RangeInt[0] = 255
			},
			wantDo: true, wantCN0: true,
		},
		{
			name: "pseudorange",
			mutate: func(m *rtcmbin.MSMHiRes) {
				m.Sig.Pseudorange[0] = -524288
			},
			wantCP: true, wantDo: true, wantCN0: true,
		},
		{
			name: "phase range",
			mutate: func(m *rtcmbin.MSMHiRes) {
				m.Sig.PhaseRange[0] = -8388608
			},
			wantPR: true, wantDo: true, wantCN0: true,
		},
		{
			name: "rough phase rate",
			mutate: func(m *rtcmbin.MSMHiRes) {
				m.Sat.PhaseRate[0] = -8192
			},
			wantPR: true, wantCP: true, wantCN0: true,
		},
		{
			name: "fine phase rate",
			mutate: func(m *rtcmbin.MSMHiRes) {
				m.Sig.PhaseRate[0] = -16384
			},
			wantPR: true, wantCP: true, wantCN0: true,
		},
		{
			name: "cn0",
			mutate: func(m *rtcmbin.MSMHiRes) {
				m.Sig.CNR[0] = 0
			},
			wantPR: true, wantCP: true, wantDo: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &testSink{}
			c := New(s, Options{})
			m := msm7GPS(tow)
			tt.mutate(m)
			ok, err := c.ConvertMSM7(m, weekFor(wantT))
			if err != nil {
				t.Fatalf("ConvertMSM7: %v", err)
			}
			if !ok || len(s.obs) != 1 {
				t.Fatalf("observations = %#v ok=%v, want one", s.obs, ok)
			}
			obs := s.obs[0]
			if obs.PR.IsSet() != tt.wantPR || obs.CP.IsSet() != tt.wantCP || obs.Do.IsSet() != tt.wantDo || obs.CN0.IsSet() != tt.wantCN0 {
				t.Fatalf("observables = %#v, want PR=%v CP=%v Do=%v CN0=%v", obs, tt.wantPR, tt.wantCP, tt.wantDo, tt.wantCN0)
			}
		})
	}
}

func TestConvertMSM7NeedsIntervalBeforeContext(t *testing.T) {
	c := New(&testSink{}, Options{})
	_, err := c.ConvertMSM7(msm7GPS(345600000), TimeInterval{})
	if err == nil {
		t.Fatal("ConvertMSM7 without week constraint succeeded")
	}
}

func TestConvertMsgStoresIntervalFromMetadata(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	_, err := c.ConvertMsg(&rtcmbin.MT1005{
		MsgHdrStationID: rtcmbin.MsgHdrStationID{StationID: 100},
	}, weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMsg metadata: %v", err)
	}
	ok, err := c.ConvertMSM7(msm7GPS(tow), TimeInterval{})
	if err != nil {
		t.Fatalf("ConvertMSM7: %v", err)
	}
	if !ok || len(s.obs) != 1 || s.obs[0].T != wantT {
		t.Fatalf("observations = %#v ok=%v", s.obs, ok)
	}
}

func TestConvertMSM7StoresIntervalForOtherGNSS(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	if _, err := c.ConvertMSM7(msm7GPS(tow), weekFor(wantT)); err != nil {
		t.Fatalf("ConvertMSM7 GPS: %v", err)
	}
	epoch := glonassEpoch(4, 3*time.Hour)
	if _, err := c.ConvertMSM7(msm7GLONASS(epoch, 7), TimeInterval{}); err != nil {
		t.Fatalf("ConvertMSM7 GLONASS: %v", err)
	}
	if len(s.obs) != 2 {
		t.Fatalf("observation count = %d, want 2", len(s.obs))
	}
	if s.obs[1].T != wantT+rinex.Time(defaultGPSUTCMS*rinexTicksPerMS) || s.obs[1].Sat != "R05" || s.obs[1].Sig != "1C" {
		t.Fatalf("GLONASS observation = %#v", s.obs[1])
	}
}

func TestConvertMSM7CarriesTimeContextAndLLI(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	m1 := msm7GPS(345600000)
	m1.Sig.HalfCycle[0] = false
	wantT := rinex.TimeFromGPSWeekMillis(2397, 345600000)
	if _, err := c.ConvertMSM7(m1, weekFor(wantT)); err != nil {
		t.Fatalf("ConvertMSM7 first: %v", err)
	}
	m2 := msm7GPS(345601000)
	m2.Sig.LockTime[0] = 5
	m2.Sig.HalfCycle[0] = false
	if _, err := c.ConvertMSM7(m2, TimeInterval{}); err != nil {
		t.Fatalf("ConvertMSM7 second: %v", err)
	}
	if len(s.obs) != 2 {
		t.Fatalf("observation count = %d, want 2", len(s.obs))
	}
	if s.obs[1].T != rinex.TimeFromGPSWeekMillis(2397, 345601000) {
		t.Fatalf("second time = %s", s.obs[1].T)
	}
	if !s.obs[1].LLI.IsSet() || s.obs[1].LLI.Get() != rinex.LLILostLock {
		t.Fatalf("second LLI = %v, want lost lock", s.obs[1].LLI)
	}
}

func TestConvertMSM7FirstZeroLockSetsLLI(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	m := msm7GPS(tow)
	m.Sig.LockTime[0] = 0
	m.Sig.HalfCycle[0] = false
	ok, err := c.ConvertMSM7(m, weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7: %v", err)
	}
	if !ok || len(s.obs) != 1 {
		t.Fatalf("observations = %#v ok=%v, want one", s.obs, ok)
	}
	if !s.obs[0].LLI.IsSet() || s.obs[0].LLI.Get() != rinex.LLILostLock {
		t.Fatalf("LLI = %v, want lost lock", s.obs[0].LLI)
	}
}

func TestConvertMSM7CarriesBlankPhaseLLI(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	m1 := msm7GPS(tow)
	m1.Sig.PhaseRange[0] = -8388608
	m1.Sig.LockTime[0] = 0
	m1.Sig.HalfCycle[0] = false
	if _, err := c.ConvertMSM7(m1, weekFor(wantT)); err != nil {
		t.Fatalf("ConvertMSM7 first: %v", err)
	}
	m2 := msm7GPS(tow + 1000)
	m2.Sig.LockTime[0] = 5
	m2.Sig.HalfCycle[0] = false
	if _, err := c.ConvertMSM7(m2, TimeInterval{}); err != nil {
		t.Fatalf("ConvertMSM7 second: %v", err)
	}
	if len(s.obs) != 2 {
		t.Fatalf("observation count = %d, want 2", len(s.obs))
	}
	if s.obs[0].CP.IsSet() || !s.obs[0].LLI.IsSet() || s.obs[0].LLI.Get() != rinex.LLILostLock {
		t.Fatalf("first observation = %#v, want blank phase lost lock", s.obs[0])
	}
	if !s.obs[1].CP.IsSet() || !s.obs[1].LLI.IsSet() || s.obs[1].LLI.Get() != rinex.LLILostLock {
		t.Fatalf("second observation = %#v, want carried lost lock", s.obs[1])
	}
}

func TestConvertMSM7CurrentIntervalOverridesContinuity(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	t1 := rinex.TimeFromGPSWeekMillis(2397, tow)
	t2 := rinex.TimeFromGPSWeekMillis(2398, tow)
	if _, err := c.ConvertMSM7(msm7GPS(tow), weekFor(t1)); err != nil {
		t.Fatalf("ConvertMSM7 first: %v", err)
	}
	if _, err := c.ConvertMSM7(msm7GPS(tow), weekFor(t2)); err != nil {
		t.Fatalf("ConvertMSM7 second: %v", err)
	}
	if len(s.obs) != 2 {
		t.Fatalf("observation count = %d, want 2", len(s.obs))
	}
	if s.obs[1].T != t2 {
		t.Fatalf("second time = %s, want %s", s.obs[1].T, t2)
	}
}

func TestConvertMSM7GLONASSFrequency(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	epoch := glonassEpoch(2, 4*time.Hour)
	wantT := timeFromWeekOffset(2397, 2*dayMS+hourMS+defaultGPSUTCMS)
	ok, err := c.ConvertMSM7(msm7GLONASS(epoch, 7), weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7 GLONASS: %v", err)
	}
	if !ok || len(s.obs) != 1 {
		t.Fatalf("observations = %#v ok=%v, want one", s.obs, ok)
	}
	obs := s.obs[0]
	if obs.T != wantT {
		t.Fatalf("T = %s, want %s", obs.T, wantT)
	}
	if !obs.Frq.IsSet() || obs.Frq.Get() != 0 {
		t.Fatalf("Frq = %v, want 0", obs.Frq)
	}
	pr := speedOfLight / 1000 * 70.5
	wl := speedOfLight / (1602.000 * 1e6)
	if !near(obs.CP.Get(), pr/wl, 1e-6) {
		t.Fatalf("CP = %.9f, want %.9f", obs.CP.Get(), pr/wl)
	}
	if !near(obs.Do.Get(), 99.8/wl, 3e-5) {
		t.Fatalf("Do = %.9f, want %.9f", obs.Do.Get(), 99.8/wl)
	}
}

func TestConvertMSM7GLONASSUnknownFrequency(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	epoch := glonassEpoch(2, 4*time.Hour)
	wantT := timeFromWeekOffset(2397, 2*dayMS+hourMS+defaultGPSUTCMS)
	ok, err := c.ConvertMSM7(msm7GLONASS(epoch, 15), weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7 GLONASS: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMSM7 GLONASS ok = false, want true")
	}
	if len(s.obs) != 1 {
		t.Fatalf("observation count = %d, want 1", len(s.obs))
	}
	obs := s.obs[0]
	if obs.T != wantT || obs.Sat != "R05" || obs.Sig != "1C" {
		t.Fatalf("observation identity = %#v", obs)
	}
	if obs.Frq.IsSet() {
		t.Fatalf("Frq = %v, want unset", obs.Frq)
	}
	if !obs.PR.IsSet() || !obs.CN0.IsSet() || obs.CP.IsSet() || obs.Do.IsSet() {
		t.Fatalf("observables = %#v", obs)
	}
}

func TestConvertMSM6Ignored(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	ok, err := c.ConvertMsg(msm7(1076, tow, 3, 2, 0), weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMsg MSM6: %v", err)
	}
	if ok || len(s.obs) != 0 {
		t.Fatalf("observations = %#v ok=%v, want none", s.obs, ok)
	}
	ok, err = c.ConvertMSM7(msm7GPS(tow), TimeInterval{})
	if err != nil {
		t.Fatalf("ConvertMSM7: %v", err)
	}
	if !ok || len(s.obs) != 1 || s.obs[0].T != wantT {
		t.Fatalf("observations = %#v ok=%v, want stored week constraint used", s.obs, ok)
	}
}

func TestConvertMSM7BeiDouTimeOffset(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	tow := uint32(604799000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow+14000)
	ok, err := c.ConvertMSM7(msm7BeiDou(tow), weekFor(wantT))
	if err != nil {
		t.Fatalf("ConvertMSM7 BeiDou: %v", err)
	}
	if !ok {
		t.Fatal("ConvertMSM7 BeiDou ok = false, want true")
	}
	if len(s.obs) != 1 || s.obs[0].T != wantT || s.obs[0].Sat != "C06" || s.obs[0].Sig != "2I" {
		t.Fatalf("observation = %#v", s.obs)
	}
}

func TestConvertMSM7WeekAmbiguous(t *testing.T) {
	c := New(&testSink{}, Options{})
	_, err := c.ConvertMSM7(msm7GLONASS(glonassEpoch(7, 4*time.Hour), 7), TimeInterval{
		Start:    timeFromWeekOffset(2397, 0).CivilTime(),
		Duration: 48 * time.Hour,
	})
	if err == nil {
		t.Fatal("ambiguous GLONASS week constraint succeeded")
	}
}

func TestConvertMSM7InvalidWeekPanics(t *testing.T) {
	for _, tt := range []struct {
		name string
		week TimeInterval
	}{
		{name: "overlong", week: TimeInterval{Start: time.Now(), Duration: 8 * 24 * time.Hour}},
		{name: "empty duration", week: TimeInterval{Start: time.Now()}},
		{name: "zero start", week: TimeInterval{Duration: time.Hour}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := New(&testSink{}, Options{})
			defer func() {
				if recover() == nil {
					t.Fatal("ConvertMSM7 with invalid week constraint did not panic")
				}
			}()
			_, _ = c.ConvertMSM7(msm7GPS(345600000), tt.week)
		})
	}
}

func msm7GPS(tow uint32) *rtcmbin.MSMHiRes {
	return msm7(1077, tow, 3, 2, 0)
}

func msm7GLONASS(epoch uint32, ext uint8) *rtcmbin.MSMHiRes {
	return msm7(1087, epoch, 5, 2, ext)
}

func msm7BeiDou(tow uint32) *rtcmbin.MSMHiRes {
	return msm7(1127, tow, 6, 2, 0)
}

func msm7(mt rtcmbin.MsgType, epoch uint32, satID, sigID, ext uint8) *rtcmbin.MSMHiRes {
	return &rtcmbin.MSMHiRes{
		MSMHeader: rtcmbin.MSMHeader{
			MsgHdrStationID: rtcmbin.MsgHdrStationID{
				MsgHdr:    rtcmbin.MsgHdr{MsgNum: mt},
				StationID: 100,
			},
			EpochTime: epoch,
			SatMask:   satMask(satID),
			SigMask:   sigMask(sigID),
		},
		CellMask: 1,
		Sat: rtcmbin.MSMSatData{
			RangeInt:  rtcmbin.Uint8Slice{70},
			ExtInfo:   rtcmbin.Uint8Slice{ext},
			RangeMod:  []uint16{512},
			PhaseRate: []int16{100},
		},
		Sig: rtcmbin.MSMHiResSigData{
			Pseudorange: []int32{0},
			PhaseRange:  []int32{0},
			LockTime:    []uint16{10},
			HalfCycle:   []bool{true},
			CNR:         []uint16{720},
			PhaseRate:   []int16{-2000},
		},
	}
}

func satMask(ids ...uint8) uint64 {
	var m uint64
	for _, id := range ids {
		m |= uint64(1) << uint(64-id)
	}
	return m
}

func sigMask(ids ...uint8) uint32 {
	var m uint32
	for _, id := range ids {
		m |= uint32(1) << uint(32-id)
	}
	return m
}

func weekFor(t rinex.Time) TimeInterval {
	return TimeInterval{Start: t.CivilTime().Add(-time.Minute), Duration: 2 * time.Minute}
}

func glonassEpoch(day int, tod time.Duration) uint32 {
	return uint32(day)<<27 | uint32(tod/time.Millisecond)
}

func near(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}
