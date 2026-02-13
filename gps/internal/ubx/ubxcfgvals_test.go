package ubx

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

func TestConfigItems_Sane(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	_, missing, err := newCfgVals().Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("configItems: %v", err)
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
	target.Props.SetTimeGNSS(gpsprot.GAL)
	m = testSanity(t, target, ver, known)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
}

func testSanity(t *testing.T, target *gpsprot.ConfigTarget, ver *Version, known *CfgVals) *CfgVals {
	items, missing, err := known.Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("configItems: %v", err)
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
	target := gpsprot.NewConfigTarget()
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	items, missing, err := newCfgVals().Transaction(target, ver, ucv.UART1, 0)

	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected items to be empty, got %v", items)
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
}

func TestConfigItems_Get(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Get = gpsprot.PropIDTimePulseWidth
	vals := newCfgVals()
	ver := &Version{}
	items, missing, err := vals.Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected items to be empty, got %v", items)
	}
	if len(missing) == 0 {
		t.Errorf("expected missing to be non-empty, but empty")
	}
	// Verify addGetKeys requests all keys needed for time pulse cooking
	missingSet := make(map[ucv.Key]bool, len(missing))
	for _, k := range missing {
		missingSet[k] = true
		vals.Map[k] = 0
	}
	for _, tk := range cfgValKeysByProp[gpsprot.PropIDTimePulse] {
		if !missingSet[tk.Key()] {
			t.Errorf("addGetKeys did not request key %v", tk.Key())
		}
	}
	cfgValsInit(vals)
	cp := new(gpsprot.ConfigProps)
	vals.Cook(ver, ucv.UART1, cp)
	val, ok := cp.GetTimePulseWidth()
	if !ok || val != time.Duration(1e5*time.Microsecond) {
		t.Errorf("expected pulse width to be 0.1s, got %v", val)
	}
}

func TestConfigItems_GNSS(t *testing.T) {
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	target := gpsprot.NewConfigTarget()
	target.Props.SetTimeGNSS(gpsprot.GAL)
	items, missing, err := newCfgVals().Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("configItems: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("expected missing to be empty, got %v", missing)
	}
	m := newCfgVals()
	m.AddItems(items)
	expectItem(t, m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gal)
	expectItem(t, m, ucv.KNavspgUtcstandard, ucv.ENavspgUtcstandardEu)
	expectItem(t, m, ucv.KRateTimeref, ucv.ERateTimerefGal)
}

func TestConfigItems_AntennaCableDelay(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	const nanos = 10
	target.Props.SetAntennaCableDelay(nanos * time.Nanosecond)
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}

	items, missing, err := newCfgVals().Transaction(target, ver, ucv.UART1, 0)
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
	target := gpsprot.NewConfigTarget()
	target.Opts.Survey = gpsprot.Survey{
		MinDur:   2000 * time.Second,
		AccLimit: 10 * gpsprot.Meter,
	}
	target.Opts.SetStatic = true
	ver := &Version{
		GNSS: gpsprot.MajorGNSSSet,
		FW:   &FWVer{ProductCategory: "TIM", Major: 8, Minor: 01},
	}
	_, missing, err := newCfgVals().Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("configItems[1]: %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("expected missing to be 1, got %v", missing)
	}
	if missing[0] != ucv.KTmodeMode.Key() {
		t.Fatalf("expected missing to be KTmodeMode, got %v", missing)
	}
	m := newCfgVals()
	cfgValSet(m, ucv.KTmodeMode, ucv.ETmodeModeDisabled)
	items, _, err := m.Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("configItems[2]: %v", err)
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

func cfgValsInit(m *CfgVals) {
	cfgValSet(m, ucv.KTpPulseDef, ucv.ETpPulseDefPeriod)
	cfgValSet(m, ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength)
	cfgValSet(m, ucv.KTpAntCabledelay, 50)
	cfgValSet(m, ucv.KTpPeriodTp1, 1e6)
	cfgValSet(m, ucv.KTpPeriodLockTp1, 1e6)
	cfgValSet(m, ucv.KTpLenTp1, 0)
	cfgValSet(m, ucv.KTpLenLockTp1, 1e5)
	cfgValSet(m, ucv.KTpAlignToTowTp1, true)
	cfgValSet(m, ucv.KTpTimegridTp1, ucv.ETpTimegridTp1Gps)
	cfgValSet(m, ucv.KTpSyncGnssTp1, true)
	cfgValSet(m, ucv.KTpUseLockedTp1, true)
	cfgValSet(m, ucv.KTpTp1Ena, true)
	cfgValSet(m, ucv.KTpPolTp1, true)
	cfgValSet(m, ucv.KTpFreqTp1, 1)
	cfgValSet(m, ucv.KTpFreqLockTp1, 1)
	cfgValSet(m, ucv.KTpDutyTp1, 0.0)
	cfgValSet(m, ucv.KTpDutyLockTp1, 0.0)
	cfgValSet(m, ucv.KRateMeas, 1e3)
	cfgValSet(m, ucv.KRateNav, 1)
	cfgValSet(m, ucv.KRateTimeref, ucv.ERateTimerefGps)
	cfgValSet(m, ucv.KNavspgUtcstandard, ucv.ENavspgUtcstandardAuto)
	cfgValSet(m, ucv.KNavspgDynmodel, ucv.ENavspgDynmodelPort)
	cfgValSet(m, ucv.KSignalGpsEna, true)
	cfgValSet(m, ucv.KSignalGloEna, false)
	cfgValSet(m, ucv.KSignalGalEna, true)
	cfgValSet(m, ucv.KSignalBdsEna, true)
}

func (r *gpsReceiver) valgetResp(pkt []byte) ubxbin.Msg {
	if r.nakPollMsgID == ubxbin.CfgValgetID {
		return nil
	}
	msg, err := ubxbin.ParseMsg(string(pkt))
	if err != nil {
		panic(err)
	}
	vg := msg.(*ubxbin.CfgValget)
	if vg.Version != ubxbin.CfgValgetVersionRequest {
		panic("unexpected version")
	}
	keys, err := ucv.UnmarshalKeys(vg.CfgData)
	if err != nil {
		panic(err)
	}
	items := make([]ucv.Item, len(keys))
	for i, k := range keys {
		val, ok := r.raw.valsPtr().Map[k]
		if !ok {
			return nil
		}
		items[i] = ucv.Item{Key: k, Value: val}
	}
	data, err := ucv.MarshalItems(items)
	if err != nil {
		panic(err)
	}
	return &ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{
			Layer:   vg.Layer,
			Version: ubxbin.CfgValgetVersionResponse,
		},
		CfgData: data,
	}
}

func TestEnableSignals(t *testing.T) {
	tests := []struct {
		supported []ucv.KeyL
		enabled   gpsprot.SignalSet
		expected  map[ucv.KeyL]bool
	}{
		{
			supported: []ucv.KeyL{
				ucv.KSignalGpsEna,
				ucv.KSignalGpsL1caEna,
				ucv.KSignalGloEna,
				ucv.KSignalGloL1Ena,
			},
			enabled: (gpsprot.BandL1 | gpsprot.BandL2).SignalSet(gpsprot.GPS, gpsprot.GAL),
			expected: map[ucv.KeyL]bool{
				ucv.KSignalGpsEna:     true,
				ucv.KSignalGpsL1caEna: true,
				ucv.KSignalGloEna:     false,
			},
		},
		{
			supported: []ucv.KeyL{
				ucv.KSignalGpsEna,
				ucv.KSignalGpsL1caEna,
				ucv.KSignalGpsL5Ena,
				ucv.KSignalGalEna,
				ucv.KSignalGalE1Ena,
				ucv.KSignalGalE5aEna,
				ucv.KSignalBdsB1Ena,
			},
			enabled: (gpsprot.BandL1 | gpsprot.BandL2 | gpsprot.BandL5).SignalSet(gpsprot.GPS, gpsprot.GLO, gpsprot.GAL),
			expected: map[ucv.KeyL]bool{
				ucv.KSignalGpsEna:     true,
				ucv.KSignalGpsL1caEna: true,
				ucv.KSignalGpsL5Ena:   true,
				ucv.KSignalGalEna:     true,
				ucv.KSignalGalE1Ena:   true,
				ucv.KSignalGalE5aEna:  true,
				ucv.KSignalBdsEna:     false,
			},
		},
		{
			supported: []ucv.KeyL{
				ucv.KSignalBdsB1Ena,
				ucv.KSignalBdsB2Ena,
				ucv.KSignalBdsB1cEna,
				ucv.KSignalBdsB2aEna,
			},
			enabled: (gpsprot.BandL1 | gpsprot.BandL5).SignalSet(gpsprot.BDS),
			expected: map[ucv.KeyL]bool{
				ucv.KSignalBdsEna:    true,
				ucv.KSignalBdsB1Ena:  true,
				ucv.KSignalBdsB2Ena:  false,
				ucv.KSignalBdsB1cEna: true,
				ucv.KSignalBdsB2aEna: true,
			},
		},
		{
			supported: []ucv.KeyL{
				ucv.KSignalGpsEna,
				ucv.KSignalGpsL1caEna,
				ucv.KSignalGpsL5Ena,
				ucv.KSignalGalEna,
				ucv.KSignalGalE1Ena,
				ucv.KSignalGalE5aEna,
				ucv.KSignalBdsB1Ena,
			},
			enabled: gpsprot.SignalSetOf(gpsprot.SigGALE1),
			expected: map[ucv.KeyL]bool{
				ucv.KSignalGpsEna:    false,
				ucv.KSignalGalEna:    true,
				ucv.KSignalGalE1Ena:  true,
				ucv.KSignalGalE5aEna: false,
				ucv.KSignalBdsEna:    false,
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			m := newCfgVals()
			for _, k := range tt.supported {
				cfgValSet(m, k, true)
			}
			ver := Version{GNSS: gpsprot.MajorGNSSSet}
			supported := m.signalsSupported(&ver)
			_, items := m.EnableSignals(tt.enabled, &ver)
			expectedItems := make([]ucv.Item, 0, len(tt.expected))
			for k, v := range tt.expected {
				ucv.AddItem(&expectedItems, k, v)
			}
			ucv.SortItems(items)
			ucv.SortItems(expectedItems)
			if !slices.Equal(items, expectedItems) {
				t.Errorf("expected items to be %v, got %v", expectedItems, items)
			}
			m = newCfgVals()
			for _, it := range items {
				m.Map[it.Key] = it.Value
			}
			result, _ := m.getSignalsEnabled()
			if tt.enabled&supported != result {
				t.Errorf("expected signals to be %v, got %v", tt.enabled&supported, result)
			}
		})
	}
}

// Test timeModeBuild function - case where missing keys need to be fetched
func TestTimeModeBuild_MissingKeys(t *testing.T) {
	tests := []struct {
		name      string
		target    *gpsprot.ConfigTarget
		known     *CfgVals
		expectErr bool
		wantKeys  []ucv.Key
	}{
		{
			name: "survey mode without resurvey needs no keys",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				return target
			}(),
			known:     newCfgVals(),
			expectErr: false,
			wantKeys:  nil,
		},
		{
			name: "non-supported product category",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				return target
			}(),
			known:     newCfgVals(),
			expectErr: false,
			wantKeys:  nil,
		},
		{
			name: "survey again needs survey parameters",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{
					Flags: gpsprot.SurveyAgain,
				}
				return target
			}(),
			known:     newCfgVals(),
			expectErr: false,
			wantKeys:  []ucv.Key{ucv.KTmodeSvinMinDur.Key(), ucv.KTmodeSvinAccLimit.Key()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &txnBuilder{
				target: tt.target,
				known:  tt.known,
			}

			err := tb.timeModeBuild()
			if (err != nil) != tt.expectErr {
				t.Errorf("timeModeBuild() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if len(tt.wantKeys) == 0 {
				if len(tb.keys) != 0 {
					t.Errorf("expected no keys, got %v", tb.keys)
				}
			} else {
				if len(tb.keys) != len(tt.wantKeys) {
					t.Errorf("expected %d keys, got %d", len(tt.wantKeys), len(tb.keys))
				}
				for _, wantKey := range tt.wantKeys {
					if !slices.Contains(tb.keys, wantKey) {
						t.Errorf("expected key %v not found in %v", wantKey, tb.keys)
					}
				}
			}
		})
	}
}

// Test timeModeBuild function - case where configuration items are generated
func TestTimeModeBuild_GenerateItems(t *testing.T) {
	tests := []struct {
		name       string
		target     *gpsprot.ConfigTarget
		known      *CfgVals
		expectErr  bool
		wantItems  []ucv.Item
		wantSurvey bool
	}{
		{
			name: "disable mode",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: false})
				return target
			}(),
			known: func() *CfgVals {
				cv := newCfgVals()
				cfgValSet(cv, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
				return cv
			}(),
			expectErr: false,
			wantItems: []ucv.Item{
				ucv.MakeItem(ucv.KTmodeMode, ucv.ETmodeModeDisabled),
			},
			wantSurvey: false,
		},
		{
			name: "enable survey mode",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{
					MinDur:   300 * time.Second,
					AccLimit: 5 * gpsprot.Meter,
				}
				return target
			}(),
			known: func() *CfgVals {
				cv := newCfgVals()
				cfgValSet(cv, ucv.KTmodeMode, ucv.ETmodeModeDisabled)
				return cv
			}(),
			expectErr: false,
			wantItems: []ucv.Item{
				ucv.MakeItem(ucv.KTmodeMode, ucv.ETmodeModeSurveyIn),
				ucv.MakeItem(ucv.KTmodeSvinMinDur, 300),
				ucv.MakeItem(ucv.KTmodeSvinAccLimit, 5*1000*10),
			},
			wantSurvey: true,
		},
		{
			name: "force resurvey by changing parameters",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{
					MinDur:   300 * time.Second,
					AccLimit: 5 * gpsprot.Meter,
					Flags:    gpsprot.SurveyAgain,
				}
				return target
			}(),
			known: func() *CfgVals {
				cv := newCfgVals()
				cfgValSet(cv, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
				cfgValSet(cv, ucv.KTmodeSvinMinDur, 300)
				cfgValSet(cv, ucv.KTmodeSvinAccLimit, 5*1000*10)
				return cv
			}(),
			expectErr: false,
			wantItems: []ucv.Item{
				ucv.MakeItem(ucv.KTmodeMode, ucv.ETmodeModeSurveyIn),
				ucv.MakeItem(ucv.KTmodeSvinMinDur, 300),
				ucv.MakeItem(ucv.KTmodeSvinAccLimit, 5*1000*10+1),
			},
			wantSurvey: true,
		},
		{
			name: "existing survey mode with same parameters",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{Static: true})
				target.Opts.Survey = gpsprot.Survey{
					MinDur:   300 * time.Second,
					AccLimit: 5 * gpsprot.Meter,
				}
				return target
			}(),
			known: func() *CfgVals {
				cv := newCfgVals()
				cfgValSet(cv, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
				cfgValSet(cv, ucv.KTmodeSvinMinDur, 300)
				cfgValSet(cv, ucv.KTmodeSvinAccLimit, 5*1000*10)
				return cv
			}(),
			expectErr: false,
			wantItems: []ucv.Item{
				ucv.MakeItem(ucv.KTmodeMode, ucv.ETmodeModeSurveyIn),
				ucv.MakeItem(ucv.KTmodeSvinMinDur, 300),
				ucv.MakeItem(ucv.KTmodeSvinAccLimit, 5*1000*10),
			},
			wantSurvey: true,
		},
		{
			name: "fixed position ECEF mode",
			target: func() *gpsprot.ConfigTarget {
				target := gpsprot.NewConfigTarget()
				target.Props.SetMode(gpsprot.Mode{
					Static:  true,
					PosType: gpsprot.PosTypeECEF,
					FixedPosECEF: [3]gpsprot.Length{
						4194304 * gpsprot.Meter, // X: 4194304m
						837860 * gpsprot.Meter,  // Y: 837860m
						4581200 * gpsprot.Meter, // Z: 4581200m
					},
					FixedPosAcc: 10 * gpsprot.Millimeter, // 1cm accuracy
				})
				return target
			}(),
			known:     newCfgVals(),
			expectErr: false,
			wantItems: []ucv.Item{
				ucv.MakeItem(ucv.KTmodeMode, ucv.ETmodeModeFixed),
				ucv.MakeItem(ucv.KTmodePosType, ucv.ETmodePosTypeEcef),
				ucv.MakeItem(ucv.KTmodeEcefX, int64(419430400)),  // 4194304m in cm
				ucv.MakeItem(ucv.KTmodeEcefY, int64(83786000)),   // 837860m in cm
				ucv.MakeItem(ucv.KTmodeEcefZ, int64(458120000)),  // 4581200m in cm
				ucv.MakeItem(ucv.KTmodeEcefXHp, int64(0)),        // No fractional part
				ucv.MakeItem(ucv.KTmodeEcefYHp, int64(0)),        // No fractional part
				ucv.MakeItem(ucv.KTmodeEcefZHp, int64(0)),        // No fractional part
				ucv.MakeItem(ucv.KTmodeFixedPosAcc, uint64(100)), // 10mm in 0.1mm units
			},
			wantSurvey: false,
		},
		{
			name:   "no mode specified, no changes needed",
			target: gpsprot.NewConfigTarget(),
			known: func() *CfgVals {
				cv := newCfgVals()
				cfgValSet(cv, ucv.KTmodeMode, ucv.ETmodeModeSurveyIn)
				return cv
			}(),
			expectErr:  false,
			wantItems:  nil,
			wantSurvey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &txnBuilder{
				target: tt.target,
				known:  tt.known,
			}

			err := tb.timeModeBuild()
			if (err != nil) != tt.expectErr {
				t.Errorf("timeModeBuild() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if len(tt.wantItems) == 0 {
				if len(tb.items) != 0 {
					t.Errorf("expected no items, got %v", tb.items)
				}
			} else {
				if len(tb.items) != len(tt.wantItems) {
					t.Errorf("expected %d items, got %d: %v", len(tt.wantItems), len(tb.items), tb.items)
				}
				for i, wantItem := range tt.wantItems {
					if i >= len(tb.items) {
						t.Errorf("missing item %d: %v", i, wantItem)
						continue
					}
					gotItem := tb.items[i]
					if gotItem.Key != wantItem.Key || gotItem.Value != wantItem.Value {
						t.Errorf("item %d: expected %v, got %v", i, wantItem, gotItem)
					}
				}
			}

			if tb.survey != tt.wantSurvey {
				t.Errorf("expected survey flag %v, got %v", tt.wantSurvey, tb.survey)
			}
		})
	}
}

func TestTimePulseRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		tp   gpsprot.TimePulse
	}{
		{
			name: "not aligned to GNSS",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "aligned to GNSS",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "only when locked",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: true,
				PolarityRising: true,
			},
		},
		{
			name: "disabled pulse",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          0,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := &Version{GNSS: gpsprot.MajorGNSSSet}
			target := gpsprot.NewConfigTarget()
			target.Props.SetTimePulse(tt.tp)
			// Build: convert abstract model to cfgvals items via Transaction.
			// Transaction may need two passes: the first returns missing keys,
			// the second uses those keys to produce items.
			known := newCfgVals()
			items, missing, err := known.Transaction(target, ver, ucv.UART1, 0)
			if err != nil {
				t.Fatalf("Transaction[1]: %v", err)
			}
			// Populate missing keys with defaults
			for _, k := range missing {
				known.Map[k] = 0
			}
			if len(missing) > 0 {
				items, missing, err = known.Transaction(target, ver, ucv.UART1, 0)
				if err != nil {
					t.Fatalf("Transaction[2]: %v", err)
				}
				if len(missing) != 0 {
					t.Fatalf("unexpected missing keys on second pass: %v", missing)
				}
			}
			// Apply items to a fresh CfgVals and Cook back
			cv := newCfgVals()
			cv.AddItems(items)
			cp := new(gpsprot.ConfigProps)
			cv.Cook(ver, ucv.UART1, cp)
			got, ok := cp.GetTimePulse()
			if !ok {
				t.Fatal("expected all TimePulse properties to be set")
			}
			if got != tt.tp {
				t.Errorf("round trip mismatch:\n  want %+v\n  got  %+v", tt.tp, got)
			}
		})
	}
}

func TestTimePulseCook(t *testing.T) {
	tests := []struct {
		name  string
		items []ucv.Item
		want  gpsprot.TimePulse
	}{
		{
			name: "not enabled means width zero",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, false),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpPeriodTp1, 1e6),
				ucv.MakeItem(ucv.KTpLenTp1, 1e5),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, false),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, false),
				ucv.MakeItem(ucv.KTpUseLockedTp1, false),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          0,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "USE_LOCKED false reads unlocked keys",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpPeriodTp1, 1e6),
				ucv.MakeItem(ucv.KTpLenTp1, 1e5),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, false),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, false),
				ucv.MakeItem(ucv.KTpUseLockedTp1, false),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "frequency mode",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefFreq),
				ucv.MakeItem(ucv.KTpFreqTp1, 10),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpLenTp1, 50000),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, false),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, false),
				ucv.MakeItem(ucv.KTpUseLockedTp1, false),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         100 * time.Millisecond,
				Width:          50 * time.Millisecond,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "duty cycle mode",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPeriodTp1, 1e6),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefRatio),
				ucv.MakeItem(ucv.KTpDutyTp1, 10.0),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, false),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, false),
				ucv.MakeItem(ucv.KTpUseLockedTp1, false),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "USE_LOCKED true reads locked keys",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpPeriodTp1, 500000),
				ucv.MakeItem(ucv.KTpPeriodLockTp1, 1e6),
				ucv.MakeItem(ucv.KTpLenTp1, 50000),
				ucv.MakeItem(ucv.KTpLenLockTp1, 1e5),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, true),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, true),
				ucv.MakeItem(ucv.KTpUseLockedTp1, true),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "only when locked",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpPeriodLockTp1, 1e6),
				ucv.MakeItem(ucv.KTpLenLockTp1, 1e5),
				ucv.MakeItem(ucv.KTpLenTp1, 0),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, true),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, true),
				ucv.MakeItem(ucv.KTpUseLockedTp1, true),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: true,
				PolarityRising: true,
			},
		},
		{
			name: "frequency mode locked",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefFreq),
				ucv.MakeItem(ucv.KTpFreqTp1, 5),
				ucv.MakeItem(ucv.KTpFreqLockTp1, 10),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpLenLockTp1, 50000),
				ucv.MakeItem(ucv.KTpLenTp1, 50000),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, true),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, true),
				ucv.MakeItem(ucv.KTpUseLockedTp1, true),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         100 * time.Millisecond,
				Width:          50 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "duty cycle mode locked",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPeriodLockTp1, 1e6),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefRatio),
				ucv.MakeItem(ucv.KTpDutyTp1, 10.0),
				ucv.MakeItem(ucv.KTpDutyLockTp1, 10.0),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, true),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, true),
				ucv.MakeItem(ucv.KTpUseLockedTp1, true),
				ucv.MakeItem(ucv.KTpPolTp1, true),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "polarity falling",
			items: []ucv.Item{
				ucv.MakeItem(ucv.KTpTp1Ena, true),
				ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
				ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
				ucv.MakeItem(ucv.KTpPeriodTp1, 1e6),
				ucv.MakeItem(ucv.KTpLenTp1, 1e5),
				ucv.MakeItem(ucv.KTpAlignToTowTp1, false),
				ucv.MakeItem(ucv.KTpSyncGnssTp1, false),
				ucv.MakeItem(ucv.KTpUseLockedTp1, false),
				ucv.MakeItem(ucv.KTpPolTp1, false),
			},
			want: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := &Version{GNSS: gpsprot.MajorGNSSSet}
			cv := newCfgVals()
			cv.AddItems(tt.items)
			cp := new(gpsprot.ConfigProps)
			cv.Cook(ver, ucv.UART1, cp)
			got, ok := cp.GetTimePulse()
			if !ok {
				t.Fatal("expected all TimePulse properties to be set")
			}
			if got != tt.want {
				t.Errorf("Cook mismatch:\n  want %+v\n  got  %+v", tt.want, got)
			}
		})
	}
}

// TestTimePulseCookMissing verifies that getTimePulse returns false
// when required keys are absent.
func TestTimePulseCookMissing(t *testing.T) {
	// A complete set of items that produces a valid TimePulse.
	full := []ucv.Item{
		ucv.MakeItem(ucv.KTpTp1Ena, true),
		ucv.MakeItem(ucv.KTpPulseDef, ucv.ETpPulseDefPeriod),
		ucv.MakeItem(ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength),
		ucv.MakeItem(ucv.KTpPeriodTp1, 1e6),
		ucv.MakeItem(ucv.KTpLenTp1, 1e5),
		ucv.MakeItem(ucv.KTpAlignToTowTp1, false),
		ucv.MakeItem(ucv.KTpSyncGnssTp1, false),
		ucv.MakeItem(ucv.KTpUseLockedTp1, false),
		ucv.MakeItem(ucv.KTpPolTp1, true),
	}
	// Sanity: the full set should succeed.
	ver := &Version{GNSS: gpsprot.MajorGNSSSet}
	cv := newCfgVals()
	cv.AddItems(full)
	cp := new(gpsprot.ConfigProps)
	cv.Cook(ver, ucv.UART1, cp)
	if _, ok := cp.GetTimePulse(); !ok {
		t.Fatal("full set should produce a valid TimePulse")
	}
	// Each required key, when removed, should cause GetTimePulse to fail.
	required := []ucv.Key{
		ucv.KTpUseLockedTp1.Key(),
		ucv.KTpPulseDef.Key(),
		ucv.KTpPeriodTp1.Key(),
		ucv.KTpTp1Ena.Key(),
		ucv.KTpPulseLengthDef.Key(),
		ucv.KTpLenTp1.Key(),
		ucv.KTpPolTp1.Key(),
	}
	for _, drop := range required {
		t.Run(drop.String(), func(t *testing.T) {
			cv := newCfgVals()
			for _, item := range full {
				if item.Key != drop {
					cv.Map[item.Key] = item.Value
				}
			}
			cp := new(gpsprot.ConfigProps)
			cv.Cook(ver, ucv.UART1, cp)
			if _, ok := cp.GetTimePulse(); ok {
				t.Errorf("expected GetTimePulse to fail without %v", drop)
			}
		})
	}
}

// TestTP2RoundTrip verifies that Transaction produces TP2 keys for a ZED-X20P
// and that Cook can read them back correctly.
func TestTP2RoundTrip(t *testing.T) {
	ver := &testVers.x20p
	tp1to2 := ucv.KeyRemap(ucv.TPKeyPairs, 0, 1)
	tests := []struct {
		name string
		tp   gpsprot.TimePulse
	}{
		{
			name: "aligned to GNSS",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
		{
			name: "only when locked",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          100 * time.Millisecond,
				AlignToGNSS:    true,
				OnlyWhenLocked: true,
				PolarityRising: true,
			},
		},
		{
			name: "disabled pulse",
			tp: gpsprot.TimePulse{
				Period:         1 * time.Second,
				Width:          0,
				AlignToGNSS:    false,
				OnlyWhenLocked: false,
				PolarityRising: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := gpsprot.NewConfigTarget()
			target.Props.SetTimePulse(tt.tp)
			known := newCfgVals()
			items, missing, err := known.Transaction(target, ver, ucv.UART1, 0)
			if err != nil {
				t.Fatalf("Transaction[1]: %v", err)
			}
			// Populate missing keys with defaults (using TP2 keys)
			for _, k := range missing {
				known.Map[k] = 0
			}
			if len(missing) > 0 {
				items, missing, err = known.Transaction(target, ver, ucv.UART1, 0)
				if err != nil {
					t.Fatalf("Transaction[2]: %v", err)
				}
				if len(missing) != 0 {
					t.Fatalf("unexpected missing keys on second pass: %v", missing)
				}
			}
			// Verify items use TP2 keys, not TP1
			for _, item := range items {
				if _, isTP1 := tp1to2[item.Key]; isTP1 {
					t.Errorf("Transaction returned TP1 key %v, expected TP2", item.Key)
				}
			}
			// Cook the TP2 items back
			cv := newCfgVals()
			cv.AddItems(items)
			cp := new(gpsprot.ConfigProps)
			cv.Cook(ver, ucv.UART1, cp)
			got, ok := cp.GetTimePulse()
			if !ok {
				t.Fatal("expected all TimePulse properties to be set")
			}
			if got != tt.tp {
				t.Errorf("round trip mismatch:\n  want %+v\n  got  %+v", tt.tp, got)
			}
		})
	}
}

// TestTP2Cook verifies that Cook correctly reads TP2 keys for a ZED-X20P.
func TestTP2Cook(t *testing.T) {
	ver := &testVers.x20p
	cv := newCfgVals()
	cfgValSet(cv, ucv.KTpTp2Ena, true)
	cfgValSet(cv, ucv.KTpPulseDef, ucv.ETpPulseDefPeriod)
	cfgValSet(cv, ucv.KTpPulseLengthDef, ucv.ETpPulseLengthDefLength)
	cfgValSet(cv, ucv.KTpPeriodLockTp2, 1e6)
	cfgValSet(cv, ucv.KTpLenLockTp2, 1e5)
	cfgValSet(cv, ucv.KTpLenTp2, 0)
	cfgValSet(cv, ucv.KTpAlignToTowTp2, true)
	cfgValSet(cv, ucv.KTpSyncGnssTp2, true)
	cfgValSet(cv, ucv.KTpUseLockedTp2, true)
	cfgValSet(cv, ucv.KTpPolTp2, true)
	cp := new(gpsprot.ConfigProps)
	cv.Cook(ver, ucv.UART1, cp)
	got, ok := cp.GetTimePulse()
	if !ok {
		t.Fatal("expected all TimePulse properties to be set")
	}
	want := gpsprot.TimePulse{
		Period:         1 * time.Second,
		Width:          100 * time.Millisecond,
		AlignToGNSS:    true,
		OnlyWhenLocked: true,
		PolarityRising: true,
	}
	if got != want {
		t.Errorf("Cook mismatch:\n  want %+v\n  got  %+v", want, got)
	}
}

// TestTP2TransactionMissingKeys verifies that Transaction returns TP2 keys
// (not TP1) when it needs to fetch additional configuration.
func TestTP2TransactionMissingKeys(t *testing.T) {
	ver := &testVers.x20p
	tp1to2 := ucv.KeyRemap(ucv.TPKeyPairs, 0, 1)
	target := gpsprot.NewConfigTarget()
	target.Get = gpsprot.PropIDTimePulse
	_, missing, err := newCfgVals().Transaction(target, ver, ucv.UART1, 0)
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if len(missing) == 0 {
		t.Fatal("expected missing keys")
	}
	for _, k := range missing {
		if _, isTP1 := tp1to2[k]; isTP1 {
			t.Errorf("missing key %v is TP1, expected TP2", k)
		}
	}
}

// Test dynModelBuild function - SPG receiver dynamic model handling
func TestDynModelBuild(t *testing.T) {
	expectStat := ucv.ENavspgDynmodelStat
	expectPort := ucv.ENavspgDynmodelPort

	tests := []struct {
		name           string
		mode           opt.Val[gpsprot.Mode]
		setStatic      bool
		expectDynModel *ucv.EnumNavspgDynmodel // nil means no item expected
	}{
		{
			name:           "SetStatic true - should set stationary",
			setStatic:      true,
			expectDynModel: &expectStat,
		},
		{
			name: "Mode.Static true - should set stationary",
			mode: func() opt.Val[gpsprot.Mode] {
				var m opt.Val[gpsprot.Mode]
				m.Set(gpsprot.Mode{Static: true})
				return m
			}(),
			expectDynModel: &expectStat,
		},
		{
			name: "Mode.Static false - should set portable",
			mode: func() opt.Val[gpsprot.Mode] {
				var m opt.Val[gpsprot.Mode]
				m.Set(gpsprot.Mode{Static: false})
				return m
			}(),
			expectDynModel: &expectPort,
		},
		{
			name:           "no Mode, no SetStatic - should do nothing",
			expectDynModel: nil,
		},
		{
			name: "SetStatic true overrides Mode.Static false",
			mode: func() opt.Val[gpsprot.Mode] {
				var m opt.Val[gpsprot.Mode]
				m.Set(gpsprot.Mode{Static: false})
				return m
			}(),
			setStatic:      true,
			expectDynModel: &expectStat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := gpsprot.NewConfigTarget()
			target.Opts.SetStatic = tt.setStatic
			if tt.mode.IsSet() {
				target.Props.SetMode(tt.mode.Get())
			}

			tb := &txnBuilder{
				target: target,
				known:  newCfgVals(),
			}

			tb.dynModelBuild()

			if tt.expectDynModel == nil {
				if len(tb.items) != 0 {
					t.Errorf("expected no items, got %v", tb.items)
				}
			} else {
				if len(tb.items) != 1 {
					t.Errorf("expected 1 item, got %d: %v", len(tb.items), tb.items)
					return
				}
				item := tb.items[0]
				if item.Key != ucv.KNavspgDynmodel.Key() {
					t.Errorf("expected KNavspgDynmodel key, got %v", item.Key)
				}
				if ucv.EnumNavspgDynmodel(item.Value) != *tt.expectDynModel {
					t.Errorf("expected dynmodel %v, got %v", *tt.expectDynModel, item.Value)
				}
			}
		})
	}
}
