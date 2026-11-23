// Package ascii provides ctypes-style classification and conversion of ASCII
// bytes: the C <ctype.h> repertoire restricted to ASCII, with Go naming that
// follows the standard unicode package where an equivalent exists. Predicates
// take a single byte; any value >= 0x80 (non-ASCII) belongs to no class. The
// package has no dependencies, does no I/O, and is safe for concurrent use.
package ascii

// Class flags held in each entry of the class table.
const (
	fDigit uint8 = 1 << iota // '0'-'9'
	fUpper                   // 'A'-'Z'
	fLower                   // 'a'-'z'
	fHexA                    // 'A'-'F'
	fHexa                    // 'a'-'f'
	fPrint                   // 0x20-0x7E
	fGraph                   // 0x21-0x7E
)

// class maps each byte to its class flags. It is indexed by a byte, so the
// lookup needs no bounds check. It is built once in init().
var class [256]uint8

func init() {
	for b := range 128 {
		var f uint8
		switch {
		case b >= '0' && b <= '9':
			f |= fDigit
		case b >= 'A' && b <= 'Z':
			f |= fUpper
			if b <= 'F' {
				f |= fHexA
			}
		case b >= 'a' && b <= 'z':
			f |= fLower
			if b <= 'f' {
				f |= fHexa
			}
		}
		if b >= 0x20 && b <= 0x7E {
			f |= fPrint
		}
		if b >= 0x21 && b <= 0x7E {
			f |= fGraph
		}
		class[b] = f
	}
}

// IsDigit reports whether b is an ASCII decimal digit, '0'-'9'.
func IsDigit(b byte) bool { return class[b]&fDigit != 0 }

// IsUpper reports whether b is an ASCII uppercase letter, 'A'-'Z'.
func IsUpper(b byte) bool { return class[b]&fUpper != 0 }

// IsLower reports whether b is an ASCII lowercase letter, 'a'-'z'.
func IsLower(b byte) bool { return class[b]&fLower != 0 }

// IsLetter reports whether b is an ASCII letter, 'A'-'Z' or 'a'-'z'.
func IsLetter(b byte) bool { return class[b]&(fUpper|fLower) != 0 }

// IsAlnum reports whether b is an ASCII letter or decimal digit.
func IsAlnum(b byte) bool { return class[b]&(fDigit|fUpper|fLower) != 0 }

// IsPrint reports whether b is a printable ASCII byte, 0x20 (space) through
// 0x7E ('~') inclusive.
func IsPrint(b byte) bool { return class[b]&fPrint != 0 }

// IsGraph reports whether b is a visible ASCII byte, 0x21 ('!') through 0x7E
// ('~'): printable excluding the space character (C isgraph).
func IsGraph(b byte) bool { return class[b]&fGraph != 0 }

// IsHexDigit reports whether b is a hexadecimal digit of either case,
// '0'-'9', 'a'-'f', or 'A'-'F'.
func IsHexDigit(b byte) bool { return class[b]&(fDigit|fHexA|fHexa) != 0 }

// IsUpperHexDigit reports whether b is an uppercase hexadecimal digit,
// '0'-'9' or 'A'-'F'. Lowercase 'a'-'f' is rejected.
func IsUpperHexDigit(b byte) bool { return class[b]&(fDigit|fHexA) != 0 }

// IsLowerHexDigit reports whether b is a lowercase hexadecimal digit,
// '0'-'9' or 'a'-'f'. Uppercase 'A'-'F' is rejected.
func IsLowerHexDigit(b byte) bool { return class[b]&(fDigit|fHexa) != 0 }

// DigitVal returns the value 0-9 of an ASCII decimal digit and true, or
// (0, false) if b is not a decimal digit.
func DigitVal(b byte) (uint8, bool) {
	if class[b]&fDigit != 0 {
		return b - '0', true
	}
	return 0, false
}

// HexVal returns the value 0-15 of a hexadecimal digit of either case and
// true, or (0, false) if b is not a hex digit.
func HexVal(b byte) (uint8, bool) {
	switch class[b] & (fDigit | fHexA | fHexa) {
	case fDigit:
		return b - '0', true
	case fHexA:
		return b - 'A' + 10, true
	case fHexa:
		return b - 'a' + 10, true
	}
	return 0, false
}

// UpperHexVal returns the value 0-15 of an uppercase hexadecimal digit and
// true, or (0, false) otherwise. Lowercase 'a'-'f' is rejected.
func UpperHexVal(b byte) (uint8, bool) {
	switch class[b] & (fDigit | fHexA) {
	case fDigit:
		return b - '0', true
	case fHexA:
		return b - 'A' + 10, true
	}
	return 0, false
}

// LowerHexVal returns the value 0-15 of a lowercase hexadecimal digit and
// true, or (0, false) otherwise. Uppercase 'A'-'F' is rejected.
func LowerHexVal(b byte) (uint8, bool) {
	switch class[b] & (fDigit | fHexa) {
	case fDigit:
		return b - '0', true
	case fHexa:
		return b - 'a' + 10, true
	}
	return 0, false
}

// ToLower returns the ASCII lowercase form of b if b is 'A'-'Z', otherwise b
// unchanged.
func ToLower(b byte) byte {
	if class[b]&fUpper != 0 {
		return b + ('a' - 'A')
	}
	return b
}

// ToUpper returns the ASCII uppercase form of b if b is 'a'-'z', otherwise b
// unchanged.
func ToUpper(b byte) byte {
	if class[b]&fLower != 0 {
		return b - ('a' - 'A')
	}
	return b
}
