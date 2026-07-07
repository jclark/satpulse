package gpsreg

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/ubx"
)

func TestVendorString(t *testing.T) {
	tests := []struct {
		vendor   Vendor
		expected string
	}{
		{VendorUnknown, "Unknown"},
		{VendorOther, "other"},
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
		input   string
		want    Vendor
		wantErr bool
	}{
		{"u-blox", VendorUblox, false},
		{"ublox", VendorUblox, false},
		{"U-BLOX", VendorUblox, false},
		{"Allystar", VendorAllystar, false},
		{"trimble", VendorTrimble, false},
		{"comnav", VendorSinoGNSS, false},
		{"other", VendorOther, false},
		{"invalid", VendorUnknown, true},
		{"", VendorUnknown, false},
	}
	for _, tt := range tests {
		got, err := ParseVendor(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseVendor(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseVendor(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestVendorUnmarshalText(t *testing.T) {
	var v Vendor
	if err := v.UnmarshalText([]byte("u-blox")); err != nil {
		t.Fatal(err)
	}
	if v != VendorUblox {
		t.Errorf("got %v, want VendorUblox", v)
	}
	if err := v.UnmarshalText([]byte("bogus")); err == nil {
		t.Error("expected error for unknown vendor")
	}
}

func TestProtocolUnmarshalText(t *testing.T) {
	tests := []struct {
		input string
		want  gpsprot.Tag
	}{
		{"", ""},
		{"ubx", TagUBX},
		{"NMEA", TagNMEA},
		{"RTCM", TagRTCM},
		{"CASBIN", TagCASICBin},
		{"asbin", TagAllystarBin},
		{"SDBP", TagSDBP},
		{"UNCB", TagUnicoreBin},
		{"unca", TagUnicoreAscii},
		{"NOVB", TagNovAtelBin},
		{"nova", TagNovAtelAscii},
	}
	for _, tt := range tests {
		var prot Protocol
		if err := prot.UnmarshalText([]byte(tt.input)); err != nil {
			t.Errorf("UnmarshalText(%q) error = %v", tt.input, err)
			continue
		}
		if got := prot.Tag(); got != tt.want {
			t.Errorf("UnmarshalText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
	var prot Protocol
	if err := prot.UnmarshalText([]byte("xyzzy")); err == nil {
		t.Error("expected error for unknown protocol")
	}
}

func hasFormat(formats []gpsprot.PacketFormat, want gpsprot.PacketFormat) bool {
	for _, f := range formats {
		if f == want {
			return true
		}
	}
	return false
}

func TestCreatePacketFormats(t *testing.T) {
	// VendorUnknown returns all formats
	all := CreatePacketFormats(VendorUnknown)
	if !hasFormat(all, nmea.PacketFormat) || !hasFormat(all, rtcm.PacketFormat) || !hasFormat(all, ubx.PacketFormat) {
		t.Error("VendorUnknown should include NMEA, RTCM, and UBX")
	}
	// VendorUblox returns NMEA + RTCM + UBX only
	ub := CreatePacketFormats(VendorUblox)
	if !hasFormat(ub, nmea.PacketFormat) || !hasFormat(ub, rtcm.PacketFormat) || !hasFormat(ub, ubx.PacketFormat) {
		t.Error("VendorUblox should include NMEA, RTCM, and UBX")
	}
	if len(ub) != 3 {
		t.Errorf("VendorUblox: got %d formats, want 3", len(ub))
	}
	// VendorOther returns NMEA + RTCM only
	other := CreatePacketFormats(VendorOther)
	if len(other) != 2 {
		t.Errorf("VendorOther: got %d formats, want 2", len(other))
	}
}

func TestCreateConfigProtocols(t *testing.T) {
	if n := len(CreateConfigProtocols(VendorUnknown)); n != 3 {
		t.Errorf("VendorUnknown: got %d config protocols, want 3", n)
	}
	if n := len(CreateConfigProtocols(VendorQuectel)); n != 1 {
		t.Errorf("VendorQuectel: got %d config protocols, want 1", n)
	}
	if n := len(CreateConfigProtocols(VendorUblox)); n != 1 {
		t.Errorf("VendorUblox: got %d config protocols, want 1", n)
	}
	if n := len(CreateConfigProtocols(VendorAllystar)); n != 0 {
		t.Errorf("VendorAllystar: got %d config protocols, want 0", n)
	}
}
