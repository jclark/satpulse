package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	rinexubx "github.com/jclark/satpulse/gps/lib/rinex/ubx"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const scanBufSize = 64 * 1024

type config struct {
	inputPath  string
	outputPath string
	metaPath   string
	meta       rinex.Metadata
	wopts      rinex.WriterOptions
	uopts      rinexubx.Options
}

func main() {
	cfg, usage, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, filepath.Base(os.Args[0])+":", err)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, filepath.Base(os.Args[0])+":", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (config, string, error) {
	var cfg config
	flags := pflag.NewFlagSet("ubx2rinex", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVarP(&cfg.outputPath, "output", "o", "", "output RINEX observation file, or - for stdout")
	flags.StringVar(&cfg.metaPath, "metadata", "", "JSON RINEX metadata file")
	flags.StringVar(&cfg.meta.MarkerName, "marker-name", "", "RINEX marker name")
	flags.StringVar(&cfg.meta.MarkerNumber, "marker-number", "", "RINEX marker number")
	flags.StringVar(&cfg.meta.MarkerType, "marker-type", "", "RINEX marker type")
	flags.StringVar(&cfg.meta.Observer, "observer", "", "RINEX observer")
	flags.StringVar(&cfg.meta.Agency, "agency", "", "RINEX agency")
	flags.StringVar(&cfg.meta.Receiver.Number, "receiver-number", "", "RINEX receiver number")
	flags.StringVar(&cfg.meta.Receiver.Type, "receiver-type", "", "RINEX receiver type")
	flags.StringVar(&cfg.meta.Receiver.Version, "receiver-version", "", "RINEX receiver version")
	flags.StringVar(&cfg.meta.Antenna.Number, "antenna-number", "", "RINEX antenna number")
	flags.StringVar(&cfg.meta.Antenna.Type, "antenna-type", "", "RINEX antenna type")
	flags.StringVar(&cfg.wopts.Program, "program", "ubx2rinex", "RINEX program field")
	flags.StringVar(&cfg.wopts.RunBy, "run-by", "", "RINEX run-by field")
	flags.Uint8Var(&cfg.uopts.PhaseThreshold, "phase-threshold", 0, "RAWX cpStdev index at which carrier phase is omitted, or 0 for RTKLIB Explorer auto")
	flags.Uint8Var(&cfg.uopts.SlipThreshold, "slip-threshold", 15, "RAWX cpStdev index that marks a cycle slip")
	if err := flags.Parse(args); err != nil {
		return cfg, usage(flags), err
	}
	switch flags.NArg() {
	case 1:
		cfg.inputPath = flags.Arg(0)
	case 2:
		cfg.inputPath = flags.Arg(0)
		if cfg.outputPath != "" {
			return cfg, usage(flags), errors.New("output specified both as argument and -o")
		}
		cfg.outputPath = flags.Arg(1)
	default:
		return cfg, usage(flags), errors.New("expected input UBX file and optional output file")
	}
	cfg.meta.Comments = append(cfg.meta.Comments, "format: u-blox UBX")
	cfg.meta.Comments = append(cfg.meta.Comments, "options: -MULTICODE")
	if cfg.inputPath == "-" {
		cfg.meta.Comments = append(cfg.meta.Comments, "log: stdin")
	} else {
		cfg.meta.Comments = append(cfg.meta.Comments, "log: "+cfg.inputPath)
	}
	return cfg, "", nil
}

func usage(flags *pflag.FlagSet) string {
	return "Usage: ubx2rinex [options] input.ubx [output.obs]\nOptions:\n" + flags.FlagUsages()
}

func run(cfg config) error {
	in, err := openInput(cfg.inputPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := openOutput(cfg.outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	sink := rinex.NewObservationSink(out, cfg.wopts)
	meta, err := loadMetadata(cfg)
	if err != nil {
		return err
	}
	if err := sink.Metadata(meta); err != nil {
		return err
	}
	conv := rinexubx.New(sink, cfg.uopts)
	if err := convert(in, conv); err != nil {
		return err
	}
	return sink.Flush()
}

func openInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

func loadMetadata(cfg config) (rinex.Metadata, error) {
	if cfg.metaPath == "" {
		return cfg.meta, nil
	}
	f, err := os.Open(cfg.metaPath)
	if err != nil {
		return rinex.Metadata{}, err
	}
	defer f.Close()
	var meta rinex.Metadata
	if err := json.NewDecoder(f).Decode(&meta); err != nil {
		return rinex.Metadata{}, fmt.Errorf("%s: %w", cfg.metaPath, err)
	}
	return rinex.MergeMetadata(meta, cfg.meta), nil
}

func openOutput(path string) (io.WriteCloser, error) {
	if path == "" || path == "-" {
		return nopWriteCloser{os.Stdout}, nil
	}
	return os.Create(path)
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

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error {
	return nil
}
