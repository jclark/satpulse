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
		{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1", Device: "/dev/gpiomem0", Length: 0x30000,
			PPS: []gpiomem.PPS{{Name: "GPIO18", Line: 18, Node: "/pps@"}, {Name: "GPIO0", Line: 0, Node: "/pps@0"}}},
		{Chip: "gpiochip3", Compatible: []string{"rockchip,gpio-bank", "rockchip,other"},
			PPS: []gpiomem.PPS{{Cells: []uint32{24, 0}, Node: "/pps-gpio12"}}},
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
			expect: "gpiochip0\n  backend: rp1\n  compatible: raspberrypi,rp1-gpio\n  device: /dev/gpiomem0\n  length: 0x30000\n" +
				"  pps:\n    gpio: GPIO18\n    line: 18\n    device: /dev/pps0\n  pps:\n    gpio: GPIO0\n    line: 0\n" +
				"gpiochip3\n  compatible: rockchip,gpio-bank, rockchip,other\n  pps:\n    cells: 24, 0\n    device: /dev/pps1\n",
		},
		{
			name:  "JSON Lines",
			jsonl: true,
			expect: `{"chip":"gpiochip0","backend":"rp1","compatible":["raspberrypi,rp1-gpio"],"device":"/dev/gpiomem0","length":196608,` +
				`"pps":[{"gpio":"GPIO18","line":18,"device":"/dev/pps0"},{"gpio":"GPIO0","line":0}]}` + "\n" +
				`{"chip":"gpiochip3","compatible":["rockchip,gpio-bank","rockchip,other"],"pps":[{"cells":[24,0],"device":"/dev/pps1"}]}` + "\n",
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
