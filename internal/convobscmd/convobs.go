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
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/rnxrtcm"
	"github.com/jclark/satpulse/gps/lib/rnxubx"
	"github.com/jclark/satpulse/gps/lib/rnxunc"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
)

const scanBufSize = 64 * 1024
const gpsWeek = 7 * 24 * time.Hour
const packetLogWeekSlack = time.Minute

var packetStreamFormats = gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)
var packetLogFormats = gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)
var packetLogFormatsByTag = packetFormatsByTag(packetLogFormats)

const summary = `[-h|--help] [-o|--output path] [-H|--header-file path]
           [-r|--from raw|ubx|rtcm|uncb|unca|rinex|obsj] [--packet-log] [--to rinex|obsj]
           [--date YYYYMMDD|--recent|-f|--date-from-filename]
           [--interval seconds] [-p|--ppp-ar]
           [--rinex-version version] [--program name] [--run-by name]
           [--antenna type] [--approx-pos x,y,z] [--comment text]
           [--rtcm-strict-prr] [--rtcm-omit-zero-do]
           [--ubx-slip-threshold n] [--ubx-bds-geo-half-cycle]
           [--unc-omit-do-without-cp]
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

var rawObservationNames = map[gpsprot.Tag]string{
	gpsreg.TagUBX:          "UBX RAWX",
	gpsreg.TagRTCM:         "RTCM MSM7",
	gpsreg.TagUnicoreBin:   "UNCB OBSVM",
	gpsreg.TagUnicoreAscii: "UNCA OBSVM",
}

var noPacketObservationMsgs = map[inputFormat]string{
	inputUBX:  "no UBX-RXM-RAWX messages found",
	inputRTCM: "no RTCM MSM7 messages found",
	inputUNCB: "no Unicore UNCB OBSVM messages found",
	inputUNCA: "no Unicore UNCA OBSVM messages found",
	inputRaw:  "no raw observation packets found (UBX RAWX, RTCM MSM7, UNCB OBSVM, UNCA OBSVM)",
}

type outputFormat string

const (
	outputRINEX   outputFormat = "rinex"
	outputObsJSON outputFormat = "obsj"
)

type flagVars struct {
	convertOptions
	inputPaths    []string
	outputPath    string
	headerFile    string
	metadataFlags *pflag.FlagSet
}

type convertOptions struct {
	from      inputFormat
	to        outputFormat
	packetLog bool
	requireCP bool
	format    formatOptions
	interval  time.Duration
	week      weekOptions
}

type formatOptions struct {
	rtcm rnxrtcm.Options
	ubx  rnxubx.Options
	unc  rnxunc.Options
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
	meta   rinex.Metadata
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
	ConvertPacket(obsPacket, WeekConstraint) (bool, error)
}

type obsPacket struct {
	format gpsprot.PacketFormat
	data   []byte
}

func (pkt obsPacket) Tag() gpsprot.Tag {
	if pkt.format == nil {
		return gpsprot.EmptyTag
	}
	return pkt.format.Tag()
}

func (pkt obsPacket) HasTag(tag gpsprot.Tag) bool {
	return pkt.format != nil && pkt.format.Tag() == tag
}

type tagInput struct {
	tag     gpsprot.Tag
	convert func([]byte, WeekConstraint) (bool, error)
	accept  func(obsPacket) bool
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
	var meta rinex.Metadata
	if v.headerFile != "" {
		f, err := os.Open(v.headerFile)
		if err != nil {
			return "", err
		}
		defer f.Close()
		meta, err = readHeaderFile(v.headerFile, f)
		if err != nil {
			return "", err
		}
	}
	setMetadataDefaults(&meta, now, v.interval)
	if err := applyMetadataFlags(&meta, v.metadataFlags); err != nil {
		return "", err
	}
	var out io.Writer = os.Stdout
	if v.outputPath != "" {
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
		meta:   meta,
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
	flags.StringVarP(&v.outputPath, "output", "o", "", "output observation file (default stdout)")
	flags.StringVarP(&from, "from", "r", from, "input observation format")
	flags.BoolVar(&v.packetLog, "packet-log", false, "input is a JSONL packet log")
	flags.StringVar(&to, "to", to, "output observation format")
	flags.StringVar(&date, "date", "", "RTCM observation civil date as `YYYYMMDD`")
	flags.BoolVar(&recent, "recent", false, "infer RTCM observations are within the last week")
	flags.BoolVarP(&dateFromFilename, "date-from-filename", "f", false, "infer RTCM date from input filename")
	flags.Float64Var(&interval, "interval", 0, "observation decimation interval in `seconds` (0 disables)")
	flags.BoolVarP(&v.requireCP, "ppp-ar", "p", false, "produce output optimized for PPP-AR")
	flags.StringVarP(&v.headerFile, "header-file", "H", "", "TOML RINEX header metadata file")
	flags.String("rinex-version", "", "RINEX observation version")
	flags.String("program", "", "RINEX program field")
	flags.String("run-by", "", "RINEX run-by field")
	flags.String("antenna", "", "RINEX antenna type")
	flags.String("approx-pos", "", "RINEX approximate XYZ position as x,y,z")
	flags.StringArray("comment", nil, "RINEX comment line")
	flags.BoolVar(&v.format.rtcm.UseSpecPhaseRangeRateSign, "rtcm-strict-prr", false, "interpret sign of PhaseRangeRate in strict conformance with RTCM3 standard (Doppler = -PRR/wavelength)")
	flags.BoolVar(&v.format.rtcm.OmitZeroDo, "rtcm-omit-zero-do", false, "omit RTCM MSM Doppler observations with numeric value zero")
	flags.Uint8Var(&v.format.ubx.SlipThreshold, "ubx-slip-threshold", 15, "RAWX cpStdev index that marks a cycle slip")
	flags.BoolVar(&v.format.ubx.BDSGeoHalfCycle, "ubx-bds-geo-half-cycle", false, "apply RTKLIB-compatible BDS GEO half-cycle carrier phase correction")
	flags.BoolVar(&v.format.unc.OmitDoWithoutCP, "unc-omit-do-without-cp", false, "omit Unicore OBSVM Doppler observations whose signal has no valid carrier phase")
	v.metadataFlags = flags
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
	if v.format.rtcm.OmitZeroDo && !v.from.mayUseRTCM() {
		return nil, usageFunc, errors.New("--rtcm-omit-zero-do is valid only with raw or RTCM input")
	}
	if flags.Changed("ubx-slip-threshold") && !v.from.mayUseUBX() {
		return nil, usageFunc, errors.New("--ubx-slip-threshold is valid only with raw or UBX input")
	}
	if v.format.unc.OmitDoWithoutCP && !v.from.mayUseUnicore() {
		return nil, usageFunc, errors.New("--unc-omit-do-without-cp is valid only with raw, UNCB, or UNCA input")
	}
	if flags.Changed("ubx-bds-geo-half-cycle") && !v.from.mayUseUBX() {
		return nil, usageFunc, errors.New("--ubx-bds-geo-half-cycle is valid only with raw or UBX input")
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

func (f inputFormat) mayUseUnicore() bool {
	return f == inputRaw || f == inputUNCB || f == inputUNCA
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

func readHeaderFile(path string, r io.Reader) (rinex.Metadata, error) {
	var meta rinex.Metadata
	if err := toml.NewDecoder(r).DisallowUnknownFields().Decode(&meta); err != nil {
		return rinex.Metadata{}, fmt.Errorf("%s: %w", path, err)
	}
	return meta, nil
}

func setMetadataDefaults(meta *rinex.Metadata, now time.Time, interval time.Duration) {
	const headerFieldWidth = 20
	truncateHeaderField := func(s string) string {
		if len(s) <= headerFieldWidth {
			return s
		}
		return s[:headerFieldWidth]
	}
	if meta.Version == "" {
		meta.Version = rinex.Version304
	}
	if meta.Run.Program == "" {
		const prefix = "satpulse"
		if v := cmd.ShortenVersion(headerFieldWidth - len(prefix) - 1); v != "" {
			meta.Run.Program = truncateHeaderField(prefix + " " + v)
		} else {
			meta.Run.Program = prefix
		}
	}
	if meta.Run.By == "" {
		u, err := user.Current()
		if err == nil {
			s := u.Name
			if s == "" {
				s = u.Username
			}
			meta.Run.By = truncateHeaderField(s)
		}
	}
	if meta.Run.Date.IsZero() {
		meta.Run.Date = now
	}
	if meta.Interval == nil && interval != 0 {
		v := interval.Seconds()
		meta.Interval = &v
	}
}

func applyMetadataFlags(meta *rinex.Metadata, flags *pflag.FlagSet) error {
	if flags.Changed("rinex-version") {
		meta.Version = rinex.Version(mustString(flags, "rinex-version"))
	}
	if flags.Changed("program") {
		meta.Run.Program = mustString(flags, "program")
	}
	if flags.Changed("run-by") {
		meta.Run.By = mustString(flags, "run-by")
	}
	if flags.Changed("antenna") {
		meta.Antenna.Type = mustString(flags, "antenna")
	}
	if flags.Changed("approx-pos") {
		s := mustString(flags, "approx-pos")
		if s == "" {
			meta.ApproxPosition = nil
		} else {
			pos, err := parseXYZ(s, "--approx-pos")
			if err != nil {
				return err
			}
			meta.ApproxPosition = &pos
		}
	}
	if flags.Changed("comment") {
		lines, err := flags.GetStringArray("comment")
		if err != nil {
			panic(err)
		}
		meta.Comment = rinex.Lines(lines)
	}
	return nil
}

func mustString(flags *pflag.FlagSet, name string) string {
	s, err := flags.GetString(name)
	if err != nil {
		panic(err)
	}
	return s
}

func parseXYZ(s, opt string) ([3]float64, error) {
	parts := strings.Split(s, ",")
	var out [3]float64
	if len(parts) != 3 {
		return out, fmt.Errorf("%s must contain three comma-separated numbers", opt)
	}
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return out, fmt.Errorf("%s contains invalid number %q", opt, p)
		}
		out[i] = v
	}
	return out, nil
}

func (cj convJob) run(lg *slog.Logger, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	opts := cj.opts
	switch opts.from {
	case inputRINEX, inputObsJSON:
		return convertObservationInputs(cj.inputs, cj.out, opts.from, opts.to, cj.meta, opts.interval, opts.requireCP)
	}
	sink, err := outputSink(cj.out, opts.to, opts.interval, opts.requireCP)
	if err != nil {
		return err
	}
	conv, err := newPacketInput(opts.from, sink, cj.meta, opts.format, lg)
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

func outputSink(w io.Writer, format outputFormat, interval time.Duration, requireCP bool) (rinex.Sink, error) {
	var sink rinex.Sink
	switch format {
	case outputRINEX:
		sink = rinex.NewObservationSink(w)
	case outputObsJSON:
		sink = rinex.NewObsJSONSink(w)
	default:
		return nil, fmt.Errorf("unsupported output format %q", format)
	}
	if interval != 0 {
		dec, err := rinex.NewDecimationSink(sink, interval)
		if err != nil {
			return nil, err
		}
		sink = dec
	}
	if requireCP {
		sink = rinex.NewRequireCPFilter(sink)
	}
	return sink, nil
}

func convertObservationInputs(inputs iter.Seq2[string, inputReader], out io.Writer, from inputFormat, to outputFormat, meta rinex.Metadata, interval time.Duration, requireCP bool) error {
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
	return convertBufferedObservations(out, to, rinex.MergeMetadata(fileMeta, meta), obs, interval, requireCP)
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

func sinkMetadata(sink rinex.Sink, meta rinex.Metadata) error {
	if meta.IsZero() {
		return nil
	}
	return sink.Metadata(meta)
}

func convertBufferedObservations(out io.Writer, to outputFormat, meta rinex.Metadata, obs []rinex.SignalObservation, interval time.Duration, requireCP bool) error {
	sink, err := outputSink(out, to, interval, requireCP)
	if err != nil {
		return err
	}
	if err := sinkMetadata(sink, meta); err != nil {
		return err
	}
	for _, o := range obs {
		if err := sink.Observation(o); err != nil {
			return err
		}
	}
	return sink.Flush()
}

func newPacketInput(from inputFormat, sink rinex.Sink, meta rinex.Metadata, format formatOptions, lg *slog.Logger) (packetInput, error) {
	if tag := inputPacketTags[from]; tag != gpsprot.EmptyTag {
		if err := sinkMetadata(sink, meta); err != nil {
			return nil, err
		}
	}
	switch from {
	case inputUBX:
		conv := rnxubx.New(sink, format.ubx)
		return &tagInput{tag: gpsreg.TagUBX, convert: func(data []byte, _ WeekConstraint) (bool, error) {
			return convertUBXData(data, conv)
		}}, nil
	case inputRTCM:
		conv := rnxrtcm.New(sink, format.rtcm)
		return &tagInput{tag: gpsreg.TagRTCM, convert: func(data []byte, week WeekConstraint) (bool, error) {
			return convertRTCMData(data, conv, week)
		}}, nil
	case inputUNCB:
		conv := rnxunc.New(sink, format.unc)
		return &tagInput{tag: gpsreg.TagUnicoreBin, accept: isRawObsTag, convert: func(data []byte, _ WeekConstraint) (bool, error) {
			return convertObsVMData(data, conv, uncmsg.ParseBinMsg)
		}}, nil
	case inputUNCA:
		conv := rnxunc.New(sink, format.unc)
		return &tagInput{tag: gpsreg.TagUnicoreAscii, accept: isRawObsTag, convert: func(data []byte, _ WeekConstraint) (bool, error) {
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
		unc:     rnxunc.New(sink, format.unc),
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
		opkt, ok, err := obsPacketFromScan(pkt)
		if err != nil {
			return n, err
		}
		if !ok {
			continue
		}
		var wc WeekConstraint
		if opkt.HasTag(gpsreg.TagRTCM) {
			if !weekUsed {
				wc = week
				weekUsed = true
			}
		}
		ok, err = in.ConvertPacket(opkt, wc)
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
		if entry.Out {
			continue
		}
		data, ok := packetLogEntryData(&entry)
		if !ok {
			continue
		}
		if from != inputRaw && entry.Tag != inputPacketTags[from] {
			continue
		}
		if from == inputRaw {
			if !supportedRawPacketLogTag(entry.Tag) {
				continue
			}
		}
		ok, err := convertPacketData(data, packetLogFormatsByTag[entry.Tag], in, packetLogWeekConstraint(entry, line))
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

func obsPacketFromScan(pkt scan.Packet) (obsPacket, bool, error) {
	if pkt.ReadError != nil {
		return obsPacket{}, false, pkt.ReadError
	}
	if pkt.Format == nil {
		return obsPacket{}, false, nil
	}
	if !pkt.ChecksumValid {
		return obsPacket{}, false, pkt.ChecksumError()
	}
	return obsPacket{format: pkt.Format, data: []byte(pkt.Data)}, true, nil
}

func packetLogEntryData(entry *gpsio.PacketLogEntry) ([]byte, bool) {
	if len(entry.Bin) != 0 {
		return []byte(entry.Bin), true
	}
	if entry.Ascii != "" {
		return []byte(entry.Ascii), true
	}
	return nil, false
}

func supportedRawPacketLogTag(tag gpsprot.Tag) bool {
	return tag == gpsreg.TagUBX || tag == gpsreg.TagRTCM || tag == gpsreg.TagUnicoreBin || tag == gpsreg.TagUnicoreAscii
}

func convertPacketData(data []byte, fmts []gpsprot.PacketFormat, in packetInput, week WeekConstraint) (bool, error) {
	for _, f := range fmts {
		if !gpsprot.IsValidPacket(f, data) {
			continue
		}
		if !slices.Equal(f.ExtractChecksum(data), f.ComputeChecksum(data)) {
			return false, packetChecksumError(f, data)
		}
		pkt := obsPacket{format: f, data: data}
		var wc WeekConstraint
		if pkt.HasTag(gpsreg.TagRTCM) {
			wc = week
		}
		return in.ConvertPacket(pkt, wc)
	}
	return false, nil
}

func packetChecksumError(f gpsprot.PacketFormat, data []byte) error {
	return fmt.Errorf("incorrect checksum: in packet %x, computed %x", f.ExtractChecksum(data), f.ComputeChecksum(data))
}

func (in *tagInput) ConvertPacket(pkt obsPacket, week WeekConstraint) (bool, error) {
	if !pkt.HasTag(in.tag) {
		return false, nil
	}
	if in.accept != nil && !in.accept(pkt) {
		return false, nil
	}
	return in.convert(pkt.data, week)
}

func (in *rawPacketInput) ConvertPacket(pkt obsPacket, week WeekConstraint) (bool, error) {
	if pkt.HasTag(gpsreg.TagRTCM) {
		return in.convertRTCM(pkt.data, week)
	}
	tag, ok := isRawObs(pkt)
	if !ok {
		return false, nil
	}
	return in.convertNonRTCMObservation(tag, pkt.data)
}

func isRawObs(pkt obsPacket) (gpsprot.Tag, bool) {
	switch tag := pkt.Tag(); tag {
	case gpsreg.TagUBX:
		return tag, ubxbin.PacketMsgId(pkt.data) == ubxbin.RxmRawxID
	case gpsreg.TagUnicoreBin:
		return tag, uncmsg.BinPacketMsgID(pkt.data) == uncmsg.ObsvMID
	case gpsreg.TagUnicoreAscii:
		return tag, pkt.format.MsgID(pkt.data) == uncmsg.ObsvMID.String()
	default:
		return gpsprot.EmptyTag, false
	}
}

func isRawObsTag(pkt obsPacket) bool {
	_, ok := isRawObs(pkt)
	return ok
}

func (in *rawPacketInput) convertNonRTCMObservation(tag gpsprot.Tag, data []byte) (bool, error) {
	if in.family == gpsprot.EmptyTag {
		in.metaBuf.Drop()
		if err := sinkMetadata(in.sink, in.meta); err != nil {
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

func (in *rawPacketInput) convertRTCM(data []byte, week WeekConstraint) (bool, error) {
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
		if err := sinkMetadata(in.sink, in.meta); err != nil {
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

func convertUBXData(data []byte, conv *rnxubx.Converter) (bool, error) {
	if ubxbin.PacketMsgId(data) != ubxbin.RxmRawxID {
		return false, nil
	}
	msg, err := ubxbin.ParseMsg(string(data))
	if err != nil {
		return false, err
	}
	rawx, ok := msg.(*ubxbin.RxmRawx)
	if !ok {
		return false, nil
	}
	return true, conv.ConvertRAWX(rawx)
}

func convertObsVMData(data []byte, conv *rnxunc.Converter, parse func([]byte) (*uncmsg.Msg, error)) (bool, error) {
	msg, err := parse(data)
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

func convertRTCMData(data []byte, conv *rnxrtcm.Converter, week WeekConstraint) (bool, error) {
	if week.err != nil {
		return false, week.err
	}
	msg, err := rtcmbin.ParseMsg(string(data))
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

func isRTCMMSM7Packet(data []byte) bool {
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
