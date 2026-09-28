package scancmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [--vendor name] file|-`

type flagVars struct {
	vendor   gpsreg.Vendor
	filePath string
}

// Cmd implements the scan subcommand.
// It reads a packet byte stream and writes a JSONL packet log.
func Cmd(_ io.Writer, _ slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return usage, err
	}
	vendors, err := cmd.ResolveVendors(v.vendor)
	if err != nil {
		return "", err
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
	err = run(input, out, vendors)
	if flushErr := out.Flush(); err == nil {
		err = flushErr
	}
	return "", err
}

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	vendorStr := ""
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVar(&vendorStr, "vendor", "", "GPS receiver `vendor` name")
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
	vendor, err := gpsreg.ParseVendor(vendorStr)
	if err != nil {
		return nil, usageFunc, err
	}
	return &flagVars{
		vendor:   vendor,
		filePath: flags.Arg(0),
	}, usageFunc, nil
}

type writeFlusher interface {
	io.Writer
	Flush() error
}

func run(input io.Reader, out writeFlusher, vendors []gpsreg.Vendor) error {
	scanner := scan.New(input, 16, gpsreg.CreatePacketFormats(vendors))
	enc := json.NewEncoder(out)
	for {
		pkt, err := scanner.Scan()
		if len(pkt.Data) != 0 {
			entry := gpsio.MakePacketLogEntry(pkt)
			if encErr := enc.Encode(&entry); encErr != nil {
				return encErr
			}
			if flushErr := out.Flush(); flushErr != nil {
				return flushErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
