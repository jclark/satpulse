package pollsim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigFaultValidation(t *testing.T) {
	for _, tc := range []struct {
		table, values, field string
	}{
		{"outage", "start = -1", "start"},
		{"stall", "duration = -1", "duration"},
		{"stalls", "rate = -1\nmin = 0.001\nmax = 0.002", "rate"},
		{"stalls", "rate = 1\nmin = 0\nmax = 0.002", "min"},
		{"slow", "factor = 0", "factor"},
		{"slows", "rate = -1\nmin = 0.001\nmax = 0.002\nfactor = 2", "rate"},
		{"slows", "rate = 1\nmin = 0.001\nmax = 100\nfactor = 2", "max"},
	} {
		t.Run(tc.table+"."+tc.field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.toml")
			if err := os.WriteFile(path, []byte("[[fault."+tc.table+"]]\n"+tc.values+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := DefaultConfig()
			err := LoadConfig(path, &cfg)
			if want := "fault." + tc.table + "[0]." + tc.field; err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() = %v, want error naming %s", err, want)
			}
		})
	}
	if cfg := DefaultConfig(); cfg.Validate() != nil {
		t.Fatal("default configuration is invalid")
	}
}
