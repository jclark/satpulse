package ascii

import (
	"reflect"
	"strconv"
	"testing"
)

// Each test exhaustively compares the table-driven implementation against an
// independently written reference over all 256 byte values, comparing the
// whole 256-entry result vector with reflect.DeepEqual.

func TestPredicates(t *testing.T) {
	digit := func(b byte) bool { return b >= '0' && b <= '9' }
	upper := func(b byte) bool { return b >= 'A' && b <= 'Z' }
	lower := func(b byte) bool { return b >= 'a' && b <= 'z' }
	upHex := func(b byte) bool { return b >= 'A' && b <= 'F' }
	loHex := func(b byte) bool { return b >= 'a' && b <= 'f' }
	tests := []struct {
		name string
		got  func(byte) bool
		ref  func(byte) bool
	}{
		{"IsDigit", IsDigit, digit},
		{"IsUpper", IsUpper, upper},
		{"IsLower", IsLower, lower},
		{"IsLetter", IsLetter, func(b byte) bool { return upper(b) || lower(b) }},
		{"IsAlnum", IsAlnum, func(b byte) bool { return digit(b) || upper(b) || lower(b) }},
		{"IsPrint", IsPrint, func(b byte) bool { return b >= 0x20 && b <= 0x7E }},
		{"IsGraph", IsGraph, func(b byte) bool { return b >= 0x21 && b <= 0x7E }},
		{"IsHexDigit", IsHexDigit, func(b byte) bool { return digit(b) || upHex(b) || loHex(b) }},
		{"IsUpperHexDigit", IsUpperHexDigit, func(b byte) bool { return digit(b) || upHex(b) }},
		{"IsLowerHexDigit", IsLowerHexDigit, func(b byte) bool { return digit(b) || loHex(b) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got, want [256]bool
			for i := range 256 {
				got[i] = tc.got(byte(i))
				want[i] = tc.ref(byte(i))
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s mismatch\ngot  %v\nwant %v", tc.name, got, want)
			}
		})
	}
}

type val struct {
	v  uint8
	ok bool
}

func TestValues(t *testing.T) {
	parse := func(base int) func(byte) val {
		return func(b byte) val {
			n, err := strconv.ParseUint(string(rune(b)), base, 8)
			return val{uint8(n), err == nil}
		}
	}
	upHexRef := func(b byte) val {
		switch {
		case b >= '0' && b <= '9':
			return val{b - '0', true}
		case b >= 'A' && b <= 'F':
			return val{b - 'A' + 10, true}
		}
		return val{}
	}
	loHexRef := func(b byte) val {
		switch {
		case b >= '0' && b <= '9':
			return val{b - '0', true}
		case b >= 'a' && b <= 'f':
			return val{b - 'a' + 10, true}
		}
		return val{}
	}
	tests := []struct {
		name string
		got  func(byte) (uint8, bool)
		ref  func(byte) val
	}{
		{"DigitVal", DigitVal, parse(10)},
		{"HexVal", HexVal, parse(16)},
		{"UpperHexVal", UpperHexVal, upHexRef},
		{"LowerHexVal", LowerHexVal, loHexRef},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got, want [256]val
			for i := range 256 {
				v, ok := tc.got(byte(i))
				got[i] = val{v, ok}
				want[i] = tc.ref(byte(i))
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s mismatch\ngot  %v\nwant %v", tc.name, got, want)
			}
		})
	}
}

func TestCase(t *testing.T) {
	tests := []struct {
		name string
		got  func(byte) byte
		ref  func(byte) byte
	}{
		{"ToLower", ToLower, func(b byte) byte {
			if b >= 'A' && b <= 'Z' {
				return b + ('a' - 'A')
			}
			return b
		}},
		{"ToUpper", ToUpper, func(b byte) byte {
			if b >= 'a' && b <= 'z' {
				return b - ('a' - 'A')
			}
			return b
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got, want [256]byte
			for i := range 256 {
				got[i] = tc.got(byte(i))
				want[i] = tc.ref(byte(i))
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s mismatch\ngot  %v\nwant %v", tc.name, got, want)
			}
		})
	}
}
