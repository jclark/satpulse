package daemon

import (
	"strings"
	"testing"

	"golang.org/x/exp/slices"
)

func TestTCPConfig(t *testing.T) {
	var testCases = []struct {
		str string
		cfg []TCPConfig
	}{
		{
			`[[tcp]]
		listen = "127.0.0.1:1234"
		readOnly = true`,
			[]TCPConfig{
				{
					Listen:   "127.0.0.1:1234",
					ReadOnly: true,
				},
			},
		},
		{
			"", []TCPConfig{},
		},
	}
	for i, tc := range testCases {
		r := strings.NewReader(tc.str)
		cfg, err := readConfig(r)
		if err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(cfg.TCP, tc.cfg) {
			t.Fatalf("TCP config %d not parsed correctly", i)
		}
	}
}
