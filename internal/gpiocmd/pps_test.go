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
	rp1 := gpiomem.Controller{Chip: "gpiochip0", Compatible: []string{"raspberrypi,rp1-gpio"}, Family: "rp1"}
	brcm := gpiomem.Controller{Chip: "gpiochip10", Compatible: []string{"brcm,brcmstb-gpio"}}
	gpio18 := gpiomem.PPS{Name: "GPIO18", Line: 18, Node: "/pps@"}
	gpio4 := gpiomem.PPS{Name: "GPIO4", Line: 4, Node: "/pps@4"}
	tests := []struct {
		name      string
		pps       []gpiomem.PPS
		expect    string
		expectErr bool
	}{
		{name: "one", pps: []gpiomem.PPS{gpio18}, expect: "GPIO18"},
		{name: "none", expectErr: true},
		{name: "two", pps: []gpiomem.PPS{gpio18, gpio4}, expectErr: true},
		{name: "two nodes on one GPIO", pps: []gpiomem.PPS{gpio18, {Name: "GPIO18", Line: 18, Node: "/pps@2"}}, expect: "GPIO18"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := rp1
			c.PPS = tc.pps
			got, err := onlyPPSInput([]gpiomem.Controller{c, brcm})
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
