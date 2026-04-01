package annotatecmd

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsdecode"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [file|-]`

// Cmd implements the annotate subcommand.
// It reads a JSONL packet log and adds decoded fields
// (header, payload, cfgData) to each entry.
func Cmd(_ io.Writer, _ slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	help := false
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	if err := flags.Parse(args); err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	if flags.NArg() > 1 {
		return usageFunc(progName), fmt.Errorf("expected at most one file argument")
	}
	path := "-"
	if flags.NArg() == 1 {
		path = flags.Arg(0)
	}
	return "", run(path)
}

func run(path string) error {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	scanner := bufio.NewScanner(r)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for scanner.Scan() {
		line := scanner.Bytes()
		processed := processLine(line)
		out.Write(processed)
		out.WriteByte('\n')
	}
	return scanner.Err()
}

func processLine(line []byte) []byte {
	var entry gpsio.PacketLogEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return line
	}
	var data []byte
	if len(entry.Bin) > 0 {
		data = entry.Bin
	} else if entry.Ascii != "" {
		data = []byte(entry.Ascii)
	} else {
		return line
	}
	_, result, err := gpsdecode.Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), data, entry.Out)
	if err != nil {
		var csErr *gpsdecode.ChecksumError
		if errors.As(err, &csErr) {
			return insertChecksumError(line, csErr)
		}
		return line
	}
	if result == nil {
		return line
	}
	return insertDecodeResult(line, result)
}

type checksumErrorJSON struct {
	InPacket string `json:"inPacket"`
	Computed string `json:"computed"`
}

func insertChecksumError(line []byte, csErr *gpsdecode.ChecksumError) []byte {
	errJSON := checksumErrorJSON{
		InPacket: hex.EncodeToString(csErr.InPacket),
		Computed: hex.EncodeToString(csErr.Computed),
	}
	b, err := json.Marshal(errJSON)
	if err != nil {
		return line
	}
	return insertField(line, "checksumError", b)
}

func insertDecodeResult(line []byte, result *gpsdecode.DecodeResult) []byte {
	if result.Header != nil {
		b, err := json.Marshal(result.Header)
		if err == nil {
			line = insertField(line, "header", b)
		}
	}
	if result.Payload != nil {
		b, err := json.Marshal(result.Payload)
		if err == nil {
			line = insertField(line, "payload", b)
		}
	}
	if result.CfgData != nil {
		b, err := json.Marshal(result.CfgData)
		if err == nil {
			line = insertField(line, "cfgData", b)
		}
	}
	return line
}

func insertField(line []byte, name string, value []byte) []byte {
	closeIdx := bytes.LastIndexByte(line, '}')
	if closeIdx == -1 {
		return line
	}
	field := fmt.Sprintf(",\"%s\":%s", name, value)
	result := make([]byte, 0, len(line)+len(field))
	result = append(result, line[:closeIdx]...)
	result = append(result, field...)
	result = append(result, line[closeIdx:]...)
	return result
}
