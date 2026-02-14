package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestMSScaledTOW(t *testing.T) {
	ms := uint32(0x80000000)
	expected := time.Millisecond / 2
	result := msScaledTOW(ms)
	if result != expected {
		t.Errorf("scaledMSToNS(0x%X) = %d; expected %d", ms, result, expected)
	}
}

func TestNavEpochTimeAcc(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	navEpochTimeAcc(&ne, 23)
	want := opt.Make(23 * time.Nanosecond)
	if ne.Acc.Time != want {
		t.Errorf("Acc.Time = %v, want %v", ne.Acc.Time, want)
	}
}
