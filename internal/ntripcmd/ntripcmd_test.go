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
