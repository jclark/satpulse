// Package rinex provides JSON-serializable data types for RINEX tooling.
package rinex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

// ObsJSONExtension is the extension for JSON lines of SignalObservation and Metadata records.
const ObsJSONExtension = ".obsj"

const (
	tickNs               int64 = 100 // Time is in ticks that correspond to tickNs nanoseconds
	msPerWeek                  = 7 * 24 * 60 * 60 * 1000
	defaultGPSUTCSeconds       = 18
	bdtGPSOffsetSeconds        = 14
	timeLayout                 = "2006-01-02T15:04:05.0000000"
)

// Epoch is the calendar label used as the zero point for Time.
var Epoch = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

// Time represents GPS time as 100 ns ticks from Epoch.
// The text form is the GPS-time calendar label with 7 digits of fractional
// precision and no time zone.
type Time int64

// SatelliteID is a RINEX satellite identifier, such as G03 or E11.
type SatelliteID string

// ObservationType is the first character of a RINEX observation code.
type ObservationType byte

const (
	TypeCode           ObservationType = 'C'
	TypePhase          ObservationType = 'L'
	TypeDoppler        ObservationType = 'D'
	TypeSignalStrength ObservationType = 'S'
)

// SignalID is the RINEX band and attribute part of an observation code.
type SignalID string

// ObservationCode is a complete three-character RINEX observation code.
type ObservationCode string

// LLI is the RINEX loss-of-lock indicator for phase observations.
type LLI uint8

const (
	LLILostLock           LLI = 1 << iota // lost lock between observations; cycle slip possible
	LLIHalfCycleAmbiguity                 // half-cycle ambiguity or slip possible
	LLIBOCTracking                        // BOC tracking of an MBOC-modulated signal
)

// SignalValues holds the values for one satellite signal observation.
type SignalValues struct {
	Frq opt.Val[int8]    `json:"frq,omitzero"` // GLONASS FDMA frequency channel k
	PR  opt.Val[float64] `json:"pr,omitzero"`  // pseudorange, meters
	CP  opt.Val[float64] `json:"cp,omitzero"`  // carrier phase, cycles
	Do  opt.Val[float64] `json:"do,omitzero"`  // RINEX Doppler, Hz; positive for approaching satellites
	CN0 opt.Val[float32] `json:"cn0,omitzero"` // carrier-to-noise density, dB-Hz
	LLI opt.Val[LLI]     `json:"lli,omitzero"` // RINEX loss-of-lock indicator for phase
	SSI opt.Val[uint8]   `json:"ssi,omitzero"` // RINEX signal strength indicator
}

// IsZero reports whether v contains no values.
func (v SignalValues) IsZero() bool {
	return !v.Frq.IsSet() && !v.PR.IsSet() && !v.CP.IsSet() && !v.Do.IsSet() && !v.CN0.IsSet() && !v.LLI.IsSet() && !v.SSI.IsSet()
}

// SignalObservation holds measurements for one satellite signal at one epoch.
// It is the JSONL record type for an obsj file and can expand to RINEX C/L/D/S observation codes.
// T is always in GPS time, independent of the satellite constellation or the
// time system used by an input or output RINEX file.
type SignalObservation struct {
	T   Time        `json:"t"`   // RINEX observation time label
	Sat SatelliteID `json:"sat"` // RINEX satellite identifier, e.g. G03
	Sig SignalID    `json:"sig"` // RINEX signal identifier, e.g. 1C for C1C/L1C/D1C/S1C
	SignalValues
}

// Version is a RINEX observation format version.
type Version string

const (
	Version304 Version = "3.04"
	Version400 Version = "4.00"
	Version401 Version = "4.01"
	Version402 Version = "4.02"
)

// MetadataRun describes the program run that created a RINEX observation file.
type MetadataRun struct {
	Program string    `json:"program,omitzero" toml:"program"`
	By      string    `json:"by,omitzero" toml:"by"`
	Date    time.Time `json:"date,omitzero" toml:"date"`
}

// Lines is a list of comment lines.
type Lines []string

// Metadata holds header-related facts in an obsj metadata record.
// Metadata records do not have a t field and may appear anywhere in an obsj file.
type Metadata struct {
	Version Version     `json:"version,omitzero" toml:"version"`
	Run     MetadataRun `json:"run,omitzero" toml:"run"`
	Comment Lines       `json:"comment,omitzero" toml:"comment,multiline"`
	Marker  struct {
		Name   string `json:"name,omitzero" toml:"name"`
		Number string `json:"number,omitzero" toml:"number"`
		Type   string `json:"type,omitzero" toml:"type"`
	} `json:"marker,omitzero" toml:"marker"`
	Observer string   `json:"observer,omitzero" toml:"observer"`
	Agency   string   `json:"agency,omitzero" toml:"agency"`
	Receiver Receiver `json:"receiver,omitzero" toml:"receiver"`
	Antenna  Antenna  `json:"antenna,omitzero" toml:"antenna"`
	// Use pointers for optional metadata values so TOML packages can decode
	// arrays and scalars directly without format-specific adapter structs.
	ApproxPosition *[3]float64 `json:"approxPosition,omitzero" toml:"approxPosition"` // APPROX POSITION XYZ, meters
	AntennaDelta   *[3]float64 `json:"antennaDelta,omitzero" toml:"antennaDelta"`     // ANTENNA: DELTA H/E/N, meters
	Interval       *float64    `json:"interval,omitzero" toml:"interval"`             // observation interval, seconds
	LeapSeconds    *int16      `json:"leapSeconds,omitzero" toml:"leapSeconds"`       // GPS-UTC offset
}

// Receiver describes the receiver used for a RINEX observation file.
type Receiver struct {
	Number  string `json:"number,omitzero" toml:"number"`
	Type    string `json:"type,omitzero" toml:"type"`
	Version string `json:"version,omitzero" toml:"version"`
}

// Antenna describes the antenna used for a RINEX observation file.
type Antenna struct {
	Number string `json:"number,omitzero" toml:"number"`
	Type   string `json:"type,omitzero" toml:"type"`
}

// Sink receives RINEX conversion records.
type Sink interface {
	Metadata(Metadata) error
	Observation(SignalObservation) error
	Flush() error
}

// ObservationSink buffers observations and writes RINEX output on Flush.
type ObservationSink struct {
	w    io.Writer
	meta Metadata
	obs  []SignalObservation
}

// ObsJSONSink writes observation records as obsj JSON lines.
type ObsJSONSink struct {
	w   *bufio.Writer
	enc *json.Encoder
}

// String formats t as an ISO8601 time label without a timezone suffix.
func (t Time) String() string {
	return t.CivilTime().Format(timeLayout)
}

// MarshalText formats t as an ISO8601 time label without a timezone suffix.
func (t Time) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText parses an ISO8601 time label without a timezone suffix.
func (t *Time) UnmarshalText(text []byte) error {
	tm, err := time.ParseInLocation(timeLayout, string(text), time.UTC)
	if err != nil {
		return fmt.Errorf("rinex: invalid time %q: %w", text, err)
	}
	*t = TimeFromCivilTime(tm)
	return nil
}

// FloorTime floors tm to the precision used by Time.
func FloorTime(tm time.Time) time.Time {
	return tm.UTC().Truncate(time.Duration(tickNs) * time.Nanosecond)
}

// TimeFromCivilTime converts the instant tm to a Time.
// No leap-second adjustment is applied, so a tm in time.UTC maps to the Time
// with the same civil label. It is the inverse of CivilTime.
func TimeFromCivilTime(tm time.Time) Time {
	return Time(FloorTime(tm).Sub(Epoch).Nanoseconds() / tickNs)
}

// CivilTime converts t to a time.Time in time.UTC.
// No leap-second adjustment is applied, so the result has the same civil label
// as t. It is the inverse of TimeFromCivilTime.
func (t Time) CivilTime() time.Time {
	sec, nsec := divMod(int64(t)*tickNs, 1e9)
	return time.Unix(Epoch.Unix()+sec, nsec).UTC()
}

// TimeFromGPSWeekMillis converts a GPS week and millisecond TOW to a Time.
func TimeFromGPSWeekMillis(week int64, towMS uint32) Time {
	return Time((week*msPerWeek + int64(towMS)) * 1e6 / tickNs)
}

// TimeFromGPSWeekSeconds converts a GPS week and TOW seconds to a Time.
func TimeFromGPSWeekSeconds(week int64, tow float64) Time {
	return Time(week*msPerWeek*1e6/tickNs + int64(tow*1e9/float64(tickNs)+0.5))
}

// GPSWeekMillis converts GPS-scale t to GPS week and millisecond TOW.
// Sub-millisecond ticks are floored.
func (t Time) GPSWeekMillis() (int64, uint32) {
	ms, _ := divMod(int64(t)*tickNs, 1e6)
	week, towMS := divMod(ms, msPerWeek)
	return week, uint32(towMS)
}

// System returns the RINEX satellite system code for id.
func (id SatelliteID) System() string {
	if len(id) == 0 {
		return ""
	}
	return string(id[0])
}

// IsValid reports whether id has the RINEX satellite identifier shape.
func (id SatelliteID) IsValid() bool {
	if len(id) != 3 || !strings.ContainsRune("GRESJCI", rune(id[0])) {
		return false
	}
	n, err := strconv.ParseUint(string(id[1:]), 10, 8)
	return err == nil && n != 0
}

// UnmarshalJSON unmarshals and validates a RINEX satellite identifier.
func (id *SatelliteID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v := SatelliteID(s)
	if !v.IsValid() {
		return fmt.Errorf("rinex: invalid satellite identifier %q", s)
	}
	*id = v
	return nil
}

// Code returns the RINEX observation code for typ on id.
func (id SignalID) Code(typ ObservationType) ObservationCode {
	if len(id) != 2 {
		return ""
	}
	return ObservationCode(string([]byte{byte(typ), id[0], id[1]}))
}

// IsValid reports whether id has the RINEX two-character signal shape.
func (id SignalID) IsValid() bool {
	if len(id) != 2 {
		return false
	}
	b := id[0]
	a := id[1]
	return b >= '1' && b <= '9' && a >= 'A' && a <= 'Z'
}

// UnmarshalJSON unmarshals and validates a RINEX signal identifier.
func (id *SignalID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v := SignalID(s)
	if !v.IsValid() {
		return fmt.Errorf("rinex: invalid signal identifier %q", s)
	}
	*id = v
	return nil
}

// UnmarshalRecord unmarshals one obsj JSONL record.
// A record with a top-level t field is a SignalObservation; every other record is Metadata.
func UnmarshalRecord(data []byte) (SignalObservation, Metadata, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return SignalObservation{}, Metadata{}, false, err
	}
	if _, ok := raw["t"]; ok {
		var obs SignalObservation
		if err := json.Unmarshal(data, &obs); err != nil {
			return SignalObservation{}, Metadata{}, true, err
		}
		return obs, Metadata{}, true, nil
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return SignalObservation{}, Metadata{}, false, err
	}
	return SignalObservation{}, meta, false, nil
}

// ReadObsJSON reads obsj JSON lines.
func ReadObsJSON(r io.Reader) (Metadata, []SignalObservation, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1024), 1024*1024)
	var meta Metadata
	var obs []SignalObservation
	line := 0
	for s.Scan() {
		line++
		b := bytes.TrimSpace(s.Bytes())
		if len(b) == 0 {
			continue
		}
		o, m, isObs, err := UnmarshalRecord(b)
		if err != nil {
			return Metadata{}, nil, fmt.Errorf("rinex: obsj line %d: %w", line, err)
		}
		if isObs {
			obs = append(obs, o)
		} else {
			meta = MergeMetadata(meta, m)
		}
	}
	if err := s.Err(); err != nil {
		return Metadata{}, nil, err
	}
	return meta, obs, nil
}

// ObservationCodes returns the RINEX observation codes present in c.
func (c SignalObservation) ObservationCodes() []ObservationCode {
	codes := make([]ObservationCode, 0, 4)
	if c.PR.IsSet() {
		codes = append(codes, c.Sig.Code(TypeCode))
	}
	if c.CP.IsSet() {
		codes = append(codes, c.Sig.Code(TypePhase))
	}
	if c.Do.IsSet() {
		codes = append(codes, c.Sig.Code(TypeDoppler))
	}
	if c.CN0.IsSet() {
		codes = append(codes, c.Sig.Code(TypeSignalStrength))
	}
	return codes
}

// System returns the RINEX satellite system code for c.
func (c SignalObservation) System() string {
	return c.Sat.System()
}

// NewObservationSink creates a sink that writes a RINEX observation file.
func NewObservationSink(w io.Writer) *ObservationSink {
	return &ObservationSink{w: w}
}

// Metadata merges m into s.
func (s *ObservationSink) Metadata(m Metadata) error {
	s.meta = MergeMetadata(s.meta, m)
	return nil
}

// Observation appends obs to s.
func (s *ObservationSink) Observation(obs SignalObservation) error {
	s.obs = append(s.obs, obs)
	return nil
}

// Flush writes all buffered observations as a RINEX observation file.
func (s *ObservationSink) Flush() error {
	return WriteObservationFile(s.w, s.meta, s.obs)
}

// NewObsJSONSink creates a sink that writes obsj JSON lines.
func NewObsJSONSink(w io.Writer) *ObsJSONSink {
	bw := bufio.NewWriter(w)
	return &ObsJSONSink{w: bw, enc: json.NewEncoder(bw)}
}

// Metadata writes m as one obsj metadata record.
func (s *ObsJSONSink) Metadata(m Metadata) error {
	return s.enc.Encode(m)
}

// Observation writes obs as one obsj signal observation record.
func (s *ObsJSONSink) Observation(obs SignalObservation) error {
	return s.enc.Encode(obs)
}

// Flush flushes buffered obsj output.
func (s *ObsJSONSink) Flush() error {
	return s.w.Flush()
}

// IsZero reports whether r has no run header data.
func (r MetadataRun) IsZero() bool {
	return r.Program == "" && r.By == "" && r.Date.IsZero()
}

// MarshalText joins lines with newlines for TOML string encoding.
func (l Lines) MarshalText() ([]byte, error) {
	return []byte(strings.Join(l, "\n")), nil
}

// UnmarshalText splits TOML comment text into lines.
func (l *Lines) UnmarshalText(text []byte) error {
	s := strings.TrimSuffix(string(text), "\n")
	if s == "" {
		*l = nil
		return nil
	}
	*l = strings.Split(s, "\n")
	return nil
}

// MarshalJSON encodes l as a JSON string array.
func (l Lines) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(l))
}

// UnmarshalJSON decodes l from a JSON string array.
func (l *Lines) UnmarshalJSON(data []byte) error {
	var lines []string
	if err := json.Unmarshal(data, &lines); err == nil {
		*l = Lines(lines)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return l.UnmarshalText([]byte(s))
}

// MarshalTOML encodes l as a TOML string.
func (l Lines) MarshalTOML() any {
	return strings.Join(l, "\n")
}

// UnmarshalTOML decodes l from a TOML string or string array.
func (l *Lines) UnmarshalTOML(v any) error {
	switch v := v.(type) {
	case string:
		return l.UnmarshalText([]byte(v))
	case []any:
		lines := make([]string, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return fmt.Errorf("rinex: comment array contains %T, want string", e)
			}
			lines = append(lines, s)
		}
		*l = Lines(lines)
		return nil
	case []string:
		*l = Lines(v)
		return nil
	default:
		return fmt.Errorf("rinex: cannot decode comment from %T", v)
	}
}

// IsZero reports whether l has no comment lines.
func (l Lines) IsZero() bool {
	return len(l) == 0
}

// IsZero reports whether m has no header data.
func (m Metadata) IsZero() bool {
	return m.Version == "" && m.Run.IsZero() && len(m.Comment) == 0 &&
		m.Marker.Name == "" && m.Marker.Number == "" && m.Marker.Type == "" &&
		m.Observer == "" && m.Agency == "" && m.Receiver.IsZero() &&
		m.Antenna.IsZero() && m.ApproxPosition == nil && m.AntennaDelta == nil &&
		m.Interval == nil && m.LeapSeconds == nil
}

// MergeMetadata merges b into a and returns the result.
func MergeMetadata(a, b Metadata) Metadata {
	if b.Version != "" {
		a.Version = b.Version
	}
	if !b.Run.IsZero() {
		a.Run = b.Run
	}
	if len(b.Comment) != 0 {
		a.Comment = append(a.Comment, b.Comment...)
	}
	if b.Marker.Name != "" {
		a.Marker.Name = b.Marker.Name
	}
	if b.Marker.Number != "" {
		a.Marker.Number = b.Marker.Number
	}
	if b.Marker.Type != "" {
		a.Marker.Type = b.Marker.Type
	}
	if b.Observer != "" {
		a.Observer = b.Observer
	}
	if b.Agency != "" {
		a.Agency = b.Agency
	}
	if !b.Receiver.IsZero() {
		a.Receiver = b.Receiver
	}
	if !b.Antenna.IsZero() {
		a.Antenna = b.Antenna
	}
	if b.ApproxPosition != nil {
		a.ApproxPosition = b.ApproxPosition
	}
	if b.AntennaDelta != nil {
		a.AntennaDelta = b.AntennaDelta
	}
	if b.Interval != nil {
		a.Interval = b.Interval
	}
	if b.LeapSeconds != nil {
		a.LeapSeconds = b.LeapSeconds
	}
	return a
}

// IsZero reports whether r has no receiver header data.
func (r Receiver) IsZero() bool {
	return r.Number == "" && r.Type == "" && r.Version == ""
}

// IsZero reports whether a has no antenna header data.
func (a Antenna) IsZero() bool {
	return a.Number == "" && a.Type == ""
}

// IsMixed reports whether observations include more than one satellite system.
func IsMixed(obs []SignalObservation) bool {
	var sys string
	for _, o := range obs {
		s := o.System()
		if s == "" {
			continue
		}
		if sys == "" {
			sys = s
			continue
		}
		if s != sys {
			return true
		}
	}
	return false
}

// ObservationCodeSet returns the observation codes present in observations, grouped by system.
func ObservationCodeSet(obs []SignalObservation) map[string][]ObservationCode {
	seen := make(map[string]map[ObservationCode]bool)
	for _, o := range obs {
		sys := o.System()
		if sys == "" {
			continue
		}
		m := seen[sys]
		if m == nil {
			m = make(map[ObservationCode]bool)
			seen[sys] = m
		}
		for _, code := range o.ObservationCodes() {
			if code != "" {
				m[code] = true
			}
		}
	}
	out := make(map[string][]ObservationCode, len(seen))
	for sys, m := range seen {
		codes := make([]ObservationCode, 0, len(m))
		for code := range m {
			codes = append(codes, code)
		}
		sortObservationCodes(codes)
		out[sys] = codes
	}
	return out
}

func divMod(n, d int64) (int64, int64) {
	q := n / d
	r := n % d
	if r < 0 {
		q--
		r += d
	}
	return q, r
}

func sortObservationCodes(codes []ObservationCode) {
	for i := 1; i < len(codes); i++ {
		for j := i; j > 0 && lessObservationCode(codes[j], codes[j-1]); j-- {
			codes[j], codes[j-1] = codes[j-1], codes[j]
		}
	}
}

func lessObservationCode(a, b ObservationCode) bool {
	as := string(a)
	bs := string(b)
	if len(as) != 3 || len(bs) != 3 {
		return as < bs
	}
	if as[1:] != bs[1:] {
		return as[1:] < bs[1:]
	}
	return observationTypeIndex(as[0]) < observationTypeIndex(bs[0])
}

func observationTypeIndex(b byte) int {
	i := strings.IndexByte("CLDS", b)
	if i < 0 {
		return 4
	}
	return i
}

func sortWriterObservationCodes(sys string, codes []ObservationCode) {
	for i := 1; i < len(codes); i++ {
		for j := i; j > 0 && lessWriterObservationCode(sys, codes[j], codes[j-1]); j-- {
			codes[j], codes[j-1] = codes[j-1], codes[j]
		}
	}
}

func lessWriterObservationCode(sys string, a, b ObservationCode) bool {
	as := string(a)
	bs := string(b)
	if len(as) != 3 || len(bs) != 3 {
		return as < bs
	}
	ai, aok := writerSignalIndex(sys, as[1:])
	bi, bok := writerSignalIndex(sys, bs[1:])
	if aok || bok {
		if ai != bi {
			return ai < bi
		}
		return observationTypeIndex(as[0]) < observationTypeIndex(bs[0])
	}
	return lessObservationCode(a, b)
}

func writerSignalIndex(sys, sig string) (int, bool) {
	if sigs, ok := writerSignalOrder[sys]; ok {
		for i, s := range sigs {
			if sig == s {
				return i, true
			}
		}
		return len(sigs), false
	}
	return 0, false
}

var writerSignalOrder = map[string][]string{
	"G": {"1C", "2W", "5Q"},
	"R": {"1C", "2C"},
	"E": {"1C", "7Q", "5Q"},
	"J": {"1C", "2X", "5Q"},
	"C": {"2I", "7I", "7D", "5P", "6I", "1P"},
}
