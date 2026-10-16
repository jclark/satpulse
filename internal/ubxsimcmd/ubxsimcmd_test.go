//go:build linux || darwin

package ubxsimcmd

import "testing"

func TestTimingFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		phc  string
		bad  bool
	}{
		{"system default", []string{"--timing", "personality.ubx"}, "", false},
		{"PHC reference", []string{"--timing", "--phc", "/dev/ptp_kvm", "personality.ubx"}, "/dev/ptp_kvm", false},
		{"PHC requires timing", []string{"--phc", "/dev/ptp0", "personality.ubx"}, "", true},
		{"old option rejected", []string{"--timing", "--time-source", "system", "personality.ubx"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, _, err := parseFlags("ubxsim", tc.args)
			if (err != nil) != tc.bad {
				t.Fatalf("parseFlags error = %v, want error %v", err, tc.bad)
			}
			if !tc.bad && (v == nil || !v.timing || v.phcPath != tc.phc) {
				t.Fatalf("parseFlags = %+v, want timing with PHC %q", v, tc.phc)
			}
		})
	}
}
