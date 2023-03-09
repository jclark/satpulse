package cfg

import (
	"math"
	"testing"

	"golang.org/x/exp/slices"
)

type marshalTestCase struct {
	schema Schema
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
