package term

import "testing"

func TestPPSModeSysctl(t *testing.T) {
	tests := []struct {
		path      string
		expect    string
		expectErr bool
	}{
		{path: "/dev/cuau0", expect: "dev.uart.0.pps_mode"},
		{path: "/dev/ttyu1", expect: "dev.uart.1.pps_mode"},
		{path: "/dev/cuau12", expect: "dev.uart.12.pps_mode"},
		{path: "/dev/cuaU0", expect: "hw.usb.ucom.pps_mode"},
		{path: "/dev/cuaU1.2", expect: "hw.usb.ucom.pps_mode"},
		{path: "/dev/ttyU0", expect: "hw.usb.ucom.pps_mode"},
		{path: "/dev/ttyv0", expectErr: true},
		{path: "/dev/cuau0.init", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, err := ppsModeSysctl(tc.path)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
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
