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
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	rnxrtcm "github.com/jclark/satpulse/gps/lib/rinex/rtcm"
	rnxubx "github.com/jclark/satpulse/gps/lib/rinex/ubx"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const scanBufSize = 64 * 1024
const gpsWeek = 7 * 24 * time.Hour
const packetLogWeekSlack = time.Minute

var packetStreamFormats = gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)
var packetLogFormats = gpsreg.CreatePacketFormats(gpsreg.VendorUblox)

const summary = `[-h|--help] [-o|--output path] [--metadata path]
           [-r|--from raw|ubx|rtcm|rinex|obsj] [--packet-log] [--to rinex|obsj]
           [--date YYYYMMDD|--recent|-f|--date-from-filename]
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
	inputRTCM    inputFormat = "rtcm"
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
	week         weekOptions
	meta         rinex.Metadata
	wopts        rinex.WriterOptions
	uopts        rnxubx.Options
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

type runOptions struct {
	now  time.Time
	week weekOptions
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

type rtcmWeekState struct {
	week WeekConstraint
	used bool
}

type ubxPacketInput struct {
	conv *rnxubx.Converter
}

var _ packetInput = (*ubxPacketInput)(nil)

type rtcmPacketInput struct {
	conv *rnxrtcm.Converter
}

var _ packetInput = (*rtcmPacketInput)(nil)

type metadataBufferSink struct {
	dst  rinex.Sink
	buf  []rinex.Metadata
	pass bool
}

var _ rinex.Sink = (*metadataBufferSink)(nil)

type rawPacketInput struct {
	sink      rinex.Sink
	uopts     rnxubx.Options
	tentative bool
	week      WeekConstraint
	meta      rinex.Metadata
	metaBuf   *metadataBufferSink
	rtcm      *rnxrtcm.Converter
	ubx       *rnxubx.Converter
}

var _ packetInput = (*rawPacketInput)(nil)

// Cmd implements the convobs subcommand.
func Cmd(_ io.Writer, _ slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if v == nil {
		return usageFunc(progName), nil
	}
	now := time.Now().UTC()
	if v.wopts.Date.IsZero() {
		v.wopts.Date = now
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
	return "", runInputsWithOptions(openInputs(v.inputPaths), out, v.inputFormat, v.outputFormat, v.packetLog, v.meta, v.wopts, v.uopts, v.interval, runOptions{
		now:  now,
		week: v.week,
	})
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
	if err := v.setWeekOptions(date, recent, dateFromFilename); err != nil {
		return nil, usageFunc, err
	}
	if v.packetLog && v.week.mode != weekAuto {
		return nil, usageFunc, errors.New("--date, --recent, and --date-from-filename are not valid with --packet-log")
	}
	if v.week.mode != weekAuto && !v.inputFormat.mayUseRTCM() {
		return nil, usageFunc, errors.New("--date, --recent, and --date-from-filename are valid only with raw or RTCM input")
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
	if v.week.mode == weekFilename {
		for _, path := range v.inputPaths {
			if path == "-" {
				return nil, usageFunc, errors.New("--date-from-filename is not valid with stdin")
			}
		}
	}
	if v.inputFormat.packetInput() {
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

func (v *flagVars) setInputFormat(s string) error {
	s = strings.ToLower(s)
	switch inputFormat(s) {
	case inputRaw, inputUBX, inputRTCM, inputRINEX, inputObsJSON:
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
	return f == inputRaw || f == inputUBX || f == inputRTCM
}

func (f inputFormat) mayUseRTCM() bool {
	return f == inputRaw || f == inputRTCM
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

func runInputs(inputs iter.Seq2[string, inputReader], out io.Writer, from inputFormat, to outputFormat, packetLog bool, meta rinex.Metadata, wopts rinex.WriterOptions, uopts rnxubx.Options, interval time.Duration) error {
	return runInputsWithOptions(inputs, out, from, to, packetLog, meta, wopts, uopts, interval, runOptions{now: time.Now().UTC()})
}

func runInputsWithOptions(inputs iter.Seq2[string, inputReader], out io.Writer, from inputFormat, to outputFormat, packetLog bool, meta rinex.Metadata, wopts rinex.WriterOptions, uopts rnxubx.Options, interval time.Duration, opts runOptions) error {
	if opts.now.IsZero() {
		opts.now = time.Now().UTC()
	}
	switch from {
	case inputRINEX, inputObsJSON:
		return convertObservationInputs(inputs, out, from, to, meta, wopts, interval)
	}
	sink, err := outputSink(out, to, wopts, interval)
	if err != nil {
		return err
	}
	conv, err := newPacketInput(from, sink, meta, uopts)
	if err != nil {
		return err
	}
	n := 0
	i := 0
	for path, in := range inputs {
		if in.err != nil {
			return inputError(path, in.err)
		}
		var week rtcmWeekState
		if !packetLog {
			week, err = fileWeekConstraint(path, in, opts, from == inputRTCM, i)
			if err != nil {
				return inputError(path, err)
			}
		}
		var c int
		if packetLog {
			c, err = convertPacketLog(in.r, from, conv)
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
		return noPacketObservationsError(from)
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

func newPacketInput(from inputFormat, sink rinex.Sink, meta rinex.Metadata, uopts rnxubx.Options) (packetInput, error) {
	switch from {
	case inputUBX:
		if err := sink.Metadata(ubxInputMetadata()); err != nil {
			return nil, err
		}
		if err := sink.Metadata(meta); err != nil {
			return nil, err
		}
		return &ubxPacketInput{conv: rnxubx.New(sink, uopts)}, nil
	case inputRTCM:
		if err := sink.Metadata(rtcmInputMetadata()); err != nil {
			return nil, err
		}
		if err := sink.Metadata(meta); err != nil {
			return nil, err
		}
		return &rtcmPacketInput{conv: rnxrtcm.New(sink, rnxrtcm.Options{})}, nil
	default:
		return newRawPacketInput(sink, meta, uopts), nil
	}
}

func ubxInputMetadata() rinex.Metadata {
	return rinex.Metadata{Comments: []string{"format: u-blox UBX", "options: -MULTICODE"}}
}

func rtcmInputMetadata() rinex.Metadata {
	return rinex.Metadata{Comments: []string{"format: RTCM"}}
}

func newRawPacketInput(sink rinex.Sink, meta rinex.Metadata, uopts rnxubx.Options) *rawPacketInput {
	buf := &metadataBufferSink{dst: sink}
	return &rawPacketInput{
		sink:      sink,
		uopts:     uopts,
		tentative: true,
		meta:      meta,
		metaBuf:   buf,
		rtcm:      rnxrtcm.New(buf, rnxrtcm.Options{}),
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

func convertPacketStream(r io.Reader, in packetInput, week rtcmWeekState) (int, error) {
	s := scan.New(r, scanBufSize, packetStreamFormats)
	n := 0
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
			wc = week.next()
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
		if entry.Tag != "" && !inputAcceptsTag(from, entry.Tag) {
			continue
		}
		ok, err := convertPacketData(entry.Data(), in, packetLogWeekConstraint(entry, line))
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

func convertPacketData(data string, in packetInput, week WeekConstraint) (bool, error) {
	s := scan.New(strings.NewReader(data), scanBufSize, packetLogFormats)
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

func inputAcceptsTag(from inputFormat, tag gpsprot.Tag) bool {
	switch from {
	case inputUBX:
		return tag == gpsreg.TagUBX
	case inputRTCM:
		return tag == gpsreg.TagRTCM
	default:
		return tag == gpsreg.TagUBX || tag == gpsreg.TagRTCM
	}
}

func packetData(pkt scan.Packet) (string, bool, error) {
	if pkt.ReadError != nil {
		return "", false, pkt.ReadError
	}
	if pkt.Format == nil {
		if len(pkt.Data) != 0 {
			return "", false, fmt.Errorf("invalid input data before packet: %d bytes", len(pkt.Data))
		}
		return "", false, nil
	}
	if !pkt.ChecksumValid {
		return "", false, pkt.ChecksumError()
	}
	return pkt.Data, true, nil
}

func (in *ubxPacketInput) ConvertPacket(pkt scan.Packet, _ WeekConstraint) (bool, error) {
	data, ok, err := packetData(pkt)
	if !ok || err != nil || !pkt.HasTag(gpsreg.TagUBX) {
		return false, err
	}
	return convertUBXData(data, in.conv)
}

func (in *rtcmPacketInput) ConvertPacket(pkt scan.Packet, week WeekConstraint) (bool, error) {
	data, ok, err := packetData(pkt)
	if !ok || err != nil || !pkt.HasTag(gpsreg.TagRTCM) {
		return false, err
	}
	return convertRTCMData(data, in.conv, week)
}

func (in *rawPacketInput) ConvertPacket(pkt scan.Packet, week WeekConstraint) (bool, error) {
	data, ok, err := packetData(pkt)
	if !ok || err != nil {
		return false, err
	}
	switch pkt.Tag() {
	case gpsreg.TagUBX:
		return in.convertUBX(data)
	case gpsreg.TagRTCM:
		return in.convertRTCM(data, week)
	default:
		return false, nil
	}
}

func (in *rawPacketInput) convertUBX(data string) (bool, error) {
	if ubxbin.PacketMsgId(data) != ubxbin.RxmRawxID {
		return false, nil
	}
	if in.tentative {
		in.metaBuf.Drop()
		if err := in.sink.Metadata(ubxInputMetadata()); err != nil {
			return false, err
		}
		if err := in.sink.Metadata(in.meta); err != nil {
			return false, err
		}
		in.rtcm = nil
		in.ubx = rnxubx.New(in.sink, in.uopts)
		in.tentative = false
	}
	if in.rtcm != nil {
		return false, errors.New("mixed raw observation input: UBX-RXM-RAWX after RTCM MSM7")
	}
	return convertUBXData(data, in.ubx)
}

func (in *rawPacketInput) convertRTCM(data string, week WeekConstraint) (bool, error) {
	msm7 := isRTCMMSM7Packet(data)
	if in.ubx != nil {
		if msm7 {
			return false, errors.New("mixed raw observation input: RTCM MSM7 after UBX-RXM-RAWX")
		}
		return false, nil
	}
	if in.tentative {
		in.noteWeek(week)
		if !msm7 {
			return convertRTCMData(data, in.rtcm, quietWeekConstraint(week))
		}
		if in.week.err != nil {
			return false, in.week.err
		}
		if err := in.sink.Metadata(rtcmInputMetadata()); err != nil {
			return false, err
		}
		if err := in.sink.Metadata(in.meta); err != nil {
			return false, err
		}
		if err := in.metaBuf.Commit(); err != nil {
			return false, err
		}
		in.tentative = false
	}
	ok, err := convertRTCMData(data, in.rtcm, week)
	if err == nil && ok && week.Interval.IsZero() {
		in.week.warnOnce()
	}
	return ok, err
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

func (s *rtcmWeekState) next() WeekConstraint {
	if s.used {
		return WeekConstraint{}
	}
	s.used = true
	return s.week
}

func fileWeekConstraint(path string, in inputReader, opts runOptions, strict bool, i int) (rtcmWeekState, error) {
	wc, err := newFileWeekConstraint(path, in, opts, i)
	if err != nil {
		return rtcmWeekState{}, err
	}
	if strict && wc.err != nil {
		return rtcmWeekState{}, wc.err
	}
	return rtcmWeekState{week: wc}, nil
}

func newFileWeekConstraint(path string, in inputReader, opts runOptions, i int) (WeekConstraint, error) {
	switch opts.week.mode {
	case weekRecent:
		return recentWeekConstraint(opts.now, ""), nil
	case weekDate:
		return dateWeekConstraint(opts.week.date, "--date"), nil
	case weekFilename:
		t, err := dateFromFilename(path)
		if err != nil {
			return WeekConstraint{}, err
		}
		return dateWeekConstraint(t, "--date-from-filename"), nil
	default:
		return automaticWeekConstraint(path, in, opts.now, i), nil
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

func noPacketObservationsError(from inputFormat) error {
	switch from {
	case inputRTCM:
		return errors.New("no RTCM MSM7 messages found")
	case inputRaw:
		return errors.New("no UBX-RXM-RAWX or RTCM MSM7 messages found")
	default:
		return errors.New("no UBX-RXM-RAWX messages found")
	}
}
