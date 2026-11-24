package gpiomem

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem/backend"
	"github.com/jclark/satpulse/gps/lib/gpiomem/bcm2835"
	"github.com/jclark/satpulse/gps/lib/gpiomem/rp1"
)

// The controllers of real systems, as their device trees give them.
var (
	pi5Ctrls = []backend.Controller{
		{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}},
		{Node: "/soc@107c000000/gpio@7d508500", Compatible: []string{"brcm,brcmstb-gpio"}},
		{Node: "/soc@107c000000/gpio@7d517c00", Compatible: []string{"brcm,brcmstb-gpio"}},
	}
	pi4Ctrls = []backend.Controller{
		{Node: "/soc/firmware/gpio", Compatible: []string{"raspberrypi,firmware-gpio"}},
		{Node: "/soc/gpio@7e200000", Compatible: []string{"brcm,bcm2711-gpio"}},
	}
	pi3Ctrls = []backend.Controller{
		{Node: "/soc/firmware/expgpio", Compatible: []string{"raspberrypi,firmware-gpio"}},
		{Node: "/soc/firmware/virtgpio", Compatible: []string{"brcm,bcm2835-virtgpio"}},
		{Node: "/soc/gpio@7e200000", Compatible: []string{"brcm,bcm2835-gpio"}},
	}
	rk3568Ctrls = []backend.Controller{
		{Node: "/pinctrl/gpio@fdd60000", Compatible: []string{"rockchip,gpio-bank"}},
		{Node: "/pinctrl/gpio@fe740000", Compatible: []string{"rockchip,gpio-bank"}},
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
		{name: "Pi 4", ctrls: pi4Ctrls, expect: bcm2835.Backend{}, expectCtrls: pi4Ctrls[1:]},
		{name: "Pi 3", ctrls: pi3Ctrls, expect: bcm2835.Backend{}, expectCtrls: pi3Ctrls[2:]},
		{name: "two backends", ctrls: append(slices.Clone(pi5Ctrls), pi4Ctrls...), expectErr: true},
		{name: "unsupported controller", ctrls: rk3568Ctrls, expectErr: true},
		{name: "controller without compatible", ctrls: []backend.Controller{{Node: "/soc/pin-controller@50002000/gpio@50002000"}}, expectErr: true},
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
	pi5[0].Phandle, pi5[1].Phandle = 0x2e, 0x30
	providers := map[uint32]provider{0x2e: {cells: 2, available: true}, 0x30: {cells: 2, available: true}}
	pps := ppsNode{path: "/pps@", gpios: []uint32{0x2e, 18, 0}}
	pi5Chips := map[string][]string{
		"/axi/pcie@1000120000/rp1/gpio@d0000": {"gpiochip0"},
		"/soc@107c000000/gpio@7d508500":       {"gpiochip10", "gpiochip13"},
	}
	rp1 := Controller{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000}
	tests := []struct {
		name   string
		ctrls  []backend.Controller
		chips  map[string][]string
		nodes  []ppsNode
		expect []Controller
	}{
		{
			name:  "Pi 5 with pps-gpio on GPIO18",
			ctrls: pi5[:2],
			chips: pi5Chips,
			nodes: []ppsNode{pps},
			expect: []Controller{
				{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Chips: []string{"gpiochip0"},
					Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000, PPS: []PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}}},
				{Node: "/soc@107c000000/gpio@7d508500", Compatible: []string{"brcm,brcmstb-gpio"}, Chips: []string{"gpiochip10", "gpiochip13"}},
			},
		},
		{
			name:  "pps-gpio on an unsupported controller",
			ctrls: pi5[:2],
			nodes: []ppsNode{{path: "/pps@", gpios: []uint32{0x30, 5, 0}}},
			expect: []Controller{
				rp1,
				{Node: "/soc@107c000000/gpio@7d508500", Compatible: []string{"brcm,brcmstb-gpio"}, PPS: []PPS{{Cells: []uint32{5, 0}, Node: "/pps@"}}},
			},
		},
		{
			name:  "line out of range",
			ctrls: pi5[:1],
			nodes: []ppsNode{{path: "/pps@", gpios: []uint32{0x2e, 54, 0}}},
			expect: []Controller{
				rp1,
			},
		},
		{
			name:  "incomplete specifier",
			ctrls: pi5[:1],
			nodes: []ppsNode{{path: "/pps@", gpios: []uint32{0x2e, 18}}},
			expect: []Controller{
				rp1,
			},
		},
		{
			name:  "Pi 4 without a phandle",
			ctrls: pi4Ctrls[1:],
			nodes: []ppsNode{{path: "/pps@12", gpios: []uint32{0, 18, 0}}},
			expect: []Controller{
				{Node: "/soc/gpio@7e200000", Compatible: []string{"brcm,bcm2711-gpio"}, Backend: "bcm2835", Device: "/dev/gpiomem", Length: 0x1000},
			},
		},
		{
			name:  "unsupported SoC",
			ctrls: rk3568Ctrls[:1],
			expect: []Controller{
				{Node: "/pinctrl/gpio@fdd60000", Compatible: []string{"rockchip,gpio-bank"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := describe(tc.ctrls, providers, tc.chips, tc.nodes); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	providers := map[uint32]provider{
		1: {cells: 2, available: true},
		2: {cells: 2},
		// A header whose pin 0 is line 18 of controller 1 (after an entry
		// for disabled controller 2), pin 1 line 5, with the flags cell
		// ignored in matching and its bit 0 passed through.
		3: {cells: 2, available: true, gpioMap: []uint32{0, 0, 2, 7, 0, 0, 0, 1, 18, 0, 1, 0, 1, 5, 0},
			mask: []uint32{0xff, 0}, passThru: []uint32{0, 1}},
		// A connector whose pin 9 is pin 1 of header 3.
		4: {cells: 1, gpioMap: []uint32{9, 3, 1, 0}},
		5: {cells: 2, gpioMap: []uint32{0, 0, 5, 0, 0}},
	}
	tests := []struct {
		name          string
		gpios         []uint32
		expectPhandle uint32
		expect        []uint32
		expectOK      bool
	}{
		{name: "controller", gpios: []uint32{1, 18, 0}, expectPhandle: 1, expect: []uint32{18, 0}, expectOK: true},
		{name: "nexus, skipping a disabled parent", gpios: []uint32{3, 0, 0}, expectPhandle: 1, expect: []uint32{18, 0}, expectOK: true},
		{name: "masked bits", gpios: []uint32{3, 0x100, 0}, expectPhandle: 1, expect: []uint32{18, 0}, expectOK: true},
		{name: "passed-through flags", gpios: []uint32{3, 1, 1}, expectPhandle: 1, expect: []uint32{5, 1}, expectOK: true},
		{name: "two nexus nodes", gpios: []uint32{4, 9}, expectPhandle: 1, expect: []uint32{5, 0}, expectOK: true},
		{name: "no matching entry", gpios: []uint32{3, 2, 0}},
		{name: "loop of nexus nodes", gpios: []uint32{5, 0, 0}},
		{name: "unknown phandle", gpios: []uint32{7, 0, 0}},
		{name: "incomplete specifier", gpios: []uint32{1, 18}},
		{name: "empty", gpios: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			phandle, spec, ok := resolve(providers, tc.gpios)
			if phandle != tc.expectPhandle || !reflect.DeepEqual(spec, tc.expect) || ok != tc.expectOK {
				t.Errorf("got  %d %v %v\nwant %d %v %v", phandle, spec, ok, tc.expectPhandle, tc.expect, tc.expectOK)
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
