package bitsenc

import (
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

func TestReadSkipsUntagged(t *testing.T) {
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
	Flag bool   `bits:"1"`
	Val  uint8  `bits:"3"`
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
