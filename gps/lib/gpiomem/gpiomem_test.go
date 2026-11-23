package gpiomem

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem/bcm2835"
	"github.com/jclark/satpulse/gps/lib/gpiomem/rp1"
	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
)

// The controllers of real systems, as their sysfs gpiochips report them.
var (
	pi5Ctrls = []soc.Controller{
		{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}},
		{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Chip: "gpiochip11", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Chip: "gpiochip12", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Chip: "gpiochip13", Compatible: []string{"brcm,brcmstb-gpio"}},
	}
	pi4Ctrls = []soc.Controller{
		{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2711-gpio"}},
		{Chip: "gpiochip1", Compatible: []string{"raspberrypi,firmware-gpio"}},
	}
	pi3Ctrls = []soc.Controller{
		{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2835-gpio"}},
		{Chip: "gpiochip1", Compatible: []string{"brcm,bcm2835-virtgpio"}},
		{Chip: "gpiochip2", Compatible: []string{"raspberrypi,firmware-gpio"}},
	}
	rk3568Ctrls = []soc.Controller{
		{Chip: "gpiochip0", Compatible: []string{"rockchip,gpio-bank"}},
		{Chip: "gpiochip1", Compatible: []string{"rockchip,gpio-bank"}},
	}
)

func TestUpperName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expect    string
		expectErr bool
	}{
		{name: "upper case", input: "GPIO18", expect: "GPIO18"},
		{name: "lower case", input: "gpio18", expect: "GPIO18"},
		{name: "mixed case", input: "Gpio3_d0", expect: "GPIO3_D0"},
		{name: "non-ASCII folding to ASCII", input: "gp\u0131o18", expectErr: true},
		{name: "empty", input: "", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := upperName(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestFindSoC(t *testing.T) {
	tests := []struct {
		name        string
		ctrls       []soc.Controller
		expect      soc.SoC
		expectCtrls []soc.Controller
		expectErr   bool
	}{
		{name: "Pi 5", ctrls: pi5Ctrls, expect: rp1.SoC{}, expectCtrls: pi5Ctrls[:1]},
		{name: "Pi 4", ctrls: pi4Ctrls, expect: bcm2835.SoC{}, expectCtrls: pi4Ctrls[:1]},
		{name: "Pi 3", ctrls: pi3Ctrls, expect: bcm2835.SoC{}, expectCtrls: pi3Ctrls[:1]},
		{name: "two families", ctrls: append(slices.Clone(pi5Ctrls), pi4Ctrls...), expectErr: true},
		{name: "unsupported controller", ctrls: rk3568Ctrls, expectErr: true},
		{name: "no controllers", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, ctrls, err := findSoC(tc.ctrls)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s != tc.expect || !reflect.DeepEqual(ctrls, tc.expectCtrls) {
				t.Errorf("got  %T %+v\nwant %T %+v", s, ctrls, tc.expect, tc.expectCtrls)
			}
		})
	}
}

func TestCompatibles(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"one", "raspberrypi,rp1-gpio\x00", []string{"raspberrypi,rp1-gpio"}},
		{"two", "fsl,imx8mp-gpio\x00fsl,imx35-gpio\x00", []string{"fsl,imx8mp-gpio", "fsl,imx35-gpio"}},
		{"unterminated", "brcm,bcm2711-gpio", []string{"brcm,bcm2711-gpio"}},
		{"empty", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := compatibles([]byte(tc.input)); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %q\nwant %q", got, tc.expect)
			}
		})
	}
}
