package stream

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// decode parses a TOML fragment into a Config.
func decode(t *testing.T, s string) *Config {
	t.Helper()
	cfg := new(Config)
	if err := toml.NewDecoder(strings.NewReader(s)).DisallowUnknownFields().Decode(cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return cfg
}

func TestConfigEmpty(t *testing.T) {
	cfg := decode(t, ``)
	if cfg.Pull != nil {
		t.Errorf("expected Pull nil, got %+v", cfg.Pull)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate empty: %v", err)
	}
}

func TestConfigPullTCP(t *testing.T) {
	cfg := decode(t, `
[pull]
tcp.address = "10.0.0.1:2006"
`)
	if cfg.Pull == nil || cfg.Pull.TCP == nil {
		t.Fatalf("expected TCP set, got %+v", cfg.Pull)
	}
	if cfg.Pull.TCP.Address != "10.0.0.1:2006" {
		t.Errorf("address = %q", cfg.Pull.TCP.Address)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestConfigPullNtrip(t *testing.T) {
	cfg := decode(t, `
[pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
ntrip.username = "u"
ntrip.password = "p"
`)
	n := cfg.Pull.Ntrip
	if n == nil {
		t.Fatal("expected Ntrip set")
	}
	if n.Address != "caster.example.com:2101" || n.Mountpoint != "RTCM" || n.Username != "u" || n.Password != "p" {
		t.Errorf("ntrip = %+v", n)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestPullConfigPrepareNil(t *testing.T) {
	var pc *PullConfig
	if got := pc.Prepare("1.0", nil, nil); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestPullConfigPrepareTCP(t *testing.T) {
	pc := &PullConfig{TCP: &TCPConfig{Address: "10.0.0.1:2006"}}
	s := pc.Prepare("1.0", nil, nil)
	if s == nil {
		t.Fatal("expected setup, got nil")
	}
	src, ok := s.source.(*TCPSource)
	if !ok {
		t.Fatalf("expected *TCPSource, got %T", s.source)
	}
	if src.Addr != "10.0.0.1:2006" {
		t.Errorf("addr = %q", src.Addr)
	}
	if s.Addr() != "10.0.0.1:2006" {
		t.Errorf("Addr() = %q", s.Addr())
	}
}

func TestPullConfigPrepareNtrip(t *testing.T) {
	pc := &PullConfig{Ntrip: &NtripConfig{
		Address:    "caster.example.com:2101",
		Mountpoint: "RTCM",
		Username:   "u",
		Password:   "p",
	}}
	s := pc.Prepare("9.9.9", nil, nil)
	if s == nil {
		t.Fatal("expected setup, got nil")
	}
	src, ok := s.source.(*NtripSource)
	if !ok {
		t.Fatalf("expected *NtripSource, got %T", s.source)
	}
	if src.Addr != "caster.example.com:2101" || src.Mountpoint != "RTCM" ||
		src.Username != "u" || src.Password != "p" {
		t.Errorf("source = %+v", src)
	}
	if src.UserAgent.Version != "9.9.9" {
		t.Errorf("UserAgent.Version = %q", src.UserAgent.Version)
	}
	if s.Addr() != "caster.example.com:2101" {
		t.Errorf("Addr() = %q", s.Addr())
	}
}

func TestConfigPullValidate(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr string
	}{
		{
			name: "tcp and ntrip",
			toml: `
[pull]
tcp.address = "1.2.3.4:5"
ntrip.address = "h:1"
ntrip.mountpoint = "M"
`,
			wantErr: "mutually exclusive",
		},
		{
			name:    "neither",
			toml:    `[pull]`,
			wantErr: "must configure either tcp or ntrip",
		},
		{
			name: "tcp missing address",
			toml: `
[pull.tcp]
`,
			wantErr: "tcp.address is required",
		},
		{
			name: "ntrip missing address",
			toml: `
[pull.ntrip]
mountpoint = "M"
`,
			wantErr: "ntrip.address is required",
		},
		{
			name: "ntrip missing mountpoint",
			toml: `
[pull.ntrip]
address = "h:1"
`,
			wantErr: "ntrip.mountpoint is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := decode(t, tt.toml)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
