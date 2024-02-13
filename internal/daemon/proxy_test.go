package daemon

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/proxy"
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
		readOnly = true`,
			proxy.Config{
				TCP: []proxy.TCPService{
					{
						Listen:  "127.0.0.1:1234",
						Options: proxy.Options{ReadOnly: true},
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
