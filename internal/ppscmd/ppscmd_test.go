package ppscmd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		wantErr string
	}{
		{args: nil},
		{args: []string{"-d", "/dev/pps0", "-t", "0", "-j"}},
		{args: []string{"-g", "18", "--cpu", "3", "--priority", "40", "--max-bracket", "5e-6", "-t", "150", "-j"}},
		{args: []string{"-t", "5"}, wantErr: "requires --pps-device or --gpio-pin"},
		{args: []string{"-d", "/dev/pps0", "-g", "18"}, wantErr: "cannot be combined"},
		{args: []string{"--cpu", "3"}, wantErr: "--cpu requires --gpio-pin"},
		{args: []string{"-g", "18", "--priority", "100"}, wantErr: "between 0 and 99"},
		{args: []string{"-g", "-1"}, wantErr: "--gpio-pin must not be negative"},
		{args: []string{"-g", "18", "--max-bracket", "NaN"}, wantErr: "--max-bracket must be"},
		{args: []string{"-g", "0", "--cpu", "0"}},
		{args: []string{"-d", "/dev/pps0", "-t", "-1"}, wantErr: "not be negative"},
		{args: []string{"-d", "/dev/pps0", "-t", "NaN"}, wantErr: "must be finite"},
		{args: []string{"-d", "/dev/pps0", "-t", "1e-10"}, wantErr: "too small"},
		{args: []string{"-d", "/dev/pps0", "-t", "1e300"}, wantErr: "too large"},
		{args: []string{"/dev/pps0"}, wantErr: "positional"},
	} {
		_, _, _, err := parseFlags("pps", tc.args)
		if tc.wantErr == "" && err != nil {
			t.Errorf("parseFlags(%q) error %v, want none", tc.args, err)
		} else if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("parseFlags(%q) error %v, want containing %q", tc.args, err, tc.wantErr)
		}
	}
}

func TestListDevices(t *testing.T) {
	dir := t.TempDir()
	write := func(dev, attr, value string) {
		if err := os.MkdirAll(filepath.Join(dir, dev), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, dev, attr), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pps0", "name", "pps@.-1")
	write("pps0", "path", "")
	write("pps0", "mode", "1151")
	write("pps1", "name", "ttyAMA0")
	write("pps1", "path", "/dev/ttyAMA0")
	write("pps1", "mode", "1133")
	write("pps2", "name", "broken")
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	var out bytes.Buffer
	if err := listDevices(lg, dir, &out, false); err != nil {
		t.Fatal(err)
	}
	want := "device=/dev/pps0 name=\"pps@.-1\" capture=assert echo=assert\n" +
		"device=/dev/pps1 name=\"ttyAMA0\" path=\"/dev/ttyAMA0\" capture=assert,clear\n"
	if out.String() != want {
		t.Errorf("listing =\n%swant\n%s", out.String(), want)
	}
	out.Reset()
	if err := listDevices(lg, dir, &out, true); err != nil {
		t.Fatal(err)
	}
	if want := `{"device":"/dev/pps1","name":"ttyAMA0","path":"/dev/ttyAMA0","capture":["assert","clear"]}`; !strings.Contains(out.String(), want+"\n") {
		t.Errorf("JSONL listing %q does not contain %q", out.String(), want)
	}
	err := listDevices(lg, filepath.Join(dir, "none"), &out, false)
	if _, ok := err.(noDataError); !ok {
		t.Errorf("listing a missing directory returned %v, want noDataError", err)
	}
}
