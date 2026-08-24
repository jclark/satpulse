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
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/scan"
)

type commandError struct {
	msg   string
	code  int
	quiet bool
}

type captureResult struct {
	packets int
	failure string
}

type detectResult struct {
	device    string
	detection gpsio.DetectResult
	failure   string
}

type operationResult interface {
	exitCode() int
	description() string
}

type speedInfo struct {
	Device string `json:"device"`
	Speed  int    `json:"speed"`

	printDevice bool
}

// Cmd executes the serial subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	v, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	if v.info {
		return "", enumerate(os.Stdout, v.jsonl, v.device)
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	result, err := run(ctx, lg, v)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	if code := result.exitCode(); code != 0 {
		return "", commandError{msg: result.description(), code: code}
	}
	detected, ok := result.(detectResult)
	if !ok {
		return "", nil
	}
	info := speedInfo{Device: detected.device, Speed: detected.detection.Speed}
	return "", printInfo(os.Stdout, &info, v.jsonl)
}

// run executes the selected operation. It is the only function that converts
// concrete result structs to operationResult. It returns either a literal nil
// interface or a non-pointer concrete value, avoiding an interface containing
// a typed nil pointer.
func run(ctx context.Context, lg *slog.Logger, v flagVars) (operationResult, error) {
	switch {
	case v.ppsPin.IsSet():
		result, err := monitorPPS(ctx, lg, v)
		if err != nil || v.all {
			return nil, err
		}
		return result, nil
	case v.all:
		return nil, scanPorts(ctx, lg, v.jsonl)
	case v.deviceSpeed.IsSet():
		result := captureDevice(ctx, lg, v.device, v.deviceSpeed.Get(), v.packetLog, v.timeout)
		return result, nil
	default:
		result := detectDevice(ctx, lg, v.device, v.packetLog)
		if ctx.Err() != nil {
			result.failure = "interrupted"
		}
		return result, nil
	}
}

func enumerate(f *os.File, jsonl bool, selector string) error {
	ports, err := serialenum.List()
	if err != nil {
		return err
	}
	return printPortInfo(f, ports, jsonl, selector)
}

func printPortInfo(f *os.File, ports []serialenum.Port, jsonl bool, selector string) error {
	if selector != "" {
		port, ok := selectPort(ports, selector)
		if !ok {
			return commandError{msg: selector + " does not match a discovered serial port", code: 2}
		}
		ports = []serialenum.Port{port}
	} else if len(ports) == 0 {
		return commandError{msg: "no serial ports found", code: 2}
	}
	return printPorts(f, ports, jsonl)
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

type portInfo serialenum.Port

func printPorts(f *os.File, ports []serialenum.Port, jsonl bool) error {
	for _, port := range ports {
		info := portInfo(port)
		if err := printInfo(f, &info, jsonl); err != nil {
			return err
		}
	}
	return nil
}

func (info *portInfo) Print(f *os.File) error {
	var b strings.Builder
	fmt.Fprintf(&b, "device=%s", info.Device)
	if info.USB != (serialenum.USBID{}) {
		fmt.Fprintf(&b, " vid=%04x pid=%04x", info.USB.VID, info.USB.PID)
	}
	if info.Serial != "" {
		fmt.Fprintf(&b, " serial=%q", info.Serial)
	}
	for _, alias := range info.Aliases {
		fmt.Fprintf(&b, " alias=%s", alias)
	}
	fmt.Fprintf(&b, " display=%q", info.Display)
	_, err := fmt.Fprintln(f, b.String())
	return err
}

func (info *speedInfo) Print(f *os.File) error {
	if !info.printDevice {
		_, err := fmt.Fprintln(f, info.Speed)
		return err
	}
	_, err := fmt.Fprintf(f, "%s %d\n", info.Device, info.Speed)
	return err
}

type printer interface {
	Print(*os.File) error
}

func printInfo(f *os.File, info printer, jsonl bool) error {
	if !jsonl {
		return info.Print(f)
	}
	b, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("encoding serial output: %w", err)
	}
	_, err = fmt.Fprintln(f, string(b))
	return err
}

type serialOperation uint8

const (
	serialDetect serialOperation = iota
	serialCapture
	serialPPS
)

func scanPorts(ctx context.Context, lg *slog.Logger, jsonl bool) error {
	ports, err := serialenum.List()
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return commandError{msg: "no serial ports found", code: 2}
	}
	return scanPortList(ctx, lg, ports, detectDevice, os.Stdout, os.Stderr, jsonl)
}

type detectFunc func(context.Context, *slog.Logger, string, string) detectResult

func scanPortList(ctx context.Context, lg *slog.Logger, ports []serialenum.Port, detect detectFunc, stdout *os.File, stderr io.Writer, jsonl bool) error {
	resultCh := make(chan detectResult, len(ports))
	var wg sync.WaitGroup
	for _, port := range ports {
		device := port.Device
		wg.Go(func() {
			resultCh <- detect(ctx, lg.With("device", device), device, "")
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
			info := speedInfo{Device: result.device, Speed: result.detection.Speed, printDevice: true}
			if err := printInfo(stdout, &info, jsonl); err != nil && outputErr == nil {
				outputErr = err
			}
			continue
		}
		if code == 2 && bestCode != 0 {
			bestCode = 2
		}
		// An interrupt fails every detection still running, so describing each one
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

func captureDevice(ctx context.Context, lg *slog.Logger, device string, speed int, packetLogPath string, timeout time.Duration) (result captureResult) {
	conn, _, err := gpsio.OpenSerial(device, speed)
	if err != nil {
		result.failure = serialCapture.describeError(err)
		return
	}

	formats := gpsreg.CreatePacketFormats(nil)
	var wg sync.WaitGroup
	pktLog, logFile, err := gpsio.LogPackets(lg, &wg, packetLogPath, false, formats)
	if err != nil {
		result.failure = "opening packet log: " + err.Error()
		closeDevice(lg, conn, device, &result.failure)
		return
	}
	if pktLog != nil {
		conn.SetPacketLog(pktLog)
	}

	scanCtx, cancelScan := context.WithCancel(context.Background())
	packetCh := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(scanCtx, lg, conn, packetCh, pktLog, formats) })
	result.packets, err = capturePackets(ctx, lg, packetCh, timeout)
	if err != nil {
		result.failure = err.Error()
	}

	conn.Stop()
	cancelScan()
	for pkt := range packetCh {
		if len(pkt.Data) != 0 {
			result.packets++
		}
	}
	wg.Wait()
	if logFile != nil {
		logFile.Close(lg)
	}
	closeDevice(lg, conn, device, &result.failure)
	return
}

func capturePackets(ctx context.Context, lg *slog.Logger, packetCh <-chan scan.Packet, timeout time.Duration) (int, error) {
	var timerC <-chan time.Time
	if timeout == 0 {
		lg.Info("capturing packets until interrupted")
	} else {
		lg.Debug("capturing packets", "duration", timeout)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timerC = timer.C
	}
	count := 0
	for {
		select {
		case <-ctx.Done():
			lg.Debug("capture interrupted")
			return count, nil
		case <-timerC:
			lg.Debug("capture complete")
			return count, nil
		case pkt, ok := <-packetCh:
			if !ok {
				return count, errors.New("serial input ended unexpectedly")
			}
			if len(pkt.Data) != 0 {
				count++
			}
		}
	}
}

func (r captureResult) exitCode() int {
	if r.failure != "" {
		return 1
	}
	if r.packets == 0 {
		return 2
	}
	return 0
}

func (r captureResult) description() string {
	if r.failure != "" {
		return r.failure
	}
	return "no output received from the device"
}

const (
	tryDuration    = 1250 * time.Millisecond
	silentTryLimit = 5
)

func detectDevice(ctx context.Context, lg *slog.Logger, device, packetLogPath string) (result detectResult) {
	result.device = device
	conn, _, err := gpsio.OpenSerial(device, 0)
	if err != nil {
		result.failure = serialDetect.describeError(err)
		return
	}

	formats := gpsreg.CreatePacketFormats(nil)
	var wg sync.WaitGroup
	pktLog, logFile, err := gpsio.LogPackets(lg, &wg, packetLogPath, false, formats)
	if err != nil {
		result.failure = "opening packet log: " + err.Error()
		closeDevice(lg, conn, device, &result.failure)
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
		result.failure = serialDetect.describeError(err)
	}

	conn.Stop()
	cancelScan()
	for range packetCh {
	}
	wg.Wait()
	if logFile != nil {
		logFile.Close(lg)
	}
	closeDevice(lg, conn, device, &result.failure)
	return
}

func (r detectResult) exitCode() int {
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

// description explains an unsuccessful detection: either why detection could not
// run, or what the device did instead of producing a speed.
func (r detectResult) description() string {
	if r.failure != "" {
		return r.failure
	}
	if r.detection.Outcome == gpsio.DetectSilent {
		return "no output received from the device"
	}
	return "output was received, but no known GNSS protocol was validated at a candidate speed"
}

// closeDevice closes conn, keeping the close error only when the read
// operation itself produced none. An existing failure is more relevant.
func closeDevice(lg *slog.Logger, conn *gpsio.SerialConn, device string, failure *string) {
	closeErr := conn.Close()
	if closeErr == nil {
		return
	}
	if *failure == "" {
		*failure = closeErr.Error()
		return
	}
	lg.Debug("closing the serial device failed after an unsuccessful read", "device", device, "error", closeErr)
}

func (op serialOperation) describeError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "interrupted"
	case errors.Is(err, os.ErrPermission):
		return "permission denied; add this user to the serial-port access group (usually dialout)"
	case isLocked(err):
		return "device is locked by another process"
	case op == serialDetect && errors.Is(err, term.ErrNotATTY):
		return "speed detection requires a serial device"
	case op == serialPPS && errors.Is(err, term.ErrNotATTY):
		return "PPS detection requires a serial device"
	default:
		return err.Error()
	}
}

func isLocked(err error) bool {
	var locked term.LockedError
	return errors.As(err, &locked) && locked.Locked()
}

func (e commandError) Error() string { return e.msg }
func (e commandError) ExitCode() int { return e.code }
func (e commandError) Quiet() bool   { return e.quiet }
