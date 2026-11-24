//go:build linux

package gpiocmd

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/lib/gpiomem"
)

func TestParseFlags(t *testing.T) {
	cpu := 3
	tests := []struct {
		name      string
		args      []string
		expect    flagVars
		expectErr bool
	}{
		{name: "list", expect: flagVars{}},
		{name: "list one GPIO", args: []string{"-j", "gpio18"}, expect: flagVars{name: "gpio18", jsonl: true}},
		{name: "poll the PPS input", args: []string{"-p"}, expect: flagVars{poll: true, timeout: defaultTimeout}},
		{name: "poll a GPIO", args: []string{"-p", "GPIO18", "--cpu", "3", "--priority", "40", "-t", "150", "-j"},
			expect: flagVars{name: "GPIO18", poll: true, cfg: pps.GPIOConfig{CPU: &cpu, Priority: 40}, timeout: 150 * time.Second, jsonl: true}},
		{name: "poll until interrupted", args: []string{"--pps", "-t", "0"}, expect: flagVars{poll: true}},
		{name: "empty name", args: []string{""}, expectErr: true},
		{name: "two names", args: []string{"GPIO18", "GPIO4"}, expectErr: true},
		{name: "cpu without pps", args: []string{"--cpu", "3"}, expectErr: true},
		{name: "priority without pps", args: []string{"--priority", "40"}, expectErr: true},
		{name: "timeout without pps", args: []string{"-t", "5"}, expectErr: true},
		{name: "negative cpu", args: []string{"-p", "--cpu", "-1"}, expectErr: true},
		{name: "priority too high", args: []string{"-p", "--priority", "100"}, expectErr: true},
		{name: "negative priority", args: []string{"-p", "--priority", "-1"}, expectErr: true},
		{name: "negative timeout", args: []string{"-p", "-t", "-1"}, expectErr: true},
		{name: "NaN timeout", args: []string{"-p", "-t", "NaN"}, expectErr: true},
		{name: "timeout too small", args: []string{"-p", "-t", "1e-10"}, expectErr: true},
		{name: "timeout too large", args: []string{"-p", "-t", "1e300"}, expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _, err := parseFlags("gpio", tc.args)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestSelectGPIO(t *testing.T) {
	ctrls := []gpiomem.Controller{
		{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
			PPS: []gpiomem.PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}, {Name: "GPIO4", Line: 4, Node: "/pps@4"}, {Cells: []uint32{60, 0}, Node: "/pps@60"}}},
		{Node: "/soc@107c000000/gpio@7d508500", Compatible: []string{"brcm,brcmstb-gpio"}},
	}
	tests := []struct {
		name   string
		gpio   gpiomem.GPIO
		expect []gpiomem.Controller
	}{
		{
			name: "PPS input",
			gpio: gpiomem.GPIO{Name: "GPIO4", Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Line: 4},
			expect: []gpiomem.Controller{{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
				PPS: []gpiomem.PPS{{Name: "GPIO4", Line: 4, Node: "/pps@4"}}}},
		},
		{
			name:   "not a PPS input",
			gpio:   gpiomem.GPIO{Name: "GPIO5", Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Line: 5},
			expect: []gpiomem.Controller{{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000}},
		},
		{
			name:   "line 0 beside an unnamed input",
			gpio:   gpiomem.GPIO{Name: "GPIO0", Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Line: 0},
			expect: []gpiomem.Controller{{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectGPIO(ctrls, tc.gpio); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestListControllers(t *testing.T) {
	ctrls := []gpiomem.Controller{
		{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Chips: []string{"gpiochip0"},
			Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
			PPS: []gpiomem.PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}, {Name: "GPIO0", Line: 0, Node: "/pps@0"}}},
		{Node: "/pinctrl/gpio@fe760000", Compatible: []string{"rockchip,gpio-bank", "rockchip,other"},
			PPS: []gpiomem.PPS{{Cells: []uint32{24, 0}, Node: "/pps-gpio12"}}},
		{Node: "/soc/pin-controller@50002000/gpio@50002000", Chips: []string{"gpiochip0", "gpiochip1"}},
	}
	devices := map[string]string{"/pps@": "/dev/pps0", "/pps-gpio12": "/dev/pps1"}
	infos := controllerInfos(ctrls, func(node string) string { return devices[node] })
	tests := []struct {
		name   string
		jsonl  bool
		expect string
	}{
		{
			name: "text",
			expect: "/axi/pcie@1000120000/rp1/gpio@d0000\n  backend: rp1\n  compatible: raspberrypi,rp1-gpio\n  chips: gpiochip0\n" +
				"  device: /dev/gpiomem0\n  length: 0x30000\n" +
				"  pps:\n    gpio: GPIO18\n    line: 18\n    device: /dev/pps0\n  pps:\n    gpio: GPIO0\n    line: 0\n" +
				"/pinctrl/gpio@fe760000\n  compatible: rockchip,gpio-bank, rockchip,other\n  pps:\n    cells: 24, 0\n    device: /dev/pps1\n" +
				"/soc/pin-controller@50002000/gpio@50002000\n  chips: gpiochip0, gpiochip1\n",
		},
		{
			name:  "JSON Lines",
			jsonl: true,
			expect: `{"node":"/axi/pcie@1000120000/rp1/gpio@d0000","backend":"rp1","compatible":["raspberrypi,rp1-gpio"],"chips":["gpiochip0"],` +
				`"device":"/dev/gpiomem0","length":196608,"pps":[{"gpio":"GPIO18","line":18,"device":"/dev/pps0"},{"gpio":"GPIO0","line":0}]}` + "\n" +
				`{"node":"/pinctrl/gpio@fe760000","compatible":["rockchip,gpio-bank","rockchip,other"],"pps":[{"cells":[24,0],"device":"/dev/pps1"}]}` + "\n" +
				`{"node":"/soc/pin-controller@50002000/gpio@50002000","chips":["gpiochip0","gpiochip1"]}` + "\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := listControllers(infos, &out, tc.jsonl); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.String() != tc.expect {
				t.Errorf("got\n%swant\n%s", out.String(), tc.expect)
			}
		})
	}
}

func TestListNoControllers(t *testing.T) {
	err := listControllers(nil, &bytes.Buffer{}, false)
	if !reflect.DeepEqual(err, noDataError{msg: "no GPIO controllers found in the device tree"}) {
		t.Errorf("got %v, want noDataError", err)
	}
}
