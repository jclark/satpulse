package cfg

import (
	"math"
	"testing"

	"golang.org/x/exp/slices"
)

type marshalTestCase struct {
	schema *Schema
	config map[string]map[string]any
	bytes  []byte
}

var marshalTestCases = []marshalTestCase{
	{schema, map[string]map[string]any{"TMODE": {"MODE": "FIXED"}}, []byte{0x01, 0x00, 0x03, 0x20, 0x02}},
}

func TestMarshal(t *testing.T) {
	for i, tc := range marshalTestCases {
		bytes, err := tc.schema.Marshal(tc.config)
		if err != nil {
			if tc.bytes != nil {
				t.Errorf("marshal test case %d marshal failed %v", i, err)
			}
		} else if tc.bytes == nil {
			t.Errorf("marshal test case %d marshal succeeded unexpectedly", i)
		} else if !slices.Equal(bytes, tc.bytes) {
			t.Errorf("wrong result for marshal test case %d (got %v, expected %v)", i, bytes, tc.bytes)
		}
	}
}

func TestValueBits(t *testing.T) {
	if valueBits(0x10260013) != 1 {
		t.Error("expected valueBits(0x10260013) == 1")
	}
	if valueBits(0x30260015) != 16 {
		t.Error("expected valueBits(0x30260015) == 16")
	}
	if valueBits(0x50110063) != 64 {
		t.Error("expected valueBits(0x50110063) == 64")
	}
}

func TestSignedWiden(t *testing.T) {
	n, ok := signedWiden(-1)
	if !ok || n != -1 {
		t.Errorf("signedWiden(-1) = %d", n)
	}
	n, ok = signedWiden(int8(-1))
	if !ok || n != -1 {
		t.Errorf("signedWiden(int8(-1)) = %d", n)
	}
	n, ok = signedWiden(int32(1827186412))
	if !ok || n != 1827186412 {
		t.Errorf("signedWiden(int32(1827186412)) = %d", n)
	}
	n, ok = signedWiden(uint64(math.MaxUint64))
	if ok {
		t.Errorf("signedWiden(uint64(math.MaxUint64)) = %d", n)
	}
}

func TestUnsignedWiden(t *testing.T) {
	n, ok := unsignedWiden(uint(1))
	if !ok || n != 1 {
		t.Errorf("unsignedWiden(1) = %d", n)
	}
	n, ok = unsignedWiden(uint8(0))
	if !ok || n != 0 {
		t.Errorf("unsignedWiden(uint8(0)) = %v, %d", ok, n)
	}
	n, ok = unsignedWiden(uint32(1827186412))
	if !ok || n != 1827186412 {
		t.Errorf("unsignedWiden(uint32(1827186412)) = %d", n)
	}
	n, ok = unsignedWiden(int(0))
	if ok {
		t.Errorf("unsignedWiden(int(0)) = %d", n)
	}
}

var testCfgs = []map[string]map[string]any{
	{
		"TMODE": {
			"MODE":     "FIXED",
			"POS_TYPE": "ECEF",
			"ECEF_X":   1234567,
			"ECEF_Y":   2345678,
			"ECEF_Z":   -3456789,
		},
		"NAVSPG": {
			"DYNMODEL": "STAT",
		},
		"HW": {
			"ANT_CFG_SHORTDET": true,
			"ANT_CFG_OPENDET":  true,
		},
		"TP": {
			"ANT_CABLEDELAY": 75,
			"LEN_TP1":        100000,
		},
	},
}

func TestRoundtrip(t *testing.T) {
	for i, cfg := range testCfgs {
		bytes, err := schema.Marshal(cfg)
		if err != nil {
			t.Errorf("could not marshal config %d: %v", i, err)
		} else {
			cfg2, err := schema.Unmarshal(bytes)
			if err != nil {
				t.Errorf("could not unmarshal config %d: %v", i, err)
			} else if !sameConfig(cfg, cfg2) {
				t.Errorf("config %d roundtrip failed", i)
			}
		}
	}
}

func sameConfig(c1, c2 map[string]map[string]any) bool {
	nItems2 := 0
	for _, m2 := range c2 {
		nItems2 += len(m2)
	}
	for g1, m1 := range c1 {
		m2 := c2[g1]
		for i1, v1 := range m1 {
			if m2 == nil {
				return false
			}
			v2, ok := m2[i1]
			if !ok {
				return false
			}
			if !sameValue(v1, v2) {
				return false
			}
			nItems2--
		}
	}
	return nItems2 == 0
}

func sameValue(v1, v2 any) bool {
	pos1, neg1, ok := intValue(v1)
	if ok {
		pos2, neg2, ok := intValue(v2)
		if !ok {
			return false
		}
		return pos1 == pos2 && neg1 == neg2
	}
	switch v1 := v1.(type) {
	case string:
		v2, ok := v2.(string)
		return ok && v1 == v2
	case bool:
		v2, ok := v2.(bool)
		return ok && v1 == v2
	case float64:
		v2, ok := v2.(float64)
		return ok && v1 == v2
	case float32:
		v2, ok := v2.(float32)
		return ok && v1 == v2
	}
	return false
}

// intValue returns a representation of v that ignores its type and considers only its value
// if the value is positive, pos will have its value and neg will be 0
// if the value is negative, neg will have its value and pos will be 0
// if the value is not an integer, ok will be false
func intValue(v any) (pos uint64, neg int64, ok bool) {
	switch v := v.(type) {
	case int:
		if v < 0 {
			neg = int64(v)
		} else {
			pos = uint64(v)
		}
	case int8:
		if v < 0 {
			neg = int64(v)
		} else {
			pos = uint64(v)
		}
	case int16:
		if v < 0 {
			neg = int64(v)
		} else {
			pos = uint64(v)
		}
	case int32:
		if v < 0 {
			neg = int64(v)
		} else {
			pos = uint64(v)
		}
	case int64:
		if v < 0 {
			neg = v
		} else {
			pos = uint64(v)
		}
	case uint:
		pos = uint64(v)
	case uint8:
		pos = uint64(v)
	case uint16:
		pos = uint64(v)
	case uint32:
		pos = uint64(v)
	case uint64:
		pos = v
	default:
		ok = false
		return
	}
	ok = true
	return
}
