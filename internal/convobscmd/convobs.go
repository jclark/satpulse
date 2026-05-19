// Package convobscmd implements the satpulsetool convobs subcommand.
package convobscmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	rinexubx "github.com/jclark/satpulse/gps/lib/rinex/ubx"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const scanBufSize = 64 * 1024

const summary = `[-h|--help] [-o|--output path] [--metadata path]
           [-r|--from raw|ubx|rinex|obsj] [--packet-log] [--to rinex|obsj]
           [--interval seconds]
           [--marker-name name] [--marker-number number] [--marker-type type]
           [--observer name] [--agency name]
           [--receiver-number number] [--receiver-type type] [--receiver-version version]
           [--antenna-number number] [--antenna-type type]
           [--program name] [--run-by name]
           [--phase-threshold n] [--slip-threshold n]
           input...`

type inputFormat string

const (
	inputRaw     inputFormat = "raw"
	inputUBX     inputFormat = "ubx"
	inputRINEX   inputFormat = "rinex"
	inputObsJSON inputFormat = "obsj"
)

type outputFormat string

const (
	outputRINEX   outputFormat = "rinex"
	outputObsJSON outputFormat = "obsj"
)

type flagVars struct {
	inputPaths   []string
	outputPath   string
	metaPath     string
	packetLog    bool
	inputFormat  inputFormat
	outputFormat outputFormat
	interval     time.Duration
	meta         rinex.Metadata
	wopts        rinex.WriterOptions
	uopts        rinexubx.Options
}

type inputReader struct {
	r   io.Reader
	err error
}

// Cmd implements the convobs subcommand.
func Cmd(_ io.Writer, _ slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
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
	var out io.Writer = os.Stdout
	if v.outputPath != "" && v.outputPath != "-" {
		f, err := os.Create(v.outputPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		out = f
	}
	return "", runInputs(openInputs(v.inputPaths), out, v.inputFormat, v.outputFormat, v.packetLog, v.meta, v.wopts, v.uopts, v.interval)
}

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	v := &flagVars{inputFormat: inputRaw, outputFormat: outputRINEX}
	if cmdName == "" {
		cmdName = "convobs"
	}
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	help := false
	from := string(v.inputFormat)
	to := string(v.outputFormat)
	interval := 0.0
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVarP(&v.outputPath, "output", "o", "", "output observation file, or - for stdout")
	flags.StringVarP(&from, "from", "r", from, "input observation format")
	flags.BoolVar(&v.packetLog, "packet-log", false, "input is a JSONL packet log")
	flags.StringVar(&to, "to", to, "output observation format")
	flags.Float64Var(&interval, "interval", 0, "observation decimation interval in `seconds` (0 disables)")
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
	flags.StringVar(&v.wopts.Program, "program", "convobs", "RINEX program field")
	flags.StringVar(&v.wopts.RunBy, "run-by", "", "RINEX run-by field")
	flags.Uint8Var(&v.uopts.PhaseThreshold, "phase-threshold", 0, "RAWX cpStdev index at which carrier phase is omitted, or 0 for RTKLIB Explorer auto")
	flags.Uint8Var(&v.uopts.SlipThreshold, "slip-threshold", 15, "RAWX cpStdev index that marks a cycle slip")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	if err := flags.Parse(args); err != nil {
		return nil, usageFunc, err
	}
	if help {
		return nil, usageFunc, nil
	}
	if err := v.setInputFormat(from); err != nil {
		return nil, usageFunc, err
	}
	if err := v.setOutputFormat(to); err != nil {
		return nil, usageFunc, err
	}
	if v.packetLog && !v.inputFormat.packetInput() {
		return nil, usageFunc, errors.New("--packet-log is valid only with packet input formats")
	}
	if math.IsNaN(interval) || math.IsInf(interval, 0) || interval < 0 {
		return nil, usageFunc, errors.New("--interval must be a finite non-negative number of seconds")
	}
	v.interval = time.Duration(math.Round(interval * float64(time.Second)))
	if interval > 0 {
		if err := rinex.ValidateDecimationInterval(v.interval); err != nil {
			return nil, usageFunc, err
		}
	}
	if flags.NArg() == 0 {
		return nil, usageFunc, errors.New("expected at least one input file")
	}
	v.inputPaths = flags.Args()
	if v.inputFormat.packetInput() {
		v.meta.Comments = append(v.meta.Comments, "format: u-blox UBX")
		v.meta.Comments = append(v.meta.Comments, "options: -MULTICODE")
		for _, path := range v.inputPaths {
			if path == "-" {
				v.meta.Comments = append(v.meta.Comments, "log: stdin")
			} else {
				v.meta.Comments = append(v.meta.Comments, "log: "+path)
			}
		}
	}
	return v, usageFunc, nil
}

func openInputs(paths []string) iter.Seq2[string, inputReader] {
	return func(yield func(string, inputReader) bool) {
		for _, path := range paths {
			if path == "-" {
				if !yield(path, inputReader{r: os.Stdin}) {
					return
				}
				continue
			}
			f, err := os.Open(path)
			if err != nil {
				yield(path, inputReader{err: err})
				return
			}
			if !yield(path, inputReader{r: f}) {
				f.Close()
				return
			}
			f.Close()
		}
	}
}

func (v *flagVars) setInputFormat(s string) error {
	s = strings.ToLower(s)
	switch inputFormat(s) {
	case inputRaw, inputUBX, inputRINEX, inputObsJSON:
		v.inputFormat = inputFormat(s)
		return nil
	default:
		return fmt.Errorf("unsupported input format %q", s)
	}
}

func (v *flagVars) setOutputFormat(s string) error {
	s = strings.ToLower(s)
	switch outputFormat(s) {
	case outputRINEX, outputObsJSON:
		v.outputFormat = outputFormat(s)
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", s)
	}
}

func (f inputFormat) packetInput() bool {
	return f == inputRaw || f == inputUBX
}

func runInputs(inputs iter.Seq2[string, inputReader], out io.Writer, from inputFormat, to outputFormat, packetLog bool, meta rinex.Metadata, wopts rinex.WriterOptions, uopts rinexubx.Options, interval time.Duration) error {
	switch from {
	case inputRINEX, inputObsJSON:
		return convertObservationInputs(inputs, out, from, to, meta, wopts, interval)
	}
	sink, err := outputSink(out, to, wopts, interval)
	if err != nil {
		return err
	}
	if err := sink.Metadata(meta); err != nil {
		return err
	}
	conv := rinexubx.New(sink, uopts)
	n := 0
	for path, in := range inputs {
		if in.err != nil {
			return inputError(path, in.err)
		}
		var c int
		if packetLog {
			c, err = convertPacketLog(in.r, conv)
		} else {
			c, err = convertPacketStream(in.r, conv)
		}
		if err != nil {
			return inputError(path, err)
		}
		n += c
	}
	if n == 0 {
		return errors.New("no UBX-RXM-RAWX messages found")
	}
	return sink.Flush()
}

func inputError(path string, err error) error {
	if path == "" {
		return err
	}
	if path == "-" {
		return fmt.Errorf("stdin: %w", err)
	}
	return fmt.Errorf("%s: %w", path, err)
}

func outputSink(w io.Writer, format outputFormat, wopts rinex.WriterOptions, interval time.Duration) (rinex.Sink, error) {
	var sink rinex.Sink
	switch format {
	case outputRINEX:
		sink = rinex.NewObservationSink(w, wopts)
	case outputObsJSON:
		sink = rinex.NewObsJSONSink(w)
	default:
		return nil, fmt.Errorf("unsupported output format %q", format)
	}
	if interval == 0 {
		return sink, nil
	}
	return rinex.NewDecimationSink(sink, interval)
}

func convertObservationInputs(inputs iter.Seq2[string, inputReader], out io.Writer, from inputFormat, to outputFormat, meta rinex.Metadata, wopts rinex.WriterOptions, interval time.Duration) error {
	var fileMeta rinex.Metadata
	var obs []rinex.SignalObservation
	for path, in := range inputs {
		if in.err != nil {
			return inputError(path, in.err)
		}
		m, o, err := readObservationInput(in.r, from)
		if err != nil {
			return inputError(path, err)
		}
		fileMeta = rinex.MergeMetadata(fileMeta, m)
		obs = append(obs, o...)
	}
	return convertBufferedObservations(out, to, rinex.MergeMetadata(fileMeta, meta), obs, wopts, interval)
}

func readObservationInput(in io.Reader, from inputFormat) (rinex.Metadata, []rinex.SignalObservation, error) {
	switch from {
	case inputRINEX:
		return rinex.ReadObservationFile(in)
	case inputObsJSON:
		return rinex.ReadObsJSON(in)
	default:
		return rinex.Metadata{}, nil, fmt.Errorf("unsupported input format %q", from)
	}
}

func convertBufferedObservations(out io.Writer, to outputFormat, meta rinex.Metadata, obs []rinex.SignalObservation, wopts rinex.WriterOptions, interval time.Duration) error {
	sink, err := outputSink(out, to, wopts, interval)
	if err != nil {
		return err
	}
	if err := sink.Metadata(meta); err != nil {
		return err
	}
	for _, o := range obs {
		if err := sink.Observation(o); err != nil {
			return err
		}
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

func convertPacketStream(r io.Reader, conv *rinexubx.Converter) (int, error) {
	s := scan.New(r, scanBufSize, gpsreg.CreatePacketFormats(gpsreg.VendorUblox))
	n := 0
	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return n, err
		}
		ok, err := convertPacket(pkt, conv)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

func convertPacketLog(r io.Reader, conv *rinexubx.Converter) (int, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1024), 1024*1024)
	n := 0
	line := 0
	for s.Scan() {
		line++
		var entry gpsio.PacketLogEntry
		if err := json.Unmarshal(s.Bytes(), &entry); err != nil {
			return n, fmt.Errorf("packet log line %d: %w", line, err)
		}
		if entry.Out || entry.Data() == "" {
			continue
		}
		ok, err := convertPacketData(entry.Data(), conv)
		if err != nil {
			return n, fmt.Errorf("packet log line %d: %w", line, err)
		}
		if ok {
			n++
		}
	}
	if err := s.Err(); err != nil {
		return n, err
	}
	return n, nil
}

func convertPacketData(data string, conv *rinexubx.Converter) (bool, error) {
	s := scan.New(strings.NewReader(data), scanBufSize, gpsreg.CreatePacketFormats(gpsreg.VendorUblox))
	n := 0
	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
		ok, err := convertPacket(pkt, conv)
		if err != nil {
			return false, err
		}
		if ok {
			n++
		}
	}
	return n != 0, nil
}

func convertPacket(pkt scan.Packet, conv *rinexubx.Converter) (bool, error) {
	if pkt.ReadError != nil {
		return false, pkt.ReadError
	}
	if pkt.Format == nil {
		if len(pkt.Data) != 0 {
			return false, fmt.Errorf("invalid input data before packet: %d bytes", len(pkt.Data))
		}
		return false, nil
	}
	if !pkt.HasTag(gpsreg.TagUBX) {
		return false, nil
	}
	if !pkt.ChecksumValid {
		return false, pkt.ChecksumError()
	}
	if ubxbin.PacketMsgId(pkt.Data) != ubxbin.RxmRawxID {
		return false, nil
	}
	msg, err := ubxbin.ParseMsg(pkt.Data)
	if err != nil {
		return false, err
	}
	rawx, ok := msg.(*ubxbin.RxmRawx)
	if !ok {
		return false, nil
	}
	return true, conv.ConvertRAWX(rawx)
}
