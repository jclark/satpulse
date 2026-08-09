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
	"path/filepath"
	"slices"
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
	summary        = `[-h|--help] [-j|--jsonl] [-s|--detect-speed] [--packet-log path] [port]`
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
	detect    bool
	packetLog string
	port      string
}

// Cmd executes the serial subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	cfg, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	if !cfg.detect {
		return "", enumerate(os.Stdout, cfg.jsonl, cfg.port)
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	if cfg.port != "" {
		result := probeDevice(ctx, lg, cfg.port, cfg.packetLog)
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
	fs.BoolVarP(&cfg.jsonl, "jsonl", "j", false, "output one JSON object per serial port instead of a display label")
	fs.BoolVarP(&cfg.detect, "detect-speed", "s", false, "detect receiver speeds instead of describing ports")
	fs.StringVar(&cfg.packetLog, "packet-log", "", "write received packets and speed changes to a JSONL file")
	fs.BoolVarP(&help, "help", "h", false, "show usage help for the serial command")
	usageFunc = cmd.UsageFunc(cmdName, summary, fs)
	if err = fs.Parse(args); err != nil || help {
		return
	}
	if fs.NArg() > 1 {
		err = fmt.Errorf("expected at most one serial port argument")
		return
	}
	if fs.NArg() == 1 {
		cfg.port = fs.Arg(0)
	}
	if cfg.jsonl && cfg.detect {
		err = fmt.Errorf("--jsonl cannot be combined with --detect-speed")
	} else if cfg.packetLog != "" && (!cfg.detect || cfg.port == "") {
		err = fmt.Errorf("--packet-log requires --detect-speed and a port")
	}
	return
}

func enumerate(w io.Writer, jsonl bool, selector string) error {
	ports, err := serialenum.List()
	if err != nil {
		return err
	}
	return printPortInfo(w, ports, jsonl, selector)
}

func printPortInfo(w io.Writer, ports []serialenum.Port, jsonl bool, selector string) error {
	if selector != "" {
		port, ok := selectPort(ports, selector)
		if !ok {
			return commandError{msg: selector + " does not match a discovered serial port", code: 2}
		}
		ports = []serialenum.Port{port}
	} else if len(ports) == 0 {
		return commandError{msg: "no serial ports found", code: 2}
	}
	return printPorts(w, ports, jsonl)
}

// selectPort finds the discovered port that selector names, matching device
// nodes and aliases first as given and then with symlinks resolved, so that a
// path like /dev/serial/by-id/... selects the port it points to.
func selectPort(ports []serialenum.Port, selector string) (serialenum.Port, bool) {
	if port, ok := matchPort(ports, selector); ok {
		return port, true
	}
	if resolved, err := filepath.EvalSymlinks(selector); err == nil {
		return matchPort(ports, resolved)
	}
	return serialenum.Port{}, false
}

func matchPort(ports []serialenum.Port, path string) (serialenum.Port, bool) {
	for _, port := range ports {
		if path == port.Device || slices.Contains(port.Aliases, path) {
			return port, true
		}
	}
	return serialenum.Port{}, false
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
		var err error
		if port.Serial == "" {
			_, err = fmt.Fprintln(w, port.Display)
		} else {
			_, err = fmt.Fprintf(w, "%s serial=%q\n", port.Display, port.Serial)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type probeResult struct {
	device    string
	detection gpsio.DetectResult
	failure   string
}

func (r probeResult) exitCode() int {
	if r.failure != "" {
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
	if r.failure != "" {
		return r.failure
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
		result.failure = describeSerialError(err)
		return
	}

	formats := gpsreg.CreatePacketFormats(nil)
	var wg sync.WaitGroup
	pktLog, logFile, err := gpsio.LogPackets(lg, &wg, packetLogPath, false, formats)
	if err != nil {
		result.failure = "opening packet log: " + err.Error()
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

	result.detection, err = gpsio.DetectSpeed(
		ctx,
		lg,
		packetCh,
		conn,
		gpsreg.CreatePacketProcessors(nil),
		gpsio.DefaultSpeedList(),
		tryDuration,
		func(tried []int) bool { return len(tried) >= silentTryLimit },
	)
	if err != nil {
		result.failure = describeSerialError(err)
	}

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
// itself produced none. A probe that already failed has a more relevant
// failure to report, and each port gets exactly one output line, so the two
// cannot be combined.
func closeDevice(lg *slog.Logger, conn *gpsio.SerialConn, result *probeResult) {
	closeErr := conn.Close()
	if closeErr == nil {
		return
	}
	if result.failure == "" {
		result.failure = closeErr.Error()
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
		device := port.Device
		wg.Go(func() {
			resultCh <- probe(ctx, lg.With("device", device), device, "")
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
		if code == 0 {
			bestCode = 0
			if _, err := fmt.Fprintf(stdout, "%s %d\n", result.device, result.detection.Speed); err != nil && outputErr == nil {
				outputErr = err
			}
			continue
		}
		if code == 2 && bestCode != 0 {
			bestCode = 2
		}
		// An interrupt fails every probe still running, so describing each one
		// would bury the interrupt in noise.
		if ctx.Err() != nil {
			continue
		}
		if _, err := fmt.Fprintf(stderr, "%s: %s\n", result.device, result.description()); err != nil && outputErr == nil {
			outputErr = err
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

func describeSerialError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "interrupted"
	case errors.Is(err, os.ErrPermission):
		return "permission denied; add this user to the serial-port access group (usually dialout)"
	case isLocked(err):
		return "device is locked by another process"
	case errors.Is(err, term.ErrNotATTY):
		return "speed detection requires a serial device"
	default:
		return err.Error()
	}
}

func isLocked(err error) bool {
	var locked term.LockedError
	return errors.As(err, &locked) && locked.Locked()
}
