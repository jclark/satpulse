package bitsenc

import (
	"iter"
	"testing"
)

func TestReadUnsigned(t *testing.T) {
	// 0xAB = 1010_1011, 0xCD = 1100_1101
	data := []byte{0xAB, 0xCD}
	var v struct {
		A uint8  `bits:"4"` // 1010 = 10
		B uint8  `bits:"4"` // 1011 = 11
		C uint16 `bits:"8"` // 1100_1101 = 205
	}
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 10 {
		t.Errorf("A = %d, want 10", v.A)
	}
	if v.B != 11 {
		t.Errorf("B = %d, want 11", v.B)
	}
	if v.C != 205 {
		t.Errorf("C = %d, want 205", v.C)
	}
}

func TestReadSigned(t *testing.T) {
	// Two's complement: 5-bit value 11111 = -1, 5-bit value 00001 = 1
	// 11111_00001_000000 = 0xF840
	data := []byte{0xF8, 0x40}
	var v struct {
		A int8 `bits:"5"` // 11111 = -1
		B int8 `bits:"5"` // 00001 = 1
	}
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.A != -1 {
		t.Errorf("A = %d, want -1", v.A)
	}
	if v.B != 1 {
		t.Errorf("B = %d, want 1", v.B)
	}
}

func TestReadBool(t *testing.T) {
	data := []byte{0xA0} // 1010_0000
	var v struct {
		A bool `bits:"1"`
		B bool `bits:"1"`
		C bool `bits:"1"`
		D bool `bits:"1"`
	}
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if !v.A {
		t.Error("A = false, want true")
	}
	if v.B {
		t.Error("B = true, want false")
	}
	if !v.C {
		t.Error("C = false, want true")
	}
	if v.D {
		t.Error("D = true, want false")
	}
}

func TestReadNativeSize(t *testing.T) {
	// Untagged fields now read at native type size.
	// uint8 = 8 bits, bool = 1 bit
	// 0xFF 0x80 => uint8=0xFF, bool=1
	data := []byte{0xFF, 0x80}
	var v struct {
		A uint8
		B bool
	}
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 0xFF {
		t.Errorf("A = 0x%X, want 0xFF", v.A)
	}
	if !v.B {
		t.Error("B = false, want true")
	}
}

func TestReadSkipsUnsupportedUntagged(t *testing.T) {
	// Untagged fields of types without a native bit size (e.g. int) are skipped.
	data := []byte{0xFF}
	var v struct {
		Skip int
		A    uint8 `bits:"4"`
	}
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Skip != 0 {
		t.Errorf("Skip = %d, want 0", v.Skip)
	}
	if v.A != 15 {
		t.Errorf("A = %d, want 15", v.A)
	}
}

func TestReadLargeSigned(t *testing.T) {
	// 38-bit signed value: test with a known ECEF-like value
	// -1 in 38 bits = 38 ones = 0x3F_FFFF_FFFF
	// Pack into 5 bytes: 11_11111111_11111111_11111111_11111111_111111_00
	// = 0xFF 0xFF 0xFF 0xFF 0xFC
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFC}
	var v struct {
		X int64 `bits:"38"`
	}
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.X != -1 {
		t.Errorf("X = %d, want -1", v.X)
	}
}

type header struct {
	MsgNum    uint16 `bits:"12"`
	StationID uint16 `bits:"12"`
}

type body struct {
	header
	Flag bool  `bits:"1"`
	Val  uint8 `bits:"3"`
}

func TestReadEmbedded(t *testing.T) {
	// header: MsgNum 12 bits, StationID 12 bits = 24 bits
	// body: Flag 1 bit, Val 3 bits = 4 bits
	// total = 28 bits = 3.5 bytes
	// 0xAB 0xCD 0xE5 = bits: 1010_1011 1100_1101 1110_0101
	// MsgNum = 1010_1011_1100 = 0xABC
	// StationID = 1101_1110_0101 = 0xDE5
	// Then byte 3 = 0x00: Flag = 0, Val = 000 = 0
	data := []byte{0xAB, 0xCD, 0xE5, 0x00}
	var v body
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.MsgNum != 0xABC {
		t.Errorf("MsgNum = 0x%X, want 0xABC", v.MsgNum)
	}
	if v.StationID != 0xDE5 {
		t.Errorf("StationID = 0x%X, want 0xDE5", v.StationID)
	}
	// Bits 24-27 from byte 3 (0x00) = 0_000
	if v.Flag {
		t.Error("Flag = true, want false")
	}
	if v.Val != 0 {
		t.Errorf("Val = %d, want 0", v.Val)
	}
}

func TestReadPastEnd(t *testing.T) {
	data := []byte{0xFF}
	var v struct {
		A uint16 `bits:"12"`
	}
	err := Read(data, &v)
	if err == nil {
		t.Error("expected error reading past end of data")
	}
}

func TestReadNotPointer(t *testing.T) {
	var v struct {
		A uint8 `bits:"4"`
	}
	err := Read([]byte{0xFF}, v)
	if err == nil {
		t.Error("expected error for non-pointer argument")
	}
}

func TestReaderUintInt(t *testing.T) {
	data := []byte{0xAB, 0xCD}
	r := NewReader(data)
	v, err := r.Uint(8)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0xAB {
		t.Errorf("Uint(8) = 0x%X, want 0xAB", v)
	}
	s, err := r.Int(8)
	if err != nil {
		t.Fatal(err)
	}
	// 0xCD sign-extended from 8 bits = -51
	if s != -51 {
		t.Errorf("Int(8) = %d, want -51", s)
	}
}

func TestReadSlice(t *testing.T) {
	// 3 x uint8 at 4 bits each = 12 bits
	// 0xABC = 1010 1011 1100
	data := []byte{0xAB, 0xC0}
	var v struct {
		Vals []uint8 `bits:"4"`
	}
	v.Vals = make([]uint8, 3)
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	want := []uint8{0xA, 0xB, 0xC}
	for i, w := range want {
		if v.Vals[i] != w {
			t.Errorf("Vals[%d] = %d, want %d", i, v.Vals[i], w)
		}
	}
}

func TestReadSliceNative(t *testing.T) {
	// 2 x uint16 at native 16-bit width = 32 bits
	data := []byte{0x00, 0x01, 0x00, 0x02}
	var v struct {
		Vals []uint16
	}
	v.Vals = make([]uint16, 2)
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Vals[0] != 1 || v.Vals[1] != 2 {
		t.Errorf("Vals = %v, want [1, 2]", v.Vals)
	}
}

type varMsg struct {
	Width uint8
	Data  uint32 `bits:"var"`
}

func (m *varMsg) VarBits() iter.Seq[int] {
	return func(yield func(int) bool) { yield(int(m.Width)) }
}

func TestReadVarBits(t *testing.T) {
	// Width = 8 bits (native uint8), then Data = Width bits
	// 0x0A = Width=10, then 10 bits = 0x2AB (1010101011)
	// 00001010 1010101011_000000
	// 0x0A 0xAA 0xC0
	data := []byte{0x0A, 0xAA, 0xC0}
	var v varMsg
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Width != 10 {
		t.Errorf("Width = %d, want 10", v.Width)
	}
	if v.Data != 0x2AB {
		t.Errorf("Data = 0x%X, want 0x2AB", v.Data)
	}
}

type sliceSizerMsg struct {
	Count uint8
	Vals  []uint8 `bits:"4"`
}

func (m *sliceSizerMsg) SizeSlices() {
	m.Vals = make([]uint8, m.Count)
}

func TestReadSliceSizer(t *testing.T) {
	// Count = 3 (8 bits native), then 3 x 4-bit values
	// 00000011 1010 1011 1100 0000
	// 0x03 0xAB 0xC0
	data := []byte{0x03, 0xAB, 0xC0}
	var v sliceSizerMsg
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Count != 3 {
		t.Errorf("Count = %d, want 3", v.Count)
	}
	want := []uint8{0xA, 0xB, 0xC}
	for i, w := range want {
		if v.Vals[i] != w {
			t.Errorf("Vals[%d] = %d, want %d", i, v.Vals[i], w)
		}
	}
}

func TestReadNamedStruct(t *testing.T) {
	// Named (non-embedded) structs are recursed into.
	type inner struct {
		A uint8 `bits:"4"`
		B uint8 `bits:"4"`
	}
	type outer struct {
		X inner
		C uint8 `bits:"4"`
	}
	data := []byte{0xAB, 0xC0}
	var v outer
	if err := Read(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.X.A != 0xA || v.X.B != 0xB || v.C != 0xC {
		t.Errorf("got {X:{%d,%d}, C:%d}, want {X:{10,11}, C:12}", v.X.A, v.X.B, v.C)
	}
}
