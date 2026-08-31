//go:build freebsd

package serialenum

import (
	"reflect"
	"testing"
)

func TestCallOutName(t *testing.T) {
	tests := []struct {
		name   string
		expect bool
	}{
		{name: "cuau0", expect: true},
		{name: "cuau12", expect: true},
		{name: "cuaU0", expect: true},
		{name: "cuaU0.1", expect: true},
		{name: "cuaU0.init", expect: false},
		{name: "cuaU0.lock", expect: false},
		{name: "cuau0.init", expect: false},
		{name: "cuau0.1", expect: false},
		{name: "cuaU", expect: false},
		{name: "cuaU.", expect: false},
		{name: "cuaU0.", expect: false},
		{name: "cuaU1x", expect: false},
		{name: "cua", expect: false},
		{name: "cuad0", expect: false},
		{name: "ttyu0", expect: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := callOutName.MatchString(tc.name); got != tc.expect {
				t.Errorf("got %v want %v", got, tc.expect)
			}
		})
	}
}

func TestParsePnpinfo(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect map[string]string
	}{
		{
			name:  "uftdi",
			input: `vendor=0x0403 product=0x6014 devclass=0x00 devsubclass=0x00 devproto=0x00 sernum="" release=0x0900 mode=host intclass=0xff intsubclass=0xff intprotocol=0xff ttyname=U0 ttyports=1`,
			expect: map[string]string{
				"vendor": "0x0403", "product": "0x6014",
				"devclass": "0x00", "devsubclass": "0x00", "devproto": "0x00",
				"sernum": "", "release": "0x0900", "mode": "host",
				"intclass": "0xff", "intsubclass": "0xff", "intprotocol": "0xff",
				"ttyname": "U0", "ttyports": "1",
			},
		},
		{
			name:   "quoted value with space",
			input:  `vendor=0x0403 sernum="AB CD" release=0x0600`,
			expect: map[string]string{"vendor": "0x0403", "sernum": "AB CD", "release": "0x0600"},
		},
		{
			name:   "unterminated quote",
			input:  `sernum="AB`,
			expect: map[string]string{"sernum": "AB"},
		},
		{
			name:   "empty",
			input:  "",
			expect: map[string]string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePnpinfo(tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestDescProduct(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "no string descriptors",
			input:  "vendor 0x0403 product 0x6014, class 0/0, rev 2.00/9.00, addr 61",
			expect: "",
		},
		{
			name:   "product text",
			input:  "Ralink 802.11 n WLAN, class 0/0, rev 2.00/1.01, addr 3",
			expect: "Ralink 802.11 n WLAN",
		},
		{
			name:   "product text with comma",
			input:  "Foo, Bar, class 0/0, rev 2.00/1.01, addr 3",
			expect: "Foo, Bar",
		},
		{
			name:   "no suffix",
			input:  "16950 or compatible",
			expect: "16950 or compatible",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := descProduct(tc.input); got != tc.expect {
				t.Errorf("got %q want %q", got, tc.expect)
			}
		})
	}
}

func TestBuildPorts(t *testing.T) {
	meta := map[string]usbMeta{
		"U0": {usb: USBID{VID: 0x0403, PID: 0x6014}},
		"U1": {usb: USBID{VID: 0x0403, PID: 0x6001}, serial: "BG03SFLC", product: "FTDI FT232R USB UART"},
		"U2": {usb: USBID{VID: 0x0403, PID: 0x6011}, product: "Quad RS232-HS"},
		"U3": {usb: USBID{VID: 0x1546, PID: 0x01a9}},
	}
	names := []string{"cuaU0", "cuaU1", "cuaU2.0", "cuaU2.1", "cuaU3", "cuaU9", "cuau0"}
	expect := []Port{
		{Device: "/dev/cuaU0", Display: "/dev/cuaU0", USB: USBID{VID: 0x0403, PID: 0x6014}},
		{Device: "/dev/cuaU1", Display: "FTDI FT232R USB UART", USB: USBID{VID: 0x0403, PID: 0x6001}, Serial: "BG03SFLC"},
		{Device: "/dev/cuaU2.0", Display: "Quad RS232-HS", USB: USBID{VID: 0x0403, PID: 0x6011}},
		{Device: "/dev/cuaU2.1", Display: "Quad RS232-HS", USB: USBID{VID: 0x0403, PID: 0x6011}},
		{Device: "/dev/cuaU3", Display: "/dev/cuaU3 (u-blox gen 9)", USB: USBID{VID: 0x1546, PID: 0x01a9}},
		{Device: "/dev/cuaU9", Display: "/dev/cuaU9"},
		{Device: "/dev/cuau0", Display: "/dev/cuau0"},
	}
	got := buildPorts(names, meta)
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
}

// TestListSystem exercises the sysctl walk against the real kernel; the port
// set is machine-dependent, so it checks only that enumeration succeeds.
func TestListSystem(t *testing.T) {
	ports, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range ports {
		t.Logf("%+v", p)
	}
}
