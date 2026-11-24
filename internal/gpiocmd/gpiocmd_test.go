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
		{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Family: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
			PPS: []gpiomem.PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}, {Name: "GPIO4", Line: 4, Node: "/pps@4"}}},
		{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio"}},
	}
	tests := []struct {
		name   string
		gpio   gpiomem.GPIO
		expect []gpiomem.Controller
	}{
		{
			name: "PPS input",
			gpio: gpiomem.GPIO{Name: "GPIO4", Chip: "gpiochip0", Line: 4},
			expect: []gpiomem.Controller{{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Family: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
				PPS: []gpiomem.PPS{{Name: "GPIO4", Line: 4, Node: "/pps@4"}}}},
		},
		{
			name:   "not a PPS input",
			gpio:   gpiomem.GPIO{Name: "GPIO5", Chip: "gpiochip0", Line: 5},
			expect: []gpiomem.Controller{{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Family: "rp1", Device: "/dev/gpiomem0", Length: 0x30000}},
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
		{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Family: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
			PPS: []gpiomem.PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}, {Name: "GPIO4", Line: 4, Node: "/pps@4"}}},
		{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio", "brcm,other"}},
	}
	devices := map[string]string{"/pps@": "/dev/pps0"}
	infos := controllerInfos(ctrls, func(node string) string { return devices[node] })
	tests := []struct {
		name   string
		jsonl  bool
		expect string
	}{
		{
			name: "text",
			expect: "chip=gpiochip0 compatible=raspberrypi,rp1-gpio family=rp1 device=/dev/gpiomem0 length=0x30000 pps=GPIO18:/dev/pps0 pps=GPIO4\n" +
				"chip=gpiochip10 compatible=brcm,brcmstb-gpio compatible=brcm,other\n",
		},
		{
			name:  "JSON Lines",
			jsonl: true,
			expect: `{"chip":"gpiochip0","compatible":["raspberrypi,rp1-gpio"],"family":"rp1","device":"/dev/gpiomem0","length":196608,` +
				`"pps":[{"gpio":"GPIO18","line":18,"device":"/dev/pps0"},{"gpio":"GPIO4","line":4}]}` + "\n" +
				`{"chip":"gpiochip10","compatible":["brcm,brcmstb-gpio","brcm,other"]}` + "\n",
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
	if !reflect.DeepEqual(err, noDataError{msg: "no GPIO controllers with a device-tree node found"}) {
		t.Errorf("got %v, want noDataError", err)
	}
}
