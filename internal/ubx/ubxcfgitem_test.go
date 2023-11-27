package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

func TestConfigItems_Sane(t *testing.T) {
	cm := &gpsprot.ConfigMap{}
	cm.SetPPS()
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	opts := gpsprot.ConfigOptions{}
	_, missing, err := configItems(cm, opts, ver, ucv.Map{}, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(missing) == 0 {
		t.Error("expected missing to be non-empty")
	}
	known := ucv.Map{}
	ucv.MapSet(known, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
	_ = testSanity(t, cm, opts, ver, known)
	known = ucv.Map{}
	ucv.MapSet(known, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Utc)
	ucv.MapSet(known, ucv.KNavspgUtcstandard, ucv.ENavspgUtcstandardAuto)
	ucv.MapSet(known, ucv.KSignalGpsEna, false)
	ucv.MapSet(known, ucv.KSignalGloEna, false)
	ucv.MapSet(known, ucv.KSignalGalEna, false)
	ucv.MapSet(known, ucv.KSignalBdsEna, true)
	m := testSanity(t, cm, opts, ver, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Bds)
	gpsprot.CfgPrimaryGNSS.Set(cm, gpsprot.GAL)
	m = testSanity(t, cm, opts, ver, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
}

func testSanity(t *testing.T, cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions, ver *Version, known ucv.Map) ucv.Map {
	items, missing, err := configItems(cm, opts, ver, known, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
	m := ucv.Map{}
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

	if tg, ok := ucv.MapGet(m, ucv.KTpTimegridTp1); ok && tg == ucv.ETpTimegridTp1Utc {
		t.Errorf("expected KTpTimegridTp1 not to be UTC")
	}
	return m
}

func expectItem[T comparable](t *testing.T, m ucv.Map, key ucv.TypedKey[T], val T) {
	got, ok := ucv.MapGet(m, key)
	if !ok {
		t.Errorf("expected db to contain %x", key)
	} else if got != val {
		t.Errorf("expected %x to be %v, got %v", key, val, got)
	}
}

func expectMissing[T comparable](t *testing.T, m ucv.Map, key ucv.TypedKey[T]) {
	_, ok := ucv.MapGet(m, key)
	if ok {
		t.Errorf("expected db not to contain %x", key)
	}
}

func TestConfigItems_GNSS(t *testing.T) {
	cm := &gpsprot.ConfigMap{}
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	opts := gpsprot.ConfigOptions{}
	gpsprot.CfgPrimaryGNSS.Set(cm, gpsprot.GAL)
	gpsprot.CfgGNSSEnabled.Set(cm, gpsprot.GNSSFlag(gpsprot.GAL))
	items, missing, err := configItems(cm, opts, ver, ucv.Map{}, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
	m := ucv.Map{}
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
	cm := &gpsprot.ConfigMap{}
	opts := gpsprot.ConfigOptions{}
	const nanos = 10
	gpsprot.CfgAntennaCableDelay.Set(cm, nanos*time.Nanosecond)
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}

	items, missing, err := configItems(cm, opts, ver, ucv.Map{}, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
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
	cm := &gpsprot.ConfigMap{}
	opts := gpsprot.ConfigOptions{}
	opts.Survey = gpsprot.Survey{
		When:     gpsprot.TimeModeFlags(gpsprot.TimeModeDisabled),
		MinDur:   2000 * time.Second,
		AccLimit: 10 * gpsprot.Meter,
	}
	ver := &Version{
		GNSS: gpsprot.MajorGNSSSet,
		FW:   &FWVer{ProductCategory: "TIM", Major: 8, Minor: 01},
	}
	_, missing, err := configItems(cm, opts, ver, ucv.Map{}, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems[1]: %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("expected missing to be 1, got %v", missing)
	}
	if missing[0] != ucv.KTmodeMode.Key() {
		t.Fatalf("expected missing to be KTmodeMode, got %v", missing)
	}
	m := ucv.Map{}
	ucv.MapSet(m, ucv.KTmodeMode, ucv.ETmodeModeDisabled)
	items, missing, err := configItems(cm, opts, ver, m, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems[2]: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected missing to be empty, got %v", missing)
	}
	m = ucv.Map{}
	m.AddItems(items)
	expectItem(t, m, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
	expectItem(t, m, ucv.KTmodeSvinMinDur, 2000)
}
