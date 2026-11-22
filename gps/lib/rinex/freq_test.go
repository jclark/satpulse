package rinex

import (
	"reflect"
	"testing"
)

func TestSignalFrequencyHz(t *testing.T) {
	k := int8(-7)
	tests := []struct {
		name     string
		sys      string
		sig      SignalID
		frq      *int8
		expectHz float64
		expectOK bool
	}{
		{name: "GLONASS FDMA", sys: "R", sig: "1C", frq: &k, expectHz: 1598.0625e6, expectOK: true},
		{name: "GLONASS FDMA without channel", sys: "R", sig: "2C"},
		{name: "GLONASS L3 CDMA without channel", sys: "R", sig: "3Q", expectHz: 1202.025e6, expectOK: true},
		{name: "NavIC L1", sys: "I", sig: "1P", expectHz: 1575.42e6, expectOK: true},
		{name: "unknown band", sys: "I", sig: "2P"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hz, ok := SignalFrequencyHz(tc.sys, tc.sig, tc.frq)
			got := []any{hz, ok}
			if expect := []any{tc.expectHz, tc.expectOK}; !reflect.DeepEqual(got, expect) {
				t.Errorf("got  %v\nwant %v", got, expect)
			}
		})
	}
}
