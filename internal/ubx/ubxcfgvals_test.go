package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

func TestConfigItems_Sane(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Map.SetPPS()
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	_, missing, survey, err := newCfgVals().Transaction(target, ver, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if survey {
		t.Errorf("expected survey to be false")
	}
	if len(missing) == 0 {
		t.Error("expected missing to be non-empty")
	}
	known := newCfgVals()
	cfgValSet(known, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
	_ = testSanity(t, target, ver, known)
	known = newCfgVals()
	cfgValSet(known, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Utc)
	cfgValSet(known, ucv.KNavspgUtcstandard, ucv.ENavspgUtcstandardAuto)
	cfgValSet(known, ucv.KSignalGpsEna, false)
	cfgValSet(known, ucv.KSignalGloEna, false)
	cfgValSet(known, ucv.KSignalGalEna, false)
	cfgValSet(known, ucv.KSignalBdsEna, true)
	m := testSanity(t, target, ver, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Bds)
	gpsprot.CfgPrimaryGNSS.Set(&target.Map, gpsprot.GAL)
	m = testSanity(t, target, ver, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
}

func testSanity(t *testing.T, target *gpsprot.ConfigTarget, ver *Version, known *CfgVals) *CfgVals {
	items, missing, survey, err := known.Transaction(target, ver, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if survey {
		t.Errorf("expected survey to be false")
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
	m := newCfgVals()
	m.AddItems(items)

	expectItem(t, m, ucv.KTpPulseDef, ucv.ETpPulseDefPeriod)
	expectItem(t, m, ucv.KTpPeriodLockTp1, 1e6)
	expectItem(t, m, ucv.KTpPeriodTp1, 1e6)
	expectItem(t, m, ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength)
	expectItem(t, m, ucv.KTpLenLockTp1, 1e5)
	expectItem(t, m, ucv.KTpLenTp1, 0)
	expectItem(t, m, ucv.KTpAlignToTowTp1, true)
	expectItem(t, m, ucv.KTpSyncGnssTp1, true)
	expectItem(t, m, ucv.KTpUseLockedTp1, true)
	expectItem(t, m, ucv.KTpTp1Ena, true)
	expectItem(t, m, ucv.KRateMeas, 1e3)
	expectItem(t, m, ucv.KRateNav, 1)

	if tg, ok := cfgValGet(m, ucv.KTpTimegridTp1); ok && tg == ucv.ETpTimegridTp1Utc {
		t.Errorf("expected KTpTimegridTp1 not to be UTC")
	}
	return m
}

func expectItem[T comparable](t *testing.T, m *CfgVals, key ucv.TypedKey[T], val T) {
	got, ok := cfgValGet(m, key)
	if !ok {
		t.Errorf("expected db to contain %x", key)
	} else if got != val {
		t.Errorf("expected %x to be %v, got %v", key, val, got)
	}
}

func expectMissing[T comparable](t *testing.T, m *CfgVals, key ucv.TypedKey[T]) {
	_, ok := cfgValGet(m, key)
	if ok {
		t.Errorf("expected db not to contain %x", key)
	}
}

func TestConfigItems_Empty(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	items, missing, survey, err := newCfgVals().Transaction(target, ver, ucv.UART1)

	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if survey {
		t.Errorf("expected survey to be false")
	}
	if len(items) != 0 {
		t.Errorf("expected items to be empty, got %v", items)
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
}

func TestConfigItems_GNSS(t *testing.T) {
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	target := &gpsprot.ConfigTarget{}
	gpsprot.CfgPrimaryGNSS.Set(&target.Map, gpsprot.GAL)
	gpsprot.CfgGNSSEnabled.Set(&target.Map, gpsprot.GNSSFlag(gpsprot.GAL))
	items, missing, survey, err := newCfgVals().Transaction(target, ver, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if survey {
		t.Errorf("expected survey to be false")
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
	m := newCfgVals()
	m.AddItems(items)
	expectItem(t, m, ucv.KSignalGpsEna, false)
	expectItem(t, m, ucv.KSignalGloEna, false)
	expectItem(t, m, ucv.KSignalGalEna, true)
	expectItem(t, m, ucv.KSignalBdsEna, false)
	expectMissing(t, m, ucv.KSignalNavicEna)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
	expectItem(t, m, ucv.KNavspgUtcstandard, ucv.ENavspgUtcstandardEu)
	expectItem(t, m, ucv.KRateTimeref, ucv.ERateTimerefGal)
}

func TestConfigItems_AntennaCableDelay(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	const nanos = 10
	gpsprot.CfgAntennaCableDelay.Set(&target.Map, nanos*time.Nanosecond)
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}

	items, missing, survey, err := newCfgVals().Transaction(target, ver, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if survey {
		t.Errorf("expected survey to be false")
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
	m := ucv.Map{}
	m.AddItems(items)
	ns, ok := ucv.MapGet(m, ucv.KTpAntCabledelay)
	if !ok {
		t.Error("expected db to contain KTpAntCabledelay")
	} else if ns != nanos {
		t.Errorf("expected KTpAntCabledelay to be %v, got %v", nanos, ns)
	}
}

func TestConfigItems_Survey(t *testing.T) {
	target := &gpsprot.ConfigTarget{}
	target.Opts.Survey = gpsprot.Survey{
		When:     gpsprot.TimeModeFlags(gpsprot.TimeModeDisabled),
		MinDur:   2000 * time.Second,
		AccLimit: 10 * gpsprot.Meter,
	}
	ver := &Version{
		GNSS: gpsprot.MajorGNSSSet,
		FW:   &FWVer{ProductCategory: "TIM", Major: 8, Minor: 01},
	}
	_, missing, survey, err := newCfgVals().Transaction(target, ver, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems[1]: %v", err)
	}
	if survey {
		t.Errorf("expected survey to be false")
	}
	if len(missing) != 1 {
		t.Fatalf("expected missing to be 1, got %v", missing)
	}
	if missing[0] != ucv.KTmodeMode.Key() {
		t.Fatalf("expected missing to be KTmodeMode, got %v", missing)
	}
	m := newCfgVals()
	cfgValSet(m, ucv.KTmodeMode, ucv.ETmodeModeDisabled)
	items, missing, survey, err := m.Transaction(target, ver, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems[2]: %v", err)
	}
	// we've disabled this in the code, so disable here too
	if false {
		if !survey {
			t.Errorf("expected survey to be true")
		}
		if len(missing) != 0 {
			t.Fatalf("expected missing to be empty, got %v", missing)
		}
		m = newCfgVals()
		m.AddItems(items)
		expectItem(t, m, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
		expectItem(t, m, ucv.KTmodeSvinMinDur, 2001)
		expectItem(t, m, ucv.KTmodeSvinAccLimit, 10*1000*10)
		items = m.Survey(target.Opts)
	}
	m = newCfgVals()
	m.AddItems(items)
	expectItem(t, m, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
	expectItem(t, m, ucv.KTmodeSvinMinDur, 2000)
	expectItem(t, m, ucv.KTmodeSvinAccLimit, 10*1000*10)
}

func newCfgVals() *CfgVals {
	vals := MakeCfgVals()
	return &vals
}

func cfgValSet[T comparable](vals *CfgVals, k ucv.TypedKey[T], v T) {
	ucv.MapSet(vals.Map, k, v)
}
