// Package decconv converts between base-10 decimal strings and int64 scaled
// integers without floating point.
package decconv

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// MaxDecimals is the maximum supported number of decimal places.
const MaxDecimals = 18

var (
	ErrInvalid    = errors.New("invalid decimal")
	ErrTooPrecise = errors.New("too precise for scale factor")
	ErrOverflow   = errors.New("overflow")

	// Accept JSON number syntax only.
	jsonNumberRE = regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$`)
)

// ParseInt64 parses s as a JSON number literal and returns an int64 equal to
// s * 10^decimals, requiring the result to be an exact integer.
// Accepted syntax matches JSON numbers (e.g. no leading '+', no leading zeros
// except "0", digits required around '.', optional exponent).
// Panics if decimals is not in [0, MaxDecimals].
func ParseInt64(s string, decimals int) (int64, error) {
	checkDecimals(decimals)

	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalid
	}
	if !jsonNumberRE.MatchString(s) {
		return 0, ErrInvalid
	}

	var r big.Rat
	if _, ok := r.SetString(s); !ok {
		return 0, ErrInvalid
	}

	// r *= 10^decimals
	var scale big.Int
	scale.Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	var sf big.Rat
	sf.SetInt(&scale)
	r.Mul(&r, &sf)

	// Must be an exact integer in the chosen scale.
	if !r.IsInt() {
		return 0, ErrTooPrecise
	}

	n := r.Num() // *big.Int

	// Range check to int64.
	if !n.IsInt64() {
		return 0, ErrOverflow
	}

	v := n.Int64()
	return v, nil
}

const zeros = "000000000000000000" // MaxDecimals zeros

var pow10 = [MaxDecimals + 1]uint64{
	1,
	10,
	100,
	1000,
	10000,
	100000,
	1000000,
	10000000,
	100000000,
	1000000000,
	10000000000,
	100000000000,
	1000000000000,
	10000000000000,
	100000000000000,
	1000000000000000,
	10000000000000000,
	100000000000000000,
	1000000000000000000,
}

// FormatInt64 formats v/10^decimals as a decimal string, stripping
// trailing fractional zeros but keeping at least minFrac digits after the
// decimal point. Panics if decimals is not in [0, MaxDecimals] or minFrac
// is not in [0, decimals].
func FormatInt64(v int64, decimals, minFrac int) string {
	checkDecimals(decimals)
	if minFrac < 0 || minFrac > decimals {
		panic("minFrac out of range [0, decimals]")
	}
	neg := v < 0
	u := uint64(v)
	if neg {
		u = -u
	}
	scale := pow10[decimals]
	s := strconv.FormatUint(u/scale, 10)
	if neg {
		s = "-" + s
	}
	if decimals == 0 {
		return s
	}
	frac := strconv.FormatUint(u%scale, 10)
	if pad := decimals - len(frac); pad > 0 {
		frac = zeros[:pad] + frac
	}
	end := len(frac)
	for end > minFrac && frac[end-1] == '0' {
		end--
	}
	if end == 0 {
		return s
	}
	return s + "." + frac[:end]
}

func checkDecimals(decimals int) {
	if decimals < 0 || decimals > MaxDecimals {
		panic("decimals out of range [0, MaxDecimals]")
	}
}
