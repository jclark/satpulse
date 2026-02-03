package decodecmd

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
	"github.com/jclark/satpulse/gps/gpsdecode"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/spf13/pflag"
)

type output struct {
	Tag gpsprot.Tag `json:"tag"`
	Msg string      `json:"msg"`
	gpsdecode.DecodeResult
}

const summary = `[-h|--help] [-c|--compact] [--out] [--packet-log path] [hex-packet]`

func Cmd(_ io.Writer, _ slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	help := false
	compact := false
	out := false
	packetLog := ""

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&compact, "compact", "c", false, "output compact JSON (single line)")
	flags.BoolVar(&out, "out", false, "treat packet as outgoing (affects CFG-VAL* decoding)")
	flags.StringVar(&packetLog, "packet-log", "", "annotate JSONL packet log (file path or - for stdin)")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	err = flags.Parse(args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	if packetLog != "" {
		if flags.NArg() != 0 {
			return usageFunc(progName), fmt.Errorf("--packet-log does not accept positional arguments")
		}
		return "", runPacketLog(packetLog)
	}
	if flags.NArg() != 1 {
		return usageFunc(progName), fmt.Errorf("expected exactly one hex packet argument")
	}
	return "", runHexPacket(flags.Arg(0), out, compact)
}

func runHexPacket(hexStr string, out, compact bool) error {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	pf, result, err := gpsdecode.Decode(gpsreg.PacketFormats, data, out)
	if err != nil {
		return err
	}
	o := output{
		Tag: pf.Tag(),
		Msg: pf.MsgID(data),
	}
	if result != nil {
		o.DecodeResult = *result
	}
	enc := json.NewEncoder(os.Stdout)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(o)
}

func runPacketLog(path string) error {
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
	// Only process binary packets
	if len(entry.Bin) == 0 {
		return line
	}
	_, result, err := gpsdecode.Decode(gpsreg.PacketFormats, entry.Bin, entry.Out)
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
	// Insert fields in order: header, payload, cfgData
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
