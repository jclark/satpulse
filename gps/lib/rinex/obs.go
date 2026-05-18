// Package rinex provides JSON-serializable data types for RINEX tooling.
package rinex

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

// ObsJSONExtension is the extension for JSON lines of SignalObservation and Metadata records.
const ObsJSONExtension = ".obsj"

const (
	tickNs     int64 = 100 // Time is in ticks that correspond to tickNs nanoseconds
	msPerWeek        = 7 * 24 * 60 * 60 * 1000
	timeLayout       = "2006-01-02T15:04:05.0000000"
)

// Epoch is the calendar label used as the zero point for Time.
var Epoch = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

// Time represents an instant in either the GPS or UTC time scale.
// Time is measured as 100 ns ticks from Epoch.
// The GPS time scale uses elapsed physical time, which includes leap seconds.
// The UTC time scale excludes leap seconds, treating each day
// as having 86400 seconds.
// The text form is ISO8601 with 7 digits of fractional precision and no time zone.
// For a UTC instant whose RFC3339 UTC representation is S+00:00, the text form is S.
// For a GPS instant at the same physical time, the text form is GPS-UTC
// seconds after S.
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

// SignalObservation holds measurements for one satellite signal at one epoch.
// It is the JSONL record type for an obsj file and can expand to RINEX C/L/D/S observation codes.
type SignalObservation struct {
	T   Time             `json:"t"`            // RINEX observation time label
	Sat SatelliteID      `json:"sat"`          // RINEX satellite identifier, e.g. G03
	Sig SignalID         `json:"sig"`          // RINEX signal identifier, e.g. 1C for C1C/L1C/D1C/S1C
	Frq opt.Val[int8]    `json:"frq,omitzero"` // GLONASS FDMA frequency channel k
	PR  opt.Val[float64] `json:"pr,omitzero"`  // pseudorange, meters
	CP  opt.Val[float64] `json:"cp,omitzero"`  // carrier phase, cycles
	Do  opt.Val[float64] `json:"do,omitzero"`  // Doppler, Hz
	CN0 opt.Val[float32] `json:"cn0,omitzero"` // carrier-to-noise density, dB-Hz
	LLI opt.Val[uint8]   `json:"lli,omitzero"` // RINEX loss-of-lock indicator for phase
	SSI opt.Val[uint8]   `json:"ssi,omitzero"` // RINEX signal strength indicator
}

// Metadata holds header-related facts in an obsj metadata record.
// Metadata records do not have a t field and may appear anywhere in an obsj file.
type Metadata struct {
	MarkerName     string              `json:"markerName,omitzero"`
	MarkerNumber   string              `json:"markerNumber,omitzero"`
	MarkerType     string              `json:"markerType,omitzero"`
	Observer       string              `json:"observer,omitzero"`
	Agency         string              `json:"agency,omitzero"`
	Receiver       Receiver            `json:"receiver,omitzero"`
	Antenna        Antenna             `json:"antenna,omitzero"`
	ApproxPosition opt.Val[[3]float64] `json:"approxPosition,omitzero"` // APPROX POSITION XYZ, meters
	AntennaDelta   opt.Val[[3]float64] `json:"antennaDelta,omitzero"`   // ANTENNA: DELTA H/E/N, meters
	LeapSeconds    opt.Val[int16]      `json:"leapSeconds,omitzero"`    // TAI-UTC offset
}

// Receiver describes the receiver used for a RINEX observation file.
type Receiver struct {
	Number  string `json:"number,omitzero"`
	Type    string `json:"type,omitzero"`
	Version string `json:"version,omitzero"`
}

// Antenna describes the antenna used for a RINEX observation file.
type Antenna struct {
	Number string `json:"number,omitzero"`
	Type   string `json:"type,omitzero"`
}

// String formats t as an ISO8601 time label without a timezone suffix.
func (t Time) String() string {
	sec, tick := divMod(int64(t), 1e9/tickNs)
	tm := time.Unix(Epoch.Unix()+sec, tick*tickNs).UTC()
	return tm.Format(timeLayout)
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
	*t = Time(((tm.Unix()-Epoch.Unix())*1e9 + int64(tm.Nanosecond())) / tickNs)
	return nil
}

// FloorTime floors tm to the precision used by Time.
func FloorTime(tm time.Time) time.Time {
	return tm.UTC().Truncate(time.Duration(tickNs) * time.Nanosecond)
}

// TimeFromUTC converts a UTC time.Time to a Time using the UTC scale.
func TimeFromUTC(tm time.Time) Time {
	return Time(FloorTime(tm).Sub(Epoch).Nanoseconds() / tickNs)
}

// UTC converts t from the UTC scale to a time.Time.
func (t Time) UTC() time.Time {
	sec, nsec := divMod(int64(t)*tickNs, 1e9)
	return time.Unix(Epoch.Unix()+sec, nsec).UTC()
}

// TimeFromGPSWeekMillis converts a GPS week and millisecond TOW to a Time.
func TimeFromGPSWeekMillis(week int64, towMS uint32) Time {
	return Time((week*msPerWeek + int64(towMS)) * 1e6 / tickNs)
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
