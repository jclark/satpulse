package gpsreg

import "testing"

func TestVendorString(t *testing.T) {
	tests := []struct {
		vendor   Vendor
		expected string
	}{
		{VendorUnknown, "Unknown"},
		{VendorUblox, "u-blox"},
		{VendorAllystar, "Allystar"},
		{VendorTrimble, "Trimble"},
		{Vendor(999), "Vendor(999)"},
		{Vendor(-1), "Vendor(-1)"},
	}

	for _, tt := range tests {
		if got := tt.vendor.String(); got != tt.expected {
			t.Errorf("Vendor(%d).String() = %q, want %q", tt.vendor, got, tt.expected)
		}
	}
}

func TestParseVendor(t *testing.T) {
	tests := []struct {
		input    string
		expected Vendor
	}{
		{"u-blox", VendorUblox},
		{"ublox", VendorUblox},
		{"U-BLOX", VendorUblox},
		{"Allystar", VendorAllystar},
		{"trimble", VendorTrimble},
		{"comnav", VendorSinoGNSS},
		{"invalid", VendorUnknown},
		{"", VendorUnknown},
	}

	for _, tt := range tests {
		if got := ParseVendor(tt.input); got != tt.expected {
			t.Errorf("ParseVendor(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}