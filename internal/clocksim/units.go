package clocksim

// Nanoseconds is a float64 representing a duration in nanoseconds.
// Float64 allows sub-nanosecond precision for noise parameters (e.g., AR1.Sigma = 0.25ns).
type Nanoseconds float64

// Seconds converts Nanoseconds to seconds (float64).
func (n Nanoseconds) Seconds() float64 {
	return float64(n) / 1e9
}

// PPB is a float64 representing a fractional frequency in parts-per-billion.
// 1 PPB = 1e-9 relative frequency offset.
type PPB float64

// Fractional converts PPB to dimensionless fractional frequency.
// Replaces manual `ppb / 1e9` or `ppb * 1e-9` conversions in clocksim code.
func (p PPB) Fractional() float64 {
	return float64(p) / 1e9
}
