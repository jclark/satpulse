package gpiomem

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem/backend"
	"github.com/jclark/satpulse/gps/lib/gpiomem/bcm2835"
	"github.com/jclark/satpulse/gps/lib/gpiomem/rp1"
)

// The controllers of real systems, as their sysfs gpiochips report them.
var (
	pi5Ctrls = []backend.Controller{
		{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}},
		{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Chip: "gpiochip11", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Chip: "gpiochip12", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Chip: "gpiochip13", Compatible: []string{"brcm,brcmstb-gpio"}},
	}
	pi4Ctrls = []backend.Controller{
		{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2711-gpio"}},
		{Chip: "gpiochip1", Compatible: []string{"raspberrypi,firmware-gpio"}},
	}
	pi3Ctrls = []backend.Controller{
		{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2835-gpio"}},
		{Chip: "gpiochip1", Compatible: []string{"brcm,bcm2835-virtgpio"}},
		{Chip: "gpiochip2", Compatible: []string{"raspberrypi,firmware-gpio"}},
	}
	rk3568Ctrls = []backend.Controller{
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

func TestFindBackend(t *testing.T) {
	tests := []struct {
		name        string
		ctrls       []backend.Controller
		expect      backend.Backend
		expectCtrls []backend.Controller
		expectErr   bool
	}{
		{name: "Pi 5", ctrls: pi5Ctrls, expect: rp1.Backend{}, expectCtrls: pi5Ctrls[:1]},
		{name: "Pi 4", ctrls: pi4Ctrls, expect: bcm2835.Backend{}, expectCtrls: pi4Ctrls[:1]},
		{name: "Pi 3", ctrls: pi3Ctrls, expect: bcm2835.Backend{}, expectCtrls: pi3Ctrls[:1]},
		{name: "two backends", ctrls: append(slices.Clone(pi5Ctrls), pi4Ctrls...), expectErr: true},
		{name: "unsupported controller", ctrls: rk3568Ctrls, expectErr: true},
		{name: "no controllers", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			be, ctrls, err := findBackend(tc.ctrls)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if be != tc.expect || !reflect.DeepEqual(ctrls, tc.expectCtrls) {
				t.Errorf("got  %T %+v\nwant %T %+v", be, ctrls, tc.expect, tc.expectCtrls)
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

func TestDescribe(t *testing.T) {
	pi5 := slices.Clone(pi5Ctrls)
	pi5[0].Phandle, pi5[0].GPIOCells = 0x2e, 2
	pi5[1].Phandle, pi5[1].GPIOCells = 0x30, 2
	pps := ppsNode{path: "/pps@", gpios: []uint32{0x2e, 18, 0}}
	tests := []struct {
		name   string
		ctrls  []backend.Controller
		nodes  []ppsNode
		expect []Controller
	}{
		{
			name:  "Pi 5 with pps-gpio on GPIO18",
			ctrls: pi5[:2],
			nodes: []ppsNode{pps},
			expect: []Controller{
				{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
					PPS: []PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}}},
				{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio"}},
			},
		},
		{
			name:  "pps-gpio on an unsupported controller",
			ctrls: pi5[:2],
			nodes: []ppsNode{{path: "/pps@", gpios: []uint32{0x30, 5, 0}}},
			expect: []Controller{
				{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000},
				{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio"}, PPS: []PPS{{Cells: []uint32{5, 0}, Node: "/pps@"}}},
			},
		},
		{
			name:  "line out of range",
			ctrls: pi5[:1],
			nodes: []ppsNode{{path: "/pps@", gpios: []uint32{0x2e, 54, 0}}},
			expect: []Controller{
				{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000},
			},
		},
		{
			name:  "incomplete specifier",
			ctrls: pi5[:1],
			nodes: []ppsNode{{path: "/pps@", gpios: []uint32{0x2e, 18}}},
			expect: []Controller{
				{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000},
			},
		},
		{
			name:  "Pi 4 without a phandle",
			ctrls: pi4Ctrls[:1],
			nodes: []ppsNode{{path: "/pps@12", gpios: []uint32{0, 18, 0}}},
			expect: []Controller{
				{Chip: "gpiochip0", Compatible: []string{"brcm,bcm2711-gpio"}, Backend: "bcm2835", Device: "/dev/gpiomem", Length: 0x1000},
			},
		},
		{
			name:  "unsupported SoC",
			ctrls: rk3568Ctrls[:1],
			expect: []Controller{
				{Chip: "gpiochip0", Compatible: []string{"rockchip,gpio-bank"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := describe(tc.ctrls, tc.nodes); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestCells(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []uint32
	}{
		{"gpios", "\x00\x00\x00\x2e\x00\x00\x00\x12\x00\x00\x00\x00", []uint32{0x2e, 18, 0}},
		{"trailing bytes", "\x00\x00\x00\x01\x02", []uint32{1}},
		{"empty", "", []uint32{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cells([]byte(tc.input)); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
}
