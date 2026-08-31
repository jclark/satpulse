package serialcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/serialpps"
	"github.com/jclark/satpulse/gps/lib/serialenum"
)

func TestEdgePrinter(t *testing.T) {
	edge := serialpps.Edge{
		Timestamp: time.Date(2026, time.August, 12, 21, 23, 5, 123_456_499, time.FixedZone("ICT", 7*60*60)),
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
	state  gpsio.SerialPinState
	next   chan gpsio.SerialPinState
	waits  int
	method gpsio.PPSMethod
}

func (c *monitorWaitConn) SerialPinState() (gpsio.SerialPinState, error) {
	return c.state, nil
}

func (c *monitorWaitConn) WaitSerialPinChange(ctx context.Context, pin gpsio.SerialPin, method gpsio.PPSMethod) (gpsio.SerialPinChange, int, error) {
	c.waits++
	c.method = method
	select {
	case c.state = <-c.next:
		t := time.Date(2026, time.August, 12, 14, 23, 5, 123_456_000, time.UTC)
		return gpsio.SerialPinChange{Timestamp: t, TRead: t, Asserted: c.state.Asserted(pin)}, 0, nil
	case <-ctx.Done():
		return gpsio.SerialPinChange{}, 0, ctx.Err()
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
	asserted := gpsio.NewSerialPinState(gpsio.SerialPinCTS)
	conn := &monitorWaitConn{
		state: asserted,
		next:  make(chan gpsio.SerialPinState, 1),
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
		count, err := detectEdges(ctx, lg, conn, serialpps.Wiring{Pin: gpsio.SerialPinCTS}, "", serialpps.Config{}, pr)
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
		next: make(chan gpsio.SerialPinState),
	}
	var logs bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	count, err := detectEdges(ctx, lg, conn, serialpps.Wiring{Pin: gpsio.SerialPinCTS}, "", serialpps.Config{Method: gpsio.PPSMethodPoll}, &edgePrinter{out: io.Discard})
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
