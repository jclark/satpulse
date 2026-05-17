package ntrip

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		users     []string
		expectErr bool
	}{
		{
			name: "minimal mountpoint",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK"}},
			},
		},
		{
			name: "no mountpoints is valid (push defaults)",
			cfg:  Config{},
		},
		{
			name: "invalid mountpoint name with slash",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "a/b"}},
			},
			expectErr: true,
		},
		{
			name: "duplicate mountpoint names",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK"}, {Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "auth anyUser alone is valid",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK", Auth: &AuthConfig{AnyUser: true}}},
			},
		},
		{
			name: "auth users list is valid",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK", Auth: &AuthConfig{Users: []string{"rover1"}}}},
			},
			users: []string{"rover1"},
		},
		{
			name: "auth empty object (closed) is valid",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK", Auth: &AuthConfig{}}},
			},
		},
		{
			name: "auth anyUser with users is rejected",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK", Auth: &AuthConfig{AnyUser: true, Users: []string{"rover1"}}}},
			},
			expectErr: true,
		},
		{
			name: "auth.users referencing defined user is valid",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK", Auth: &AuthConfig{Users: []string{"rover1"}}}},
			},
			users: []string{"rover1"},
		},
		{
			name: "auth.users referencing undefined user is rejected",
			cfg: Config{
				Mountpoint: []MountConfig{{Name: "BKK", Auth: &AuthConfig{Users: []string{"ghost"}}}},
			},
			users:     []string{"rover1"},
			expectErr: true,
		},
		{
			name: "lat without lon",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{Lat: opt.Make(gpsprot.DegreesFromFloat(13.76))},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "lon without lat",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{Lon: opt.Make(gpsprot.DegreesFromFloat(100.5))},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "lat and lon together",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{
					Lat: opt.Make(gpsprot.DegreesFromFloat(13.76)),
					Lon: opt.Make(gpsprot.DegreesFromFloat(100.5)),
				},
				Mountpoint: []MountConfig{{Name: "BKK"}},
			},
		},
		{
			name: "negative shared bitrate",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{Bitrate: -1},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "negative mountpoint bitrate",
			cfg: Config{
				Mountpoint: []MountConfig{{
					Name:         "BKK",
					StreamConfig: StreamConfig{Bitrate: -1},
				}},
			},
			expectErr: true,
		},
		{
			name: "invalid formatDetails",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{FormatDetails: "1005,bad spaces"},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "valid formatDetails",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{FormatDetails: "1005(1),1074(1)"},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
		},
		{
			name: "semicolon in country",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{Country: "TH;A"},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "newline in network",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{Network: "Net\nA"},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "carriage return in generator",
			cfg: Config{
				SharedStreamConfig: SharedStreamConfig{Generator: "Gen\rA"},
				Mountpoint:         []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "semicolon in description",
			cfg: Config{
				Mountpoint: []MountConfig{{
					Name:         "BKK",
					StreamConfig: StreamConfig{Description: "Bang;kok"},
				}},
			},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			users := make(map[string]struct{}, len(tc.users))
			for _, u := range tc.users {
				users[u] = struct{}{}
			}
			err := tc.cfg.Validate(users)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
