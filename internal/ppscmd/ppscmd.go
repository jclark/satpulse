// Package ppscmd implements the pps subcommand of satpulsetool, which lists
// kernel PPS devices and prints the timestamps of one as they arrive.
package ppscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
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
		devices, err := kpps.ListDevices()
		if err != nil {
			return "", err
		}
		return "", listDevices(devices, os.Stdout, v.jsonl)
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

// deviceInfo is the listing's description of a kernel PPS device.
type deviceInfo struct {
	Device     string   `json:"device"`
	Name       string   `json:"name"`
	SourcePath string   `json:"sourcePath,omitempty"`
	Capture    []string `json:"capture"`
	Echo       []string `json:"echo,omitempty"`
}

// listDevices prints the devices.
func listDevices(devices []kpps.Device, out io.Writer, jsonl bool) error {
	if len(devices) == 0 {
		return noDataError{msg: "no PPS devices found"}
	}
	for _, d := range devices {
		info := deviceInfo{Device: d.Path, Name: d.Name, SourcePath: d.SourcePath,
			Capture: edgeNames(d.Mode, kpps.CaptureAssert, kpps.CaptureClear),
			Echo:    edgeNames(d.Mode, kpps.EchoAssert, kpps.EchoClear)}
		var err error
		if jsonl {
			err = json.NewEncoder(out).Encode(&info)
		} else {
			_, err = fmt.Fprintln(out, info.String())
		}
		if err != nil {
			return err
		}
	}
	return nil
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
	if info.SourcePath != "" {
		fmt.Fprintf(&b, " sourcePath=%q", info.SourcePath)
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
// first edge printed is one captured after the command started. Clear edges
// are counted, since a device whose capture mode another consumer has set
// to clear only reports no asserts at all.
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
