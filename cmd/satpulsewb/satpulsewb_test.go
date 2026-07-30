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

func TestNoOpenBrowserShorthand(t *testing.T) {
	v, usage, err := parseFlags([]string{"-n"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.noOpen {
		t.Error("-n did not set no-open-browser")
	}
	s := usage("satpulsewb")
	for _, want := range []string{"[-n|--no-open-browser]", "-n, --no-open-browser"} {
		if !strings.Contains(s, want) {
			t.Errorf("help does not contain %q:\n%s", want, s)
		}
	}
}

func TestConnectionFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		device      string
		speed       int
		autoConnect bool
	}{
		{name: "defaults"},
		{name: "device only", args: []string{"-d", "DEV"}, device: "DEV"},
		{name: "speed only", args: []string{"-s", "38400"}, speed: 38400},
		{name: "device and speed", args: []string{"-d", "DEV", "-s", "38400"}, device: "DEV", speed: 38400, autoConnect: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, _, err := parseFlags(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if v.device != tc.device || v.speed != tc.speed || v.autoConnect != tc.autoConnect {
				t.Errorf("connection flags = (%q, %d, %v), want (%q, %d, %v)",
					v.device, v.speed, v.autoConnect, tc.device, tc.speed, tc.autoConnect)
			}
		})
	}
}

func TestZeroDeviceSpeedRejected(t *testing.T) {
	for _, speed := range []string{"0", "-1"} {
		if _, _, err := parseFlags([]string{"-s", speed}); err == nil {
			t.Errorf("-s %s succeeded", speed)
		}
	}
}
