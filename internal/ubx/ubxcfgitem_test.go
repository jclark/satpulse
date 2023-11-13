package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsmsg"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

func TestConfigItems_Sane(t *testing.T) {
	config := &gpsmsg.Config{}
	config.SetSane()
	_, missing, err := configItems(config, ucv.Map{}, ucv.UART1)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(missing) == 0 {
		t.Error("expected missing to be non-empty")
	}
	known := ucv.Map{}
	ucv.MapSet(known, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
	_ = testSanity(t, config, known)
	known = ucv.Map{}
	ucv.MapSet(known, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Utc)
	ucv.MapSet(known, ucv.KNavspgUtcstandard, ucv.ENavspgUtcstandardAuto)
	ucv.MapSet(known, ucv.KSignalGpsEna, false)
	ucv.MapSet(known, ucv.KSignalGloEna, false)
	ucv.MapSet(known, ucv.KSignalGalEna, false)
	ucv.MapSet(known, ucv.KSignalBdsEna, true)
	m := testSanity(t, config, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Bds)
	gpsmsg.CfgPrimaryGNSS.Set(config, gpsmsg.Galileo)
	m = testSanity(t, config, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
}

func testSanity(t *testing.T, config *gpsmsg.Config, known ucv.Map) ucv.Map {
	items, missing, err := configItems(config, known, ucv.UART1)
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
		t.Errorf("expected db to contain %v", key)
	} else if got != val {
		t.Errorf("expected %x to be %v, got %v", key, val, got)
	}
}

func TestConfigItems_AntennaCableDelay(t *testing.T) {
	config := &gpsmsg.Config{}
	const nanos = 10
	gpsmsg.CfgAntennaCableDelay.Set(config, nanos*time.Nanosecond)

	items, missing, err := configItems(config, ucv.Map{}, ucv.UART1)
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
