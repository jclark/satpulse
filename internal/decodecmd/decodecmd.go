package decodecmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/gpsdecode"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/spf13/pflag"
)

type output struct {
	Tag gpsprot.Tag `json:"tag"`
	Msg string      `json:"msg"`
	gpsdecode.DecodeResult
}

const summary = `[-h|--help] [-c|--compact] [--out] [--bin|--line] data`

// Cmd implements the decode subcommand.
// DATA is a required positional argument containing the packet data.
// By default, DATA is auto-detected as hex (if all characters are hex digits)
// or ASCII text (with \r\n appended).
// --bin forces hex interpretation; --line forces ASCII interpretation.
func Cmd(_ io.Writer, _ slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	help := false
	compact := false
	out := false
	binFlag := false
	lineFlag := false

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&compact, "compact", "c", false, "output compact JSON (single line)")
	flags.BoolVar(&out, "out", false, "treat packet as outgoing (affects CFG-VAL* decoding)")
	flags.BoolVar(&binFlag, "bin", false, "treat data as hex-encoded binary")
	flags.BoolVar(&lineFlag, "line", false, "treat data as ASCII text (\\r\\n appended)")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	if err := flags.Parse(args); err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	if flags.NArg() != 1 {
		return usageFunc(progName), fmt.Errorf("expected exactly one argument")
	}
	if flags.Arg(0) == "" {
		return "", fmt.Errorf("packet data must not be empty")
	}
	if binFlag && lineFlag {
		return usageFunc(progName), fmt.Errorf("--bin and --line are mutually exclusive")
	}
	data := flags.Arg(0)
	var pktBytes []byte
	switch {
	case binFlag:
		pktBytes, err = hex.DecodeString(data)
		if err != nil {
			return "", fmt.Errorf("invalid hex: %w", err)
		}
	case lineFlag:
		pktBytes = []byte(data + "\r\n")
	default:
		if isAllHex(data) {
			pktBytes, err = hex.DecodeString(data)
			if err != nil {
				return "", fmt.Errorf("invalid hex: %w", err)
			}
		} else {
			pktBytes = []byte(data + "\r\n")
		}
	}
	return "", runDecode(pktBytes, out, compact)
}

func isAllHex(s string) bool {
	for _, c := range s {
		if !isHexDigit(c) {
			return false
		}
	}
	return true
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func runDecode(data []byte, out, compact bool) error {
	pf, result, err := gpsdecode.Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), data, out)
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
