package tai

import (
	"fmt"
	"time"
)

// Time in TAI timescale represented as nanoseconds since 1970-01-01T00:00:00 TAI
type Time int64

// GPS epoch
var epochGPS = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

// Offset in seconds between TAI and UTC at GPS epoch
const epochGPSOffset = 19

// week is number of complete weeks since start of first Sunday in 1980
// iTOW is milliseconds since start of week (Sunday)
func GPS(week int16, iTOW uint32) Time {
	weekMillis := int64(week) * 7 * 24 * 60 * 60 * 1000
	weekMillis += epochGPS.UnixMilli() + epochGPSOffset*1000
	return Time((weekMillis + int64(iTOW)) * 1e6)
}

func (t Time) String() string {
	n := int64(t)
	// FIXME deal with negative here
	return fmt.Sprintf("%d.%09d", n/1e9, n%1e9)
}
