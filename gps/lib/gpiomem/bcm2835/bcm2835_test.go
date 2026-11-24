package bcm2835

import (
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
)

var (
	pi3 = soc.Controller{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2835-gpio"}}
	pi4 = soc.Controller{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2711-gpio"}}
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name       string
		gpio       string
		ctrls      []soc.Controller
		expectLine int
		expectErr  bool
	}{
		{name: "Pi 3", gpio: "GPIO18", ctrls: []soc.Controller{pi3}, expectLine: 18},
		{name: "Pi 3 last", gpio: "GPIO53", ctrls: []soc.Controller{pi3}, expectLine: 53},
		{name: "Pi 3 out of range", gpio: "GPIO54", ctrls: []soc.Controller{pi3}, expectErr: true},
		{name: "Pi 4 last", gpio: "GPIO57", ctrls: []soc.Controller{pi4}, expectLine: 57},
		{name: "Pi 4 out of range", gpio: "GPIO58", ctrls: []soc.Controller{pi4}, expectErr: true},
		{name: "bad name", gpio: "PD4", ctrls: []soc.Controller{pi4}, expectErr: true},
		{name: "two controllers", gpio: "GPIO18", ctrls: []soc.Controller{pi3, pi4}, expectErr: true},
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
		{name: "Pi 4 GPIO18", ctrl: pi4, line: 18, expect: "GPIO18", expectOK: true},
		{name: "Pi 4 last", ctrl: pi4, line: 57, expect: "GPIO57", expectOK: true},
		{name: "Pi 3 beyond its last", ctrl: pi3, line: 54},
		{name: "negative", ctrl: pi4, line: -1},
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
		{18, 0x34, 18},
		{31, 0x34, 31},
		{32, 0x38, 0},
		{40, 0x38, 8},
	}
	for _, tc := range tests {
		if offset, bit := (SoC{}).Level(pi4, tc.line); offset != tc.expectOffset || bit != tc.expectBit {
			t.Errorf("Level(%d) = %#x, %d; want %#x, %d", tc.line, offset, bit, tc.expectOffset, tc.expectBit)
		}
	}
}
