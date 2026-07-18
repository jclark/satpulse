package main

import (
	"log/slog"
	"strings"
	"testing"
)

func TestVerboseHelp(t *testing.T) {
	_, usage, err := parseFlags([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	s := usage("satpulsewb")
	for _, want := range []string{"[-v|--verbose]", "-v, --verbose", "increase logging verbosity"} {
		if !strings.Contains(s, want) {
			t.Errorf("help does not contain %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "--verbose count") {
		t.Errorf("help gives --verbose a count argument:\n%s", s)
	}
}

func TestVerboseRepeated(t *testing.T) {
	for _, args := range [][]string{{"-v"}, {"-vv"}, {"--verbose", "--verbose"}} {
		v, _, err := parseFlags(args)
		if err != nil {
			t.Errorf("parseFlags(%q): %v", args, err)
			continue
		}
		if v.logLevel != slog.LevelDebug {
			t.Errorf("parseFlags(%q) log level = %v, want %v", args, v.logLevel, slog.LevelDebug)
		}
	}
}

func TestTokenShorthand(t *testing.T) {
	v, usage, err := parseFlags([]string{"-t"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.token {
		t.Error("-t did not enable the access token")
	}
	s := usage("satpulsewb")
	for _, want := range []string{"[-t|--token]", "-t, --token"} {
		if !strings.Contains(s, want) {
			t.Errorf("help does not contain %q:\n%s", want, s)
		}
	}
}
