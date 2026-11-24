//go:build linux

package gpiocmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/lib/gpiomem"
)

func TestOnlyPPSInput(t *testing.T) {
	rp1 := gpiomem.Controller{Node: "/axi/pcie@1000120000/rp1/gpio@d0000", Compatible: []string{"raspberrypi,rp1-gpio"}, Backend: "rp1"}
	brcm := gpiomem.Controller{Node: "/soc@107c000000/gpio@7d508500", Compatible: []string{"brcm,brcmstb-gpio"}}
	gpio18 := gpiomem.PPS{Name: "GPIO18", Line: 18, Node: "/pps@"}
	gpio4 := gpiomem.PPS{Name: "GPIO4", Line: 4, Node: "/pps@4"}
	unnamed := gpiomem.PPS{Cells: []uint32{24, 0}, Node: "/pps-gpio12"}
	rk := gpiomem.Controller{Node: "/pinctrl/gpio@fe760000", Compatible: []string{"rockchip,gpio-bank"}, PPS: []gpiomem.PPS{unnamed}}
	tests := []struct {
		name      string
		pps       []gpiomem.PPS
		ctrls     []gpiomem.Controller
		expect    string
		expectErr string
	}{
		{name: "one", pps: []gpiomem.PPS{gpio18}, expect: "GPIO18"},
		{name: "none", expectErr: "no GPIO that a backend handles is set up as a PPS input; give the name of the GPIO to poll"},
		{name: "two", pps: []gpiomem.PPS{gpio18, gpio4}, expectErr: "several GPIOs are set up as PPS inputs (GPIO18, GPIO4); give the name of the one to poll"},
		{name: "one and an unnamed input", pps: []gpiomem.PPS{gpio18, unnamed}, expect: "GPIO18"},
		{name: "only an unnamed input", pps: []gpiomem.PPS{unnamed}, expectErr: "no GPIO that a backend handles is set up as a PPS input; give the name of the GPIO to poll"},
		{name: "two nodes on one GPIO", pps: []gpiomem.PPS{gpio18, {Name: "GPIO18", Line: 18, Node: "/pps@2"}}, expect: "GPIO18"},
		{name: "no backend", ctrls: []gpiomem.Controller{rk}, expectErr: "no supported GPIO controller found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrls := tc.ctrls
			if ctrls == nil {
				c := rp1
				c.PPS = tc.pps
				ctrls = []gpiomem.Controller{c, brcm}
			}
			got, err := onlyPPSInput(ctrls)
			if tc.expectErr != "" {
				if err == nil || err.Error() != tc.expectErr {
					t.Fatalf("got error %v, want %q", err, tc.expectErr)
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

func TestPrintEdge(t *testing.T) {
	ce := pps.CandidateEdge{
		Edge:        pps.Edge{Timestamp: time.Date(2026, 11, 24, 2, 22, 30, 1118, time.UTC)},
		Uncertainty: [2]time.Duration{1486, 1477},
		PollWidths:  [2]time.Duration{1166, 1148},
		Reject:      pps.RejectAcquiring,
	}
	tests := []struct {
		name   string
		jsonl  bool
		expect string
	}{
		{name: "text", expect: "02:22:30.000001\n"},
		{
			name:  "JSON Lines",
			jsonl: true,
			expect: `{"gpio":"GPIO18","t":"2026-11-24T02:22:30.000001118Z","uncertainty":[0.000001486,0.000001477],` +
				`"pollWidths":[0.000001166,0.000001148],"reject":"acquiring"}` + "\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := printEdge(&out, "GPIO18", ce, tc.jsonl); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.String() != tc.expect {
				t.Errorf("got  %q\nwant %q", out.String(), tc.expect)
			}
		})
	}
}
