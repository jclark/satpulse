package ubxsim

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

// testDefaults is a small key inventory: two RATE keys (group 0x21) and
// three MSGOUT keys (group 0x91).
func testDefaults() ucv.Map {
	return ucv.Map{
		ucv.KRateMeas.Key():                  1000,
		ucv.KRateNav.Key():                   1,
		ucv.KUbxNavSat.KeyU(ucv.UART1).Key(): 0,
		ucv.KUbxRxmCor.KeyU(ucv.UART1).Key(): 0,
		ucv.KNmeaIdGga.KeyU(ucv.UART1).Key(): 1,
	}
}

func valgetPoll(layer ubxbin.CfgValgetLayer, position uint16, keys ...ucv.Key) *ubxbin.CfgValget {
	return &ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{Layer: layer, Position: position},
		CfgData:        ucv.MarshalKeys(keys),
	}
}

func TestValget(t *testing.T) {
	navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
	gga := ucv.KNmeaIdGga.KeyU(ucv.UART1).Key()
	tests := []struct {
		name        string
		poll        *ubxbin.CfgValget
		expectNak   bool
		expectItems []ucv.Item
	}{
		{
			name:        "single key RAM",
			poll:        valgetPoll(ubxbin.CfgValgetLayerRAM, 0, ucv.KRateMeas.Key()),
			expectItems: []ucv.Item{{Key: ucv.KRateMeas.Key(), Value: 1000}},
		},
		{
			name:      "unknown key",
			poll:      valgetPoll(ubxbin.CfgValgetLayerRAM, 0, ucv.KUbxTimSvin.KeyU(ucv.UART1).Key()),
			expectNak: true,
		},
		{
			name:      "invalid layer",
			poll:      valgetPoll(3, 0, ucv.KRateMeas.Key()),
			expectNak: true,
		},
		{
			name: "group wildcard",
			poll: valgetPoll(ubxbin.CfgValgetLayerRAM, 0, ucv.Key(0x2091ffff)),
			expectItems: []ucv.Item{
				{Key: ucv.KUbxNavSat.KeyU(ucv.UART1).Key(), Value: 0},
				{Key: ucv.KNmeaIdGga.KeyU(ucv.UART1).Key(), Value: 1},
				{Key: ucv.KUbxRxmCor.KeyU(ucv.UART1).Key(), Value: 0},
			},
		},
		{
			name:        "wildcard of empty group",
			poll:        valgetPoll(ubxbin.CfgValgetLayerRAM, 0, ucv.Key(0x2092ffff)),
			expectItems: []ucv.Item{},
		},
		{
			// Mirrors the recorded signals poll of a real ZED-F9P
			// (gps/testdata/config/u-blox/ZED-F9P/gpshwtest001/019.jsonl):
			// a populated group wildcard plus one matching no keys is
			// ACKed with exactly the populated group's items.
			name: "populated plus empty group wildcards",
			poll: valgetPoll(ubxbin.CfgValgetLayerRAM, 0, ucv.Key(0x2091ffff), ucv.Key(0x2092ffff)),
			expectItems: []ucv.Item{
				{Key: ucv.KUbxNavSat.KeyU(ucv.UART1).Key(), Value: 0},
				{Key: ucv.KNmeaIdGga.KeyU(ucv.UART1).Key(), Value: 1},
				{Key: ucv.KUbxRxmCor.KeyU(ucv.UART1).Key(), Value: 0},
			},
		},
		{
			name: "all groups wildcard default layer",
			poll: valgetPoll(ubxbin.CfgValgetLayerDefault, 0, ucv.Key(0x0fffffff)),
			expectItems: []ucv.Item{
				{Key: navSat, Value: 0},
				{Key: gga, Value: 1},
				{Key: ucv.KUbxRxmCor.KeyU(ucv.UART1).Key(), Value: 0},
				{Key: ucv.KRateMeas.Key(), Value: 1000},
				{Key: ucv.KRateNav.Key(), Value: 1},
			},
		},
		{
			name:        "position pagination",
			poll:        valgetPoll(ubxbin.CfgValgetLayerDefault, 4, ucv.Key(0x0fffffff)),
			expectItems: []ucv.Item{{Key: ucv.KRateNav.Key(), Value: 1}},
		},
		{
			name:        "position past the end",
			poll:        valgetPoll(ubxbin.CfgValgetLayerDefault, 100, ucv.Key(0x0fffffff)),
			expectItems: []ucv.Item{},
		},
		{
			name:        "BBR of unstored key is empty",
			poll:        valgetPoll(ubxbin.CfgValgetLayerBBR, 0, ucv.KRateMeas.Key()),
			expectItems: []ucv.Item{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newCfgDB(testDefaults())
			resp, ok := db.valget(tc.poll)
			if tc.expectNak {
				if ok {
					t.Fatalf("expected NAK")
				}
				return
			}
			if !ok {
				t.Fatalf("unexpected NAK")
			}
			if resp.Version != ubxbin.CfgValgetVersionResponse || resp.Layer != tc.poll.Layer || resp.Position != tc.poll.Position {
				t.Errorf("bad response header %+v", resp.CfgValgetFixed)
			}
			items, err := ucv.UnmarshalItems(resp.CfgData)
			if err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if !reflect.DeepEqual(items, tc.expectItems) {
				t.Errorf("got  %+v\nwant %+v", items, tc.expectItems)
			}
		})
	}
}

func TestValgetPageLimit(t *testing.T) {
	dflt := make(ucv.Map)
	for i := range 100 {
		dflt[ucv.Key(0x20910000+i)] = uint64(i)
	}
	db := newCfgDB(dflt)
	resp, ok := db.valget(valgetPoll(ubxbin.CfgValgetLayerRAM, 0, ucv.Key(0x0fffffff)))
	if !ok {
		t.Fatalf("unexpected NAK")
	}
	items, err := ucv.UnmarshalItems(resp.CfgData)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(items) != 64 {
		t.Fatalf("got %d items, want 64", len(items))
	}
	resp, ok = db.valget(valgetPoll(ubxbin.CfgValgetLayerRAM, 64, ucv.Key(0x0fffffff)))
	if !ok {
		t.Fatalf("unexpected NAK")
	}
	items, err = ucv.UnmarshalItems(resp.CfgData)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(items) != 36 || items[0].Key != ucv.Key(0x20910040) {
		t.Fatalf("second page: got %d items first %v", len(items), items[0].Key)
	}
}

func valsetMsg(layers ubxbin.CfgValsetLayer, items ...ucv.Item) *ubxbin.CfgValset {
	data, err := ucv.MarshalItems(items)
	if err != nil {
		panic(err)
	}
	return &ubxbin.CfgValset{
		CfgValsetFixed: ubxbin.CfgValsetFixed{Layers: layers},
		CfgData:        data,
	}
}

func TestValset(t *testing.T) {
	navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
	tests := []struct {
		name      string
		msg       *ubxbin.CfgValset
		expectNak bool
	}{
		{
			name: "RAM set",
			msg:  valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: navSat, Value: 1}),
		},
		{
			name:      "unknown key",
			msg:       valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: ucv.KUbxTimSvin.KeyU(ucv.UART1).Key(), Value: 1}),
			expectNak: true,
		},
		{
			name:      "no layer",
			msg:       valsetMsg(0, ucv.Item{Key: navSat, Value: 1}),
			expectNak: true,
		},
		{
			name:      "wildcard key",
			msg:       valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: ucv.Key(0x2091ffff), Value: 1}),
			expectNak: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newCfgDB(testDefaults())
			ok := db.valset(tc.msg)
			if ok == tc.expectNak {
				t.Fatalf("ack %v, want %v", ok, !tc.expectNak)
			}
			if ok && db.ramUint(navSat) != 1 {
				t.Errorf("RAM value not applied")
			}
		})
	}
}

func TestValsetLayersAndValdel(t *testing.T) {
	navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
	db := newCfgDB(testDefaults())
	if !db.valset(valsetMsg(ubxbin.CfgValsetLayerRAM|ubxbin.CfgValsetLayerBBR|ubxbin.CfgValsetLayerFlash,
		ucv.Item{Key: navSat, Value: 1})) {
		t.Fatalf("valset NAKed")
	}
	for _, layer := range []ubxbin.CfgValgetLayer{ubxbin.CfgValgetLayerRAM, ubxbin.CfgValgetLayerBBR, ubxbin.CfgValgetLayerFlash} {
		resp, ok := db.valget(valgetPoll(layer, 0, navSat))
		if !ok {
			t.Fatalf("valget layer %d NAKed", layer)
		}
		items, err := ucv.UnmarshalItems(resp.CfgData)
		if err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		expect := []ucv.Item{{Key: navSat, Value: 1}}
		if !reflect.DeepEqual(items, expect) {
			t.Errorf("layer %d: got %+v want %+v", layer, items, expect)
		}
	}
	// The default layer is untouched.
	resp, _ := db.valget(valgetPoll(ubxbin.CfgValgetLayerDefault, 0, navSat))
	items, _ := ucv.UnmarshalItems(resp.CfgData)
	if !reflect.DeepEqual(items, []ucv.Item{{Key: navSat, Value: 0}}) {
		t.Errorf("default layer changed: %+v", items)
	}
	// VALDEL with no layer selected is NAKed.
	del := &ubxbin.CfgValdel{CfgData: ucv.MarshalKeys([]ucv.Key{navSat})}
	if db.valdel(del) {
		t.Fatalf("valdel with no layers not NAKed")
	}
	// Group wildcard delete from BBR only.
	del = &ubxbin.CfgValdel{
		CfgValdelFixed: ubxbin.CfgValdelFixed{Layers: ubxbin.CfgValdelLayerBBR},
		CfgData:        ucv.MarshalKeys([]ucv.Key{ucv.Key(0x2091ffff)}),
	}
	if !db.valdel(del) {
		t.Fatalf("valdel NAKed")
	}
	resp, _ = db.valget(valgetPoll(ubxbin.CfgValgetLayerBBR, 0, navSat))
	items, _ = ucv.UnmarshalItems(resp.CfgData)
	if len(items) != 0 {
		t.Errorf("BBR still has %+v after delete", items)
	}
	resp, _ = db.valget(valgetPoll(ubxbin.CfgValgetLayerFlash, 0, navSat))
	items, _ = ucv.UnmarshalItems(resp.CfgData)
	if !reflect.DeepEqual(items, []ucv.Item{{Key: navSat, Value: 1}}) {
		t.Errorf("Flash lost the item: %+v", items)
	}
	// A wildcard delete of an empty group deletes nothing and is valid,
	// like deleting items that are not stored.
	del = &ubxbin.CfgValdel{
		CfgValdelFixed: ubxbin.CfgValdelFixed{Layers: ubxbin.CfgValdelLayerFlash},
		CfgData:        ucv.MarshalKeys([]ucv.Key{ucv.Key(0x2092ffff)}),
	}
	if !db.valdel(del) {
		t.Errorf("valdel of empty-group wildcard NAKed")
	}
}

func TestTooManyItems(t *testing.T) {
	dflt := make(ucv.Map)
	keys := make([]ucv.Key, 65)
	items := make([]ucv.Item, 65)
	for i := range 65 {
		k := ucv.Key(0x20910000 + i)
		dflt[k] = 0
		keys[i] = k
		items[i] = ucv.Item{Key: k, Value: 1}
	}
	db := newCfgDB(dflt)
	if _, ok := db.valget(valgetPoll(ubxbin.CfgValgetLayerRAM, 0, keys...)); ok {
		t.Errorf("valget with 65 keys not NAKed")
	}
	if db.valset(valsetMsg(ubxbin.CfgValsetLayerRAM, items...)) {
		t.Errorf("valset with 65 items not NAKed")
	}
	del := &ubxbin.CfgValdel{
		CfgValdelFixed: ubxbin.CfgValdelFixed{Layers: ubxbin.CfgValdelLayerBBR},
		CfgData:        ucv.MarshalKeys(keys),
	}
	if db.valdel(del) {
		t.Errorf("valdel with 65 keys not NAKed")
	}
}

func TestReboot(t *testing.T) {
	navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
	db := newCfgDB(testDefaults())
	// An unsaved RAM change is discarded by a reboot.
	db.valset(valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: navSat, Value: 1}))
	db.reboot()
	if v := db.ramUint(navSat); v != 0 {
		t.Fatalf("after reboot got %d, want default 0", v)
	}
	// The rebuild takes BBR over Flash over Default, and does not touch
	// the saved layers.
	db.valset(valsetMsg(ubxbin.CfgValsetLayerFlash, ucv.Item{Key: navSat, Value: 3}))
	db.valset(valsetMsg(ubxbin.CfgValsetLayerBBR, ucv.Item{Key: navSat, Value: 2}))
	db.reboot()
	if v := db.ramUint(navSat); v != 2 {
		t.Fatalf("after reboot got %d, want BBR value 2", v)
	}
	resp, _ := db.valget(valgetPoll(ubxbin.CfgValgetLayerFlash, 0, navSat))
	items, _ := ucv.UnmarshalItems(resp.CfgData)
	if !reflect.DeepEqual(items, []ucv.Item{{Key: navSat, Value: 3}}) {
		t.Errorf("reboot changed the Flash layer: %+v", items)
	}
}

func TestCfgCfg(t *testing.T) {
	navSat := ucv.KUbxNavSat.KeyU(ucv.UART1).Key()
	db := newCfgDB(testDefaults())
	load := &ubxbin.CfgCfg{CfgCfgFixed: ubxbin.CfgCfgFixed{LoadMask: ubxbin.CfgCfgSectionMaskAll}}
	// A RAM-only change is discarded by a load.
	db.valset(valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: navSat, Value: 1}))
	if !db.cfgcfg(load) {
		t.Fatalf("load NAKed")
	}
	if v := db.ramUint(navSat); v != 0 {
		t.Fatalf("after load got %d, want default 0", v)
	}
	// On load BBR has priority over Flash, both over Default.
	db.valset(valsetMsg(ubxbin.CfgValsetLayerBBR, ucv.Item{Key: navSat, Value: 2}))
	db.valset(valsetMsg(ubxbin.CfgValsetLayerFlash, ucv.Item{Key: navSat, Value: 3}))
	db.cfgcfg(load)
	if v := db.ramUint(navSat); v != 2 {
		t.Fatalf("after load got %d, want BBR value 2", v)
	}
	// Clearing BBR only (deviceMask) leaves the Flash value to win.
	db.cfgcfg(&ubxbin.CfgCfg{
		CfgCfgFixed: ubxbin.CfgCfgFixed{ClearMask: ubxbin.CfgCfgSectionMaskAll},
		DeviceMask:  []ubxbin.CfgCfgDeviceMask{ubxbin.CfgCfgDevBBR},
	})
	db.cfgcfg(load)
	if v := db.ramUint(navSat); v != 3 {
		t.Fatalf("after clear BBR and load got %d, want Flash value 3", v)
	}
	// Save copies the current configuration to the selected layers.
	db.valset(valsetMsg(ubxbin.CfgValsetLayerRAM, ucv.Item{Key: navSat, Value: 5}))
	db.cfgcfg(&ubxbin.CfgCfg{CfgCfgFixed: ubxbin.CfgCfgFixed{SaveMask: ubxbin.CfgCfgSectionMaskAll}})
	db.cfgcfg(load)
	if v := db.ramUint(navSat); v != 5 {
		t.Fatalf("after save and load got %d, want saved value 5", v)
	}
}
