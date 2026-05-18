package ubx

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
	if s.obs[0].LLI.IsSet() {
		t.Errorf("LLI = %v, want unset", s.obs[0].LLI)
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
	if !s.obs[0].LLI.IsSet() || s.obs[0].LLI.Get() != 1 {
		t.Errorf("LLI = %v, want 1", s.obs[0].LLI)
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
	if s.obs[0].LLI.IsSet() {
		t.Errorf("LLI = %v, want unset", s.obs[0].LLI)
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
