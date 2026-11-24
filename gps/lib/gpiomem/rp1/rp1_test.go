package rp1

import (
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
)

var pi5 = soc.Controller{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}}

func TestLookup(t *testing.T) {
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
			ctrl, line, err := SoC{}.Lookup(tc.gpio, tc.ctrls)
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

func TestName(t *testing.T) {
	tests := []struct {
		name     string
		ctrl     soc.Controller
		line     int
		expect   string
		expectOK bool
	}{
		{name: "GPIO18", ctrl: pi5, line: 18, expect: "GPIO18", expectOK: true},
		{name: "last", ctrl: pi5, line: 53, expect: "GPIO53", expectOK: true},
		{name: "out of range", ctrl: pi5, line: 54},
		{name: "negative", ctrl: pi5, line: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := SoC{}.Name([]soc.Controller{tc.ctrl}, 0, tc.line)
			if got != tc.expect || ok != tc.expectOK {
				t.Errorf("got %q, %v; want %q, %v", got, ok, tc.expect, tc.expectOK)
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
