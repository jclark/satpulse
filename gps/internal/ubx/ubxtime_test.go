package ubx

import (
	"testing"
	"time"
)

func TestMSScaledTOW(t *testing.T) {
	ms := uint32(0x80000000)
	expected := time.Millisecond / 2
	result := msScaledTOW(ms)
	if result != expected {
		t.Errorf("scaledMSToNS(0x%X) = %d; expected %d", ms, result, expected)
	}
}
