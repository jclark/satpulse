package replaycmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/spf13/pflag"
)

type flagVars struct {
	vendor   gpsreg.Vendor
	idleGap  time.Duration
	filePath string
}

// Cmd implements the replay subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	vendors, err := cmd.ResolveVendors(v.vendor)
	if err != nil {
		return "", err
	}
	lg := cmd.NewLogger(logWriter, logLevel)
	var input io.Reader
	if v.filePath == "-" {
		input = os.Stdin
	} else {
		f, err := os.Open(v.filePath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		input = f
	}
	return "", run(lg, input, vendors, v.idleGap)
}

const summary = `[-h|--help] [--vendor name] [--idle-gap seconds] file`

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	vendorStr := ""
	idleGap := 0.15
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVar(&vendorStr, "vendor", "", "GPS vendor `name` (e.g. u-blox, Unicore)")
	flags.Float64Var(&idleGap, "idle-gap", 0.15, "idle gap threshold in `seconds` (0 disables)")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	if err := flags.Parse(args); err != nil {
		return nil, usageFunc, err
	}
	if help {
		return nil, usageFunc, nil
	}
	if idleGap < 0 || idleGap >= 1 {
		return nil, usageFunc, fmt.Errorf("--idle-gap must be >= 0 and < 1")
	}
	if flags.NArg() != 1 {
		return nil, usageFunc, fmt.Errorf("expected exactly one file argument")
	}
	vendor, err := gpsreg.ParseVendor(vendorStr)
	if err != nil {
		return nil, usageFunc, err
	}
	return &flagVars{
		vendor:   vendor,
		idleGap:  time.Duration(idleGap * float64(time.Second)),
		filePath: flags.Arg(0),
	}, nil, nil
}

func run(lg *slog.Logger, input io.Reader, vendors []gpsreg.Vendor, idleGap time.Duration) error {
	pktProcs := gpsreg.CreatePacketProcessors(vendors)
	rh := &replayHandler{
		out: bufio.NewWriter(os.Stdout),
		lg:  lg,
	}
	gpsprot.SetAllMsgHandlers(pktProcs, &gpsprot.GenericHandler{Handle: rh.handle})
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	var tPrev time.Time
	started := false
	for scanner.Scan() {
		var entry gpsio.PacketLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			lg.Warn("skipping invalid JSONL line", "err", err)
			continue
		}
		if entry.Out || entry.Tag == "" {
			continue
		}
		t := time.Time(entry.T)
		if !started {
			rh.tStart = t
			started = true
		}
		pp, ok := pktProcs[entry.Tag]
		if !ok {
			lg.Warn("unknown tag, skipping", "tag", entry.Tag)
			continue
		}
		if idleGap > 0 && !tPrev.IsZero() && t.Sub(tPrev) > idleGap {
			for _, p := range pktProcs {
				p.Idle(tPrev)
			}
		}
		if _, err := pp.ProcessPacket(entry.Data(), t); err != nil {
			lg.Warn("error processing packet", "tag", entry.Tag, "err", err)
		}
		if rh.err != nil {
			return rh.err
		}
		tPrev = t
	}
	if idleGap > 0 && !tPrev.IsZero() {
		for _, p := range pktProcs {
			p.Idle(tPrev)
		}
	}
	if err := rh.out.Flush(); err != nil {
		return err
	}
	return scanner.Err()
}

type replayHandler struct {
	out    *bufio.Writer
	tStart time.Time
	lg     *slog.Logger
	err    error
}

func (h *replayHandler) handle(msg gpsprot.Msg, tRead time.Time) {
	if h.err != nil {
		return
	}
	mono := gpsprot.Duration(tRead.Sub(h.tStart)) / gpsprot.Microsecond * gpsprot.Microsecond
	ev := gpsprot.NewMsgEvent(msg, tRead, mono)
	b, err := json.Marshal(&ev)
	if err != nil {
		h.lg.Warn("failed to marshal event", "err", err)
		return
	}
	b = append(b, '\n')
	if _, err = h.out.Write(b); err != nil {
		h.err = err
	}
}
