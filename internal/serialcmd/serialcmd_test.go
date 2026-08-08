package serialcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/lib/term"
)

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		want     flags
		wantErr  bool
		wantHelp bool
	}{
		{name: "describe all"},
		{name: "jsonl", args: []string{"-j"}, want: flags{jsonl: true}},
		{name: "describe selected", args: []string{"/dev/ttyS0"}, want: flags{port: "/dev/ttyS0"}},
		{name: "jsonl selected", args: []string{"-j", "/dev/ttyS0"}, want: flags{jsonl: true, port: "/dev/ttyS0"}},
		{name: "detect all", args: []string{"-s"}, want: flags{detect: true}},
		{name: "detect selected", args: []string{"--detect-speed", "--packet-log", "capture.jsonl", "/dev/ttyS0"}, want: flags{detect: true, packetLog: "capture.jsonl", port: "/dev/ttyS0"}},
		{name: "help", args: []string{"-h"}, wantHelp: true},
		{name: "too many ports", args: []string{"one", "two"}, wantErr: true},
		{name: "jsonl detection conflict", args: []string{"--jsonl", "--detect-speed"}, wantErr: true},
		{name: "packet log without detection", args: []string{"--packet-log", "capture.jsonl", "/dev/ttyS0"}, wantErr: true},
		{name: "packet log without port", args: []string{"-s", "--packet-log", "capture.jsonl"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, help, _, err := parseFlags("serial", tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseFlags() error = %v, wantErr %v", err, tc.wantErr)
			}
			if help != tc.wantHelp {
				t.Errorf("help = %v, want %v", help, tc.wantHelp)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("flags = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCmdUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--bogus"},
		{"--jsonl", "--detect-speed"},
	} {
		usage, err := Cmd(io.Discard, slog.LevelInfo, "satpulsetool", "serial", args)
		if err == nil {
			t.Fatalf("Cmd(%q) returned nil error", args)
		}
		if usage == "" {
			t.Errorf("Cmd(%q) returned empty usage", args)
		}
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			t.Errorf("Cmd(%q) error has explicit exit code %d", args, exitErr.ExitCode())
		}
	}
}

func TestPrintPorts(t *testing.T) {
	ports := []serialenum.Port{
		{Device: "/dev/ttyS0", Display: "/dev/ttyS0"},
		{
			Device:  "/dev/ttyACM0",
			Display: "/dev/ttyACM0 (/dev/gps0, u-blox gen 10)",
			USB:     serialenum.USBID{VID: 0x1546, PID: 0x01a4},
			Serial:  "BG02DBNX",
		},
	}
	for _, tc := range []struct {
		name  string
		jsonl bool
		want  string
	}{
		{
			name: "human",
			want: "/dev/ttyS0\n/dev/ttyACM0 (/dev/gps0, u-blox gen 10) serial=\"BG02DBNX\"\n",
		},
		{
			name:  "jsonl",
			jsonl: true,
			want:  "{\"device\":\"/dev/ttyS0\",\"display\":\"/dev/ttyS0\"}\n{\"device\":\"/dev/ttyACM0\",\"display\":\"/dev/ttyACM0 (/dev/gps0, u-blox gen 10)\",\"usb\":{\"vid\":5446,\"pid\":420},\"serial\":\"BG02DBNX\"}\n",
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

func TestPrintPortInfo(t *testing.T) {
	ports := []serialenum.Port{
		{Device: "/dev/ttyS0", Display: "/dev/ttyS0"},
		{Device: "/dev/ttyACM0", Display: "/dev/ttyACM0 (/dev/gps0)", Serial: "BG02DBNX", Aliases: []string{"/dev/gps0"}},
	}
	for _, tc := range []struct {
		name     string
		selector string
		want     string
		wantCode int
	}{
		{name: "all", want: "/dev/ttyS0\n/dev/ttyACM0 (/dev/gps0) serial=\"BG02DBNX\"\n"},
		{name: "device", selector: "/dev/ttyACM0", want: "/dev/ttyACM0 (/dev/gps0) serial=\"BG02DBNX\"\n"},
		{name: "alias", selector: "/dev/gps0", want: "/dev/ttyACM0 (/dev/gps0) serial=\"BG02DBNX\"\n"},
		{name: "unclean", selector: "/dev//ttyS0", want: "/dev/ttyS0\n"},
		{name: "unmatched", selector: "/dev/ttyUSB0", wantCode: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := printPortInfo(&buf, ports, false, tc.selector)
			if tc.wantCode != 0 {
				var cmdErr commandError
				if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != tc.wantCode {
					t.Fatalf("printPortInfo() error = %#v, want exit code %d", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintPortInfoNoPorts(t *testing.T) {
	err := printPortInfo(io.Discard, nil, false, "")
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 2 {
		t.Fatalf("printPortInfo() error = %#v, want exit code 2", err)
	}
}

func TestProbeResultExitCode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result probeResult
		want   int
	}{
		{"found", probeResult{detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 38400}}, 0},
		{"silent", probeResult{}, 2},
		{"unrecognized", probeResult{detection: gpsio.DetectResult{Outcome: gpsio.DetectUnrecognized}}, 1},
		{"error", probeResult{failure: "speed detection requires a serial device"}, 1},
		{"error outranks silence", probeResult{failure: "close failed"}, 1},
	} {
		if got := tc.result.exitCode(); got != tc.want {
			t.Errorf("%s: exitCode() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestProbeResultDescription(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result probeResult
		want   string
	}{
		{"silent", probeResult{}, "no output received from the device"},
		{"unrecognized", probeResult{detection: gpsio.DetectResult{Outcome: gpsio.DetectUnrecognized}},
			"output was received, but no known GNSS protocol was validated at a candidate speed"},
		{"described error", probeResult{failure: "speed detection requires a serial device"},
			"speed detection requires a serial device"},
		{"packet log permission", probeResult{failure: "opening packet log: permission denied"},
			"opening packet log: permission denied"},
	} {
		if got := tc.result.description(); got != tc.want {
			t.Errorf("%s: description() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDescribeSerialError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.Canceled, "interrupted"},
		{fmtWrap(os.ErrPermission), "permission denied; add this user to the serial-port access group (usually dialout)"},
		{fmtWrap(testLockedError{}), "device is locked by another process"},
		{term.ErrNotATTY, "speed detection requires a serial device"},
		{errors.New("gone"), "gone"},
	} {
		if got := describeSerialError(tc.err); got != tc.want {
			t.Errorf("describeSerialError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("wrapper"), err) }

type testLockedError struct{}

func (testLockedError) Error() string { return "locked" }
func (testLockedError) Locked() bool  { return true }

func TestScanPortList(t *testing.T) {
	ports := []serialenum.Port{{Device: "detected"}, {Device: "silent"}, {Device: "error"}}
	probe := func(_ context.Context, lg *slog.Logger, device, packetLog string) probeResult {
		if packetLog != "" {
			t.Errorf("scan packet log = %q, want empty", packetLog)
		}
		lg.Info("probing")
		switch device {
		case "detected":
			return probeResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 38400}}
		case "silent":
			return probeResult{device: device}
		default:
			return probeResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectUnrecognized}}
		}
	}
	var stdout, stderr, logBuf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logBuf, nil))
	if err := scanPortList(context.Background(), lg, ports, probe, &stdout, &stderr); err != nil {
		t.Fatalf("scanPortList() error = %v, want nil because one device was detected", err)
	}
	if got := stdout.String(); got != "detected 38400\n" {
		t.Errorf("stdout = %q", got)
	}
	for _, want := range []string{
		"silent: no output received from the device\n",
		"error: output was received, but no known GNSS protocol was validated at a candidate speed\n",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr %q does not contain %q", stderr.String(), want)
		}
	}
	for _, port := range ports {
		if want := "device=" + port.Device; !strings.Contains(logBuf.String(), want) {
			t.Errorf("log %q does not contain %q", logBuf.String(), want)
		}
	}
}

func TestScanPortListBestFailure(t *testing.T) {
	ports := []serialenum.Port{{Device: "silent"}, {Device: "error"}}
	probe := func(_ context.Context, _ *slog.Logger, device, _ string) probeResult {
		if device == "silent" {
			return probeResult{device: device}
		}
		return probeResult{device: device, failure: "failed"}
	}
	err := scanPortList(context.Background(), slog.Default(), ports, probe, io.Discard, io.Discard)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 2 || !cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want quiet exit code 2", err)
	}
}

func TestScanPortListInterruptedOverridesDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ports := []serialenum.Port{{Device: "detected"}, {Device: "cancelled"}}
	probe := func(_ context.Context, _ *slog.Logger, device, _ string) probeResult {
		if device == "detected" {
			return probeResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 115200}}
		}
		return probeResult{device: device, failure: "interrupted"}
	}
	var stderr bytes.Buffer
	err := scanPortList(ctx, slog.Default(), ports, probe, io.Discard, &stderr)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || !cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want quiet exit code 1", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want nothing reported for an interrupted scan", got)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestScanPortListOutputError(t *testing.T) {
	ports := []serialenum.Port{{Device: "detected"}}
	probe := func(_ context.Context, _ *slog.Logger, device, _ string) probeResult {
		return probeResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 38400}}
	}
	err := scanPortList(context.Background(), slog.Default(), ports, probe, errorWriter{}, io.Discard)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want non-quiet exit code 1", err)
	}
}
