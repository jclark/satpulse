package gpsreg

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestVendorString(t *testing.T) {
	tests := []struct {
		vendor   Vendor
		expected string
	}{
		{Vendor(0), "Vendor(0)"},
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
		{"invalid", 0, true},
		{"", 0, false},
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

func TestEnvVendors(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		expect    []Vendor
		expectErr bool
	}{
		{"empty", "", nil, false},
		{"single", "u-blox", []Vendor{VendorUblox}, false},
		{"alias", "ublox", []Vendor{VendorUblox}, false},
		{"list", "u-blox,Unicore", []Vendor{VendorUblox, VendorUnicore}, false},
		{"list with alias", "ublox,comnav", []Vendor{VendorUblox, VendorSinoGNSS}, false},
		{"all", "all", allVendors, false},
		{"all mixed with names", "all,u-blox", nil, true},
		{"unknown name", "bogus", nil, true},
		{"duplicates dropped in order", "u-blox,ublox,Unicore", []Vendor{VendorUblox, VendorUnicore}, false},
		{"empty element", "u-blox,,Unicore", nil, true},
		{"whitespace around names", " u-blox, Unicore ", []Vendor{VendorUblox, VendorUnicore}, false},
		{"whitespace around all", " all ", allVendors, false},
		{"whitespace only", "  ", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envVendorsVar, tc.env)
			got, err := EnvVendors()
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
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
}

// formatTags reduces packet formats to their tags: PacketFormat values
// cannot be compared directly (some hold func fields), but their tags
// identify them and are safe to compare.
func formatTags(formats []gpsprot.PacketFormat) []gpsprot.Tag {
	tags := make([]gpsprot.Tag, len(formats))
	for i, f := range formats {
		tags[i] = f.Tag()
	}
	return tags
}

func TestCreatePacketFormats(t *testing.T) {
	full := []gpsprot.Tag{
		TagNMEA, TagRTCM, TagUBX, TagCASICBin, TagAllystarBin, TagSDBP,
		TagUnicoreBin, TagUnicoreAscii, TagNovAtelBin, TagNovAtelAscii,
		TagNovAtelAbbrevAscii, TagSBF, TagSeptentrioReply,
	}
	tests := []struct {
		name    string
		vendors []Vendor
		expect  []gpsprot.Tag
	}{
		{"empty list is the whole flat list", nil, full},
		{"u-blox", []Vendor{VendorUblox}, []gpsprot.Tag{TagNMEA, TagRTCM, TagUBX}},
		{"vendor without formats", []Vendor{VendorOther}, []gpsprot.Tag{TagNMEA, TagRTCM}},
		{
			// Unicore's entry includes the nov formats, so a list with
			// both keeps them once, in flat-list order.
			"unicore and novatel deduped",
			[]Vendor{VendorUnicore, VendorNovAtel},
			[]gpsprot.Tag{TagNMEA, TagRTCM, TagUnicoreBin, TagUnicoreAscii, TagNovAtelBin, TagNovAtelAscii, TagNovAtelAbbrevAscii},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatTags(CreatePacketFormats(tc.vendors))
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %v\nwant %v", got, tc.expect)
			}
		})
	}
}

func TestCreateConfigProtocols(t *testing.T) {
	tests := []struct {
		name    string
		vendors []Vendor
		expect  int
	}{
		{"empty list is the default probe set", nil, 2},
		{"u-blox", []Vendor{VendorUblox}, 1},
		{"unicore", []Vendor{VendorUnicore}, 1},
		{"both defaults", []Vendor{VendorUblox, VendorUnicore}, 2},
		{"vendor without protocol", []Vendor{VendorAllystar}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if n := len(CreateConfigProtocols(tc.vendors)); n != tc.expect {
				t.Errorf("got %d config protocols, want %d", n, tc.expect)
			}
		})
	}
}
