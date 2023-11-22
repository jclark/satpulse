package gpsmsg

import (
	"fmt"
	"math"
)

var maxConv = math.Nextafter(float64(math.MaxInt64), math.Inf(-1))
var minConv = math.Nextafter(float64(math.MinInt64), math.Inf(1))

func float64ToInt64(f float64) (int64, error) {
	if minConv <= f && f <= maxConv {
		return int64(f), nil
	}
	return 0, fmt.Errorf("float64 value %v is out of int64 range", f)
}
