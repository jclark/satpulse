package gpiomem

import (
	"reflect"
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

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		gpio      string
		ctrls     []soc.Controller
		expect    gpio
		expectErr bool
	}{
		{name: "Pi 5", gpio: "GPIO18", ctrls: pi5Ctrls, expect: gpio{rp1.SoC{}, pi5Ctrls[0], 18}},
		{name: "lower case", gpio: "gpio18", ctrls: pi5Ctrls, expect: gpio{rp1.SoC{}, pi5Ctrls[0], 18}},
		{name: "mixed case", gpio: "Gpio27", ctrls: pi5Ctrls, expect: gpio{rp1.SoC{}, pi5Ctrls[0], 27}},
		{name: "Pi 4", gpio: "GPIO57", ctrls: pi4Ctrls, expect: gpio{bcm2835.SoC{}, pi4Ctrls[0], 57}},
		{name: "Pi 3", gpio: "GPIO18", ctrls: pi3Ctrls, expect: gpio{bcm2835.SoC{}, pi3Ctrls[0], 18}},
		{name: "Pi 3 out of range", gpio: "GPIO57", ctrls: pi3Ctrls, expectErr: true},
		{name: "number only", gpio: "18", ctrls: pi5Ctrls, expectErr: true},
		{name: "non-ASCII folding to ASCII", gpio: "gp\u0131o18", ctrls: pi5Ctrls, expectErr: true},
		{name: "empty", gpio: "", ctrls: pi5Ctrls, expectErr: true},
		{name: "unsupported controller", gpio: "GPIO3_D0", ctrls: rk3568Ctrls, expectErr: true},
		{name: "no controllers", gpio: "GPIO18", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, err := resolve(tc.gpio, tc.ctrls)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(g, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", g, tc.expect)
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
