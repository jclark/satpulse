package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	rinexubx "github.com/jclark/satpulse/gps/lib/rinex/ubx"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const scanBufSize = 64 * 1024

const summary = `[-h|--help] [options] input.ubx [output.obs]`

type flagVars struct {
	inputPath  string
	outputPath string
	metaPath   string
	meta       rinex.Metadata
	wopts      rinex.WriterOptions
	uopts      rinexubx.Options
}

func main() {
	usage, err := Cmd(os.Stderr, slog.LevelWarn, os.Args[0], "", os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, filepath.Base(os.Args[0])+":", err)
	}
	fmt.Fprint(os.Stderr, usage)
	if err == nil {
		return
	}
	if usage != "" {
		os.Exit(2)
	}
	os.Exit(1)
}

// Cmd implements the ubx2rinex command.
func Cmd(_ io.Writer, _ slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseArgs(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if v == nil {
		return usageFunc(progName), nil
	}
	if v.wopts.Date.IsZero() {
		v.wopts.Date = time.Now().UTC()
	}
	if v.metaPath != "" {
		f, err := os.Open(v.metaPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if err := loadMetadata(&v.meta, v.metaPath, f); err != nil {
			return "", err
		}
	}
	var in io.Reader
	if v.inputPath == "-" {
		in = os.Stdin
	} else {
		f, err := os.Open(v.inputPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		in = f
	}
	var out io.Writer = os.Stdout
	if v.outputPath != "" && v.outputPath != "-" {
		f, err := os.Create(v.outputPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		out = f
	}
	return "", run(in, out, v.meta, v.wopts, v.uopts)
}

func parseArgs(cmdName string, args []string) (*flagVars, func(string) string, error) {
	v := &flagVars{}
	flagName := cmdName
	if flagName == "" {
		flagName = "ubx2rinex"
	}
	flags := pflag.NewFlagSet(flagName, pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	help := false
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVarP(&v.outputPath, "output", "o", "", "output RINEX observation file, or - for stdout")
	flags.StringVar(&v.metaPath, "metadata", "", "JSON RINEX metadata file")
	flags.StringVar(&v.meta.MarkerName, "marker-name", "", "RINEX marker name")
	flags.StringVar(&v.meta.MarkerNumber, "marker-number", "", "RINEX marker number")
	flags.StringVar(&v.meta.MarkerType, "marker-type", "", "RINEX marker type")
	flags.StringVar(&v.meta.Observer, "observer", "", "RINEX observer")
	flags.StringVar(&v.meta.Agency, "agency", "", "RINEX agency")
	flags.StringVar(&v.meta.Receiver.Number, "receiver-number", "", "RINEX receiver number")
	flags.StringVar(&v.meta.Receiver.Type, "receiver-type", "", "RINEX receiver type")
	flags.StringVar(&v.meta.Receiver.Version, "receiver-version", "", "RINEX receiver version")
	flags.StringVar(&v.meta.Antenna.Number, "antenna-number", "", "RINEX antenna number")
	flags.StringVar(&v.meta.Antenna.Type, "antenna-type", "", "RINEX antenna type")
	flags.StringVar(&v.wopts.Program, "program", "ubx2rinex", "RINEX program field")
	flags.StringVar(&v.wopts.RunBy, "run-by", "", "RINEX run-by field")
	flags.Uint8Var(&v.uopts.PhaseThreshold, "phase-threshold", 0, "RAWX cpStdev index at which carrier phase is omitted, or 0 for RTKLIB Explorer auto")
	flags.Uint8Var(&v.uopts.SlipThreshold, "slip-threshold", 15, "RAWX cpStdev index that marks a cycle slip")
	usageFunc := usage(cmdName, flags)
	if err := flags.Parse(args); err != nil {
		return nil, usageFunc, err
	}
	if help {
		return nil, usageFunc, nil
	}
	switch flags.NArg() {
	case 1:
		v.inputPath = flags.Arg(0)
	case 2:
		v.inputPath = flags.Arg(0)
		if v.outputPath != "" {
			return nil, usageFunc, errors.New("output specified both as argument and -o")
		}
		v.outputPath = flags.Arg(1)
	default:
		return nil, usageFunc, errors.New("expected input UBX file and optional output file")
	}
	v.meta.Comments = append(v.meta.Comments, "format: u-blox UBX")
	v.meta.Comments = append(v.meta.Comments, "options: -MULTICODE")
	if v.inputPath == "-" {
		v.meta.Comments = append(v.meta.Comments, "log: stdin")
	} else {
		v.meta.Comments = append(v.meta.Comments, "log: "+v.inputPath)
	}
	return v, usageFunc, nil
}

func usage(cmdName string, flags *pflag.FlagSet) func(string) string {
	return func(progName string) string {
		name := progName
		if cmdName != "" {
			name += " " + cmdName
		}
		return fmt.Sprintf("Usage: %s %s\nOptions:\n%s", name, summary, flags.FlagUsages())
	}
}

func run(in io.Reader, out io.Writer, meta rinex.Metadata, wopts rinex.WriterOptions, uopts rinexubx.Options) error {
	sink := rinex.NewObservationSink(out, wopts)
	if err := sink.Metadata(meta); err != nil {
		return err
	}
	conv := rinexubx.New(sink, uopts)
	if err := convert(in, conv); err != nil {
		return err
	}
	return sink.Flush()
}

func loadMetadata(dst *rinex.Metadata, path string, r io.Reader) error {
	var meta rinex.Metadata
	if err := json.NewDecoder(r).Decode(&meta); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	*dst = rinex.MergeMetadata(meta, *dst)
	return nil
}

func convert(r io.Reader, conv *rinexubx.Converter) error {
	s := scan.New(r, scanBufSize, gpsreg.CreatePacketFormats(gpsreg.VendorUblox))
	n := 0
	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if pkt.ReadError != nil {
			return pkt.ReadError
		}
		if pkt.Format == nil {
			if len(pkt.Data) != 0 {
				return fmt.Errorf("invalid input data before packet: %d bytes", len(pkt.Data))
			}
			continue
		}
		if !pkt.HasTag(gpsreg.TagUBX) {
			continue
		}
		if !pkt.ChecksumValid {
			return pkt.ChecksumError()
		}
		if ubxbin.PacketMsgId(pkt.Data) != ubxbin.RxmRawxID {
			continue
		}
		msg, err := ubxbin.ParseMsg(pkt.Data)
		if err != nil {
			return err
		}
		rawx, ok := msg.(*ubxbin.RxmRawx)
		if !ok {
			continue
		}
		if err := conv.ConvertRAWX(rawx); err != nil {
			return err
		}
		n++
	}
	if n == 0 {
		return errors.New("no UBX-RXM-RAWX messages found")
	}
	return nil
}
