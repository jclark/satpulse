package packcmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [-t|--tag TAG] [-m|--msg MSG] [-r|--realtime FACTOR] file|-`

type flagVars struct {
	tag      string
	msg      string
	realtime float64
	filePath string
}

// Cmd implements the pack subcommand.
// It reads a JSONL packet log and writes the selected packets as a byte stream.
func Cmd(_ io.Writer, _ slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return usage, err
	}

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

	out := bufio.NewWriter(os.Stdout)
	return "", run(input, out, runConfig{
		tag:    v.tag,
		msg:    v.msg,
		factor: v.realtime,
	})
}

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	v := &flagVars{}
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVarP(&v.tag, "tag", "t", "", "packet log tag to include")
	flags.StringVarP(&v.msg, "msg", "m", "", "packet log message ID to include")
	flags.Float64VarP(&v.realtime, "realtime", "r", 0, "replay with realtime inter-packet spacing divided by FACTOR (1.0 = original timing)")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	if err := flags.Parse(args); err != nil {
		return nil, usageFunc, err
	}
	if help {
		return nil, usageFunc, nil
	}
	if flags.NArg() != 1 {
		return nil, usageFunc, fmt.Errorf("expected exactly one file argument")
	}
	if v.msg != "" && v.tag == "" {
		return nil, usageFunc, fmt.Errorf("--msg requires --tag")
	}
	if flags.Changed("realtime") {
		if math.IsNaN(v.realtime) || math.IsInf(v.realtime, 0) || v.realtime <= 0 {
			return nil, usageFunc, fmt.Errorf("--realtime factor must be a positive finite number")
		}
	}
	v.filePath = flags.Arg(0)
	return v, usageFunc, nil
}

type runConfig struct {
	tag    string
	msg    string
	factor float64 // realtime speedup factor; 0 disables realtime spacing
}

type writeFlusher interface {
	io.Writer
	Flush() error
}

func run(input io.Reader, out writeFlusher, cfg runConfig) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var baseEntryT time.Time
	var baseWall time.Time
	haveBase := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		entry, err := unmarshalEntry(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		data, ok := selectPacket(&entry, cfg)
		if !ok {
			continue
		}
		if cfg.factor > 0 {
			entryT := time.Time(entry.T)
			if entryT.IsZero() {
				return fmt.Errorf("line %d: --realtime requires selected packets to have a non-zero t", lineNo)
			}
			if haveBase {
				// Schedule against a fixed base (the first packet's actual
				// emission and log time), not the previous packet. Anchoring to
				// the previous write re-added each Sleep's wakeup overshoot every
				// packet, which accumulates into linear drift; on a virtualized
				// host (macOS CI) it compounded to ~7 ms/packet. Absolute
				// scheduling bounds the error to a single overshoot and catches
				// up after a brief output stall.
				target := baseWall.Add(time.Duration(float64(entryT.Sub(baseEntryT)) / cfg.factor))
				if sleepFor := time.Until(target); sleepFor > 0 {
					time.Sleep(sleepFor)
				}
			}
			if _, err := out.Write(data); err != nil {
				return fmt.Errorf("line %d: write packet: %w", lineNo, err)
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("line %d: flush packet: %w", lineNo, err)
			}
			if !haveBase {
				// The first Flush can block until a FIFO reader attaches; anchor
				// the base after it returns so that one-time wait is not charged
				// against the schedule of later packets.
				baseWall = time.Now()
				baseEntryT = entryT
				haveBase = true
			}
			continue
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("line %d: write packet: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if cfg.factor == 0 {
		return out.Flush()
	}
	return nil
}

func unmarshalEntry(line []byte) (gpsio.PacketLogEntry, error) {
	var entry gpsio.PacketLogEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		if strings.Contains(err.Error(), "encoding/hex") {
			return entry, fmt.Errorf("invalid bin hex: %w", err)
		}
		return entry, fmt.Errorf("invalid JSON: %w", err)
	}
	return entry, nil
}

func selectPacket(entry *gpsio.PacketLogEntry, cfg runConfig) ([]byte, bool) {
	if entry.Out {
		return nil, false
	}
	if len(entry.Bin) == 0 && entry.Ascii == "" {
		return nil, false
	}
	if cfg.tag != "" && !strings.EqualFold(string(entry.Tag), cfg.tag) {
		return nil, false
	}
	if cfg.msg != "" && !strings.EqualFold(entry.Msg, cfg.msg) {
		return nil, false
	}
	if len(entry.Bin) != 0 {
		return []byte(entry.Bin), true
	}
	return []byte(entry.Ascii), true
}
