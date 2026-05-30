package phc

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

func TestMultiSamplePrecision(t *testing.T) {
	ms := MultiSample{
		PHC: []ptime.Time{1, 2},
		Sys: []time.Time{
			time.Unix(10, 0),
			time.Unix(10, 3),
			time.Unix(10, 20),
		},
	}
	_, _, p := ms.Extract(0)
	if p != samplePrecisionMin {
		t.Fatalf("precision for short bracket = %v, want %v", p, samplePrecisionMin)
	}
	_, _, p = ms.Extract(1)
	if p != 17*time.Nanosecond {
		t.Fatalf("precision for normal bracket = %v, want 17ns", p)
	}
}
