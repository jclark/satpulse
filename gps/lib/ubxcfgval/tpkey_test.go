package ubxcfgval

import (
	"slices"
	"testing"
)

func TestKeyRemap(t *testing.T) {
	pairs := [][2]AnyTypedKey{
		{KTpPolTp1, KTpPolTp2},
		{KTpTp1Ena, KTpTp2Ena},
	}
	m := KeyRemap(pairs, 0, 1)
	if m[KTpPolTp1.Key()] != KTpPolTp2.Key() {
		t.Error("expected TP1->TP2 mapping for KTpPolTp1")
	}
	if m[KTpTp1Ena.Key()] != KTpTp2Ena.Key() {
		t.Error("expected TP1->TP2 mapping for KTpTp1Ena")
	}
	m = KeyRemap(pairs, 1, 0)
	if m[KTpPolTp2.Key()] != KTpPolTp1.Key() {
		t.Error("expected TP2->TP1 mapping for KTpPolTp2")
	}
}

func TestRemapMap(t *testing.T) {
	remap := map[Key]Key{KTpPolTp1.Key(): KTpPolTp2.Key()}
	src := Map{KTpPolTp1.Key(): 1, KTpPulseDef.Key(): 2}
	dst := RemapMap(src, remap)
	if _, ok := dst[KTpPolTp1.Key()]; ok {
		t.Error("TP1 key should not be in remapped map")
	}
	if dst[KTpPolTp2.Key()] != 1 {
		t.Error("expected TP2 key with value 1")
	}
	if dst[KTpPulseDef.Key()] != 2 {
		t.Error("non-remapped key should pass through")
	}
	// src should be unmodified
	if src[KTpPolTp1.Key()] != 1 {
		t.Error("source map should not be modified")
	}
}

func TestRemapItems(t *testing.T) {
	remap := map[Key]Key{KTpPolTp1.Key(): KTpPolTp2.Key()}
	src := []Item{
		{KTpPolTp1.Key(), 1},
		{KTpPulseDef.Key(), 2},
	}
	dst := RemapItems(src, remap)
	if dst[0].Key != KTpPolTp2.Key() || dst[0].Value != 1 {
		t.Errorf("expected remapped item, got %v", dst[0])
	}
	if dst[1].Key != KTpPulseDef.Key() || dst[1].Value != 2 {
		t.Errorf("expected passthrough item, got %v", dst[1])
	}
	if src[0].Key != KTpPolTp1.Key() {
		t.Error("source slice should not be modified")
	}
}

func TestRemapKeys(t *testing.T) {
	remap := map[Key]Key{KTpPolTp1.Key(): KTpPolTp2.Key()}
	src := []Key{KTpPolTp1.Key(), KTpPulseDef.Key()}
	dst := RemapKeys(src, remap)
	want := []Key{KTpPolTp2.Key(), KTpPulseDef.Key()}
	if !slices.Equal(dst, want) {
		t.Errorf("expected %v, got %v", want, dst)
	}
	if src[0] != KTpPolTp1.Key() {
		t.Error("source slice should not be modified")
	}
}

func TestTPKeyPairsCoverage(t *testing.T) {
	// Verify all pairs have distinct TP1 and TP2 keys
	seen := map[Key]bool{}
	for _, p := range TPKeyPairs {
		k1, k2 := p[0].Key(), p[1].Key()
		if k1 == k2 {
			t.Errorf("pair has same key for TP1 and TP2: %v", k1)
		}
		if seen[k1] {
			t.Errorf("duplicate TP1 key: %v", k1)
		}
		if seen[k2] {
			t.Errorf("duplicate TP2 key: %v", k2)
		}
		seen[k1] = true
		seen[k2] = true
	}
}
