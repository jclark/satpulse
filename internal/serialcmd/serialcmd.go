// Package serialcmd implements the satpulsetool serial subcommand.
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

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const (
	summary        = `[-h|--help] [-j|--jsonl] [-s|--scan] [--packet-log path] [device]`
	tryDuration    = 1250 * time.Millisecond
	silentTryLimit = 5
)

type commandError struct {
	msg   string
	code  int
	quiet bool
}

func (e commandError) Error() string { return e.msg }
func (e commandError) ExitCode() int { return e.code }
func (e commandError) Quiet() bool   { return e.quiet }

type flags struct {
	jsonl     bool
	scan      bool
	packetLog string
	device    string
}

// Cmd executes the serial subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	cfg, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), commandError{msg: err.Error(), code: 1}
	}
	if help {
		return usageFunc(progName), nil
	}
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	if cfg.device == "" && !cfg.scan {
		return "", enumerate(os.Stdout, cfg.jsonl)
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	if cfg.device != "" {
		result := probeDevice(ctx, lg, cfg.device, cfg.packetLog)
		if ctx.Err() != nil {
			return "", commandError{msg: "interrupted", code: 1}
		}
		if code := result.exitCode(); code != 0 {
			return "", commandError{msg: result.description(), code: code}
		}
		_, err = fmt.Fprintln(os.Stdout, result.detection.Speed)
		return "", err
	}
	return "", scanPorts(ctx, lg)
}

func parseFlags(cmdName string, args []string) (cfg flags, help bool, usageFunc func(string) string, err error) {
	fs := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	fs.BoolVarP(&cfg.jsonl, "jsonl", "j", false, "output one JSON object per serial port when enumerating")
	fs.BoolVarP(&cfg.scan, "scan", "s", false, "detect the speed of every enumerated serial port")
	fs.StringVar(&cfg.packetLog, "packet-log", "", "write received packets and speed changes to a JSONL file")
	fs.BoolVarP(&help, "help", "h", false, "show usage help for the serial command")
	usageFunc = cmd.UsageFunc(cmdName, summary, fs)
	if err = fs.Parse(args); err != nil || help {
		return
	}
	if fs.NArg() > 1 {
		err = fmt.Errorf("expected at most one serial device argument")
		return
	}
	if fs.NArg() == 1 {
		cfg.device = fs.Arg(0)
	}
	if cfg.scan && cfg.device != "" {
		err = fmt.Errorf("--scan cannot be combined with a device argument")
	} else if cfg.jsonl && (cfg.scan || cfg.device != "") {
		err = fmt.Errorf("--jsonl applies only to serial port enumeration")
	} else if cfg.packetLog != "" && cfg.device == "" {
		err = fmt.Errorf("--packet-log requires a device argument")
	}
	return
}

func enumerate(w io.Writer, jsonl bool) error {
	ports, err := serialenum.List()
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return commandError{msg: "no serial ports found", code: 2}
	}
	return printPorts(w, ports, jsonl)
}

func printPorts(w io.Writer, ports []serialenum.Port, jsonl bool) error {
	for _, port := range ports {
		if jsonl {
			b, err := json.Marshal(port)
			if err != nil {
				return fmt.Errorf("encoding serial port: %w", err)
			}
			if _, err := fmt.Fprintln(w, string(b)); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintln(w, port.Display); err != nil {
			return err
		}
	}
	return nil
}

type probeResult struct {
	device    string
	detection gpsio.DetectResult
	err       error
}

func (r probeResult) exitCode() int {
	if r.err != nil {
		return 1
	}
	switch r.detection.Outcome {
	case gpsio.DetectFound:
		return 0
	case gpsio.DetectSilent:
		return 2
	}
	return 1
}

// description explains an unsuccessful probe: either why detection could not
// run, or what the device did instead of producing a speed.
func (r probeResult) description() string {
	if r.err != nil {
		return describeProbeError(r.err)
	}
	if r.detection.Outcome == gpsio.DetectSilent {
		return "no output received from the device"
	}
	return "output was received, but no known GNSS protocol was validated at a candidate speed"
}

func probeDevice(ctx context.Context, lg *slog.Logger, device, packetLogPath string) (result probeResult) {
	result.device = device
	conn, _, err := gpsio.OpenSerial(device, 0)
	if err != nil {
		result.err = err
		return
	}

	formats := gpsreg.CreatePacketFormats(nil)
	var wg sync.WaitGroup
	pktLog, logFile, err := gpsio.LogPackets(lg, &wg, packetLogPath, false, formats)
	if err != nil {
		result.err = fmt.Errorf("opening packet log: %w", err)
		closeDevice(lg, conn, &result)
		return
	}
	if pktLog != nil {
		conn.SetPacketLog(pktLog)
	}

	// Detection owns signal cancellation: DetectSpeed honors ctx itself. The
	// scan worker gets a context of its own so that Ctrl-C cannot close the
	// packet channel under the window DetectSpeed is still consuming.
	scanCtx, cancelScan := context.WithCancel(context.Background())
	packetCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(scanCtx, lg, conn, packetCh, pktLog, formats) })

	result.detection, result.err = gpsio.DetectSpeed(
		ctx,
		lg,
		packetCh,
		conn,
		gpsreg.CreatePacketProcessors(nil),
		gpsio.DefaultSpeedList(),
		tryDuration,
		func(tried []int) bool { return len(tried) >= silentTryLimit },
	)

	conn.Stop()
	cancelScan()
	for range packetCh {
	}
	wg.Wait()
	if logFile != nil {
		logFile.Close(lg)
	}
	closeDevice(lg, conn, &result)
	return
}

// closeDevice closes conn, keeping the close error only when the probe
// itself produced none. A probe that already failed has a better error to
// report, and each port gets exactly one output line, so the two cannot be
// combined.
func closeDevice(lg *slog.Logger, conn *gpsio.SerialConn, result *probeResult) {
	closeErr := conn.Close()
	if closeErr == nil {
		return
	}
	if result.err == nil {
		result.err = closeErr
		return
	}
	lg.Debug("closing the serial device failed after an unsuccessful probe", "device", result.device, "error", closeErr)
}

func scanPorts(ctx context.Context, lg *slog.Logger) error {
	ports, err := serialenum.List()
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return commandError{msg: "no serial ports found", code: 2}
	}
	return scanPortList(ctx, lg, ports, probeDevice, os.Stdout, os.Stderr)
}

type probeFunc func(context.Context, *slog.Logger, string, string) probeResult

func scanPortList(ctx context.Context, lg *slog.Logger, ports []serialenum.Port, probe probeFunc, stdout, stderr io.Writer) error {
	resultCh := make(chan probeResult, len(ports))
	var wg sync.WaitGroup
	for _, port := range ports {
		wg.Go(func() {
			resultCh <- probe(ctx, lg, port.Device, "")
		})
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	bestCode := 1
	var outputErr error
	for result := range resultCh {
		code := result.exitCode()
		switch code {
		case 0:
			if _, err := fmt.Fprintf(stdout, "%s %d\n", result.device, result.detection.Speed); err != nil && outputErr == nil {
				outputErr = err
			}
			bestCode = 0
		case 2:
			if _, err := fmt.Fprintf(stderr, "%s: %s\n", result.device, result.description()); err != nil && outputErr == nil {
				outputErr = err
			}
			if bestCode != 0 {
				bestCode = 2
			}
		default:
			if _, err := fmt.Fprintf(stderr, "%s: %s\n", result.device, result.description()); err != nil && outputErr == nil {
				outputErr = err
			}
		}
	}
	if outputErr != nil {
		return commandError{msg: fmt.Sprintf("writing serial scan output: %v", outputErr), code: 1}
	}
	if ctx.Err() != nil {
		return commandError{msg: "serial scan interrupted", code: 1, quiet: true}
	}
	if bestCode == 0 {
		return nil
	}
	return commandError{msg: "serial scan did not detect a device", code: bestCode, quiet: true}
}

func describeProbeError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "interrupted"
	case errors.Is(err, os.ErrPermission):
		return "permission denied; add this user to the serial-port access group (usually dialout)"
	case isLocked(err):
		return "device is locked by another process"
	case errors.Is(err, gpsio.ErrNotSerial):
		return "speed detection requires a serial device"
	case errors.Is(err, gpsio.ErrCurrentSpeedUnknown):
		return "the device's current serial speed could not be determined"
	default:
		return err.Error()
	}
}

func isLocked(err error) bool {
	var locked term.LockedError
	return errors.As(err, &locked) && locked.Locked()
}
