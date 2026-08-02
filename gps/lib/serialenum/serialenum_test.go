package serialenum

import (
	"encoding/json"
	"testing"
)

func TestEnumeratorDisplay(t *testing.T) {
	tests := []struct {
		name    string
		device  string
		product string
		vid     uint16
		pid     uint16
		want    string
	}{
		{
			name:   "bare device",
			device: "/dev/ttyS0",
			want:   "/dev/ttyS0",
		},
		{
			name:    "product",
			device:  "/dev/ttyUSB0",
			product: "USB UART",
			want:    "USB UART",
		},
		{
			name:   "u-blox without product",
			device: "/dev/ttyACM0",
			vid:    0x1546,
			pid:    0x01a9,
			want:   "/dev/ttyACM0 (u-blox gen 9)",
		},
		{
			name:    "u-blox product",
			device:  "/dev/ttyACM0",
			product: "u-blox GNSS receiver",
			vid:     0x1546,
			pid:     0x01a9,
			want:    "u-blox GNSS receiver - u-blox gen 9",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := enumeratorDisplay(test.device, test.product, test.vid, test.pid)
			if got != test.want {
				t.Errorf("enumeratorDisplay() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUbloxTag(t *testing.T) {
	tests := []struct {
		vid  uint16
		pid  uint16
		want string
	}{
		{0x1546, 0x01a4, "u-blox gen 4"},
		{0x1546, 0x01af, "u-blox gen 15"},
		{0x1546, 0x01a3, "u-blox"},
		{0x0403, 0x6001, ""},
	}
	for _, test := range tests {
		if got := ubloxTag(test.vid, test.pid); got != test.want {
			t.Errorf("ubloxTag(%#x, %#x) = %q, want %q", test.vid, test.pid, got, test.want)
		}
	}
}

func TestParseUSBID(t *testing.T) {
	tests := []struct {
		in      string
		want    uint16
		wantErr bool
	}{
		{"1546", 0x1546, false},
		{"01A9", 0x01a9, false},
		{"", 0, false},
		{"10000", 0, true},
		{"xyz", 0, true},
	}
	for _, test := range tests {
		got, err := parseUSBID(test.in)
		if got != test.want || (err != nil) != test.wantErr {
			t.Errorf("parseUSBID(%q) = (%#x, %v), want (%#x, error %v)",
				test.in, got, err, test.want, test.wantErr)
		}
	}
}

func TestParseEnumeratorUSBID(t *testing.T) {
	tests := []struct {
		vid  string
		pid  string
		want USBID
	}{
		{"1546", "01A9", USBID{VID: 0x1546, PID: 0x01a9}},
		{"1234", "0000", USBID{VID: 0x1234, PID: 0}},
		{"", "", USBID{}},
		{"1546", "", USBID{}},
		{"xyz", "01A9", USBID{}},
		{"1546", "xyz", USBID{}},
	}
	for _, test := range tests {
		if got := parseEnumeratorUSBID(test.vid, test.pid); got != test.want {
			t.Errorf("parseEnumeratorUSBID(%q, %q) = %#v, want %#v",
				test.vid, test.pid, got, test.want)
		}
	}
}

func TestPortJSONOmitsEmptyUSBIDs(t *testing.T) {
	port := Port{Device: "/dev/ttyS0", Display: "/dev/ttyS0"}
	got, err := json.Marshal(port)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"device":"/dev/ttyS0","display":"/dev/ttyS0"}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestPortJSONUsesNumericUSBIDs(t *testing.T) {
	port := Port{
		Device:  "/dev/ttyACM0",
		Display: "u-blox GNSS receiver",
		USB: USBID{
			VID: 0x1546,
			PID: 0x01a9,
		},
	}
	got, err := json.Marshal(port)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"device":"/dev/ttyACM0","display":"u-blox GNSS receiver","usb":{"vid":5446,"pid":425}}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestPortJSONIncludesZeroPID(t *testing.T) {
	port := Port{
		Device:  "/dev/ttyACM0",
		Display: "USB device",
		USB: USBID{
			VID: 0x1234,
			PID: 0,
		},
	}
	got, err := json.Marshal(port)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"device":"/dev/ttyACM0","display":"USB device","usb":{"vid":4660,"pid":0}}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
