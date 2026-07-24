package cmd

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
)

func TestResolveVendors(t *testing.T) {
	tests := []struct {
		name   string
		vendor gpsreg.Vendor
		env    string
		expect []gpsreg.Vendor
	}{
		{"nothing specified", 0, "", nil},
		{"explicit vendor", gpsreg.VendorUblox, "", []gpsreg.Vendor{gpsreg.VendorUblox}},
		{"explicit vendor wins over env", gpsreg.VendorNovAtel, "u-blox,Unicore", []gpsreg.Vendor{gpsreg.VendorNovAtel}},
		{"env declaration", 0, "u-blox,Unicore", []gpsreg.Vendor{gpsreg.VendorUblox, gpsreg.VendorUnicore}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SATPULSE_VENDORS", tc.env)
			got, err := ResolveVendors(tc.vendor)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
	t.Run("malformed env", func(t *testing.T) {
		t.Setenv("SATPULSE_VENDORS", "bogus")
		if _, err := ResolveVendors(0); err == nil {
			t.Fatalf("expected error")
		}
	})
}
