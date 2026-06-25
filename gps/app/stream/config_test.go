package stream

import (
	"strings"
	"testing"
	"time"

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
	if cfg.Pull.TCP != nil || cfg.Pull.Ntrip != nil {
		t.Errorf("expected Pull unset, got %+v", cfg.Pull)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate empty: %v", err)
	}
}

func TestConfigPullEmpty(t *testing.T) {
	cfg := decode(t, "[pull]\n")
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate empty [pull]: %v", err)
	}
}

func TestConfigPullTCP(t *testing.T) {
	cfg := decode(t, `
[pull]
tcp.address = "10.0.0.1:2006"
`)
	if cfg.Pull.TCP == nil {
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
ntrip.nmeaSend = true
`)
	n := cfg.Pull.Ntrip
	if n == nil {
		t.Fatal("expected Ntrip set")
	}
	if n.Address != "caster.example.com:2101" || n.Mountpoint != "RTCM" || n.Username != "u" || n.Password != "p" || !n.NMEASend {
		t.Errorf("ntrip = %+v", n)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestConfigPullNtripNMEASendInterval(t *testing.T) {
	cfg := decode(t, `
[pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
ntrip.nmeaSend = true
ntrip.nmeaSendInterval = 2.5
`)
	n := cfg.Pull.Ntrip
	if n.NMEASendInterval == nil || *n.NMEASendInterval != 2.5 {
		t.Fatalf("NMEASendInterval = %v, want 2.5", n.NMEASendInterval)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if got := cfg.Pull.nmeaSendInterval(); got != 2500*time.Millisecond {
		t.Errorf("nmeaSendInterval() = %v, want 2.5s", got)
	}
}

func TestPullConfigNMEAInterval(t *testing.T) {
	zero := 0.0
	tests := []struct {
		name  string
		ntrip *NtripConfig
		want  time.Duration
	}{
		{"unset defaults to 5s", &NtripConfig{NMEASend: true}, 5 * time.Second},
		{"explicit zero means once", &NtripConfig{NMEASend: true, NMEASendInterval: &zero}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &PullConfig{Ntrip: tt.ntrip}
			if got := pc.nmeaSendInterval(); got != tt.want {
				t.Errorf("nmeaSendInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPullConfigValidateNMEAInterval(t *testing.T) {
	neg := -1.0
	pc := &PullConfig{Ntrip: &NtripConfig{
		Address:          "caster.example.com",
		Mountpoint:       "RTCM",
		NMEASend:         true,
		NMEASendInterval: &neg,
	}}
	if err := pc.Validate(); err == nil {
		t.Error("expected error for negative nmeaSendInterval")
	}
}

func TestNtripNormalizeAddress(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"caster.example.com", "caster.example.com:2101"},
		{"caster.example.com:2102", "caster.example.com:2102"},
		{"::1", "[::1]:2101"},
		{"[::1]", "[::1]:2101"},
		{"[::1]:2102", "[::1]:2102"},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := NtripNormalizeAddress(tt.addr); got != tt.want {
				t.Errorf("NtripNormalizeAddress(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

func TestValidateDoesNotNormalizeNtripAddress(t *testing.T) {
	cfg := decode(t, `
[pull]
ntrip.address = "pull.example.com"
ntrip.mountpoint = "PULL"

[[push]]
ntrip.address = "push.example.com"
ntrip.mountpoint = "PUSH"
ntrip.password = "p"
`)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Pull.Ntrip.Address != "pull.example.com" {
		t.Errorf("pull address = %q", cfg.Pull.Ntrip.Address)
	}
	if cfg.Push[0].Ntrip.Address != "push.example.com" {
		t.Errorf("push address = %q", cfg.Push[0].Ntrip.Address)
	}
}

func TestPullConfigNewPullDisabled(t *testing.T) {
	pc := &PullConfig{}
	got, addr := pc.NewPull("1.0", nil, nil, nil, nil)
	if got != nil || addr != "" {
		t.Errorf("NewPull() = %+v, %q; want nil, empty", got, addr)
	}
}

func TestPullConfigNewPullTCP(t *testing.T) {
	pc := &PullConfig{TCP: &TCPConfig{Address: "10.0.0.1:2006"}}
	s, addr := pc.NewPull("1.0", nil, nil, nil, nil)
	if s == nil {
		t.Fatal("expected pull, got nil")
	}
	src, ok := s.source.(*TCPSource)
	if !ok {
		t.Fatalf("expected *TCPSource, got %T", s.source)
	}
	if src.Addr != "10.0.0.1:2006" {
		t.Errorf("addr = %q", src.Addr)
	}
	if addr != "10.0.0.1:2006" {
		t.Errorf("returned addr = %q", addr)
	}
}

func TestPullConfigNewPullNtrip(t *testing.T) {
	pc := &PullConfig{Ntrip: &NtripConfig{
		Address:    "caster.example.com",
		Mountpoint: "RTCM",
		Username:   "u",
		Password:   "p",
		NMEASend:   true,
	}}
	s, addr := pc.NewPull("9.9.9", nil, nil, nil, nil)
	if s == nil {
		t.Fatal("expected pull, got nil")
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
	if !pc.NMEASend() {
		t.Errorf("NMEASend() = false, want true")
	}
	if s.nmeaSendInterval != 5*time.Second {
		t.Errorf("nmeaSendInterval = %v, want 5s (default)", s.nmeaSendInterval)
	}
	if addr != "caster.example.com:2101" {
		t.Errorf("returned addr = %q", addr)
	}
	if pc.Ntrip.Address != "caster.example.com" {
		t.Errorf("config address = %q", pc.Ntrip.Address)
	}
}

func TestConfigPushNtrip(t *testing.T) {
	cfg := decode(t, `
[[push]]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "MY_BASE"
ntrip.password = "secret"
ntrip.description = "Bangkok"
ntrip.bitrate = 4800
ntrip.msm7to4 = true
`)
	if len(cfg.Push) != 1 {
		t.Fatalf("expected 1 push entry, got %d", len(cfg.Push))
	}
	p := cfg.Push[0]
	if p.Ntrip == nil {
		t.Fatal("expected ntrip set")
	}
	if p.Ntrip.Address != "caster.example.com:2101" {
		t.Errorf("address = %q", p.Ntrip.Address)
	}
	if p.Ntrip.Description != "Bangkok" {
		t.Errorf("description = %q", p.Ntrip.Description)
	}
	if p.Ntrip.Bitrate != 4800 {
		t.Errorf("bitrate = %d", p.Ntrip.Bitrate)
	}
	if !p.Ntrip.MSM7to4 {
		t.Errorf("msm7to4 = false, want true")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	// protocol defaulted to RTCM after Validate.
	if cfg.Push[0].Protocol.Tag() != "RTCM" {
		t.Errorf("Protocol after default = %q, want RTCM", cfg.Push[0].Protocol.Tag())
	}
}

func TestConfigPushUDP(t *testing.T) {
	cfg := decode(t, `
[[push]]
udp.address = "127.0.0.1:10110"
`)
	if len(cfg.Push) != 1 {
		t.Fatalf("expected 1 push entry, got %d", len(cfg.Push))
	}
	p := cfg.Push[0]
	if p.UDP == nil {
		t.Fatal("expected udp set")
	}
	if p.UDP.Address != "127.0.0.1:10110" {
		t.Errorf("address = %q", p.UDP.Address)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if cfg.Push[0].Protocol.Tag() != "" {
		t.Errorf("Protocol after Validate = %q, want empty", cfg.Push[0].Protocol.Tag())
	}
}

func TestConfigPushValidate(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr string
	}{
		{
			name:    "no transport",
			toml:    `[[push]]`,
			wantErr: "must configure either ntrip or udp",
		},
		{
			name: "multiple transports",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "p"
udp.address = "127.0.0.1:10110"
`,
			wantErr: "mutually exclusive",
		},
		{
			name: "missing address",
			toml: `
[[push]]
ntrip.mountpoint = "M"
ntrip.password = "p"
`,
			wantErr: "ntrip.address is required",
		},
		{
			name: "missing udp address",
			toml: `
[[push]]
udp.address = ""
`,
			wantErr: "udp.address is required",
		},
		{
			name: "missing mountpoint",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.password = "p"
`,
			wantErr: "ntrip.mountpoint is required",
		},
		{
			name: "msm7to4 with non-RTCM protocol",
			toml: `
[[push]]
protocol = "UBX"
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "p"
ntrip.msm7to4 = true
`,
			wantErr: "msm7to4 requires protocol",
		},
		{
			name: "invalid mountpoint name",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "bad/mp"
ntrip.password = "p"
`,
			wantErr: "invalid mountpoint name",
		},
		{
			name: "missing password",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
`,
			wantErr: "ntrip.password",
		},
		{
			name: "password with space",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "let me in"
`,
			wantErr: "ntrip.password",
		},
		{
			name: "password with CR",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "bad\rhdr"
`,
			wantErr: "ntrip.password",
		},
		{
			name: "description with semicolon",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "p"
ntrip.description = "BKK;injected"
`,
			wantErr: "ntrip.description",
		},
		{
			name: "description with CRLF",
			toml: `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "p"
ntrip.description = "BKK\r\nX: y"
`,
			wantErr: "ntrip.description",
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

func TestConfigHasNtripPush(t *testing.T) {
	cfg := decode(t, ``)
	if cfg.HasNtripPush() {
		t.Errorf("empty config: HasNtripPush = true")
	}
	cfg = decode(t, `
[[push]]
ntrip.address = "h:1"
ntrip.mountpoint = "M"
ntrip.password = "p"
`)
	if !cfg.HasNtripPush() {
		t.Errorf("with ntrip push: HasNtripPush = false")
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
