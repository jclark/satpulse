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
)

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		want     flags
		wantErr  bool
		wantHelp bool
	}{
		{name: "enumerate"},
		{name: "jsonl", args: []string{"-j"}, want: flags{jsonl: true}},
		{name: "device", args: []string{"--packet-log", "capture.jsonl", "/dev/ttyS0"}, want: flags{packetLog: "capture.jsonl", device: "/dev/ttyS0"}},
		{name: "scan", args: []string{"-s"}, want: flags{scan: true}},
		{name: "help", args: []string{"-h"}, wantHelp: true},
		{name: "too many devices", args: []string{"one", "two"}, wantErr: true},
		{name: "scan device conflict", args: []string{"--scan", "/dev/ttyS0"}, wantErr: true},
		{name: "json detection conflict", args: []string{"--jsonl", "/dev/ttyS0"}, wantErr: true},
		{name: "packet log enumeration conflict", args: []string{"--packet-log", "capture.jsonl"}, wantErr: true},
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

func TestProbeResultExitCode(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{nil, 0},
		{gpsio.ErrSilent, 2},
		{gpsio.ErrSpeedNotDetected, 1},
	} {
		if got := (probeResult{err: tc.err}).exitCode(); got != tc.want {
			t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}

func TestDescribeProbeError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.Canceled, "interrupted"},
		{fmtWrap(os.ErrPermission), "permission denied; add this user to the serial-port access group (usually dialout)"},
		{fmtWrap(testLockedError{}), "device is locked by another process"},
		{gpsio.ErrSilent, "no output received from the device"},
		{gpsio.ErrSpeedNotDetected, "output was received, but no known GNSS protocol was validated at a candidate speed"},
		{gpsio.ErrNotSerial, "speed detection requires a serial device"},
		{gpsio.ErrCurrentSpeedUnknown, "the device's current serial speed is not supported"},
		{errors.New("gone"), "gone"},
	} {
		if got := describeProbeError(tc.err); got != tc.want {
			t.Errorf("describeProbeError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("wrapper"), err) }

type testLockedError struct{}

func (testLockedError) Error() string { return "locked" }
func (testLockedError) Locked() bool  { return true }

func TestScanPortList(t *testing.T) {
	ports := []serialenum.Port{{Device: "detected"}, {Device: "silent"}, {Device: "error"}}
	probe := func(_ context.Context, _ *slog.Logger, device, packetLog string) probeResult {
		if packetLog != "" {
			t.Errorf("scan packet log = %q, want empty", packetLog)
		}
		switch device {
		case "detected":
			return probeResult{device: device, speed: 38400}
		case "silent":
			return probeResult{device: device, err: gpsio.ErrSilent}
		default:
			return probeResult{device: device, err: gpsio.ErrSpeedNotDetected}
		}
	}
	var stdout, stderr bytes.Buffer
	if err := scanPortList(context.Background(), slog.Default(), ports, probe, &stdout, &stderr); err != nil {
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
}

func TestScanPortListBestFailure(t *testing.T) {
	ports := []serialenum.Port{{Device: "silent"}, {Device: "error"}}
	probe := func(_ context.Context, _ *slog.Logger, device, _ string) probeResult {
		if device == "silent" {
			return probeResult{device: device, err: gpsio.ErrSilent}
		}
		return probeResult{device: device, err: errors.New("failed")}
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
			return probeResult{device: device, speed: 115200}
		}
		return probeResult{device: device, err: context.Canceled}
	}
	err := scanPortList(ctx, slog.Default(), ports, probe, io.Discard, io.Discard)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || !cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want quiet exit code 1", err)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestScanPortListOutputError(t *testing.T) {
	ports := []serialenum.Port{{Device: "detected"}}
	probe := func(_ context.Context, _ *slog.Logger, device, _ string) probeResult {
		return probeResult{device: device, speed: 38400}
	}
	err := scanPortList(context.Background(), slog.Default(), ports, probe, errorWriter{}, io.Discard)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want non-quiet exit code 1", err)
	}
}

func TestProbeResultCleanupFailure(t *testing.T) {
	r := probeResult{
		err:           errors.Join(gpsio.ErrSilent, errors.New("close failed")),
		cleanupFailed: true,
	}
	if got := r.exitCode(); got != 1 {
		t.Errorf("exitCode() = %d, want 1", got)
	}
	if got := r.description(); got != "no output from serial device\nclose failed" {
		t.Errorf("description() = %q", got)
	}
}
