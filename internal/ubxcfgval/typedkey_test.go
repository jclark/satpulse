package ubxcfgval

import "testing"

func TestTypedKey(t *testing.T) {
	testKV(t, KTpLenTp1, 1000)
	testKV(t, KTpDutyTp1, 0.5)
	testKV(t, KTpPulseDef, ETpPulseDefFreq)
	testKV(t, KTpSyncGnssTp1, true)
	testKV(t, KTpSyncGnssTp1, false)
	testKV(t, KTpAntCabledelay, 50)
	testKV(t, KTpAntCabledelay, 1000)
	testKV(t, KTpAntCabledelay, -50)
	testKV(t, KTpAntCabledelay, -1000)
}

func testKV[T comparable](t *testing.T, k TypedKey[T], v T) {
	raw := k.Raw(v)
	v2 := k.Typed(raw)
	if v != v2 {
		t.Errorf("wrong value for %T: %v", k, v2)
	}
}
