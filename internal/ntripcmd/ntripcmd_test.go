package ntripcmd

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

func frame(payload string) string {
	return fmt.Sprintf("$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload)))
}

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
			name: "nmea send pos with height",
			args: []string{"--nmea-send-pos", "13.731826167,100.644802333,42.5", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:        "caster.example:2101",
				Mountpoint:  "MNT",
				NMEASendPos: &nmeaSendPos{LatLon: [2]float64{13.731826167, 100.644802333}, Height: 42.5},
			},
		},
		{
			name: "nmea send pos default height",
			args: []string{"--nmea-send-pos", "13.731826167,100.644802333", "caster.example", "MNT"},
			expect: &flagConfig{
				Addr:        "caster.example:2101",
				Mountpoint:  "MNT",
				NMEASendPos: &nmeaSendPos{LatLon: [2]float64{13.731826167, 100.644802333}},
			},
		},
		{
			name:      "nmea send pos bad latitude",
			args:      []string{"--nmea-send-pos", "91,0", "caster.example", "MNT"},
			expectErr: true,
		},
		{
			name:      "gga removed",
			args:      []string{"--gga", "$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47", "caster.example", "MNT"},
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

func TestParseNMEASendPos(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *nmeaSendPos
		wantErr bool
	}{
		{"two fields", "13.731826167,100.644802333", &nmeaSendPos{LatLon: [2]float64{13.731826167, 100.644802333}}, false},
		{"three fields", "13.731826167,100.644802333,42.5", &nmeaSendPos{LatLon: [2]float64{13.731826167, 100.644802333}, Height: 42.5}, false},
		{"spaces", " 13 , 100 , 42.5 ", &nmeaSendPos{LatLon: [2]float64{13, 100}, Height: 42.5}, false},
		{"empty", "", nil, true},
		{"missing lon", "13", nil, true},
		{"too many", "13,100,0,0", nil, true},
		{"empty height", "13,100,", nil, true},
		{"bad lat", "nope,100", nil, true},
		{"nan", "NaN,100", nil, true},
		{"inf", "13,+Inf", nil, true},
		{"lat too high", "91,100", nil, true},
		{"lon too low", "13,-181", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNMEASendPos(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestMakeNMEASendPosGGA(t *testing.T) {
	tm := time.Date(2026, 6, 24, 12, 34, 56, 0, time.UTC)
	got, err := makeNMEASendPosGGA(tm, &nmeaSendPos{LatLon: [2]float64{13.731826167, 100.644802333}, Height: 42.5})
	if err != nil {
		t.Fatalf("make GGA: %v", err)
	}
	want := frame("GNGGA,123456.00,1343.90957,N,10038.68814,E,1,12,1,42.5,M,0,M,,")
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	got, err = makeNMEASendPosGGA(tm, nil)
	if err != nil {
		t.Fatalf("make nil GGA: %v", err)
	}
	if got != "" {
		t.Errorf("nil position GGA = %q, want empty", got)
	}
}
