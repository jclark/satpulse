package serialcmd

import (
	"bytes"
	"testing"

	"github.com/jclark/satpulse/gps/lib/serialenum"
)

func TestParseFlags(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, help, _, err := parseFlags("serial", nil)
		if err != nil || help || cfg.jsonl {
			t.Fatalf("parseFlags() = %+v, %v, %v", cfg, help, err)
		}
	})
	t.Run("jsonl", func(t *testing.T) {
		cfg, help, _, err := parseFlags("serial", []string{"-j"})
		if err != nil || help || !cfg.jsonl {
			t.Fatalf("parseFlags() = %+v, %v, %v", cfg, help, err)
		}
	})
	t.Run("argument", func(t *testing.T) {
		_, _, _, err := parseFlags("serial", []string{"/dev/ttyS0"})
		if err == nil {
			t.Fatal("parseFlags() succeeded with an argument")
		}
	})
}

func TestPrintPorts(t *testing.T) {
	ports := []serialenum.Port{
		{Device: "/dev/ttyS0", Display: "/dev/ttyS0"},
		{
			Device:  "/dev/ttyACM0",
			Display: "/dev/ttyACM0 (/dev/gps0, u-blox gen 10)",
			USB:     serialenum.USBID{VID: 0x1546, PID: 0x01a4},
		},
	}
	for _, tc := range []struct {
		name  string
		jsonl bool
		want  string
	}{
		{
			name: "human",
			want: "/dev/ttyS0\n/dev/ttyACM0 (/dev/gps0, u-blox gen 10)\n",
		},
		{
			name:  "jsonl",
			jsonl: true,
			want:  "{\"device\":\"/dev/ttyS0\",\"display\":\"/dev/ttyS0\"}\n{\"device\":\"/dev/ttyACM0\",\"display\":\"/dev/ttyACM0 (/dev/gps0, u-blox gen 10)\",\"usb\":{\"vid\":5446,\"pid\":420}}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := printPorts(&buf, ports, tc.jsonl); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("output:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestNoPortsExitCode(t *testing.T) {
	if got := (noPortsError{}).ExitCode(); got != 2 {
		t.Errorf("ExitCode() = %d, want 2", got)
	}
}
