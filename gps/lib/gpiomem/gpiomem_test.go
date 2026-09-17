//go:build linux && arm64

package gpiomem

import "testing"

func TestModelFor(t *testing.T) {
	for _, tc := range []struct {
		compatible string
		expectSoC  string
		expectOK   bool
	}{
		{"raspberrypi,5-model-b\x00brcm,bcm2712\x00", "brcm,bcm2712", true},
		{"raspberrypi,4-model-b\x00brcm,bcm2711\x00", "brcm,bcm2711", true},
		{"brcm,bcm2835\x00", "", false},
	} {
		if m, ok := modelFor([]byte(tc.compatible)); ok != tc.expectOK || m.soc != tc.expectSoC {
			t.Errorf("modelFor(%q) = %q, %v; want %q, %v", tc.compatible, m.soc, ok, tc.expectSoC, tc.expectOK)
		}
	}
}

func TestReg(t *testing.T) {
	for _, tc := range []struct {
		reg          func(int) (int, uint)
		gpio         int
		expectOffset int
		expectBit    uint
	}{
		{bcm283xReg, 18, 0x34, 18},
		{bcm283xReg, 40, 0x38, 8},
		{rp1Reg, 18, 0x10008, 18},
		{rp1Reg, 30, 0x14008, 2},
		{rp1Reg, 40, 0x18008, 6},
	} {
		if offset, bit := tc.reg(tc.gpio); offset != tc.expectOffset || bit != tc.expectBit {
			t.Errorf("reg(%d) = %#x, %d; want %#x, %d", tc.gpio, offset, bit, tc.expectOffset, tc.expectBit)
		}
	}
}
