package packcmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [--tag TAG] [--msg MSG] [--timing] file|-`

type flagVars struct {
	tag      string
	msg      string
	timing   bool
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
		timing: v.timing,
		clock:  realClock{},
	})
}

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	v := &flagVars{}
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVar(&v.tag, "tag", "", "packet log tag to include")
	flags.StringVar(&v.msg, "msg", "", "packet log message ID to include")
	flags.BoolVar(&v.timing, "timing", false, "preserve inter-packet timing")
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
	v.filePath = flags.Arg(0)
	return v, usageFunc, nil
}

type runConfig struct {
	tag    string
	msg    string
	timing bool
	clock  clock
}

type clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

type writeFlusher interface {
	io.Writer
	Flush() error
}

func run(input io.Reader, out writeFlusher, cfg runConfig) error {
	if cfg.clock == nil {
		cfg.clock = realClock{}
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var prevEntryT time.Time
	var prevWriteStart time.Time
	havePrev := false
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
		if cfg.timing {
			entryT := time.Time(entry.T)
			if entryT.IsZero() {
				return fmt.Errorf("line %d: --timing requires selected packets to have a non-zero t", lineNo)
			}
			if havePrev {
				delta := entryT.Sub(prevEntryT)
				if delta > 0 {
					targetWriteStart := prevWriteStart.Add(delta)
					if sleepFor := targetWriteStart.Sub(cfg.clock.Now()); sleepFor > 0 {
						cfg.clock.Sleep(sleepFor)
					}
				}
			}
			writeStart := cfg.clock.Now()
			if _, err := out.Write(data); err != nil {
				return fmt.Errorf("line %d: write packet: %w", lineNo, err)
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("line %d: flush packet: %w", lineNo, err)
			}
			prevEntryT = entryT
			prevWriteStart = writeStart
			havePrev = true
			continue
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("line %d: write packet: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !cfg.timing {
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
