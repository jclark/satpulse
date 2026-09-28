package ubxcfgval

import (
	_ "embed"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"
)

type marshalTestCase struct {
	schema *Schema
	config map[string]map[string]any
	bytes  []byte
}

var marshalTestCases = []marshalTestCase{
	{dfltSchema, map[string]map[string]any{"TMODE": {"MODE": "FIXED"}}, []byte{0x01, 0x00, 0x03, 0x20, 0x02}},
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
		bytes, err := dfltSchema.Marshal(cfg)
		if err != nil {
			t.Errorf("could not marshal config %d: %v", i, err)
		} else {
			cfg2, unknown, err := dfltSchema.UnmarshalItems(bytes)
			if err != nil {
				t.Errorf("could not unmarshal config %d: %v", i, err)
			} else if !sameConfig(cfg, cfg2) || len(unknown) != 0 {
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

const (
	keyGroupMask    = 0xFF0000
	keyReservedMask = 0x8F00F000
)

// This text file is how u-center saved the configuration of a F9P receiver
//
//go:embed f9p_cfg.txt
var f9pCfgTxt string

func TestUnmarshal(t *testing.T) {
	valgets, err := parseUCenterConfig(f9pCfgTxt)
	if err != nil || len(valgets) == 0 {
		t.Fatalf("could not parse config text")
	}
	unknownKeys := make(map[Key]struct{})
	recognizedCount := 0
	for i, msg := range valgets {
		// U-center format doesn't include the two sync bytes not the two-byte checksum.
		// So to get the config data, we just have to skip cls+id, 2-byte length, and 4 byte fixed part.
		cfgData := msg[8:]
		cfg, unknown, err := dfltSchema.UnmarshalItems(cfgData)
		if err != nil {
			t.Errorf("test %d: could not unmarshal: %v", i, err)
		} else {
			for _, m := range cfg {
				recognizedCount += len(m)
			}
			for k := range unknown {
				if (k & keyReservedMask) != 0 {
					t.Errorf("unknown key 0x%x has non-zero reserved bits", k)
				}
				unknownKeys[k] = struct{}{}
			}
		}
	}
	if recognizedCount < 100 {
		t.Errorf("recognized %d config items, expected at least 100", recognizedCount)
	} else {
		t.Logf("recognized %d keys", recognizedCount)
	}
	uk := make([]Key, 0, len(unknownKeys))
	for k := range unknownKeys {
		uk = append(uk, k)
	}
	sort.Slice(uk, func(i1, i2 int) bool {
		k1 := uk[i1] & keyGroupMask
		k2 := uk[i2] & keyGroupMask
		if k1 < k2 {
			return true
		}
		if k1 == k2 && uk[i1] < uk[i2] {
			return true
		}
		return false
	})

	uks := ""
	for k := range uk {
		uks = fmt.Sprintf("%s 0x%x", uks, uk[k])
	}
	if uks != "" {
		t.Logf("unknown keys:%s", uks)
	}
}

func parseUCenterConfig(txt string) ([][]byte, error) {
	lines := strings.Split(txt, "\n")
	var valgets [][]byte
	for _, line := range lines {
		const cfgValgetTxtPrefix = "CFG-VALGET - "
		if strings.HasPrefix(line, cfgValgetTxtPrefix) {
			byteStrs := strings.Split(line[len(cfgValgetTxtPrefix):], " ")
			bytes := make([]byte, 0)
			for _, bs := range byteStrs {
				var b byte
				n, err := fmt.Sscanf(bs, "%02x", &b)
				if n == 1 {
					bytes = append(bytes, b)
				} else if bs != "" {
					return nil, err
				}
			}
			valgets = append(valgets, bytes)
		}
	}
	return valgets, nil
}

func TestUnmarshalItemsFlat(t *testing.T) {
	s := GetDfltSchema()
	
	data, err := hex.DecodeString("0100311001030031100107003110010a003110010d003110010e003110011200311001150031100118003110011a003110011f0031100121003110012200311001240031100125003110012700311001")
	if err != nil {
		t.Fatal(err)
	}
	
	wantKeys := []string{
		"CFG-SIGNAL-GPS_L1CA_ENA",
		"CFG-SIGNAL-GPS_L2C_ENA", 
		"CFG-SIGNAL-GAL_E1_ENA",
		"CFG-SIGNAL-GAL_E5B_ENA",
		"CFG-SIGNAL-BDS_B1_ENA",
		"CFG-SIGNAL-BDS_B2_ENA",
		"CFG-SIGNAL-QZSS_L1CA_ENA",
		"CFG-SIGNAL-QZSS_L2C_ENA",
		"CFG-SIGNAL-GLO_L1_ENA",
		"CFG-SIGNAL-GLO_L2_ENA",
		"CFG-SIGNAL-GPS_ENA",
		"CFG-SIGNAL-GAL_ENA",
		"CFG-SIGNAL-BDS_ENA",
		"CFG-SIGNAL-QZSS_ENA",
		"CFG-SIGNAL-GLO_ENA",
		"0x10310027",
	}
	wantValues := []any{
		true, true, true, true, true, true, true, true, true, true, true, true, true, true, true, uint64(1),
	}
	
	gotKeys, gotValues, err := s.UnmarshalItemsFlat(data)
	if err != nil {
		t.Fatal(err)
	}
	
	if !slices.Equal(wantKeys, gotKeys) {
		t.Errorf("keys mismatch: got %v, want %v", gotKeys, wantKeys)
	}
	if !slices.Equal(wantValues, gotValues) {
		t.Errorf("values mismatch: got %v, want %v", gotValues, wantValues)
	}
}

func TestUnmarshalKeysFlat(t *testing.T) {
	s := GetDfltSchema()
	
	data, err := hex.DecodeString("ffff3110")
	if err != nil {
		t.Fatal(err)
	}
	
	want := []string{
		"0x1031ffff",
	}
	
	got, err := s.UnmarshalKeysFlat(data)
	if err != nil {
		t.Fatal(err)
	}
	
	if !slices.Equal(want, got) {
		t.Errorf("UnmarshalKeysFlat() mismatch: got %v, want %v", got, want)
	}
}
