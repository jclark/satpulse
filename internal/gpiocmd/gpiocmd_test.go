//go:build linux

package gpiocmd

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/gpiomem"
)

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
