package daemon

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/proxy"
	"github.com/jclark/satpulse/internal/rtcm"
	"golang.org/x/exp/slices"
)

func TestProxyConfig(t *testing.T) {
	var testCases = []struct {
		str string
		cfg proxy.Config
	}{
		{
			`[[proxy.tcp]]
		listen = "127.0.0.1:1234"
		protocol = "nmea"`,
			proxy.Config{
				TCP: []proxy.TCPService{
					{
						Listen:  "127.0.0.1:1234",
						Options: proxy.Options{Protocol: gpsreg.Protocol(nmea.Tag)},
					},
				},
			},
		},
		{
			"", proxy.Config{},
		},
	}
	for i, tc := range testCases {
		r := strings.NewReader(tc.str)
		cfg, err := readConfig(r)
		if err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(cfg.Proxy.TCP, tc.cfg.TCP) {
			t.Fatalf("proxy config %d not parsed correctly", i)
		}
	}
}

func TestProxyProtocol(t *testing.T) {
	cfgStr := `[[proxy.tcp]]
	listen = ":1234"
	protocol = "rtcm"
	readOnly = true
	[[proxy.tcp]]
	listen = ":5678"`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxy.TCP) != 2 {
		t.Fatal("TCP proxy config not parsed correctly")
	}
	if cfg.Proxy.TCP[0].Listen != ":1234" || cfg.Proxy.TCP[1].Listen != ":5678" {
		t.Fatal("TCP proxy config not parsed correctly")
	}
	if cfg.Proxy.TCP[0].Protocol.Tag() != rtcm.Tag || !cfg.Proxy.TCP[1].Protocol.Tag().IsZero() {
		t.Fatal("TCP proxy tags not parsed correctly")
	}
	if cfg.Proxy.TCP[0].ReadOnly == nil || *cfg.Proxy.TCP[0].ReadOnly != true || cfg.Proxy.TCP[1].ReadOnly != nil {
		t.Fatal("TCP proxy readOnly not parsed correctly")
	}
	cfgStr = `[[proxy.tcp]]
	protocol = "xyzzy"
	listen = ":1234"`
	r = strings.NewReader(cfgStr)
	_, err = readConfig(r)
	if err == nil {
		t.Fatal("expected error for unknown protocol")
	}
	if !strings.Contains(err.Error(), "unknown protocol") {
		t.Fatalf("unexpected error: %v", err)
	}
}
