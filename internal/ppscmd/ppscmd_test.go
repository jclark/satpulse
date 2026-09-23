package ppscmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/kpps"
)

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		wantErr string
	}{
		{args: nil},
		{args: []string{"-d", "/dev/pps0", "-t", "0", "-j"}},
		{args: []string{"-t", "5"}, wantErr: "requires --pps-device"},
		{args: []string{"-d", ""}, wantErr: "must not be empty"},
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
	devices := []kpps.Device{
		{Path: "/dev/pps0", Name: "pps@.-1", Mode: 0x1151},
		{Path: "/dev/pps1", Name: "ttyAMA0", SourcePath: "/dev/ttyAMA0", Mode: 0x1133},
	}
	var out bytes.Buffer
	if err := listDevices(devices, &out, false); err != nil {
		t.Fatal(err)
	}
	expect := "device=/dev/pps0 name=\"pps@.-1\" capture=assert echo=assert\n" +
		"device=/dev/pps1 name=\"ttyAMA0\" sourcePath=\"/dev/ttyAMA0\" capture=assert,clear\n"
	if out.String() != expect {
		t.Errorf("listing =\n%swant\n%s", out.String(), expect)
	}
	out.Reset()
	if err := listDevices(devices, &out, true); err != nil {
		t.Fatal(err)
	}
	if expect := `{"device":"/dev/pps1","name":"ttyAMA0","sourcePath":"/dev/ttyAMA0","capture":["assert","clear"]}`; !strings.Contains(out.String(), expect+"\n") {
		t.Errorf("JSONL listing %q does not contain %q", out.String(), expect)
	}
	if err := listDevices(nil, &out, false); err == nil || err.(noDataError).msg == "" {
		t.Errorf("listing no devices returned %v, want noDataError", err)
	}
}
