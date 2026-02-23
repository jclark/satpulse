package decconv

import (
	"math"
	"testing"
)

func TestParseInt64(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		decimals int
		want     int64
		wantErr  error
	}{
		// Basic integers.
		{"zero", "0", 0, 0, nil},
		{"one", "1", 0, 1, nil},
		{"negative", "-1", 0, -1, nil},
		{"large int", "123456789", 0, 123456789, nil},
		// Decimals=0 with fractional input.
		{"frac with zero decimals", "1.5", 0, 0, ErrTooPrecise},
		// Scaling.
		{"scale 1", "1.5", 1, 15, nil},
		{"scale 2", "1.23", 2, 123, nil},
		{"scale 3", "0.001", 3, 1, nil},
		{"integer scaled", "42", 3, 42000, nil},
		{"negative scaled", "-1.25", 2, -125, nil},
		// Whitespace trimming.
		{"leading space", "  1", 0, 1, nil},
		{"trailing space", "1  ", 0, 1, nil},
		// Exponent notation.
		{"exponent", "1e3", 0, 1000, nil},
		{"exponent negative", "1e-3", 3, 1, nil},
		{"exponent with plus", "5E+2", 0, 500, nil},
		{"exponent fractional", "1.5e2", 0, 150, nil},
		// Too precise.
		{"too precise", "1.111", 2, 0, ErrTooPrecise},
		{"too precise repeating", "1e-1", 0, 0, ErrTooPrecise},
		// Overflow.
		{"overflow positive", "9223372036854775808", 0, 0, ErrOverflow},
		{"overflow negative", "-9223372036854775809", 0, 0, ErrOverflow},
		{"overflow from scale", "10", 18, 0, ErrOverflow},
		// Max/min int64.
		{"max int64", "9223372036854775807", 0, math.MaxInt64, nil},
		{"min int64", "-9223372036854775808", 0, math.MinInt64, nil},
		{"max int64 scaled", "9223372036854775.807", 3, math.MaxInt64, nil},
		{"max int64 scaled overflow", "9223372036854775.808", 3, 0, ErrOverflow},
		{"min int64 scaled", "-9223372036854775.808", 3, math.MinInt64, nil},
		{"min int64 scaled overflow", "-9223372036854775.809", 3, 0, ErrOverflow},
		// Invalid syntax.
		{"empty", "", 0, 0, ErrInvalid},
		{"only spaces", "   ", 0, 0, ErrInvalid},
		{"leading plus", "+1", 0, 0, ErrInvalid},
		{"leading zero", "01", 0, 0, ErrInvalid},
		{"trailing dot", "1.", 0, 0, ErrInvalid},
		{"leading dot", ".1", 0, 0, ErrInvalid},
		{"letters", "abc", 0, 0, ErrInvalid},
		{"double negative", "--1", 0, 0, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInt64(tt.s, tt.decimals)
			if err != tt.wantErr {
				t.Fatalf("ParseInt64(%q, %d) error = %v, want %v", tt.s, tt.decimals, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ParseInt64(%q, %d) = %d, want %d", tt.s, tt.decimals, got, tt.want)
			}
		})
	}
}

func TestParseInt64_BadDecimals(t *testing.T) {
	for _, d := range []int{-1, MaxDecimals + 1} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			ParseInt64("0", d)
		})
	}
}

func TestFormatInt64(t *testing.T) {
	tests := []struct {
		name     string
		v        int64
		decimals int
		minFrac  int
		want     string
	}{
		// No decimals.
		{"integer zero", 0, 0, 0, "0"},
		{"integer pos", 42, 0, 0, "42"},
		{"integer neg", -7, 0, 0, "-7"},
		// Trailing zero stripping.
		{"strip all", 12300, 5, 0, "0.123"},
		{"strip some", 12340, 5, 0, "0.1234"},
		{"strip none", 12345, 5, 0, "0.12345"},
		{"all frac zeros", 0, 3, 0, "0"},
		// minFrac preservation.
		{"minFrac keeps zeros", 0, 3, 2, "0.00"},
		{"minFrac partial strip", 12300, 5, 4, "0.1230"},
		{"minFrac no effect", 12345, 5, 2, "0.12345"},
		{"minFrac equals decimals", 100, 3, 3, "0.100"},
		// Integer part with fractional.
		{"int and frac", 1500, 3, 0, "1.5"},
		{"int and frac minFrac", 1500, 3, 2, "1.50"},
		{"large int part", 123456789, 2, 0, "1234567.89"},
		// Negative values.
		{"neg frac", -1500, 3, 0, "-1.5"},
		{"neg strip", -12300, 5, 0, "-0.123"},
		{"neg int only", -100, 2, 0, "-1"},
		{"neg minFrac", -100, 2, 2, "-1.00"},
		// Boundary values.
		{"max int64", math.MaxInt64, 0, 0, "9223372036854775807"},
		{"min int64", math.MinInt64, 0, 0, "-9223372036854775808"},
		{"max int64 scaled", math.MaxInt64, 3, 0, "9223372036854775.807"},
		{"min int64 scaled", math.MinInt64, 3, 0, "-9223372036854775.808"},
		// Sub-unit values (integer part is zero).
		{"small frac", 1, 3, 0, "0.001"},
		{"small frac minFrac", 10, 3, 1, "0.01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatInt64(tt.v, tt.decimals, tt.minFrac)
			if got != tt.want {
				t.Fatalf("FormatInt64(%d, %d, %d) = %q, want %q", tt.v, tt.decimals, tt.minFrac, got, tt.want)
			}
		})
	}
}

func TestFormatInt64_BadMinFrac(t *testing.T) {
	for _, mf := range []int{-1, 4} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			FormatInt64(0, 3, mf)
		})
	}
}
