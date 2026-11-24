//go:build linux

// Package gpiocmd implements the gpio subcommand of satpulsetool, which lists
// the GPIO controllers of the system and the GPIOs the device tree sets up as
// PPS inputs.
package gpiocmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/lib/gpiomem"
	"github.com/jclark/satpulse/gps/lib/kpps"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [-j|--jsonl]`

// Cmd executes the gpio subcommand with the given arguments.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	jsonl, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	ctrls, err := gpiomem.List()
	if err != nil {
		return "", err
	}
	ppsDevice := func(node string) string {
		device, err := kpps.DevicePathForGPIONode(node)
		if err != nil {
			lg.Debug("no PPS device found for pps-gpio node", "node", node, "error", err)
		}
		return device
	}
	return "", listControllers(controllerInfos(ctrls, ppsDevice), os.Stdout, jsonl)
}

func parseFlags(cmdName string, args []string) (jsonl, help bool, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SortFlags = false
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the gpio command")
	flags.BoolVarP(&jsonl, "jsonl", "j", false, "write output in JSON Lines format")
	usageFunc = cmd.UsageFunc(cmdName, summary, flags)
	if err = flags.Parse(args); err != nil || help {
		return
	}
	if flags.NArg() != 0 {
		err = fmt.Errorf("gpio command does not accept positional arguments")
	}
	return
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
// name and line, or, where the GPIO has no name, the cells of its GPIO
// specifier.
type ppsInfo struct {
	GPIO   string   `json:"gpio,omitempty"`
	Line   *int     `json:"line,omitempty"`
	Cells  []uint32 `json:"cells,omitzero"`
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
