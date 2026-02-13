package ubxcfgval

// TPKeyPairs maps TP1 keys to their TP2 equivalents.
// Each entry is [TP1, TP2]. Keys without a Tp1/Tp2 suffix
// (e.g. KTpPulseDef, KTpPulseLengthDef, KTpAntCabledelay)
// are shared and not included.
var TPKeyPairs = [][2]AnyTypedKey{
	{KTpAlignToTowTp1, KTpAlignToTowTp2},
	{KTpDrstrTp1, KTpDrstrTp2},
	{KTpDutyLockTp1, KTpDutyLockTp2},
	{KTpDutyTp1, KTpDutyTp2},
	{KTpFreqLockTp1, KTpFreqLockTp2},
	{KTpFreqTp1, KTpFreqTp2},
	{KTpLenLockTp1, KTpLenLockTp2},
	{KTpLenTp1, KTpLenTp2},
	{KTpPeriodLockTp1, KTpPeriodLockTp2},
	{KTpPeriodTp1, KTpPeriodTp2},
	{KTpPolTp1, KTpPolTp2},
	{KTpSyncGnssTp1, KTpSyncGnssTp2},
	{KTpTimegridTp1, KTpTimegridTp2},
	{KTpTp1Ena, KTpTp2Ena},
	{KTpUseLockedTp1, KTpUseLockedTp2},
	{KTpUserDelayTp1, KTpUserDelayTp2},
}

// KeyRemap builds a Key-to-Key mapping from a set of key pairs.
// from and to select which element of each pair is the source and destination:
// e.g. KeyRemap(pairs, 1, 0) maps from the second element to the first.
func KeyRemap(pairs [][2]AnyTypedKey, from, to int) map[Key]Key {
	m := make(map[Key]Key, len(pairs))
	for _, p := range pairs {
		m[p[from].Key()] = p[to].Key()
	}
	return m
}

// RemapMap returns a copy of src with keys remapped according to remap.
// Keys not in remap are copied unchanged.
func RemapMap(src Map, remap map[Key]Key) Map {
	dst := make(Map, len(src))
	for k, v := range src {
		if k2, ok := remap[k]; ok {
			k = k2
		}
		dst[k] = v
	}
	return dst
}

// RemapItems returns a copy of items with keys remapped according to remap.
// Keys not in remap are copied unchanged.
func RemapItems(items []Item, remap map[Key]Key) []Item {
	dst := make([]Item, len(items))
	for i, it := range items {
		if k2, ok := remap[it.Key]; ok {
			it.Key = k2
		}
		dst[i] = it
	}
	return dst
}

// RemapKeys returns a copy of keys with values remapped according to remap.
// Keys not in remap are copied unchanged.
func RemapKeys(keys []Key, remap map[Key]Key) []Key {
	dst := make([]Key, len(keys))
	for i, k := range keys {
		if k2, ok := remap[k]; ok {
			k = k2
		}
		dst[i] = k
	}
	return dst
}
