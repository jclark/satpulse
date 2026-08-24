// Package serialcmd implements the satpulsetool serial subcommand.
package serialcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/serialpps"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

type flagVars struct {
	all              bool
	device           string
	info             bool
	ppsPin           opt.Val[gpsio.ModemControlPin]
	invertPolarity   bool
	ppsMethod        gpsio.PPSMethod
	pollPreWarm      float64
	maxWakeupLatency opt.Val[float64]
	packetLog        string
	deviceSpeed      opt.Val[int]
	timeout          time.Duration
	jsonl            bool
}

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
	if v.ppsPin.IsSet() {
		return "", monitorPPS(ctx, lg, v)
	}
	if v.all {
		return "", scanPorts(ctx, lg, v.jsonl)
	}
	if v.deviceSpeed.IsSet() {
		result := captureDevice(ctx, lg, v.device, v.deviceSpeed.Get(), v.packetLog, v.timeout)
		if code := result.exitCode(); code != 0 {
			return "", commandError{msg: result.description(), code: code}
		}
		return "", nil
	}
	result := detectDevice(ctx, lg, v.device, v.packetLog)
	if ctx.Err() != nil {
		return "", commandError{msg: "interrupted", code: 1}
	}
	if code := result.exitCode(); code != 0 {
		return "", commandError{msg: result.description(), code: code}
	}
	info := speedInfo{Device: result.device, Speed: result.detection.Speed}
	return "", printInfo(os.Stdout, &info, v.jsonl)
}

const summary = `[-h|--help] [-a|--all | -d|--serial-device path] [-i|--info]
              [-p|--pps-pin pin] [-I|--invert-polarity] [-m|--pps-method method]
              [--poll-pre-warm seconds]
              [--max-wakeup-latency seconds] [--packet-log path]
              [-s|--device-speed bps]
              [-t|--timeout seconds] [-j|--jsonl]`

const defaultPPSTimeout = 10 * time.Second

func parseFlags(cmdName string, args []string) (v flagVars, help bool, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SortFlags = false
	var ppsPinName, ppsMethodName string
	var maxWakeupLatency, timeoutSec float64
	var deviceSpeed int
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the serial command")
	flags.BoolVarP(&v.all, "all", "a", false, "operate on all discovered serial ports")
	flags.StringVarP(&v.device, "serial-device", "d", "", "operate on the serial port at `path`")
	flags.BoolVarP(&v.info, "info", "i", false, "show information about serial ports without opening them")
	flags.StringVarP(&ppsPinName, "pps-pin", "p", "", "detect PPS edges on the modem-control input `pin` (cts, dcd, dsr, or ri)")
	flags.BoolVarP(&v.invertPolarity, "invert-polarity", "I", false, "invert the usual PPS pulse polarity, for edges that trail the start of the second by the pulse width")
	flags.StringVarP(&ppsMethodName, "pps-method", "m", "", "force the PPS edge detection `method` (poll, wait, or kernel)")
	flags.Float64Var(&v.pollPreWarm, "poll-pre-warm", 0, "busy-wait `seconds` before each poll window, for hosts whose reads slow down while idle")
	flags.Float64Var(&maxWakeupLatency, "max-wakeup-latency", 0, "limit CPU wakeup latency to `seconds` while detecting PPS")
	flags.StringVar(&v.packetLog, "packet-log", "", "write received packets to a JSON Lines log at `path`")
	flags.IntVarP(&deviceSpeed, "device-speed", "s", 0, "set the serial port speed to `bps` (0 uses the current speed)")
	flags.Float64VarP(&timeoutSec, "timeout", "t", 0, "stop detecting PPS edges or capturing packets after `seconds` (0 = until interrupted)")
	flags.BoolVarP(&v.jsonl, "jsonl", "j", false, "write output in JSON Lines format")
	usageFunc = cmd.UsageFunc(cmdName, summary, flags)
	if err = flags.Parse(args); err != nil || help {
		return
	}
	if flags.NArg() != 0 {
		err = fmt.Errorf("serial command does not accept positional arguments")
		return
	}
	deviceSet := flags.Lookup("serial-device").Changed
	if flags.Lookup("device-speed").Changed {
		v.deviceSpeed.Set(deviceSpeed)
	}
	timeoutSet := flags.Lookup("timeout").Changed
	packetLogSet := flags.Lookup("packet-log").Changed
	methodSet := flags.Lookup("pps-method").Changed
	preWarmSet := flags.Lookup("poll-pre-warm").Changed
	if flags.Lookup("max-wakeup-latency").Changed {
		v.maxWakeupLatency.Set(maxWakeupLatency)
	}
	if flags.Lookup("pps-pin").Changed {
		var pin gpsio.ModemControlPin
		if pin, err = parsePin(ppsPinName); err != nil {
			return
		}
		v.ppsPin.Set(pin)
	}
	if methodSet {
		if v.ppsMethod, err = gpsio.ParsePPSMethod(ppsMethodName); err != nil {
			err = fmt.Errorf("--pps-method must be one of poll, wait, or kernel")
			return
		}
	}
	if preWarmSet {
		if err = checkNonNegative(v.pollPreWarm, 1, "--poll-pre-warm"); err != nil {
			return
		}
	}
	if v.maxWakeupLatency.IsSet() {
		if err = checkNonNegative(v.maxWakeupLatency.Get(), 1, "--max-wakeup-latency"); err != nil {
			return
		}
	}
	if deviceSet && v.device == "" {
		err = fmt.Errorf("--serial-device must not be empty")
		return
	}
	if v.all && deviceSet {
		err = fmt.Errorf("--all cannot be combined with --serial-device")
		return
	}
	if v.deviceSpeed.IsSet() && v.deviceSpeed.Get() < 0 {
		err = fmt.Errorf("--device-speed must not be negative")
		return
	}
	if timeoutSet {
		switch {
		case math.IsNaN(timeoutSec) || math.IsInf(timeoutSec, 0):
			err = fmt.Errorf("--timeout must be finite")
			return
		case timeoutSec < 0:
			err = fmt.Errorf("--timeout must not be negative")
			return
		case timeoutSec >= float64(math.MaxInt64)/float64(time.Second):
			err = fmt.Errorf("--timeout is too large")
			return
		case timeoutSec > 0 && ptime.Seconds(timeoutSec) == 0:
			err = fmt.Errorf("--timeout is too small")
			return
		}
	}
	if packetLogSet && v.packetLog == "" {
		err = fmt.Errorf("--packet-log must not be empty")
		return
	}
	if v.info && (v.ppsPin.IsSet() || v.invertPolarity || methodSet || preWarmSet || v.maxWakeupLatency.IsSet() || v.deviceSpeed.IsSet() || timeoutSet || packetLogSet) {
		err = fmt.Errorf("--info cannot be combined with --pps-pin, --invert-polarity, --pps-method, --poll-pre-warm, --max-wakeup-latency, --device-speed, --timeout, or --packet-log")
		return
	}
	if v.ppsPin.IsSet() && !deviceSet && !v.all {
		err = fmt.Errorf("--pps-pin requires --serial-device or --all")
		return
	}
	if v.invertPolarity && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--invert-polarity requires --pps-pin")
		return
	}
	if methodSet && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--pps-method requires --pps-pin")
		return
	}
	if preWarmSet && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--poll-pre-warm requires --pps-pin")
		return
	}
	if v.maxWakeupLatency.IsSet() && !v.ppsPin.IsSet() {
		err = fmt.Errorf("--max-wakeup-latency requires --pps-pin")
		return
	}
	if timeoutSet && !v.ppsPin.IsSet() && !(v.deviceSpeed.IsSet() && packetLogSet) {
		err = fmt.Errorf("--timeout requires --pps-pin, or --device-speed and --packet-log")
		return
	}
	if v.deviceSpeed.IsSet() && !deviceSet {
		err = fmt.Errorf("--device-speed requires --serial-device")
		return
	}
	if v.deviceSpeed.IsSet() && !v.ppsPin.IsSet() && !packetLogSet {
		err = fmt.Errorf("--device-speed requires --pps-pin or --packet-log")
		return
	}
	if packetLogSet && !deviceSet {
		err = fmt.Errorf("--packet-log requires --serial-device")
		return
	}
	if timeoutSet {
		v.timeout = ptime.Seconds(timeoutSec)
	} else if v.ppsPin.IsSet() {
		v.timeout = defaultPPSTimeout
	}
	if !deviceSet && !v.all {
		v.info = true
		v.all = true
	}
	return
}

// checkNonNegative rejects val outside [0, limit); NaN fails both comparisons.
func checkNonNegative(val, limit float64, flag string) error {
	if val >= 0 && val < limit {
		return nil
	}
	return fmt.Errorf("%s must be at least 0 and less than %g", flag, limit)
}

func parsePin(name string) (gpsio.ModemControlPin, error) {
	switch name {
	case "cts":
		return gpsio.ModemCTS, nil
	case "dcd":
		return gpsio.ModemDCD, nil
	case "dsr":
		return gpsio.ModemDSR, nil
	case "ri":
		return gpsio.ModemRI, nil
	default:
		return 0, fmt.Errorf("--pps-pin must be one of cts, dcd, dsr, or ri")
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

func monitorPPS(ctx context.Context, lg *slog.Logger, v flagVars) error {
	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	pr := &edgePrinter{out: os.Stdout, jsonl: v.jsonl, withDevice: v.all, cancel: cancel}
	if v.all {
		return scanPPSPorts(ctx, lg, v.wiring(), v.ppsConfig(), pr)
	}
	result := monitorDevice(ctx, lg, v.device, v.deviceSpeed.Get(), v.wiring(), v.ppsConfig(), v.packetLog, pr)
	if code := result.exitCode(); code != 0 {
		return commandError{msg: result.description(), code: code}
	}
	return nil
}

// ppsConfig is the detection configuration the PPS flags describe; the other
// Config fields matter only when edges are turned into samples.
func (v flagVars) ppsConfig() serialpps.Config {
	cfg := serialpps.Config{Method: v.ppsMethod, PollPreWarm: v.pollPreWarm}
	cfg.MaxWakeupLatency = v.maxWakeupLatency.Ptr()
	return cfg
}

// wiring is the pulse wiring the PPS flags describe.
func (v flagVars) wiring() serialpps.Wiring {
	w := serialpps.Wiring{Pin: v.ppsPin.Get()}
	if v.invertPolarity {
		w.Polarity = serialpps.PolarityAssert
	}
	return w
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

type serialOperation uint8

const (
	serialDetect serialOperation = iota
	serialCapture
	serialPPS
)

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
