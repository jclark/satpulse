package ubx

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestSplitLength(t *testing.T) {
	const mm10 = gpsprot.Micrometer * 100

	testCases := []struct {
		length       gpsprot.Length
		expectedCm   int32
		expectedMm10 int8
	}{
		{105 * mm10, 1, 5},
		{250 * mm10, 3, -50},
		{-105 * mm10, -1, -5},
		{-250 * mm10, -3, 50},
		{10475 * gpsprot.Micrometer, 1, 5},
	}

	for _, tc := range testCases {
		cm, mm10, err := splitLength(tc.length)
		if err != nil {
			t.Errorf("splitLength returned error: %v", err)
		} else if mm10 < -99 || mm10 > 99 {
			t.Errorf("splitLength(%v) = (%v, %v), want mm10 in [-99, 99]", tc.length, cm, mm10)
		} else if cm != tc.expectedCm || mm10 != tc.expectedMm10 {
			t.Errorf("splitLength(%v) = (%v, %v), want (%v, %v)", tc.length, cm, mm10, tc.expectedCm, tc.expectedMm10)
		}
	}
}
