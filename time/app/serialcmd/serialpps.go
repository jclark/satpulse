package serialcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/serialpps"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/scan"
)

type ppsOutputError struct {
	err error
}

// edgePrinter serializes PPS edge output from concurrently monitored ports.
type edgePrinter struct {
	mu         sync.Mutex
	out        io.Writer
	jsonl      bool
	withDevice bool
	cancel     context.CancelCauseFunc
	failure    *ppsOutputError
}

type ppsResult struct {
	device  string
	edges   int
	failure string
}

func monitorPPS(ctx context.Context, lg *slog.Logger, v flagVars) (ppsResult, error) {
	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	pr := &edgePrinter{out: os.Stdout, jsonl: v.jsonl, withDevice: v.all, cancel: cancel}
	if v.all {
		return ppsResult{}, scanPPSPorts(ctx, lg, v.wiring(), v.ppsConfig(), pr)
	}
	result := monitorDevice(ctx, lg, v.device, v.deviceSpeed.Get(), v.wiring(), v.ppsConfig(), v.packetLog, pr)
	return result, nil
}

func scanPPSPorts(ctx context.Context, lg *slog.Logger, w serialpps.Wiring, ppsCfg serialpps.Config, pr *edgePrinter) error {
	ports, err := serialenum.List()
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return commandError{msg: "no serial ports found", code: 2}
	}
	monitor := func(ctx context.Context, lg *slog.Logger, device string) ppsResult {
		return monitorDevice(ctx, lg, device, 0, w, ppsCfg, "", pr)
	}
	return monitorPortList(ctx, lg, ports, monitor, os.Stderr)
}

type monitorFunc func(context.Context, *slog.Logger, string) ppsResult

func monitorPortList(ctx context.Context, lg *slog.Logger, ports []serialenum.Port, monitor monitorFunc, stderr io.Writer) error {
	resultCh := make(chan ppsResult, len(ports))
	var wg sync.WaitGroup
	for _, port := range ports {
		device := port.Device
		wg.Go(func() {
			resultCh <- monitor(ctx, lg.With("device", device), device)
		})
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	bestCode := 1
	var outputErr error
	for result := range resultCh {
		if ppsOutputFailure(ctx) != nil {
			continue
		}
		code := result.exitCode()
		if code == 0 {
			bestCode = 0
			continue
		}
		if code == 2 && bestCode != 0 {
			bestCode = 2
		}
		if _, err := fmt.Fprintf(stderr, "%s: %s\n", result.device, result.description()); err != nil && outputErr == nil {
			outputErr = err
		}
	}
	if err := ppsOutputFailure(ctx); err != nil {
		return commandError{msg: err.Error(), code: 1}
	}
	if outputErr != nil {
		return commandError{msg: fmt.Sprintf("writing PPS scan output: %v", outputErr), code: 1}
	}
	if bestCode == 0 {
		return nil
	}
	return commandError{msg: "no PPS edges detected on any port", code: bestCode, quiet: true}
}

func ppsOutputFailure(ctx context.Context) *ppsOutputError {
	var err *ppsOutputError
	if errors.As(context.Cause(ctx), &err) {
		return err
	}
	return nil
}

// monitorDevice opens device and prints the timestamp of each PPS edge
// detected on the wired pin, keeping the receive side drained so receiver
// traffic cannot stall the port.
func monitorDevice(ctx context.Context, lg *slog.Logger, device string, speed int, w serialpps.Wiring, ppsCfg serialpps.Config, packetLogPath string, pr *edgePrinter) (result ppsResult) {
	result.device = device
	conn, _, err := gpsio.OpenSerial(device, speed)
	if err != nil {
		result.failure = serialPPS.describeError(err)
		return
	}
	stopDrain, err := drainPackets(lg, conn, packetLogPath)
	if err != nil {
		result.failure = err.Error()
		closeDevice(lg, conn, device, &result.failure)
		return
	}
	result.edges, err = detectEdges(ctx, lg, conn, w, device, ppsCfg, pr)
	if err != nil {
		result.failure = serialPPS.describeError(err)
	}
	conn.Stop()
	stopDrain()
	closeDevice(lg, conn, device, &result.failure)
	return
}

// drainPackets discards conn's received bytes, or, when packetLogPath is
// non-empty, runs them through the scan pipeline so the drained packets are
// recorded. The returned stop function must be called after conn.Stop.
func drainPackets(lg *slog.Logger, conn *gpsio.SerialConn, packetLogPath string) (stop func(), err error) {
	if packetLogPath == "" {
		done := drainInput(conn)
		return func() { <-done }, nil
	}
	formats := gpsreg.CreatePacketFormats(nil)
	var wg sync.WaitGroup
	pktLog, logFile, err := gpsio.LogPackets(lg, &wg, packetLogPath, false, formats)
	if err != nil {
		return nil, fmt.Errorf("opening packet log: %w", err)
	}
	if pktLog != nil {
		conn.SetPacketLog(pktLog)
	}
	scanCtx, cancelScan := context.WithCancel(context.Background())
	packetCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(scanCtx, lg, conn, packetCh, pktLog, formats) })
	wg.Go(func() {
		for range packetCh {
		}
	})
	return func() {
		cancelScan()
		wg.Wait()
		if logFile != nil {
			logFile.Close(lg)
		}
	}, nil
}

// drainInput consumes the receiver's output until the port is stopped. The
// wait backend depends on it: a USB serial driver reports modem-control pin
// changes only as it delivers received data, so an unread port throttles the
// driver and the wait stops waking. A temporary read error is a condition on
// the wire, and opening the port often counts one overrun.
func drainInput(conn *gpsio.SerialConn) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			} else if temp, ok := err.(scan.TemporaryError); ok && temp.Temporary() {
				continue
			} else if err != nil {
				return
			}
		}
	}()
	return done
}

type ppsConn interface {
	serialpps.StateReader
}

func detectEdges(parent context.Context, lg *slog.Logger, conn ppsConn, w serialpps.Wiring, device string, ppsCfg serialpps.Config, pr *edgePrinter) (int, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	edges := make(chan serialpps.CandidateEdge)
	errCh := make(chan error, 1)
	stats := new(serialpps.PollStats)
	if !lg.Enabled(ctx, slog.LevelInfo) {
		stats = nil
	}
	go func() {
		err := serialpps.Detect(ctx, lg, conn, w, ppsCfg, edges, stats)
		stats.Log(lg)
		errCh <- err
	}()

	count := 0
	var outputErr error
	for {
		select {
		case edge := <-edges:
			if outputErr != nil {
				continue
			}
			if err := pr.print(device, edge); err != nil {
				outputErr = err
				cancel()
				continue
			}
			count++
		case err := <-errCh:
			if outputErr != nil {
				return count, outputErr
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return count, nil
			}
			return count, err
		}
	}
}

type ppsEvent struct {
	Device      string  `json:"device"`
	T           string  `json:"t"`
	Uncertainty float64 `json:"uncertainty,omitzero"`
	Settling    bool    `json:"settling,omitzero"`
}

func (e *ppsOutputError) Error() string {
	return "writing PPS timestamp: " + e.err.Error()
}

func (e *ppsOutputError) Unwrap() error {
	return e.err
}

func (p *edgePrinter) print(device string, edge serialpps.CandidateEdge) error {
	t := edge.Timestamp.UTC().Round(time.Microsecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failure != nil {
		return p.failure
	}
	var err error
	if !p.jsonl {
		if p.withDevice {
			_, err = fmt.Fprintf(p.out, "%s %s\n", device, t.Format("15:04:05.000000"))
		} else {
			_, err = fmt.Fprintln(p.out, t.Format("15:04:05.000000"))
		}
	} else {
		event := ppsEvent{
			Device:      device,
			T:           t.Format("2006-01-02T15:04:05.000000Z"),
			Uncertainty: edge.Uncertainty.Seconds(),
			Settling:    !edge.Settled,
		}
		err = json.NewEncoder(p.out).Encode(&event)
	}
	if err == nil {
		return nil
	}
	p.failure = &ppsOutputError{err: err}
	if p.cancel != nil {
		p.cancel(p.failure)
	}
	return p.failure
}

func (r ppsResult) exitCode() int {
	if r.failure != "" {
		return 1
	}
	if r.edges == 0 {
		return 2
	}
	return 0
}

func (r ppsResult) description() string {
	if r.failure != "" {
		return r.failure
	}
	return "no PPS edges detected"
}
