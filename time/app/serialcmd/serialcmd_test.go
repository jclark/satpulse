package serialcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/serialpps"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/lib/wakeup"
	"github.com/jclark/satpulse/gps/scan"
)

func TestParseFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		want     flagVars
		wantErr  bool
		wantHelp bool
	}{
		{name: "default info all", want: flagVars{info: true, all: true}},
		{name: "jsonl info all", args: []string{"-j"}, want: flagVars{jsonl: true, info: true, all: true}},
		{name: "explicit info all", args: []string{"-i"}, want: flagVars{info: true, all: true}},
		{name: "info selected", args: []string{"-i", "-d", "/dev/ttyS0"}, want: flagVars{info: true, device: "/dev/ttyS0"}},
		{name: "detect all", args: []string{"-a"}, want: flagVars{all: true}},
		{name: "detect selected", args: []string{"-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0"}},
		{name: "detect with packet log", args: []string{"-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, want: flagVars{device: "/dev/ttyS0", packetLog: "capture.jsonl"}},
		{name: "capture at speed", args: []string{"-j", "-s", "38400", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, want: flagVars{jsonl: true, device: "/dev/ttyS0", deviceSpeed: 38400, deviceSpeedSet: true, packetLog: "capture.jsonl"}},
		{name: "timed capture", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "1.25"}, want: flagVars{device: "/dev/ttyS0", deviceSpeed: 38400, deviceSpeedSet: true, packetLog: "capture.jsonl", timeout: 1250 * time.Millisecond}},
		{name: "capture at current speed", args: []string{"-s", "0", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, want: flagVars{device: "/dev/ttyS0", deviceSpeedSet: true, packetLog: "capture.jsonl"}},
		{name: "pps on cts", args: []string{"-p", "cts", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemCTS, timeout: defaultPPSTimeout}},
		{name: "pps all ports", args: []string{"--pps-pin", "dcd", "-a"}, want: flagVars{all: true, ppsSet: true, ppsPin: gpsio.ModemDCD, timeout: defaultPPSTimeout}},
		{name: "pps at speed", args: []string{"-p", "ri", "-s", "38400", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemRI, deviceSpeed: 38400, deviceSpeedSet: true, timeout: defaultPPSTimeout}},
		{name: "pps with packet log and timeout", args: []string{"-p", "dsr", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl", "-t", "30"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemDSR, packetLog: "capture.jsonl", timeout: 30 * time.Second}},
		{name: "pps until interrupted", args: []string{"-p", "cts", "-t", "0", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemCTS}},
		{name: "pps JSONL", args: []string{"-j", "-p", "cts", "-a"}, want: flagVars{jsonl: true, all: true, ppsSet: true, ppsPin: gpsio.ModemCTS, timeout: defaultPPSTimeout}},
		{name: "pps by polling", args: []string{"-p", "cts", "-m", "poll", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemCTS, ppsMethod: gpsio.PPSMethodPoll, timeout: defaultPPSTimeout}},
		{name: "pps by waiting", args: []string{"-p", "cts", "--pps-method", "wait", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemCTS, ppsMethod: gpsio.PPSMethodWait, timeout: defaultPPSTimeout}},
		{name: "pps with kernel timestamps", args: []string{"-p", "dcd", "--pps-method", "kernel", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemDCD, ppsMethod: gpsio.PPSMethodKernel, timeout: defaultPPSTimeout}},
		{name: "pps with wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "10.5", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemCTS, maxWakeupLatency: 10*time.Microsecond + 500*time.Nanosecond, maxWakeupLatencySet: true, timeout: defaultPPSTimeout}},
		{name: "pps with zero wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "0", "-d", "/dev/ttyS0"}, want: flagVars{device: "/dev/ttyS0", ppsSet: true, ppsPin: gpsio.ModemCTS, maxWakeupLatencySet: true, timeout: defaultPPSTimeout}},
		{name: "help", args: []string{"-h"}, wantHelp: true},
		{name: "positional port", args: []string{"/dev/ttyS0"}, wantErr: true},
		{name: "all and device", args: []string{"-a", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "info and speed", args: []string{"-i", "-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "info and packet log", args: []string{"-i", "-d", "/dev/ttyS0", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "info and timeout", args: []string{"-i", "-d", "/dev/ttyS0", "-t", "1"}, wantErr: true},
		{name: "info and pps", args: []string{"-i", "-p", "cts", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "method without pps", args: []string{"-m", "poll", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "invalid method", args: []string{"-p", "cts", "-m", "sideband", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "wakeup latency without pps", args: []string{"--max-wakeup-latency", "10", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "negative wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "-1", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "NaN wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "NaN", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "infinite wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "+Inf", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "overflowing wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "1e20", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "under-resolution wakeup latency", args: []string{"-p", "cts", "--max-wakeup-latency", "1e-10", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "pps without target", args: []string{"-p", "cts"}, wantErr: true},
		{name: "pps invalid pin", args: []string{"-p", "rts", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "pps value required", args: []string{"--pps-pin", "-d", "/dev/ttyS0"}, wantErr: true},
		{name: "pps speed without device", args: []string{"-a", "-p", "cts", "-s", "38400"}, wantErr: true},
		{name: "pps packet log with all", args: []string{"-a", "-p", "cts", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "speed without device", args: []string{"-s", "38400", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "speed with all", args: []string{"-a", "-s", "38400", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "speed without purpose", args: []string{"-d", "/dev/ttyS0", "-s", "38400"}, wantErr: true},
		{name: "negative speed", args: []string{"-d", "/dev/ttyS0", "-s", "-1", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "non-numeric speed", args: []string{"-d", "/dev/ttyS0", "-s", "auto", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "packet log without device", args: []string{"-a", "--packet-log", "capture.jsonl"}, wantErr: true},
		{name: "timeout without purpose", args: []string{"-d", "/dev/ttyS0", "-t", "1"}, wantErr: true},
		{name: "timeout with detection", args: []string{"-d", "/dev/ttyS0", "--packet-log", "capture.jsonl", "-t", "1"}, wantErr: true},
		{name: "negative timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "-1"}, wantErr: true},
		{name: "negative pps timeout", args: []string{"-p", "cts", "-d", "/dev/ttyS0", "-t", "-1"}, wantErr: true},
		{name: "NaN timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "NaN"}, wantErr: true},
		{name: "infinite timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "+Inf"}, wantErr: true},
		{name: "overflowing timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "1e20"}, wantErr: true},
		{name: "underflowing timeout", args: []string{"-d", "/dev/ttyS0", "-s", "38400", "--packet-log", "capture.jsonl", "-t", "1e-10"}, wantErr: true},
		{name: "empty device", args: []string{"--serial-device", ""}, wantErr: true},
		{name: "empty packet log", args: []string{"--serial-device", "/dev/ttyS0", "--packet-log", ""}, wantErr: true},
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

func TestParseWakeupLatencyResolution(t *testing.T) {
	resolution := wakeup.LatencyResolution.Seconds() * 1e6
	if _, err := parseWakeupLatency(resolution / 2); err == nil {
		t.Fatal("parseWakeupLatency accepted a positive value below LatencyResolution")
	}
	if got, err := parseWakeupLatency(resolution); err != nil || got != wakeup.LatencyResolution {
		t.Fatalf("parseWakeupLatency(LatencyResolution) = %v, %v", got, err)
	}
}

func TestCmdUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--bogus"},
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

func TestParseFlagsTimeoutDependencies(t *testing.T) {
	const want = "--timeout requires --pps-pin, or --device-speed and --packet-log"
	for _, args := range [][]string{
		{"-d", "/dev/ttyS0", "-t", "1"},
		{"-d", "/dev/ttyS0", "-s", "38400", "-t", "1"},
		{"-d", "/dev/ttyS0", "--packet-log", "capture.jsonl", "-t", "1"},
	} {
		_, _, _, err := parseFlags("serial", args)
		if err == nil || err.Error() != want {
			t.Errorf("parseFlags(%q) error = %v, want %q", args, err, want)
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
			Aliases: []string{"/dev/gps0"},
		},
	}
	for _, tc := range []struct {
		name  string
		jsonl bool
		want  string
	}{
		{
			name: "human",
			want: "device=/dev/ttyS0 display=\"/dev/ttyS0\"\n" +
				"device=/dev/ttyACM0 vid=1546 pid=01a4 serial=\"BG02DBNX\" alias=/dev/gps0 display=\"/dev/ttyACM0 (/dev/gps0, u-blox gen 10)\"\n",
		},
		{
			name:  "jsonl",
			jsonl: true,
			want:  "{\"device\":\"/dev/ttyS0\",\"display\":\"/dev/ttyS0\"}\n{\"device\":\"/dev/ttyACM0\",\"display\":\"/dev/ttyACM0 (/dev/gps0, u-blox gen 10)\",\"usb\":{\"vid\":5446,\"pid\":420},\"serial\":\"BG02DBNX\",\"aliases\":[\"/dev/gps0\"]}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := outputFile(t)
			if err := printPorts(f, ports, tc.jsonl); err != nil {
				t.Fatal(err)
			}
			if got := outputString(t, f); got != tc.want {
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
		{name: "all", want: "device=/dev/ttyS0 display=\"/dev/ttyS0\"\ndevice=/dev/ttyACM0 serial=\"BG02DBNX\" alias=/dev/gps0 display=\"/dev/ttyACM0 (/dev/gps0)\"\n"},
		{name: "device", selector: "/dev/ttyACM0", want: "device=/dev/ttyACM0 serial=\"BG02DBNX\" alias=/dev/gps0 display=\"/dev/ttyACM0 (/dev/gps0)\"\n"},
		{name: "alias", selector: "/dev/gps0", want: "device=/dev/ttyACM0 serial=\"BG02DBNX\" alias=/dev/gps0 display=\"/dev/ttyACM0 (/dev/gps0)\"\n"},
		{name: "unmatched", selector: "/dev/ttyUSB0", wantCode: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := outputFile(t)
			err := printPortInfo(f, ports, false, tc.selector)
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
			if got := outputString(t, f); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintPortInfoNoPorts(t *testing.T) {
	err := printPortInfo(outputFile(t), nil, false, "")
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 2 {
		t.Fatalf("printPortInfo() error = %#v, want exit code 2", err)
	}
}

func TestPrintSpeedInfo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		jsonl bool
		want  string
	}{
		{name: "human", want: "38400\n"},
		{name: "JSONL", jsonl: true, want: "{\"device\":\"/dev/ttyUSB0\",\"speed\":38400}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := outputFile(t)
			info := speedInfo{Device: "/dev/ttyUSB0", Speed: 38400}
			if err := printInfo(f, &info, tc.jsonl); err != nil {
				t.Fatal(err)
			}
			if got := outputString(t, f); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEdgePrinter(t *testing.T) {
	edge := serialpps.Edge{
		Wall: time.Date(2026, time.August, 12, 21, 23, 5, 123_456_499, time.FixedZone("ICT", 7*60*60)),
	}
	for _, tc := range []struct {
		name       string
		edge       serialpps.CandidateEdge
		jsonl      bool
		withDevice bool
		want       string
	}{
		{name: "human", edge: serialpps.CandidateEdge{Edge: edge}, want: "14:23:05.123456\n"},
		{name: "human with device", edge: serialpps.CandidateEdge{Edge: edge}, withDevice: true, want: "/dev/ttyS0 14:23:05.123456\n"},
		{name: "wait JSONL", edge: serialpps.CandidateEdge{Edge: edge, Settled: true}, jsonl: true,
			want: "{\"device\":\"/dev/ttyS0\",\"t\":\"2026-08-12T14:23:05.123456Z\"}\n"},
		{name: "settling poll JSONL", edge: serialpps.CandidateEdge{Edge: edge, Uncertainty: 16 * time.Microsecond}, jsonl: true,
			want: "{\"device\":\"/dev/ttyS0\",\"t\":\"2026-08-12T14:23:05.123456Z\",\"uncertainty\":0.000016,\"settling\":true}\n"},
		{name: "settled poll JSONL", edge: serialpps.CandidateEdge{Edge: edge, Uncertainty: 16 * time.Microsecond, Settled: true}, jsonl: true,
			want: "{\"device\":\"/dev/ttyS0\",\"t\":\"2026-08-12T14:23:05.123456Z\",\"uncertainty\":0.000016}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			pr := &edgePrinter{out: &output, jsonl: tc.jsonl, withDevice: tc.withDevice}
			if err := pr.print("/dev/ttyS0", tc.edge); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

type monitorWaitConn struct {
	state  gpsio.ModemControlPinState
	next   chan gpsio.ModemControlPinState
	waits  int
	method gpsio.PPSMethod
}

func (c *monitorWaitConn) ModemControlPinState() (gpsio.ModemControlPinState, error) {
	return c.state, nil
}

func (c *monitorWaitConn) WaitModemControlPinChange(ctx context.Context, pin gpsio.ModemControlPin, method gpsio.PPSMethod) (gpsio.ModemControlPinChange, int, error) {
	c.waits++
	c.method = method
	select {
	case c.state = <-c.next:
		t := time.Date(2026, time.August, 12, 14, 23, 5, 123_456_000, time.UTC)
		return gpsio.ModemControlPinChange{Wall: t, Mono: t, Asserted: c.state.Asserted(pin)}, 0, nil
	case <-ctx.Done():
		return gpsio.ModemControlPinChange{}, 0, ctx.Err()
	}
}

type notifyingWriter struct {
	bytes.Buffer
	wrote chan struct{}
	once  sync.Once
}

func (w *notifyingWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.once.Do(func() { close(w.wrote) })
	return n, err
}

func TestDetectEdgesAutomaticKernelMethod(t *testing.T) {
	asserted := gpsio.ModemControlPinState(1 << gpsio.ModemCTS)
	conn := &monitorWaitConn{
		state: asserted,
		next:  make(chan gpsio.ModemControlPinState, 1),
	}
	conn.next <- 0
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	output := &notifyingWriter{wrote: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		count int
		err   error
	}, 1)
	go func() {
		pr := &edgePrinter{out: output}
		count, err := detectEdges(ctx, lg, conn, gpsio.ModemCTS, "", 0, pr)
		result <- struct {
			count int
			err   error
		}{count, err}
	}()
	select {
	case <-output.wrote:
	case <-time.After(time.Second):
		t.Fatal("detectEdges did not print the kernel edge")
	}
	cancel()
	select {
	case got := <-result:
		if got.count != 1 || got.err != nil {
			t.Fatalf("detectEdges = %d, %v; want 1, nil", got.count, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("detectEdges did not stop the kernel method after cancellation")
	}
	if got := output.String(); got != "14:23:05.123456\n" {
		t.Errorf("output = %q, want one kernel timestamp", got)
	}
	if strings.Contains(logs.String(), "serial PPS polling statistics") {
		t.Errorf("kernel run logged polling statistics: %q", logs.String())
	}
	if conn.method != gpsio.PPSMethodKernel {
		t.Errorf("selected method = %v, want kernel", conn.method)
	}
}

func TestDetectEdgesForcedPollingSkipsWaitBackend(t *testing.T) {
	conn := &monitorWaitConn{
		next: make(chan gpsio.ModemControlPinState),
	}
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	count, err := detectEdges(ctx, lg, conn, gpsio.ModemCTS, "", gpsio.PPSMethodPoll, &edgePrinter{out: io.Discard})
	if err != nil || count != 0 {
		t.Fatalf("detectEdges = %d, %v; want 0, nil", count, err)
	}
	if conn.waits != 0 {
		t.Errorf("wait backend called %d times, want 0", conn.waits)
	}
	if !strings.Contains(logs.String(), "serial PPS polling statistics") {
		t.Errorf("forced polling run did not log polling statistics: %q", logs.String())
	}
}

func TestPPSResult(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   ppsResult
		wantCode int
		wantDesc string
	}{
		{name: "edges", result: ppsResult{edges: 3}},
		{name: "no edges", wantCode: 2, wantDesc: "no PPS edges detected"},
		{name: "failure", result: ppsResult{failure: "device is locked"}, wantCode: 1, wantDesc: "device is locked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.exitCode(); got != tc.wantCode {
				t.Errorf("exitCode() = %d, want %d", got, tc.wantCode)
			}
			if tc.wantCode != 0 {
				if got := tc.result.description(); got != tc.wantDesc {
					t.Errorf("description() = %q, want %q", got, tc.wantDesc)
				}
			}
		})
	}
}

func TestMonitorPortList(t *testing.T) {
	ports := []serialenum.Port{{Device: "pulsing"}, {Device: "quiet"}, {Device: "error"}}
	monitor := func(_ context.Context, lg *slog.Logger, device string) ppsResult {
		lg.Info("monitoring")
		switch device {
		case "pulsing":
			return ppsResult{device: device, edges: 5}
		case "quiet":
			return ppsResult{device: device}
		default:
			return ppsResult{device: device, failure: "device is locked by another process"}
		}
	}
	var stderr, logBuf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logBuf, nil))
	if err := monitorPortList(context.Background(), lg, ports, monitor, &stderr); err != nil {
		t.Fatalf("monitorPortList() error = %v, want nil because one port pulsed", err)
	}
	for _, want := range []string{
		"quiet: no PPS edges detected\n",
		"error: device is locked by another process\n",
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

func TestMonitorPortListNoPulses(t *testing.T) {
	ports := []serialenum.Port{{Device: "quiet"}, {Device: "error"}}
	monitor := func(_ context.Context, _ *slog.Logger, device string) ppsResult {
		if device == "quiet" {
			return ppsResult{device: device}
		}
		return ppsResult{device: device, failure: "failed"}
	}
	err := monitorPortList(context.Background(), slog.Default(), ports, monitor, io.Discard)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 2 || !cmdErr.Quiet() {
		t.Fatalf("monitorPortList() error = %#v, want quiet exit code 2", err)
	}
}

func TestMonitorPortListOutputFailure(t *testing.T) {
	parent, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	reader, writer := io.Pipe()
	reader.CloseWithError(errors.New("broken pipe"))
	defer writer.Close()
	pr := &edgePrinter{out: writer, cancel: cancel}
	ports := []serialenum.Port{{Device: "writer"}, {Device: "waiting"}}
	monitor := func(ctx context.Context, _ *slog.Logger, device string) ppsResult {
		if device == "writer" {
			err := pr.print(device, serialpps.CandidateEdge{})
			return ppsResult{device: device, failure: err.Error()}
		}
		<-ctx.Done()
		return ppsResult{device: device}
	}
	var stderr bytes.Buffer
	err := monitorPortList(ctx, slog.Default(), ports, monitor, &stderr)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || cmdErr.Error() != "writing PPS timestamp: broken pipe" {
		t.Fatalf("monitorPortList() error = %#v, want output failure with exit code 1", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want no per-port errors", stderr.String())
	}
}

func TestDetectResultExitCode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result detectResult
		want   int
	}{
		{"found", detectResult{detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 38400}}, 0},
		{"silent", detectResult{}, 2},
		{"unrecognized", detectResult{detection: gpsio.DetectResult{Outcome: gpsio.DetectUnrecognized}}, 1},
		{"error", detectResult{failure: "speed detection requires a serial device"}, 1},
		{"error outranks silence", detectResult{failure: "close failed"}, 1},
	} {
		if got := tc.result.exitCode(); got != tc.want {
			t.Errorf("%s: exitCode() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestDetectResultDescription(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result detectResult
		want   string
	}{
		{"silent", detectResult{}, "no output received from the device"},
		{"unrecognized", detectResult{detection: gpsio.DetectResult{Outcome: gpsio.DetectUnrecognized}},
			"output was received, but no known GNSS protocol was validated at a candidate speed"},
		{"described error", detectResult{failure: "speed detection requires a serial device"},
			"speed detection requires a serial device"},
		{"packet log permission", detectResult{failure: "opening packet log: permission denied"},
			"opening packet log: permission denied"},
	} {
		if got := tc.result.description(); got != tc.want {
			t.Errorf("%s: description() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCaptureResult(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   captureResult
		wantCode int
		wantDesc string
	}{
		{name: "packets", result: captureResult{packets: 3}},
		{name: "no packets", wantCode: 2, wantDesc: "no output received from the device"},
		{name: "failure", result: captureResult{failure: "device is locked"}, wantCode: 1, wantDesc: "device is locked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.exitCode(); got != tc.wantCode {
				t.Errorf("exitCode() = %d, want %d", got, tc.wantCode)
			}
			if tc.wantCode != 0 {
				if got := tc.result.description(); got != tc.wantDesc {
					t.Errorf("description() = %q, want %q", got, tc.wantDesc)
				}
			}
		})
	}
}

func TestCapturePacketsInputEnded(t *testing.T) {
	packetCh := make(chan scan.Packet, 1)
	packetCh <- scan.Packet{Data: "packet"}
	close(packetCh)
	count, err := capturePackets(context.Background(), slog.Default(), packetCh, time.Second)
	if err == nil || err.Error() != "serial input ended unexpectedly" {
		t.Fatalf("capturePackets() error = %v, want serial input ended unexpectedly", err)
	}
	if count != 1 {
		t.Errorf("capturePackets() count = %d, want 1", count)
	}
}

func TestSerialOperationDescribeError(t *testing.T) {
	for _, tc := range []struct {
		op   serialOperation
		err  error
		want string
	}{
		{serialCapture, context.Canceled, "interrupted"},
		{serialCapture, fmtWrap(os.ErrPermission), "permission denied; add this user to the serial-port access group (usually dialout)"},
		{serialCapture, fmtWrap(testLockedError{}), "device is locked by another process"},
		{serialDetect, term.ErrNotATTY, "speed detection requires a serial device"},
		{serialPPS, term.ErrNotATTY, "PPS detection requires a serial device"},
		{serialCapture, term.ErrNotATTY, "not a serial device"},
		{serialCapture, errors.New("gone"), "gone"},
	} {
		if got := tc.op.describeError(tc.err); got != tc.want {
			t.Errorf("describeError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("wrapper"), err) }

type testLockedError struct{}

func (testLockedError) Error() string { return "locked" }
func (testLockedError) Locked() bool  { return true }

func TestScanPortList(t *testing.T) {
	ports := []serialenum.Port{{Device: "detected"}, {Device: "silent"}, {Device: "error"}}
	detect := func(_ context.Context, lg *slog.Logger, device, packetLog string) detectResult {
		if packetLog != "" {
			t.Errorf("scan packet log = %q, want empty", packetLog)
		}
		lg.Info("detecting")
		switch device {
		case "detected":
			return detectResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 38400}}
		case "silent":
			return detectResult{device: device}
		default:
			return detectResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectUnrecognized}}
		}
	}
	stdout := outputFile(t)
	var stderr, logBuf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logBuf, nil))
	if err := scanPortList(context.Background(), lg, ports, detect, stdout, &stderr, false); err != nil {
		t.Fatalf("scanPortList() error = %v, want nil because one device was detected", err)
	}
	if got := outputString(t, stdout); got != "detected 38400\n" {
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

func TestScanPortListJSONL(t *testing.T) {
	ports := []serialenum.Port{{Device: "/dev/ttyUSB0"}}
	detect := func(_ context.Context, _ *slog.Logger, device, _ string) detectResult {
		return detectResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 115200}}
	}
	stdout := outputFile(t)
	if err := scanPortList(context.Background(), slog.Default(), ports, detect, stdout, io.Discard, true); err != nil {
		t.Fatal(err)
	}
	want := "{\"device\":\"/dev/ttyUSB0\",\"speed\":115200}\n"
	if got := outputString(t, stdout); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestScanPortListBestFailure(t *testing.T) {
	ports := []serialenum.Port{{Device: "silent"}, {Device: "error"}}
	detect := func(_ context.Context, _ *slog.Logger, device, _ string) detectResult {
		if device == "silent" {
			return detectResult{device: device}
		}
		return detectResult{device: device, failure: "failed"}
	}
	err := scanPortList(context.Background(), slog.Default(), ports, detect, outputFile(t), io.Discard, false)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 2 || !cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want quiet exit code 2", err)
	}
}

func TestScanPortListInterruptedOverridesDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ports := []serialenum.Port{{Device: "detected"}, {Device: "cancelled"}}
	detect := func(_ context.Context, _ *slog.Logger, device, _ string) detectResult {
		if device == "detected" {
			return detectResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 115200}}
		}
		return detectResult{device: device, failure: "interrupted"}
	}
	var stderr bytes.Buffer
	err := scanPortList(ctx, slog.Default(), ports, detect, outputFile(t), &stderr, false)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || !cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want quiet exit code 1", err)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want nothing reported for an interrupted scan", got)
	}
}

func TestScanPortListOutputError(t *testing.T) {
	ports := []serialenum.Port{{Device: "detected"}}
	detect := func(_ context.Context, _ *slog.Logger, device, _ string) detectResult {
		return detectResult{device: device, detection: gpsio.DetectResult{Outcome: gpsio.DetectFound, Speed: 38400}}
	}
	stdout := outputFile(t)
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	err := scanPortList(context.Background(), slog.Default(), ports, detect, stdout, io.Discard, false)
	var cmdErr commandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode() != 1 || cmdErr.Quiet() {
		t.Fatalf("scanPortList() error = %#v, want non-quiet exit code 1", err)
	}
}

func outputFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func outputString(t *testing.T, f *os.File) string {
	t.Helper()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
