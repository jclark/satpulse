package ubxsim

import (
	"maps"
	"slices"
	"sync"

	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

// maxItems is the per-message item limit for CFG-VALGET, CFG-VALSET and
// CFG-VALDEL, and the page size of CFG-VALGET responses to wild-card
// polls, from the interface description.
const maxItems = 64

// cfgDB is the layered configuration database. The Default layer holds a
// value for every key the personality knows, so it doubles as the key
// inventory; RAM is seeded from it at construction, the way a receiver
// boots. BBR and Flash hold only explicitly stored items.
//
// Transactions are not modelled: CFG-VALSET and CFG-VALDEL are handled
// with transactionless semantics whatever their version and transaction
// fields say (more permissive than real firmware, acceptable at smoke
// depth). Configuration validity is not checked either: any well-formed
// set of known keys is accepted.
type cfgDB struct {
	mu   sync.Mutex
	dflt ucv.Map
	ram  ucv.Map
	bbr  ucv.Map
	flh  ucv.Map
}

func newCfgDB(dflt ucv.Map) *cfgDB {
	return &cfgDB{
		dflt: dflt,
		ram:  maps.Clone(dflt),
		bbr:  make(ucv.Map),
		flh:  make(ucv.Map),
	}
}

// valget answers a CFG-VALGET poll. It returns the response message and
// true, or nil and false when the poll must be NAKed: an unknown key, an
// invalid layer, more than 64 key IDs, or a malformed payload.
func (db *cfgDB) valget(m *ubxbin.CfgValget) (*ubxbin.CfgValget, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	layer := db.layerMap(m.Layer)
	if layer == nil {
		return nil, false
	}
	keys, err := ucv.UnmarshalKeys(m.CfgData)
	if err != nil || len(keys) > maxItems {
		return nil, false
	}
	expanded, ok := db.expandKeys(keys)
	if !ok {
		return nil, false
	}
	items := make([]ucv.Item, 0, maxItems)
	skip := int(m.Position)
	for _, k := range expanded {
		v, ok := layer[k]
		if !ok {
			continue
		}
		if skip > 0 {
			skip--
			continue
		}
		if len(items) == maxItems {
			break
		}
		items = append(items, ucv.Item{Key: k, Value: v})
	}
	data, err := ucv.MarshalItems(items)
	if err != nil {
		return nil, false
	}
	resp := &ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{
			Version:  ubxbin.CfgValgetVersionResponse,
			Layer:    m.Layer,
			Position: m.Position,
		},
		CfgData: data,
	}
	return resp, true
}

// valset applies a CFG-VALSET. It reports whether the message is ACKed;
// on NAK (unknown key, no layer selected, more than 64 items, malformed
// payload) nothing is applied.
func (db *cfgDB) valset(m *ubxbin.CfgValset) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	items, err := ucv.UnmarshalItems(m.CfgData)
	if err != nil || len(items) > maxItems {
		return false
	}
	const layerAll = ubxbin.CfgValsetLayerRAM | ubxbin.CfgValsetLayerBBR | ubxbin.CfgValsetLayerFlash
	if m.Layers&layerAll == 0 {
		return false
	}
	for _, it := range items {
		if !db.dflt.Contains(it.Key) {
			return false
		}
	}
	for _, target := range []struct {
		bit ubxbin.CfgValsetLayer
		m   ucv.Map
	}{{ubxbin.CfgValsetLayerRAM, db.ram}, {ubxbin.CfgValsetLayerBBR, db.bbr}, {ubxbin.CfgValsetLayerFlash, db.flh}} {
		if m.Layers&target.bit != 0 {
			target.m.AddItems(items)
		}
	}
	return true
}

// valdel applies a CFG-VALDEL. It reports whether the message is ACKed;
// on NAK (unknown key, no layer selected, more than 64 keys, malformed
// payload) nothing is deleted. Deleting items that are not stored is
// valid, per the interface description.
func (db *cfgDB) valdel(m *ubxbin.CfgValdel) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	keys, err := ucv.UnmarshalKeys(m.CfgData)
	if err != nil || len(keys) > maxItems {
		return false
	}
	if m.Layers&(ubxbin.CfgValdelLayerBBR|ubxbin.CfgValdelLayerFlash) == 0 {
		return false
	}
	expanded, ok := db.expandKeys(keys)
	if !ok {
		return false
	}
	for _, k := range expanded {
		if m.Layers&ubxbin.CfgValdelLayerBBR != 0 {
			delete(db.bbr, k)
		}
		if m.Layers&ubxbin.CfgValdelLayerFlash != 0 {
			delete(db.flh, k)
		}
	}
	return true
}

// cfgcfg applies a UBX-CFG-CFG. Per the interface description, the
// masks have lost their section meaning: any bit set in clearMask
// deletes all saved configuration from the selected non-volatile
// layers, any bit in saveMask copies all current configuration to
// them, and any bit in loadMask discards the current configuration and
// rebuilds it from the lower layers (Default, then Flash, then BBR on
// top, per layer priority). The sequence is clear, save, then load.
// Without a deviceMask both non-volatile layers are selected.
func (db *cfgDB) cfgcfg(m *ubxbin.CfgCfg) bool {
	db.mu.Lock()
	defer db.mu.Unlock()
	bbr, flh := true, true
	if len(m.DeviceMask) == 1 {
		bbr = m.DeviceMask[0]&ubxbin.CfgCfgDevBBR != 0
		flh = m.DeviceMask[0]&ubxbin.CfgCfgDevFlash != 0
	}
	if m.ClearMask != 0 {
		if bbr {
			clear(db.bbr)
		}
		if flh {
			clear(db.flh)
		}
	}
	if m.SaveMask != 0 {
		if bbr {
			maps.Copy(db.bbr, db.ram)
		}
		if flh {
			maps.Copy(db.flh, db.ram)
		}
	}
	if m.LoadMask != 0 {
		db.rebuildRAM()
	}
	return true
}

// reboot rebuilds the RAM layer from the layers below, the way a real
// receiver reconstructs its active configuration coming out of a reset:
// "The RAM layer is always rebuilt from the layers below when the chip's
// processor comes out from reset" (F9 interface description 6.7). The
// BBR and Flash configuration layers are left as they are; the
// navBbrMask of CFG-RST clears navigation backup data (ephemeris,
// almanac, position, ...), not the BBR configuration layer.
func (db *cfgDB) reboot() {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.rebuildRAM()
}

// rebuildRAM discards the current RAM configuration and rebuilds it from
// Default, then Flash, then BBR on top, per layer priority. The caller
// holds db.mu.
func (db *cfgDB) rebuildRAM() {
	db.ram = maps.Clone(db.dflt)
	maps.Copy(db.ram, db.flh)
	maps.Copy(db.ram, db.bbr)
}

// layerMap returns the map for a CFG-VALGET layer field, or nil if the
// layer is invalid.
func (db *cfgDB) layerMap(l ubxbin.CfgValgetLayer) ucv.Map {
	switch l {
	case ubxbin.CfgValgetLayerRAM:
		return db.ram
	case ubxbin.CfgValgetLayerBBR:
		return db.bbr
	case ubxbin.CfgValgetLayerFlash:
		return db.flh
	case ubxbin.CfgValgetLayerDefault:
		return db.dflt
	}
	return nil
}

// expandKeys expands wild-card keys against the key inventory using the
// interface description's key arithmetic: item part 0xffff means all
// items in the group, group part 0xfff means all items in all groups.
// Expansions are in ascending key order; complete keys keep their
// request order. It returns false only if a complete key is unknown
// (the interface description's NAK rule); a group wildcard with no
// items in the inventory expands to nothing, as a recorded ZED-F9P
// ACKed the Configurator's signals poll whose second group wildcard
// matched no keys (gpshwtest001/019.jsonl).
func (db *cfgDB) expandKeys(keys []ucv.Key) ([]ucv.Key, bool) {
	out := make([]ucv.Key, 0, len(keys))
	for _, k := range keys {
		if (k>>16)&0xfff == 0xfff {
			out = append(out, db.sortedKeys(func(ucv.Key) bool { return true })...)
		} else if k&0xffff == 0xffff {
			group := (k >> 16) & 0xfff
			out = append(out, db.sortedKeys(func(k ucv.Key) bool { return (k>>16)&0xfff == group })...)
		} else if db.dflt.Contains(k) {
			out = append(out, k)
		} else {
			return nil, false
		}
	}
	return out, true
}

func (db *cfgDB) sortedKeys(match func(ucv.Key) bool) []ucv.Key {
	ks := make([]ucv.Key, 0, len(db.dflt))
	for k := range db.dflt {
		if match(k) {
			ks = append(ks, k)
		}
	}
	slices.Sort(ks)
	return ks
}

// ramUint returns the RAM-layer value of a key, or 0 if the key is
// unknown. The NAV engine uses it to read MSGOUT gates and pacing keys.
func (db *cfgDB) ramUint(k ucv.Key) uint64 {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.ram[k]
}
