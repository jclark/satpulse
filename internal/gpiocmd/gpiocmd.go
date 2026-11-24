//go:build linux

// Package gpiocmd implements the gpio subcommand of satpulsetool, which lists
// the GPIO controllers of the system and the GPIOs the device tree sets up as
// PPS inputs, or polls a GPIO for PPS edges.
package gpiocmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/lib/gpiomem"
	"github.com/jclark/satpulse/gps/lib/kpps"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/spf13/pflag"
)

type flagVars struct {
	name    string
	poll    bool
	cfg     pps.GPIOConfig
	timeout time.Duration
	jsonl   bool
}

const summary = `[-h|--help] [-j|--jsonl] [-p|--pps] [--cpu N] [--priority N]
            [-t|--timeout seconds] [name]`

const defaultTimeout = 10 * time.Second

// Cmd executes the gpio subcommand with the given arguments.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	v, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	if !v.poll {
		return "", list(lg, v)
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	return "", pollPPS(ctx, lg, v, os.Stdout)
}

func parseFlags(cmdName string, args []string) (v flagVars, help bool, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SortFlags = false
	var cpu int
	var timeoutSec float64
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the gpio command")
	flags.BoolVarP(&v.jsonl, "jsonl", "j", false, "write output in JSON Lines format")
	flags.BoolVarP(&v.poll, "pps", "p", false, "poll the GPIO for PPS edges and print the time of each")
	flags.IntVar(&cpu, "cpu", 0, "run the poller on CPU `N`")
	flags.IntVar(&v.cfg.Priority, "priority", 0, "run the poller at SCHED_FIFO priority `N` (1 to 99; 0 = normal priority)")
	flags.Float64VarP(&timeoutSec, "timeout", "t", defaultTimeout.Seconds(), "stop polling after `seconds` (0 = until interrupted)")
	usageFunc = cmd.UsageFunc(cmdName, summary, flags)
	if err = flags.Parse(args); err != nil || help {
		return
	}
	if flags.NArg() > 1 {
		err = fmt.Errorf("too many arguments: expected at most one GPIO name, got %d", flags.NArg())
		return
	}
	if flags.NArg() == 1 {
		if v.name = flags.Arg(0); v.name == "" {
			err = fmt.Errorf("GPIO name must not be empty")
			return
		}
	}
	if !v.poll {
		for _, name := range []string{"cpu", "priority", "timeout"} {
			if flags.Changed(name) {
				err = fmt.Errorf("--%s requires --pps", name)
				return
			}
		}
		return
	}
	if flags.Changed("cpu") {
		if cpu < 0 {
			err = fmt.Errorf("--cpu must not be negative")
			return
		}
		v.cfg.CPU = &cpu
	}
	if v.cfg.Priority < 0 || v.cfg.Priority > 99 {
		err = fmt.Errorf("--priority must be from 0 to 99")
		return
	}
	if math.IsNaN(timeoutSec) || math.IsInf(timeoutSec, 0) {
		err = fmt.Errorf("--timeout must be finite")
	} else if timeoutSec < 0 {
		err = fmt.Errorf("--timeout must not be negative")
	} else if timeoutSec >= float64(math.MaxInt64)/float64(time.Second) {
		err = fmt.Errorf("--timeout is too large")
	} else if v.timeout = ptime.Seconds(timeoutSec); timeoutSec > 0 && v.timeout == 0 {
		err = fmt.Errorf("--timeout is too small")
	}
	return
}

// list prints the GPIO controllers, or, if v names a GPIO, the controller
// that has it.
func list(lg *slog.Logger, v flagVars) error {
	ctrls, err := gpiomem.List()
	if err != nil {
		return err
	}
	if v.name != "" && len(ctrls) > 0 {
		gpio, err := gpiomem.Lookup(v.name)
		if err != nil {
			return err
		}
		ctrls = selectGPIO(ctrls, gpio)
	}
	ppsDevice := func(node string) string {
		device, err := kpps.DevicePathForGPIONode(node)
		if err != nil {
			lg.Debug("no PPS device found for pps-gpio node", "node", node, "error", err)
		}
		return device
	}
	return listControllers(controllerInfos(ctrls, ppsDevice), os.Stdout, v.jsonl)
}

// selectGPIO restricts ctrls to the controller that has gpio, and that
// controller's PPS inputs to gpio.
func selectGPIO(ctrls []gpiomem.Controller, gpio gpiomem.GPIO) []gpiomem.Controller {
	for _, c := range ctrls {
		if c.Node != gpio.Node {
			continue
		}
		var inputs []gpiomem.PPS
		for _, p := range c.PPS {
			if p.Line == gpio.Line {
				inputs = append(inputs, p)
			}
		}
		c.PPS = inputs
		return []gpiomem.Controller{c}
	}
	return nil
}

// controllerInfo is the listing's description of a GPIO controller.
type controllerInfo struct {
	Node       string    `json:"node"`
	Backend    string    `json:"backend,omitempty"`
	Compatible []string  `json:"compatible,omitempty"`
	Chips      []string  `json:"chips,omitempty"`
	Device     string    `json:"device,omitempty"`
	Length     int       `json:"length,omitempty"`
	PPS        []ppsInfo `json:"pps,omitempty"`
}

// ppsInfo is the listing's description of a GPIO set up as a PPS input: its
// name and line, or, on a controller that no backend handles, the cells of
// its GPIO specifier.
type ppsInfo struct {
	GPIO   string   `json:"gpio,omitempty"`
	Line   *int     `json:"line,omitempty"`
	Cells  []uint32 `json:"cells,omitempty"`
	Device string   `json:"device,omitempty"`
}

// controllerInfos converts ctrls for the listing, using ppsDevice to find the
// PPS device of a pps-gpio node, which is empty if none is found.
func controllerInfos(ctrls []gpiomem.Controller, ppsDevice func(node string) string) []controllerInfo {
	infos := make([]controllerInfo, len(ctrls))
	for i, c := range ctrls {
		infos[i] = controllerInfo{Node: c.Node, Backend: c.Backend, Compatible: c.Compatible, Chips: c.Chips, Device: c.Device, Length: c.Length}
		for _, p := range c.PPS {
			pi := ppsInfo{GPIO: p.Name, Cells: p.Cells, Device: ppsDevice(p.Node)}
			if p.Name != "" {
				pi.Line = &p.Line
			}
			infos[i].PPS = append(infos[i].PPS, pi)
		}
	}
	return infos
}

// noDataError reports that nothing was found, with exit code 2 as for the
// serial and sdp commands.
type noDataError struct {
	msg string
}

func (e noDataError) Error() string { return e.msg }
func (e noDataError) ExitCode() int { return 2 }

// listControllers prints infos.
func listControllers(infos []controllerInfo, out io.Writer, jsonl bool) error {
	if len(infos) == 0 {
		return noDataError{msg: "no GPIO controllers found in the device tree"}
	}
	for _, info := range infos {
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

// String formats info as an indented block: the device-tree node, then a
// line for each field and a block for each PPS input.
func (info *controllerInfo) String() string {
	var b strings.Builder
	b.WriteString(info.Node)
	if info.Backend != "" {
		fmt.Fprintf(&b, "\n  backend: %s", info.Backend)
	}
	if len(info.Compatible) > 0 {
		fmt.Fprintf(&b, "\n  compatible: %s", strings.Join(info.Compatible, ", "))
	}
	if len(info.Chips) > 0 {
		fmt.Fprintf(&b, "\n  chips: %s", strings.Join(info.Chips, ", "))
	}
	if info.Device != "" {
		fmt.Fprintf(&b, "\n  device: %s\n  length: %#x", info.Device, info.Length)
	}
	for _, p := range info.PPS {
		b.WriteString("\n  pps:")
		if p.GPIO != "" {
			fmt.Fprintf(&b, "\n    gpio: %s\n    line: %d", p.GPIO, *p.Line)
		} else {
			cells := make([]string, len(p.Cells))
			for i, c := range p.Cells {
				cells[i] = strconv.FormatUint(uint64(c), 10)
			}
			fmt.Fprintf(&b, "\n    cells: %s", strings.Join(cells, ", "))
		}
		if p.Device != "" {
			fmt.Fprintf(&b, "\n    device: %s", p.Device)
		}
	}
	return b.String()
}
