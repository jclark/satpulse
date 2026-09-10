// Package ppscmd implements the pps subcommand of satpulsetool, which lists
// kernel PPS devices and prints the timestamps of one as they arrive.
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
	"github.com/jclark/satpulse/gps/lib/kpps"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/spf13/pflag"
)

type flagVars struct {
	device  string
	timeout time.Duration
	jsonl   bool
}

const summary = `[-h|--help] [-d|--pps-device path] [-t|--timeout seconds] [-j|--jsonl]`

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
	if v.device == "" {
		return "", listDevices(lg, sysClassPPS, os.Stdout, v.jsonl)
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	return "", watchDevice(ctx, lg, v)
}

func parseFlags(cmdName string, args []string) (v flagVars, help bool, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SortFlags = false
	var timeoutSec float64
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the pps command")
	flags.StringVarP(&v.device, "pps-device", "d", "", "print the assert timestamps of the PPS device at `path`")
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
	if v.device == "" && flags.Changed("timeout") {
		err = fmt.Errorf("--timeout requires --pps-device")
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
	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}
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

func printEvent(enc *json.Encoder, v flagVars, e kpps.Edge) error {
	t := e.T.UTC()
	if !v.jsonl {
		_, err := fmt.Fprintln(os.Stdout, t.Format("15:04:05.000000000"))
		return err
	}
	return enc.Encode(&ppsEvent{Device: v.device, T: t.Format("2006-01-02T15:04:05.000000000Z"), Seq: e.Sequence})
}
