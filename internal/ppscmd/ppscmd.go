// Package ppscmd implements the pps subcommand of satpulsetool, which lists
// kernel PPS devices, prints the timestamps of one as they arrive, or polls
// a GPIO for PPS edges.
package ppscmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/lib/kpps"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/spf13/pflag"
)

type flagVars struct {
	device  string
	gpio    *int // nil when not given
	cfg     pps.GPIOConfig
	timeout time.Duration
	jsonl   bool
}

const summary = `[-h|--help] [-d|--pps-device path] [-g|--gpio-pin N]
              [--cpu N] [--priority N] [--max-bracket seconds]
              [-t|--timeout seconds] [-j|--jsonl]`

// Cmd executes the pps subcommand with the given arguments.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	v, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	if v.device == "" && v.gpio == nil {
		return "", listDevices(lg, sysClassPPS, os.Stdout, v.jsonl)
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	if v.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}
	if v.gpio != nil {
		return "", pollGPIO(ctx, lg, v)
	}
	return "", watchDevice(ctx, lg, v)
}

func parseFlags(cmdName string, args []string) (v flagVars, help bool, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SortFlags = false
	var timeoutSec float64
	var gpio, cpu int
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the pps command")
	flags.StringVarP(&v.device, "pps-device", "d", "", "print the assert timestamps of the PPS device at `path`")
	flags.IntVarP(&gpio, "gpio-pin", "g", 0, "poll GPIO `N` for PPS edges")
	flags.IntVar(&cpu, "cpu", 0, "pin the GPIO poller to CPU `N`")
	flags.IntVar(&v.cfg.Priority, "priority", 0, "run the GPIO poller at SCHED_FIFO priority `N`")
	flags.Float64Var(&v.cfg.MaxBracket, "max-bracket", pps.DefaultGPIOConfig().MaxBracket,
		"report edges bracketed more widely than `seconds` as settling; 0 disables")
	flags.Float64VarP(&timeoutSec, "timeout", "t", 10, "stop after `seconds` (0 = until interrupted)")
	flags.BoolVarP(&v.jsonl, "jsonl", "j", false, "write output in JSON Lines format")
	usageFunc = cmd.UsageFunc(cmdName, summary, flags)
	if err = flags.Parse(args); err != nil || help {
		return
	}
	if flags.NArg() != 0 {
		err = fmt.Errorf("pps command does not accept positional arguments")
		return
	}
	if flags.Changed("gpio-pin") {
		if gpio < 0 {
			err = fmt.Errorf("--gpio-pin must not be negative")
			return
		}
		v.gpio = &gpio
	}
	if v.device != "" && v.gpio != nil {
		err = fmt.Errorf("--pps-device cannot be combined with --gpio-pin")
		return
	}
	if v.gpio == nil {
		for _, name := range []string{"cpu", "priority", "max-bracket"} {
			if flags.Changed(name) {
				err = fmt.Errorf("--%s requires --gpio-pin", name)
				return
			}
		}
	}
	if flags.Changed("cpu") {
		if cpu < 0 {
			err = fmt.Errorf("--cpu must not be negative")
			return
		}
		v.cfg.CPU = &cpu
	}
	if v.cfg.Priority < 0 || v.cfg.Priority > 99 {
		err = fmt.Errorf("--priority must be between 0 and 99")
		return
	}
	if !(v.cfg.MaxBracket >= 0 && v.cfg.MaxBracket < 1) {
		err = fmt.Errorf("--max-bracket must be at least 0 and less than 1")
		return
	}
	if v.device == "" && v.gpio == nil && flags.Changed("timeout") {
		err = fmt.Errorf("--timeout requires --pps-device or --gpio-pin")
		return
	}
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
	v.timeout = ptime.Seconds(timeoutSec)
	return
}

// noDataError reports that nothing was found or received, with exit code 2
// as for the serial and sdp commands.
type noDataError struct {
	msg string
}

func (e noDataError) Error() string { return e.msg }
func (e noDataError) ExitCode() int { return 2 }

const sysClassPPS = "/sys/class/pps"

// deviceInfo describes a kernel PPS device from its sysfs attributes.
type deviceInfo struct {
	Device  string   `json:"device"`
	Name    string   `json:"name"`
	Path    string   `json:"path,omitempty"`
	Capture []string `json:"capture"`
	Echo    []string `json:"echo,omitempty"`
}

// listDevices prints every PPS device registered in sysDir. A device can be
// unregistered during the scan, so one whose attributes cannot be read is
// skipped with a warning.
func listDevices(lg *slog.Logger, sysDir string, out io.Writer, jsonl bool) error {
	entries, err := os.ReadDir(sysDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	n := 0
	for _, e := range entries {
		info, err := readDeviceInfo(sysDir, e.Name())
		if err != nil {
			lg.Warn("cannot read PPS device attributes", "device", e.Name(), "err", err)
			continue
		}
		if jsonl {
			err = json.NewEncoder(out).Encode(&info)
		} else {
			_, err = fmt.Fprintln(out, info.String())
		}
		if err != nil {
			return err
		}
		n++
	}
	if n == 0 {
		return noDataError{msg: "no PPS devices found"}
	}
	return nil
}

func readDeviceInfo(sysDir, name string) (deviceInfo, error) {
	attr := func(a string) (string, error) {
		b, err := os.ReadFile(filepath.Join(sysDir, name, a))
		return strings.TrimSuffix(string(b), "\n"), err
	}
	info := deviceInfo{Device: "/dev/" + name}
	var err error
	if info.Name, err = attr("name"); err != nil {
		return deviceInfo{}, err
	}
	if info.Path, err = attr("path"); err != nil {
		return deviceInfo{}, err
	}
	s, err := attr("mode")
	if err != nil {
		return deviceInfo{}, err
	}
	// The mode attribute is the device's capabilities as PPS_GETCAP
	// reports them, in hex.
	mode, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil {
		return deviceInfo{}, fmt.Errorf("mode attribute %q: %w", s, err)
	}
	info.Capture = edgeNames(kpps.Mode(mode), kpps.CaptureAssert, kpps.CaptureClear)
	info.Echo = edgeNames(kpps.Mode(mode), kpps.EchoAssert, kpps.EchoClear)
	return info, nil
}

func edgeNames(mode, assert, clear kpps.Mode) []string {
	var names []string
	if mode&assert != 0 {
		names = append(names, "assert")
	}
	if mode&clear != 0 {
		names = append(names, "clear")
	}
	return names
}

func (info deviceInfo) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "device=%s name=%q", info.Device, info.Name)
	if info.Path != "" {
		fmt.Fprintf(&b, " path=%q", info.Path)
	}
	fmt.Fprintf(&b, " capture=%s", strings.Join(info.Capture, ","))
	if len(info.Echo) > 0 {
		fmt.Fprintf(&b, " echo=%s", strings.Join(info.Echo, ","))
	}
	return b.String()
}

type ppsEvent struct {
	Device string `json:"device"`
	T      string `json:"t"`
	Seq    uint32 `json:"seq"`
}

// watchDevice prints the assert timestamps of the PPS device as they arrive
// until ctx is done. The device reports its most recently captured edge at
// once, which may be old, so that reading only sets the baseline and the
// first edge printed is one captured after the command started.
func watchDevice(ctx context.Context, lg *slog.Logger, v flagVars) error {
	src, err := kpps.Open(v.device)
	if err != nil {
		return err
	}
	defer src.Close()
	// Closing the source is what interrupts a waiting Fetch.
	defer context.AfterFunc(ctx, func() { src.Close() })()
	prev, err := src.Fetch(kpps.Info{}, 0)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	n := 0
	for {
		info, err := src.Fetch(prev, -1)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return err
		}
		assert := info.Assert
		if assert.Sequence != prev.Assert.Sequence {
			if missed := assert.Sequence - prev.Assert.Sequence - 1; missed > 0 {
				lg.Warn("missed PPS assert events", "device", v.device, "missed", missed)
			}
			if err := printEvent(enc, v, assert); err != nil {
				return err
			}
			n++
		}
		prev = info
	}
	if n == 0 && v.timeout > 0 {
		return noDataError{msg: "no PPS timestamps received"}
	}
	return nil
}

const (
	timeOfDayFormat = "15:04:05.000000000"
	rfc3339Format   = "2006-01-02T15:04:05.000000000Z"
)

func printEvent(enc *json.Encoder, v flagVars, e kpps.Edge) error {
	t := e.T.UTC()
	if !v.jsonl {
		_, err := fmt.Fprintln(os.Stdout, t.Format(timeOfDayFormat))
		return err
	}
	return enc.Encode(&ppsEvent{Device: v.device, T: t.Format(rfc3339Format), Seq: e.Sequence})
}

type gpioEvent struct {
	GPIO        int     `json:"gpio"`
	T           string  `json:"t"`
	Uncertainty float64 `json:"uncertainty,omitzero"`
	Settling    bool    `json:"settling,omitzero"`
}

// pollGPIO polls the GPIO with the daemon's poller and prints every edge it
// catches, settling ones included, until ctx is done.
func pollGPIO(ctx context.Context, lg *slog.Logger, v flagVars) error {
	edges := make(chan pps.CandidateEdge, 16)
	errCh := make(chan error, 1)
	stats := new(pps.PollStats)
	if !lg.Enabled(ctx, slog.LevelInfo) {
		stats = nil
	}
	go func() {
		err := pps.DetectGPIO(ctx, lg, *v.gpio, v.cfg, edges, stats)
		stats.Log(lg)
		errCh <- err
	}()
	enc := json.NewEncoder(os.Stdout)
	n := 0
	for {
		select {
		case ce := <-edges:
			t := ce.Timestamp.UTC()
			var err error
			if v.jsonl {
				err = enc.Encode(&gpioEvent{GPIO: *v.gpio, T: t.Format(rfc3339Format),
					Uncertainty: ce.Uncertainty.Seconds(), Settling: !ce.Settled})
			} else {
				_, err = fmt.Fprintln(os.Stdout, t.Format(timeOfDayFormat))
			}
			if err != nil {
				return err
			}
			n++
		case err := <-errCh:
			if err != nil && ctx.Err() == nil {
				return err
			}
			if n == 0 && v.timeout > 0 {
				return noDataError{msg: "no PPS edges detected"}
			}
			return nil
		}
	}
}
