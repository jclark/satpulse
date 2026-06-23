package ntripcmd

import (
	"reflect"
	"testing"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		expect     *flagConfig
		expectHelp bool
		expectErr  bool
	}{
		{
			name: "minimal",
			args: []string{"caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
			},
		},
		{
			name: "explicit port",
			args: []string{"caster.example:9999", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:9999",
				Mountpoint: "MNT",
			},
		},
		{
			name: "ipv6 with port",
			args: []string{"[::1]:2101", "MNT"},
			expect: &flagConfig{
				Addr:       "[::1]:2101",
				Mountpoint: "MNT",
			},
		},
		{
			name: "ipv6 without port",
			args: []string{"::1", "MNT"},
			expect: &flagConfig{
				Addr:       "[::1]:2101",
				Mountpoint: "MNT",
			},
		},
		{
			name: "user without password",
			args: []string{"--user", "jjc", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
				Username:   "jjc",
			},
		},
		{
			name: "user with password",
			args: []string{"--user", "jjc:xyzzy", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
				Username:   "jjc",
				Password:   "xyzzy",
			},
		},
		{
			name: "user with colon in password",
			args: []string{"--user", "jjc:x:y", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
				Username:   "jjc",
				Password:   "x:y",
			},
		},
		{
			name: "user with trailing colon",
			args: []string{"--user", "jjc:", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
				Username:   "jjc",
			},
		},
		{
			name: "bin flag",
			args: []string{"--bin", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
				Bin:        true,
			},
		},
		{
			name: "gga valid",
			args: []string{"--gga", "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:       "caster.example:2101",
				Mountpoint: "MNT",
				GGA:        "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47\r\n",
			},
		},
		{
			name:      "gga bad checksum",
			args:      []string{"--gga", "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*00", "caster.example", "MNT"},
			expectErr: true,
		},
		{
			name:       "help short",
			args:       []string{"-h"},
			expectHelp: true,
		},
		{
			name:       "help long",
			args:       []string{"--help"},
			expectHelp: true,
		},
		{
			name:      "no args",
			args:      []string{},
			expectErr: true,
		},
		{
			name:      "one positional",
			args:      []string{"caster.example"},
			expectErr: true,
		},
		{
			name:      "three positionals",
			args:      []string{"caster.example", "MNT", "extra"},
			expectErr: true,
		},
		{
			name:      "unknown flag",
			args:      []string{"--nope", "caster.example", "MNT"},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, help, usageFunc, err := parseFlags("ntrip", tc.args)
			if usageFunc == nil {
				t.Fatalf("usageFunc is nil")
			}
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got cfg=%+v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if help != tc.expectHelp {
				t.Errorf("help = %v, want %v", help, tc.expectHelp)
			}
			if !reflect.DeepEqual(cfg, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", cfg, tc.expect)
			}
		})
	}
}

func TestValidateGGA(t *testing.T) {
	const gga = "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47"
	tests := []struct {
		name string
		in   string
		want string // expected wire output; "" means expect an error
	}{
		{"gp", gga, gga + "\r\n"},
		{"gn", "$GNGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*59", "$GNGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*59\r\n"},
		{"trailing newline tolerated", gga + "\r\n", gga + "\r\n"},
		{"bad checksum", "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*00", ""},
		{"not gga", "$GPGLL,4916.45,N,12311.12,W,225444,A*31", ""},
		{"not nmea", "GPGGA,nope", ""},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateGGA(tc.in)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
