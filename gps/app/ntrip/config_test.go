package ntrip

import (
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
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
			name: "user without username",
			cfg: Config{
				Users:      []UserConfig{{Password: "x"}},
				Mountpoint: []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "duplicate users",
			cfg: Config{
				Users:      []UserConfig{{Username: "a"}, {Username: "a"}},
				Mountpoint: []MountConfig{{Name: "BKK"}},
			},
			expectErr: true,
		},
		{
			name: "mountpoint references undefined user",
			cfg: Config{
				Users:      []UserConfig{{Username: "rover1"}},
				Mountpoint: []MountConfig{{Name: "BKK", Users: []string{"ghost"}}},
			},
			expectErr: true,
		},
		{
			name: "mountpoint references defined user",
			cfg: Config{
				Users:      []UserConfig{{Username: "rover1"}},
				Mountpoint: []MountConfig{{Name: "BKK", Users: []string{"rover1"}}},
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
			err := tc.cfg.Validate()
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
