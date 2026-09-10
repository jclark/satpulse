//go:build linux

package gpiomem

import "testing"

func TestModelFor(t *testing.T) {
	m, ok := modelFor([]byte("raspberrypi,5-model-b\x00brcm,bcm2712\x00"))
	if !ok || m.soc != "brcm,bcm2712" {
		t.Errorf("modelFor(Pi 5) = %v, %v; want bcm2712", m.soc, ok)
	}
	if _, ok := modelFor([]byte("brcm,bcm2835\x00")); ok {
		t.Error("modelFor(bcm2835) found a model; want unsupported")
	}
}

func TestLevel(t *testing.T) {
	for _, tc := range []struct {
		level      func(int) (int, uint)
		gpio       int
		wantOffset int
		wantBit    uint
	}{
		{bcm283xLevel, 18, 0x34, 18},
		{bcm283xLevel, 40, 0x38, 8},
		{rp1Level, 18, 0x10008, 18},
		{rp1Level, 30, 0x14008, 2},
		{rp1Level, 40, 0x18008, 6},
	} {
		if offset, bit := tc.level(tc.gpio); offset != tc.wantOffset || bit != tc.wantBit {
			t.Errorf("level(%d) = %#x, %d; want %#x, %d", tc.gpio, offset, bit, tc.wantOffset, tc.wantBit)
		}
	}
}
