package rnxubx

import (
	"testing"

	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
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

func TestConverterLeavesZeroLLIUnsetForValidPhase(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	if err := c.ConvertRAWX(rawx(ubxbin.RxmRawxMeas{
		PrMes:    23956830.530,
		CpMes:    125893980.172,
		GNSSID:   ubxbin.GPS,
		SVID:     7,
		SigID:    0,
		LockTime: 100,
		TrkStat:  ubxbin.RxmRawxPrValid | ubxbin.RxmRawxCpValid | ubxbin.RxmRawxHalfCyc,
	})); err != nil {
		t.Fatalf("ConvertRAWX: %v", err)
	}
	if len(s.obs) != 1 {
		t.Fatalf("len observations = %d, want 1", len(s.obs))
	}
	if s.obs[0].Arc != 0 || s.obs[0].HC || s.obs[0].BT {
		t.Errorf("Arc/HC/BT = %v/%v/%v, want 0/false/false", s.obs[0].Arc, s.obs[0].HC, s.obs[0].BT)
	}
}

func TestConverterSetsNonzeroLLI(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	if err := c.ConvertRAWX(rawx(ubxbin.RxmRawxMeas{
		PrMes:   23956830.530,
		CpMes:   125893980.172,
		GNSSID:  ubxbin.GPS,
		SVID:    7,
		SigID:   0,
		TrkStat: ubxbin.RxmRawxPrValid | ubxbin.RxmRawxCpValid | ubxbin.RxmRawxHalfCyc,
	})); err != nil {
		t.Fatalf("ConvertRAWX: %v", err)
	}
	if len(s.obs) != 1 {
		t.Fatalf("len observations = %d, want 1", len(s.obs))
	}
	if s.obs[0].Arc != 1 || s.obs[0].HC || s.obs[0].BT {
		t.Errorf("Arc/HC/BT = %v/%v/%v, want 1/false/false", s.obs[0].Arc, s.obs[0].HC, s.obs[0].BT)
	}
}

func TestConverterLeavesLLIUnsetWithoutPhaseIndicator(t *testing.T) {
	s := &testSink{}
	c := New(s, Options{})
	if err := c.ConvertRAWX(rawx(ubxbin.RxmRawxMeas{
		PrMes:    23956830.530,
		GNSSID:   ubxbin.GPS,
		SVID:     7,
		SigID:    0,
		LockTime: 100,
		TrkStat:  ubxbin.RxmRawxPrValid,
	})); err != nil {
		t.Fatalf("ConvertRAWX: %v", err)
	}
	if len(s.obs) != 1 {
		t.Fatalf("len observations = %d, want 1", len(s.obs))
	}
	if s.obs[0].Arc != 0 || s.obs[0].HC || s.obs[0].BT {
		t.Errorf("Arc/HC/BT = %v/%v/%v, want 0/false/false", s.obs[0].Arc, s.obs[0].HC, s.obs[0].BT)
	}
}

func TestConverterBDSGeoHalfCycleOption(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		meas ubxbin.RxmRawxMeas
		want float64
	}{
		{
			name: "default BDS GEO unchanged",
			meas: phaseMeas(ubxbin.BDS, 2),
			want: 100.25,
		},
		{
			name: "enabled BDS GEO shifted",
			opts: Options{BDSGeoHalfCycle: true},
			meas: phaseMeas(ubxbin.BDS, 2),
			want: 100.75,
		},
		{
			name: "enabled non-BDS-GEO unchanged",
			opts: Options{BDSGeoHalfCycle: true},
			meas: phaseMeas(ubxbin.BDS, 6),
			want: 100.25,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &testSink{}
			c := New(s, tt.opts)
			if err := c.ConvertRAWX(rawx(tt.meas)); err != nil {
				t.Fatalf("ConvertRAWX: %v", err)
			}
			if len(s.obs) != 1 {
				t.Fatalf("len observations = %d, want 1", len(s.obs))
			}
			if !s.obs[0].CP.IsSet() || s.obs[0].CP.Get() != tt.want {
				t.Fatalf("CP = %v, want %v", s.obs[0].CP, tt.want)
			}
		})
	}
}

func phaseMeas(gnssID ubxbin.GNSSID, svid byte) ubxbin.RxmRawxMeas {
	return ubxbin.RxmRawxMeas{
		CpMes:    100.25,
		GNSSID:   gnssID,
		SVID:     svid,
		SigID:    0,
		LockTime: 100,
		TrkStat:  ubxbin.RxmRawxCpValid | ubxbin.RxmRawxHalfCyc,
	}
}

func rawx(meas ...ubxbin.RxmRawxMeas) *ubxbin.RxmRawx {
	return &ubxbin.RxmRawx{
		RxmRawxFixed: ubxbin.RxmRawxFixed{
			RcvTow:  1,
			Week:    1,
			NumMeas: uint8(len(meas)),
			Version: 1,
		},
		Meas: meas,
	}
}
