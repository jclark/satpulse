package rp1

import (
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
)

var pi5 = soc.Controller{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}}

func TestResolve(t *testing.T) {
	tests := []struct {
		name       string
		gpio       string
		ctrls      []soc.Controller
		expectLine int
		expectErr  bool
	}{
		{name: "GPIO18", gpio: "GPIO18", ctrls: []soc.Controller{pi5}, expectLine: 18},
		{name: "last", gpio: "GPIO53", ctrls: []soc.Controller{pi5}, expectLine: 53},
		{name: "out of range", gpio: "GPIO54", ctrls: []soc.Controller{pi5}, expectErr: true},
		{name: "bad name", gpio: "GPIO3_D0", ctrls: []soc.Controller{pi5}, expectErr: true},
		{name: "two controllers", gpio: "GPIO18", ctrls: []soc.Controller{pi5, pi5}, expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl, line, err := SoC{}.Resolve(tc.gpio, tc.ctrls)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ctrl != 0 || line != tc.expectLine {
				t.Errorf("got %d, %d; want 0, %d", ctrl, line, tc.expectLine)
			}
		})
	}
}

func TestLevel(t *testing.T) {
	tests := []struct {
		line         int
		expectOffset int
		expectBit    uint
	}{
		{18, 0x10008, 18},
		{27, 0x10008, 27},
		{28, 0x14008, 0},
		{30, 0x14008, 2},
		{34, 0x18008, 0},
		{40, 0x18008, 6},
		{53, 0x18008, 19},
	}
	for _, tc := range tests {
		if offset, bit := (SoC{}).Level(pi5, tc.line); offset != tc.expectOffset || bit != tc.expectBit {
			t.Errorf("Level(%d) = %#x, %d; want %#x, %d", tc.line, offset, bit, tc.expectOffset, tc.expectBit)
		}
	}
}
