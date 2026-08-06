//go:build darwin

package term

import "testing"

func TestD2XXPathSerial(t *testing.T) {
	tests := []struct {
		path   string
		serial string
		ok     bool
	}{
		{path: "/dev/cu.usbserial-BG02DBNX", serial: "BG02DBNX", ok: true},
		{path: "/dev/tty.usbserial-1234", serial: "1234", ok: true},
		{path: "/dev/cu.usbserialABC123", serial: "ABC123", ok: true},
		{path: "/dev/cu.usbmodem1234"},
		{path: "/dev/cu.usbserial-"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			serial, ok := d2xxPathSerial(tc.path)
			if serial != tc.serial || ok != tc.ok {
				t.Fatalf("d2xxPathSerial(%q) = %q, %v, want %q, %v", tc.path, serial, ok, tc.serial, tc.ok)
			}
		})
	}
}
