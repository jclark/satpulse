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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	rnxrtcm "github.com/jclark/satpulse/gps/lib/rinex/rtcm"
	rnxubx "github.com/jclark/satpulse/gps/lib/rinex/ubx"
	rnxunc "github.com/jclark/satpulse/gps/lib/rinex/unc"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const scanBufSize = 64 * 1024
const gpsWeek = 7 * 24 * time.Hour
const packetLogWeekSlack = time.Minute
const uncaObsVMMsg = "OBSVMA"

var packetStreamFormats = gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)
var packetLogFormats = gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)
var packetLogFormatsByTag = packetFormatsByTag(packetLogFormats)

const summary = `[-h|--help] [-o|--output path] [--metadata path]
           [-r|--from raw|ubx|rtcm|uncb|unca|rinex|obsj] [--packet-log] [--to rinex|obsj]
           [--date YYYYMMDD|--recent|-f|--date-from-filename]
           [--interval seconds]
           [--marker-name name] [--marker-number number] [--marker-type type]
           [--observer name] [--agency name]
           [--receiver-number number] [--receiver-type type] [--receiver-version version]
           [--antenna-number number] [--antenna-type type]
           [--program name] [--run-by name]
           [--rtcm-strict-prr]
           [--ubx-phase-threshold n] [--ubx-slip-threshold n]
           input...`

type inputFormat string

const (
	inputRaw     inputFormat = "raw"
	inputUBX     inputFormat = "ubx"
	inputRTCM    inputFormat = "rtcm"
	inputUNCB    inputFormat = "uncb"
	inputUNCA    inputFormat = "unca"
	inputRINEX   inputFormat = "rinex"
	inputObsJSON inputFormat = "obsj"
)

var inputPacketTags = map[inputFormat]gpsprot.Tag{
	inputUBX:  gpsreg.TagUBX,
	inputRTCM: gpsreg.TagRTCM,
	inputUNCB: gpsreg.TagUnicoreBin,
	inputUNCA: gpsreg.TagUnicoreAscii,
}

var inputMetadataByTag = map[gpsprot.Tag]rinex.Metadata{
	gpsreg.TagUBX:          {Comments: []string{"format: u-blox UBX", "options: -MULTICODE"}},
	gpsreg.TagRTCM:         {Comments: []string{"format: RTCM"}},
	gpsreg.TagUnicoreBin:   {Comments: []string{"format: Unicore UNCB"}},
	gpsreg.TagUnicoreAscii: {Comments: []string{"format: Unicore UNCA"}},
}

var rawObservationNames = map[gpsprot.Tag]string{
	gpsreg.TagUBX:          "UBX RAWX",
	gpsreg.TagRTCM:         "RTCM MSM7",
	gpsreg.TagUnicoreBin:   "UNCB OBSVM",
	gpsreg.TagUnicoreAscii: "UNCA OBSVMA",
}

var noPacketObservationMsgs = map[inputFormat]string{
	inputUBX:  "no UBX-RXM-RAWX messages found",
	inputRTCM: "no RTCM MSM7 messages found",
	inputUNCB: "no Unicore UNCB OBSVM messages found",
	inputUNCA: "no Unicore UNCA OBSVMA messages found",
	inputRaw:  "no raw observation packets found (UBX RAWX, RTCM MSM7, UNCB OBSVM, UNCA OBSVMA)",
}

type outputFormat string

const (
	outputRINEX   outputFormat = "rinex"
	outputObsJSON outputFormat = "obsj"
)

type flagVars struct {
	convertOptions
	inputPaths []string
	outputPath string
	metaPath   string
}

type convertOptions struct {
	from      inputFormat
	to        outputFormat
	packetLog bool
	meta      rinex.Metadata
	write     rinex.WriterOptions
	format    formatOptions
	interval  time.Duration
	week      weekOptions
}

type formatOptions struct {
	rtcm rnxrtcm.Options
	ubx  rnxubx.Options
}

type inputReader struct {
	r          io.Reader
	err        error
	modTime    time.Time
	hasModTime bool
}

type weekMode int

const (
	weekAuto weekMode = iota
	weekRecent
	weekDate
	weekFilename
)

type weekOptions struct {
	mode weekMode
	date time.Time
}

type convJob struct {
	inputs iter.Seq2[string, inputReader]
	out    io.Writer
	opts   convertOptions
}

// WeekConstraint carries an RTCM week interval and its user-facing diagnostics.
type WeekConstraint struct {
	Interval rnxrtcm.TimeInterval
	Errf     string
	WarnMsg  string
	WarnArgs []any

	err  error
	warn *weekWarning
}

type weekWarning struct {
	emitted bool
}

type packetInput interface {
	ConvertPacket(scan.Packet, WeekConstraint) (bool, error)
}

type tagInput struct {
	tag     gpsprot.Tag
	convert func(string, WeekConstraint) (bool, error)
	accept  func(scan.Packet, string) bool
}

type metadataBufferSink struct {
	dst  rinex.Sink
	buf  []rinex.Metadata
	pass bool
}

var _ rinex.Sink = (*metadataBufferSink)(nil)

type rawPacketInput struct {
	sink    rinex.Sink
	lg      *slog.Logger
	family  gpsprot.Tag
	warned  bool
	week    WeekConstraint
	meta    rinex.Metadata
	metaBuf *metadataBufferSink
	rtcm    *rnxrtcm.Converter
	ubx     *rnxubx.Converter
	unc     *rnxunc.Converter
}

var _ packetInput = (*rawPacketInput)(nil)

// Cmd implements the convobs subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	v, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if v == nil {
		return usageFunc(progName), nil
	}
	now := time.Now().UTC()
	if v.write.Date.IsZero() {
		v.write.Date = now
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
	cj := convJob{
		inputs: openInputs(v.inputPaths),
		out:    out,
		opts:   v.convertOptions,
	}
	return "", cj.run(lg, now)
}

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	v := &flagVars{convertOptions: convertOptions{from: inputRaw, to: outputRINEX}}
	if cmdName == "" {
		cmdName = "convobs"
	}
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	help := false
	from := string(v.from)
	to := string(v.to)
	interval := 0.0
	date := ""
	recent := false
	dateFromFilename := false
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVarP(&v.outputPath, "output", "o", "", "output observation file, or - for stdout")
	flags.StringVarP(&from, "from", "r", from, "input observation format")
	flags.BoolVar(&v.packetLog, "packet-log", false, "input is a JSONL packet log")
	flags.StringVar(&to, "to", to, "output observation format")
	flags.StringVar(&date, "date", "", "RTCM observation civil date as `YYYYMMDD`")
	flags.BoolVar(&recent, "recent", false, "infer RTCM observations are within the last week")
	flags.BoolVarP(&dateFromFilename, "date-from-filename", "f", false, "infer RTCM date from input filename")
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
	flags.StringVar(&v.write.Program, "program", "convobs", "RINEX program field")
	flags.StringVar(&v.write.RunBy, "run-by", "", "RINEX run-by field")
	flags.BoolVar(&v.format.rtcm.UseSpecPhaseRangeRateSign, "rtcm-strict-prr", false, "interpret sign of PhaseRangeRate in strict conformance with RTCM3 standard (Doppler = -PRR/wavelength)")
	flags.Uint8Var(&v.format.ubx.PhaseThreshold, "ubx-phase-threshold", 0, "RAWX cpStdev index at which carrier phase is omitted, or 0 for RTKLIB Explorer auto")
	flags.Uint8Var(&v.format.ubx.SlipThreshold, "ubx-slip-threshold", 15, "RAWX cpStdev index that marks a cycle slip")
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
	if v.packetLog && !v.from.packetInput() {
		return nil, usageFunc, errors.New("--packet-log is valid only with packet input formats")
	}
	if err := v.setWeekOptions(date, recent, dateFromFilename); err != nil {
		return nil, usageFunc, err
	}
	if v.packetLog && v.week.mode != weekAuto {
		return nil, usageFunc, errors.New("--date, --recent, and --date-from-filename are not valid with --packet-log")
	}
	if v.week.mode != weekAuto && !v.from.mayUseRTCM() {
		return nil, usageFunc, errors.New("--date, --recent, and --date-from-filename are valid only with raw or RTCM input")
	}
	if v.format.rtcm.UseSpecPhaseRangeRateSign && !v.from.mayUseRTCM() {
		return nil, usageFunc, errors.New("--rtcm-strict-prr is valid only with raw or RTCM input")
	}
	if flags.Changed("ubx-phase-threshold") && !v.from.mayUseUBX() {
		return nil, usageFunc, errors.New("--ubx-phase-threshold is valid only with raw or UBX input")
	}
	if flags.Changed("ubx-slip-threshold") && !v.from.mayUseUBX() {
		return nil, usageFunc, errors.New("--ubx-slip-threshold is valid only with raw or UBX input")
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
	if v.week.mode == weekFilename && slices.Contains(v.inputPaths, "-") {
		return nil, usageFunc, errors.New("--date-from-filename is not valid with stdin")
	}
	if v.from.packetInput() {
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
			info, err := f.Stat()
			if err != nil {
				f.Close()
				yield(path, inputReader{err: err})
				return
			}
			if !yield(path, inputReader{r: f, modTime: info.ModTime(), hasModTime: true}) {
				f.Close()
				return
			}
			f.Close()
		}
	}
}

func packetFormatsByTag(fmts []gpsprot.PacketFormat) map[gpsprot.Tag][]gpsprot.PacketFormat {
	m := make(map[gpsprot.Tag][]gpsprot.PacketFormat)
	for _, f := range fmts {
		m[f.Tag()] = append(m[f.Tag()], f)
	}
	return m
}

func (v *flagVars) setInputFormat(s string) error {
	s = strings.ToLower(s)
	switch inputFormat(s) {
	case inputRaw, inputUBX, inputRTCM, inputUNCB, inputUNCA, inputRINEX, inputObsJSON:
		v.from = inputFormat(s)
		return nil
	default:
		return fmt.Errorf("unsupported input format %q", s)
	}
}

func (v *flagVars) setOutputFormat(s string) error {
	s = strings.ToLower(s)
	switch outputFormat(s) {
	case outputRINEX, outputObsJSON:
		v.to = outputFormat(s)
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", s)
	}
}

func (f inputFormat) packetInput() bool {
	return f == inputRaw || inputPacketTags[f] != gpsprot.EmptyTag
}

func (f inputFormat) mayUseRTCM() bool {
	return f == inputRaw || f == inputRTCM
}

func (f inputFormat) mayUseUBX() bool {
	return f == inputRaw || f == inputUBX
}

func (v *flagVars) setWeekOptions(date string, recent, dateFromFilename bool) error {
	n := 0
	if date != "" {
		n++
	}
	if recent {
		n++
	}
	if dateFromFilename {
		n++
	}
	if n > 1 {
		return errors.New("--date, --recent, and --date-from-filename are mutually exclusive")
	}
	switch {
	case date != "":
		t, err := parseYYYYMMDD(date)
		if err != nil {
			return err
		}
		v.week = weekOptions{mode: weekDate, date: t}
	case recent:
		v.week = weekOptions{mode: weekRecent}
	case dateFromFilename:
		v.week = weekOptions{mode: weekFilename}
	}
	return nil
}

func (cj convJob) run(lg *slog.Logger, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	opts := cj.opts
	switch opts.from {
	case inputRINEX, inputObsJSON:
		return convertObservationInputs(cj.inputs, cj.out, opts.from, opts.to, opts.meta, opts.write, opts.interval)
	}
	sink, err := outputSink(cj.out, opts.to, opts.write, opts.interval)
	if err != nil {
		return err
	}
	conv, err := newPacketInput(opts.from, sink, opts.meta, opts.format, lg)
	if err != nil {
		return err
	}
	n := 0
	i := 0
	for path, in := range cj.inputs {
		if in.err != nil {
			return inputError(path, in.err)
		}
		var week WeekConstraint
		if !opts.packetLog {
			week, err = fileWeekConstraint(path, in, opts.week, now, opts.from == inputRTCM, i)
			if err != nil {
				return inputError(path, err)
			}
		}
		var c int
		if opts.packetLog {
			c, err = convertPacketLog(in.r, opts.from, conv)
		} else {
			c, err = convertPacketStream(in.r, conv, week)
		}
		if err != nil {
			return inputError(path, err)
		}
		n += c
		i++
	}
	if n == 0 {
		return errors.New(noPacketObservationMsgs[opts.from])
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

func newPacketInput(from inputFormat, sink rinex.Sink, meta rinex.Metadata, format formatOptions, lg *slog.Logger) (packetInput, error) {
	if tag := inputPacketTags[from]; tag != gpsprot.EmptyTag {
		if err := sink.Metadata(inputMetadataByTag[tag]); err != nil {
			return nil, err
		}
		if err := sink.Metadata(meta); err != nil {
			return nil, err
		}
	}
	switch from {
	case inputUBX:
		conv := rnxubx.New(sink, format.ubx)
		return &tagInput{tag: gpsreg.TagUBX, convert: func(data string, _ WeekConstraint) (bool, error) {
			return convertUBXData(data, conv)
		}}, nil
	case inputRTCM:
		conv := rnxrtcm.New(sink, format.rtcm)
		return &tagInput{tag: gpsreg.TagRTCM, convert: func(data string, week WeekConstraint) (bool, error) {
			return convertRTCMData(data, conv, week)
		}}, nil
	case inputUNCB:
		conv := rnxunc.New(sink, rnxunc.Options{})
		return &tagInput{tag: gpsreg.TagUnicoreBin, accept: isRawObsTag, convert: func(data string, _ WeekConstraint) (bool, error) {
			return convertObsVMData(data, conv, uncmsg.ParseBinMsg)
		}}, nil
	case inputUNCA:
		conv := rnxunc.New(sink, rnxunc.Options{})
		return &tagInput{tag: gpsreg.TagUnicoreAscii, accept: isRawObsTag, convert: func(data string, _ WeekConstraint) (bool, error) {
			return convertObsVMData(data, conv, uncmsg.ParseAsciiMessage)
		}}, nil
	default:
		return newRawPacketInput(sink, meta, format, lg), nil
	}
}

func newRawPacketInput(sink rinex.Sink, meta rinex.Metadata, format formatOptions, lg *slog.Logger) *rawPacketInput {
	buf := &metadataBufferSink{dst: sink}
	return &rawPacketInput{
		sink:    sink,
		lg:      lg,
		meta:    meta,
		metaBuf: buf,
		rtcm:    rnxrtcm.New(buf, format.rtcm),
		ubx:     rnxubx.New(sink, format.ubx),
		unc:     rnxunc.New(sink, rnxunc.Options{}),
	}
}

func (s *metadataBufferSink) Metadata(m rinex.Metadata) error {
	if s.pass {
		return s.dst.Metadata(m)
	}
	s.buf = append(s.buf, m)
	return nil
}

func (s *metadataBufferSink) Observation(obs rinex.SignalObservation) error {
	return s.dst.Observation(obs)
}

func (s *metadataBufferSink) Flush() error {
	return s.dst.Flush()
}

func (s *metadataBufferSink) Commit() error {
	for _, m := range s.buf {
		if err := s.dst.Metadata(m); err != nil {
			return err
		}
	}
	s.buf = nil
	s.pass = true
	return nil
}

func (s *metadataBufferSink) Drop() {
	s.buf = nil
	s.pass = true
}

func convertPacketStream(r io.Reader, in packetInput, week WeekConstraint) (int, error) {
	s := scan.New(r, scanBufSize, packetStreamFormats)
	n := 0
	weekUsed := false
	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return n, err
		}
		var wc WeekConstraint
		if pkt.HasTag(gpsreg.TagRTCM) {
			if !weekUsed {
				wc = week
				weekUsed = true
			}
		}
		ok, err := in.ConvertPacket(pkt, wc)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

func convertPacketLog(r io.Reader, from inputFormat, in packetInput) (int, error) {
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
		if from != inputRaw && entry.Tag != inputPacketTags[from] {
			continue
		}
		if from == inputRaw {
			if _, ok := inputMetadataByTag[entry.Tag]; !ok {
				continue
			}
		}
		ok, err := convertPacketData(entry.Data(), packetLogFormatsByTag[entry.Tag], in, packetLogWeekConstraint(entry, line))
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

func convertPacketData(data string, fmts []gpsprot.PacketFormat, in packetInput, week WeekConstraint) (bool, error) {
	s := scan.New(strings.NewReader(data), scanBufSize, fmts)
	seen := false
	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return seen, err
		}
		var wc WeekConstraint
		if pkt.HasTag(gpsreg.TagRTCM) {
			wc = week
		}
		ok, err := in.ConvertPacket(pkt, wc)
		if err != nil {
			return seen, err
		}
		if ok {
			seen = true
		}
	}
	return seen, nil
}

func packetData(pkt scan.Packet) (string, bool, error) {
	if pkt.ReadError != nil {
		return "", false, pkt.ReadError
	}
	if pkt.Format == nil {
		return "", false, nil
	}
	if !pkt.ChecksumValid {
		return "", false, pkt.ChecksumError()
	}
	return pkt.Data, true, nil
}

func (in *tagInput) ConvertPacket(pkt scan.Packet, week WeekConstraint) (bool, error) {
	data, ok, err := packetData(pkt)
	if !ok || err != nil || !pkt.HasTag(in.tag) {
		return false, err
	}
	if in.accept != nil && !in.accept(pkt, data) {
		return false, nil
	}
	return in.convert(data, week)
}

func (in *rawPacketInput) ConvertPacket(pkt scan.Packet, week WeekConstraint) (bool, error) {
	data, ok, err := packetData(pkt)
	if !ok || err != nil {
		return false, err
	}
	if pkt.HasTag(gpsreg.TagRTCM) {
		return in.convertRTCM(data, week)
	}
	tag, ok := isRawObs(pkt, data)
	if !ok {
		return false, nil
	}
	return in.convertNonRTCMObservation(tag, data)
}

func isRawObs(pkt scan.Packet, data string) (gpsprot.Tag, bool) {
	switch tag := pkt.Tag(); tag {
	case gpsreg.TagUBX:
		return tag, ubxbin.PacketMsgId(data) == ubxbin.RxmRawxID
	case gpsreg.TagUnicoreBin:
		return tag, uncmsg.BinPacketMsgID([]byte(data)) == uncmsg.ObsvMID
	case gpsreg.TagUnicoreAscii:
		return tag, pkt.Format.MsgID([]byte(data)) == uncaObsVMMsg
	default:
		return gpsprot.EmptyTag, false
	}
}

func isRawObsTag(pkt scan.Packet, data string) bool {
	_, ok := isRawObs(pkt, data)
	return ok
}

func (in *rawPacketInput) convertNonRTCMObservation(tag gpsprot.Tag, data string) (bool, error) {
	if in.family == gpsprot.EmptyTag {
		in.metaBuf.Drop()
		if err := in.sink.Metadata(inputMetadataByTag[tag]); err != nil {
			return false, err
		}
		if err := in.sink.Metadata(in.meta); err != nil {
			return false, err
		}
		in.rtcm = nil
		in.family = tag
	}
	if in.family != tag {
		if !in.warned {
			warnMixedRawObservation(in.lg, tag, in.family)
			in.warned = true
		}
		return false, nil
	}
	switch tag {
	case gpsreg.TagUBX:
		return convertUBXData(data, in.ubx)
	case gpsreg.TagUnicoreBin:
		return convertObsVMData(data, in.unc, uncmsg.ParseBinMsg)
	case gpsreg.TagUnicoreAscii:
		return convertObsVMData(data, in.unc, uncmsg.ParseAsciiMessage)
	}
	return false, nil
}

func (in *rawPacketInput) convertRTCM(data string, week WeekConstraint) (bool, error) {
	msm7 := isRTCMMSM7Packet(data)
	if in.family != gpsprot.EmptyTag && in.family != gpsreg.TagRTCM {
		if msm7 {
			if !in.warned {
				warnMixedRawObservation(in.lg, gpsreg.TagRTCM, in.family)
				in.warned = true
			}
		}
		return false, nil
	}
	if in.family == gpsprot.EmptyTag {
		in.noteWeek(week)
		if !msm7 {
			return convertRTCMData(data, in.rtcm, quietWeekConstraint(week))
		}
		if in.week.err != nil {
			return false, in.week.err
		}
		if err := in.sink.Metadata(inputMetadataByTag[gpsreg.TagRTCM]); err != nil {
			return false, err
		}
		if err := in.sink.Metadata(in.meta); err != nil {
			return false, err
		}
		if err := in.metaBuf.Commit(); err != nil {
			return false, err
		}
		in.family = gpsreg.TagRTCM
	}
	ok, err := convertRTCMData(data, in.rtcm, week)
	if err == nil && ok && week.Interval.IsZero() {
		in.week.warnOnce()
	}
	return ok, err
}

func warnMixedRawObservation(lg *slog.Logger, got, selected gpsprot.Tag) {
	gotName, selectedName := rawObservationNames[got], rawObservationNames[selected]
	if gotName == "" {
		gotName = "unknown"
	}
	if selectedName == "" {
		selectedName = "unknown"
	}
	lg.Warn("ignoring mixed raw observation input", "got", gotName, "selected", selectedName)
}

func convertUBXData(data string, conv *rnxubx.Converter) (bool, error) {
	if ubxbin.PacketMsgId(data) != ubxbin.RxmRawxID {
		return false, nil
	}
	msg, err := ubxbin.ParseMsg(data)
	if err != nil {
		return false, err
	}
	rawx, ok := msg.(*ubxbin.RxmRawx)
	if !ok {
		return false, nil
	}
	return true, conv.ConvertRAWX(rawx)
}

func convertObsVMData(data string, conv *rnxunc.Converter, parse func([]byte) (*uncmsg.Msg, error)) (bool, error) {
	msg, err := parse([]byte(data))
	if err != nil {
		return false, err
	}
	obs, ok := msg.Body.(*uncmsg.ObsVM)
	if !ok {
		return false, nil
	}
	return conv.ConvertObsVM(&msg.Hdr, obs)
}

func (in *rawPacketInput) noteWeek(week WeekConstraint) {
	if !week.Interval.IsZero() || week.err != nil || week.WarnMsg != "" {
		in.week = week
	}
}

func quietWeekConstraint(week WeekConstraint) WeekConstraint {
	week.err = nil
	week.WarnMsg = ""
	week.WarnArgs = nil
	week.warn = nil
	return week
}

func convertRTCMData(data string, conv *rnxrtcm.Converter, week WeekConstraint) (bool, error) {
	if week.err != nil {
		return false, week.err
	}
	msg, err := rtcmbin.ParseMsg(data)
	if err != nil {
		return false, err
	}
	ok, err := conv.ConvertMsg(msg, week.Interval)
	if err != nil {
		return false, week.wrap(err)
	}
	if ok {
		week.warnOnce()
	}
	return isRTCMMSM7Packet(data), nil
}

func isRTCMMSM7Packet(data string) bool {
	mt := rtcmbin.ExtractMsgType(data)
	return mt.IsMSM() && mt%10 == 7
}

func fileWeekConstraint(path string, in inputReader, week weekOptions, now time.Time, strict bool, i int) (WeekConstraint, error) {
	wc, err := newFileWeekConstraint(path, in, week, now, i)
	if err != nil {
		return WeekConstraint{}, err
	}
	if strict && wc.err != nil {
		return WeekConstraint{}, wc.err
	}
	return wc, nil
}

func newFileWeekConstraint(path string, in inputReader, week weekOptions, now time.Time, i int) (WeekConstraint, error) {
	switch week.mode {
	case weekRecent:
		return recentWeekConstraint(now, ""), nil
	case weekDate:
		return dateWeekConstraint(week.date, "--date"), nil
	case weekFilename:
		t, err := dateFromFilename(path)
		if err != nil {
			return WeekConstraint{}, err
		}
		return dateWeekConstraint(t, "--date-from-filename"), nil
	default:
		return automaticWeekConstraint(path, in, now, i), nil
	}
}

func recentWeekConstraint(now time.Time, warnPath string) WeekConstraint {
	wc := WeekConstraint{
		Interval: rnxrtcm.TimeInterval{Start: now.Add(-gpsWeek), Duration: gpsWeek},
		Errf:     "RTCM epoch does not match recent week inference: %w",
	}
	if warnPath != "" {
		wc.WarnMsg = "assuming undated RTCM input contains observations from the last week"
		wc.WarnArgs = []any{"input", warnPath}
		wc.warn = &weekWarning{}
	}
	return wc
}

func dateWeekConstraint(date time.Time, opt string) WeekConstraint {
	return WeekConstraint{
		Interval: rnxrtcm.TimeInterval{
			Start:    date.Add(-14 * time.Hour),
			Duration: 50 * time.Hour,
		},
		Errf: fmt.Sprintf("RTCM epoch does not match %s: %%w", opt),
	}
}

func automaticWeekConstraint(path string, in inputReader, now time.Time, i int) WeekConstraint {
	wc := recentWeekConstraint(now, inputName(path, i))
	start := wc.Interval.Start
	if in.hasModTime && in.modTime.Before(start) {
		wc.err = fmt.Errorf("RTCM input is older than one week; provide --date, --recent, or --date-from-filename")
	}
	return wc
}

func packetLogWeekConstraint(entry gpsio.PacketLogEntry, line int) WeekConstraint {
	t := time.Time(entry.T)
	if t.IsZero() {
		return WeekConstraint{err: fmt.Errorf("RTCM packet log line %d has no timestamp", line)}
	}
	return WeekConstraint{
		Interval: rnxrtcm.TimeInterval{
			Start:    t.UTC().Add(-packetLogWeekSlack),
			Duration: 2 * packetLogWeekSlack,
		},
		Errf: "RTCM epoch does not match packet-log timestamp: %w",
	}
}

func (wc WeekConstraint) wrap(err error) error {
	if wc.Errf == "" {
		return err
	}
	return fmt.Errorf(wc.Errf, err)
}

func (wc WeekConstraint) warnOnce() {
	if wc.WarnMsg == "" || wc.warn == nil || wc.warn.emitted {
		return
	}
	slog.Warn(wc.WarnMsg, wc.WarnArgs...)
	wc.warn.emitted = true
}

func inputName(path string, i int) string {
	if path == "-" {
		return "stdin"
	}
	if path != "" {
		return path
	}
	return fmt.Sprintf("input%d", i)
}

func parseYYYYMMDD(s string) (time.Time, error) {
	if len(s) != 8 {
		return time.Time{}, fmt.Errorf("--date must be in YYYYMMDD format")
	}
	t, err := time.Parse("20060102", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("--date must be a valid YYYYMMDD date")
	}
	return t, nil
}

func dateFromFilename(path string) (time.Time, error) {
	dates := make(map[string]time.Time)
	base := filepath.Base(path)
	for _, s := range digitRuns(base) {
		if t, ok := parseFilenameDateRunPrefix(s); ok {
			dates[t.Format("20060102")] = t
		}
	}
	if len(dates) == 0 {
		return time.Time{}, fmt.Errorf("no date found in filename %q", path)
	}
	if len(dates) > 1 {
		return time.Time{}, fmt.Errorf("multiple conflicting dates found in filename %q", path)
	}
	for _, t := range dates {
		return t, nil
	}
	return time.Time{}, nil
}

func digitRuns(s string) []string {
	var runs []string
	for i := 0; i < len(s); {
		if !isDigit(s[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		runs = append(runs, s[i:j])
		i = j
	}
	return runs
}

func parseFilenameDateRunPrefix(s string) (time.Time, bool) {
	if len(s) < 8 {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102", s[:8])
	return t, err == nil
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
