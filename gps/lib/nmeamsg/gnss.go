package nmeamsg

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
	"github.com/jclark/satpulse/gps/lib/opt"
)

// SentenceFields is the typed field set of an approved NMEA sentence. It
// provides the sentence format (e.g. "GGA") that names the address.
type SentenceFields interface {
	SentenceFormat() string
}

// Sentence is an addressed approved NMEA sentence: a talker ID plus the
// typed field set F. It is the generic carrier; GNSSTalkerIDMsg is the
// interface used where the concrete sentence type is not statically known.
type Sentence[F SentenceFields] struct {
	talkerID string
	Fields   F
}

// GNSSTalkerIDMsg is a decoded approved NMEA sentence with a GNSS talker ID,
// of a format not known statically. Every Sentence[F] implements it.
type GNSSTalkerIDMsg interface {
	TalkerID() string
	SentenceFormat() string
	Body() any
}

func (s Sentence[F]) TalkerID() string       { return s.talkerID }
func (s Sentence[F]) SentenceFormat() string { return s.Fields.SentenceFormat() }
func (s Sentence[F]) Body() any              { return s.Fields }

// ErrUnsupportedSentence reports a valid GNSS-talker sentence whose format
// this package does not decode. It is distinct from a (nil, nil) result,
// which means the input is not a GNSS-talker sentence at all.
var ErrUnsupportedSentence = errors.New("unsupported NMEA sentence")

// ParseGNSSTalkerPayload decodes the payload of an approved GNSS-talker NMEA
// sentence (the bytes between '$' and '*'). flags are the syntax flags the
// caller already computed with CheckSyntax; the checksum is assumed verified.
//
// It returns (nil, nil) when the sentence is not an approved GNSS-talker
// sentence (the caller should try other handlers), (nil, ErrUnsupportedSentence)
// for a GNSS-talker sentence of an unhandled format, and (nil, err) when a
// known format is malformed.
func ParseGNSSTalkerPayload(payload string, flags SentenceSyntaxFlags) (GNSSTalkerIDMsg, error) {
	if !flags.IsValidGNSSTalkerNMEA() {
		return nil, nil
	}
	fields := strings.Split(payload, ",")
	addr := fields[0]
	fields = fields[1:]
	if flags&SentenceNoCarets == 0 {
		for i := range fields {
			fields[i] = unescape(fields[i])
		}
	}
	talker := addr[:2]
	switch addr[2:] {
	case "GGA":
		return asMsg(decode[GGAFields](talker, fields))
	case "RMC":
		return asMsg(decodeRange[RMCFields](talker, fields, 12, 13))
	default:
		return nil, ErrUnsupportedSentence
	}
}

// decode parses fields into the field set F, requiring exactly the number of
// wire fields F declares (one struct field per wire field). fieldenc.Decode
// rejects extra fields but tolerates short input, silently leaving trailing
// struct fields unset, so a truncated sentence would otherwise decode to a
// half-populated struct. PartialDecode reports how many fields it consumed;
// requiring that to equal both the input length and the struct field count
// rejects truncated and over-long sentences as malformed.
func decode[F SentenceFields](talker string, fields []string) (Sentence[F], error) {
	var f F
	want := reflect.TypeOf(f).NumField()
	return decodeRange[F](talker, fields, want, want)
}

func decodeRange[F SentenceFields](talker string, fields []string, min, max int) (Sentence[F], error) {
	var f F
	n, err := fieldenc.PartialDecode(fields, &f)
	if err != nil {
		return Sentence[F]{}, fmt.Errorf("%s: %w", f.SentenceFormat(), err)
	}
	if n != len(fields) || n < min || n > max {
		return Sentence[F]{}, fieldCountErr(f.SentenceFormat(), min, max, len(fields))
	}
	return Sentence[F]{talkerID: talker, Fields: f}, nil
}

func fieldCountErr(format string, min, max, got int) error {
	if min == max {
		return fmt.Errorf("%s: expected %d fields, got %d", format, min, got)
	}
	return fmt.Errorf("%s: expected %d or %d fields, got %d", format, min, max, got)
}

// asMsg boxes a decoded sentence as a GNSSTalkerIDMsg, returning a nil
// interface on error.
func asMsg[F SentenceFields](s Sentence[F], err error) (GNSSTalkerIDMsg, error) {
	if err != nil {
		return nil, err
	}
	return s, nil
}

// SerializeMsg encodes m as a complete NMEA sentence: '$', the talker, format,
// and fieldenc-encoded fields, then '*', the checksum, and CRLF.
func SerializeMsg(m GNSSTalkerIDMsg) ([]byte, error) {
	fields, err := fieldenc.Encode(m.Body())
	if err != nil {
		return nil, err
	}
	p := m.TalkerID() + m.SentenceFormat()
	if len(fields) > 0 {
		p += "," + strings.Join(fields, ",")
	}
	return fmt.Appendf(nil, "$%s*%02X\r\n", p, Checksum([]byte(p))), nil
}

// unescape decodes NMEA caret escaping (^hh) in a data field. The input is
// assumed to be well-formed (SentenceValidCaretEscaping), so every '^' is
// followed by two hex digits.
func unescape(s string) string {
	if !strings.Contains(s, "^") {
		return s
	}
	var b strings.Builder
	for s != "" {
		before, after, ok := strings.Cut(s, "^")
		b.WriteString(before)
		if !ok {
			break
		}
		b.WriteByte(hexByte(after[0], after[1]))
		s = after[2:]
	}
	return b.String()
}

// GGAFields is the typed field set of a GGA sentence (NMEA 4.00 8.3.35).
// Latitude and longitude are each two wire fields (magnitude + hemisphere),
// so they are separate struct fields. Only Quality is mandatory; the rest are
// optional because valid no-fix output leaves them empty.
type GGAFields struct {
	Time      opt.Val[todUTC]     `json:"time,omitzero"`
	Lat       opt.Val[latCoord]   `json:"lat,omitzero"`
	LatSign   latSign             `json:"latSign,omitzero"`
	Lon       opt.Val[lonCoord]   `json:"lon,omitzero"`
	LonSign   lonSign             `json:"lonSign,omitzero"`
	Quality   uint8Dec            `json:"quality"`
	NumSats   opt.Val[uint8Dec2]  `json:"numSats,omitzero"`
	HDOP      opt.Val[float32]    `json:"hdop,omitzero"`
	Alt       opt.Val[float64]    `json:"alt,omitzero"`
	AltUnit   string              `json:"altUnit,omitempty"`
	GeoidSep  opt.Val[float64]    `json:"geoidSep,omitzero"`
	GeoidUnit string              `json:"geoidUnit,omitempty"`
	DGPSAge   opt.Val[float32]    `json:"dgpsAge,omitzero"`
	DGPSID    opt.Val[uint16Dec4] `json:"dgpsId,omitzero"`
}

// GGASentence is a typed NMEA GGA sentence.
type GGASentence = Sentence[GGAFields]

// SentenceFormat returns the NMEA sentence format for GGA.
func (GGAFields) SentenceFormat() string { return "GGA" }

// MakeGGA builds a complete GGA sentence from a synthesized fix. Hemispheres
// come from the signs of lat and lon; height and differential fields are left
// unset.
func MakeGGA(talker string, t time.Time, lat, lon float64, quality, numSats uint8, hdop float64) Sentence[GGAFields] {
	f := GGAFields{
		Time:    opt.Make(makeTodUTC(t)),
		LatSign: 1,
		LonSign: 1,
		Quality: uint8Dec(quality),
		NumSats: opt.Make(uint8Dec2(numSats)),
		HDOP:    opt.Make(float32(hdop)),
	}
	if lat < 0 {
		f.LatSign, lat = -1, -lat
	}
	if lon < 0 {
		f.LonSign, lon = -1, -lon
	}
	f.Lat = opt.Make(latCoord{makeCoord(lat)})
	f.Lon = opt.Make(lonCoord{makeCoord(lon)})
	return Sentence[GGAFields]{talkerID: talker, Fields: f}
}

// LatLon recombines the magnitude and hemisphere fields into signed degrees.
// Unset coordinate fields yield 0.
func (g GGAFields) LatLon() (lat, lon float64) {
	if g.Lat.IsSet() {
		lat = g.Lat.Get().degrees()
		if g.LatSign < 0 {
			lat = -lat
		}
	}
	if g.Lon.IsSet() {
		lon = g.Lon.Get().degrees()
		if g.LonSign < 0 {
			lon = -lon
		}
	}
	return lat, lon
}

// RMCFields is the typed field set of an RMC sentence (NMEA 4.00 8.3.67). The
// final NavStatus field is present only on NMEA 4.x output and is left unset
// otherwise.
type RMCFields struct {
	Time    opt.Val[todUTC]   `json:"time,omitzero"`
	Status  string            `json:"status,omitempty"`
	Lat     opt.Val[latCoord] `json:"lat,omitzero"`
	LatSign latSign           `json:"latSign,omitzero"`
	Lon     opt.Val[lonCoord] `json:"lon,omitzero"`
	LonSign lonSign           `json:"lonSign,omitzero"`
	Speed   opt.Val[float64]  `json:"speed,omitzero"`  // knots
	Course  opt.Val[float64]  `json:"course,omitzero"` // degrees, true
	Date    opt.Val[dateDMY]  `json:"date,omitzero"`
	// MagVar is the magnetic variation magnitude in degrees; MagVarSign is its
	// direction, +1 for E and -1 for W (the same convention as LonSign). The
	// magnetic course is the true Course less the signed variation:
	// magneticCourse = Course - MagVar*MagVarSign.
	MagVar     opt.Val[float64] `json:"magVar,omitzero"`
	MagVarSign lonSign          `json:"magVarSign,omitzero"`
	Mode       string           `json:"mode,omitempty"`
	NavStatus  string           `json:"navStatus,omitempty"`
}

// SentenceFormat returns the NMEA sentence format for RMC.
func (RMCFields) SentenceFormat() string { return "RMC" }

// Timestamp combines the date and time-of-day into an absolute UTC time. It
// returns ok == false when either field is absent. A leap second collapses
// into the following minute, the inherent limit of time.Time.
func (r RMCFields) Timestamp() (time.Time, bool) {
	if r.Date.IsZero() || r.Time.IsZero() {
		return time.Time{}, false
	}
	d := time.Time(r.Date.Get())
	h, m, s, cc := r.Time.Get().clock()
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, s, cc*10000000, time.UTC), true
}

// todUTC is a UTC time-of-day held as centiseconds since midnight. The
// representation has no date, so it composes with a separate date field, and
// it can hold a leap second (23:59:60) that time.Time cannot.
type todUTC int32

// makeTodUTC converts the time-of-day of t (in UTC) to a todUTC.
func makeTodUTC(t time.Time) todUTC {
	t = t.UTC()
	h, m, s := t.Clock()
	return todUTC(h*360000 + m*6000 + s*100 + t.Nanosecond()/10000000)
}

// clock splits the centisecond count into wall-clock parts. The end-of-day
// boundary (24:00:00) is rendered as the leap second 23:59:60.
func (t todUTC) clock() (h, m, s, cc int) {
	n := int(t)
	cc = n % 100
	n /= 100
	s = n % 60
	n /= 60
	m = n % 60
	h = n / 60
	if h == 24 {
		h, m, s = 23, 59, 60
	}
	return h, m, s, cc
}

// MarshalText implements encoding.TextMarshaler, producing "hhmmss.ss".
func (t todUTC) MarshalText() ([]byte, error) {
	h, m, s, cc := t.clock()
	return fmt.Appendf(nil, "%02d%02d%02d.%02d", h, m, s, cc), nil
}

// UnmarshalText implements encoding.TextUnmarshaler. The fraction is taken to
// centisecond resolution to match the "ss" field.
func (t *todUTC) UnmarshalText(b []byte) error {
	hms, frac, _ := strings.Cut(string(b), ".")
	cc, ok := parseFixed(frac, 2)
	if len(hms) != 6 || !isAllDigits(hms) || !ok {
		return fmt.Errorf("invalid time-of-day %q", b)
	}
	h, _ := strconv.Atoi(hms[0:2])
	m, _ := strconv.Atoi(hms[2:4])
	s, _ := strconv.Atoi(hms[4:6])
	if h > 23 || m > 59 || s > 60 || (s == 60 && (h != 23 || m != 59)) {
		return fmt.Errorf("invalid time-of-day %q", b)
	}
	*t = todUTC(h*360000 + m*6000 + s*100 + int(cc))
	return nil
}

// MarshalJSON renders the time-of-day as a readable "hh:mm:ss.ss" string.
func (t todUTC) MarshalJSON() ([]byte, error) {
	h, m, s, cc := t.clock()
	return json.Marshal(fmt.Sprintf("%02d:%02d:%02d.%02d", h, m, s, cc))
}

// coord is one NMEA latitude or longitude magnitude, mirroring the wire form
// exactly: whole degrees plus minutes as fixed-point scaled by 1e5, so
// 0 <= min < 60e5. This keeps wire round-tripping exact with no float.
type coord struct {
	deg uint16
	min int64
}

// minScale is the fixed-point scale of coord.min (5 fractional digits).
const minScale = 100000

// degrees returns the magnitude as decimal degrees.
func (c coord) degrees() float64 {
	return float64(c.deg) + float64(c.min)/(60*minScale)
}

// MarshalJSON renders the magnitude as a {deg, min} object: integer degrees
// and float64 minutes. The sign (hemisphere) lives in the sibling N/S or E/W
// field.
func (c coord) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Deg uint16  `json:"deg"`
		Min float64 `json:"min"`
	}{c.deg, float64(c.min) / minScale})
}

// marshal formats the coord with deg zero-padded to degWidth digits.
func (c coord) marshal(degWidth int) []byte {
	return fmt.Appendf(nil, "%0*d%02d.%05d", degWidth, c.deg, c.min/minScale, c.min%minScale)
}

// unmarshal parses a coord whose degree part is degWidth digits wide.
// Fractional minutes beyond 5 digits are truncated to the 1e5 scale.
func (c *coord) unmarshal(text string, degWidth int) error {
	intPart, frac, _ := strings.Cut(text, ".")
	fracMin, ok := parseFixed(frac, 5)
	if len(intPart) != degWidth+2 || !isAllDigits(intPart) || !ok {
		return fmt.Errorf("invalid coordinate %q", text)
	}
	wholeMin, _ := strconv.ParseUint(intPart[degWidth:], 10, 8)
	if wholeMin >= 60 {
		return fmt.Errorf("minutes out of range in coordinate %q", text)
	}
	deg, _ := strconv.ParseUint(intPart[:degWidth], 10, 16)
	c.deg = uint16(deg)
	c.min = int64(wholeMin)*minScale + fracMin
	return nil
}

// makeCoord splits absolute decimal degrees into the exact coord wire
// representation, carrying rounded 60.00000 minutes up into degrees.
func makeCoord(absDeg float64) coord {
	deg := uint16(absDeg)
	min := int64(math.Round((absDeg - float64(deg)) * 60 * minScale))
	if min >= 60*minScale {
		deg++
		min -= 60 * minScale
	}
	return coord{deg: deg, min: min}
}

// latCoord is a 2-digit-degree latitude magnitude ("ddmm.mmmmm").
type latCoord struct{ coord }

func (c latCoord) MarshalText() ([]byte, error)  { return c.marshal(2), nil }
func (c *latCoord) UnmarshalText(b []byte) error { return c.unmarshal(string(b), 2) }

// lonCoord is a 3-digit-degree longitude magnitude ("dddmm.mmmmm").
type lonCoord struct{ coord }

func (c lonCoord) MarshalText() ([]byte, error)  { return c.marshal(3), nil }
func (c *lonCoord) UnmarshalText(b []byte) error { return c.unmarshal(string(b), 3) }

// sign encodes a hemisphere as +1 or -1, or 0 when the field is absent. The
// latSign and lonSign specializations differ only in their letter pair, and
// both serialize as a signed number in JSON.
type sign int8

func (s sign) text(pos, neg byte) []byte {
	switch s {
	case 1:
		return []byte{pos}
	case -1:
		return []byte{neg}
	}
	return nil
}

func (s *sign) parse(text []byte, pos, neg byte) error {
	switch {
	case len(text) == 0:
		*s = 0
	case len(text) == 1 && text[0] == pos:
		*s = 1
	case len(text) == 1 && text[0] == neg:
		*s = -1
	default:
		return fmt.Errorf("invalid hemisphere %q", text)
	}
	return nil
}

// latSign is the latitude hemisphere: +1 for N, -1 for S.
type latSign sign

func (s latSign) MarshalText() ([]byte, error)  { return sign(s).text('N', 'S'), nil }
func (s *latSign) UnmarshalText(b []byte) error { return (*sign)(s).parse(b, 'N', 'S') }
func (s latSign) MarshalJSON() ([]byte, error)  { return json.Marshal(sign(s)) }

// lonSign is the longitude hemisphere: +1 for E, -1 for W.
type lonSign sign

func (s lonSign) MarshalText() ([]byte, error)  { return sign(s).text('E', 'W'), nil }
func (s *lonSign) UnmarshalText(b []byte) error { return (*sign)(s).parse(b, 'E', 'W') }
func (s lonSign) MarshalJSON() ([]byte, error)  { return json.Marshal(sign(s)) }

// dateDMY is an NMEA calendar date ("ddmmyy"). A date has no leap-second
// hazard, so time.Time is a safe underlying type: it gives validated parsing,
// the two-digit-year century rule (the spec defines none; time uses 69-99 ->
// 1900s, 00-68 -> 2000s), and a direct combine with the time-of-day.
type dateDMY time.Time

func (d dateDMY) MarshalText() ([]byte, error) {
	return []byte(time.Time(d).Format("020106")), nil
}

func (d *dateDMY) UnmarshalText(b []byte) error {
	t, err := time.Parse("020106", string(b))
	if err != nil {
		return fmt.Errorf("invalid date %q", b)
	}
	*d = dateDMY(t)
	return nil
}

// MarshalJSON renders the date as an ISO "yyyy-mm-dd" string.
func (d dateDMY) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(d).Format(time.DateOnly))
}

// uint8Dec is a base-10 uint8 wire field with no fixed width. The custom type
// forces decimal parsing; fieldenc parses bare uints with base 0, which would
// misread zero-padded values like "08" as octal.
type uint8Dec uint8

func (n uint8Dec) MarshalText() ([]byte, error) { return strconv.AppendUint(nil, uint64(n), 10), nil }
func (n uint8Dec) MarshalJSON() ([]byte, error) { return json.Marshal(uint8(n)) }

func (n *uint8Dec) UnmarshalText(b []byte) error {
	v, err := strconv.ParseUint(string(b), 10, 8)
	*n = uint8Dec(v)
	return err
}

// uint8Dec2 is a base-10 uint8 wire field serialized zero-padded to two digits
// (NMEA "xx").
type uint8Dec2 uint8

func (n uint8Dec2) MarshalText() ([]byte, error) { return fmt.Appendf(nil, "%02d", uint8(n)), nil }
func (n uint8Dec2) MarshalJSON() ([]byte, error) { return json.Marshal(uint8(n)) }

func (n *uint8Dec2) UnmarshalText(b []byte) error {
	v, err := strconv.ParseUint(string(b), 10, 8)
	*n = uint8Dec2(v)
	return err
}

// uint16Dec4 is a base-10 uint16 wire field serialized zero-padded to four
// digits (NMEA "xxxx").
type uint16Dec4 uint16

func (n uint16Dec4) MarshalText() ([]byte, error) { return fmt.Appendf(nil, "%04d", uint16(n)), nil }
func (n uint16Dec4) MarshalJSON() ([]byte, error) { return json.Marshal(uint16(n)) }

func (n *uint16Dec4) UnmarshalText(b []byte) error {
	v, err := strconv.ParseUint(string(b), 10, 16)
	*n = uint16Dec4(v)
	return err
}

// parseFixed parses an optional decimal fraction (the part after the point)
// into a fixed-point integer with width digits, truncating extra digits and
// right-padding short ones. An empty fraction yields 0.
func parseFixed(frac string, width int) (int64, bool) {
	if frac == "" {
		return 0, true
	}
	if !isAllDigits(frac) {
		return 0, false
	}
	if len(frac) > width {
		frac = frac[:width]
	} else {
		frac += strings.Repeat("0", width-len(frac))
	}
	n, _ := strconv.ParseInt(frac, 10, 64)
	return n, true
}

// isAllDigits reports whether s is non-empty and all ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// hexByte combines two ASCII hex digits into a byte.
func hexByte(hi, lo byte) byte {
	return hexVal(hi)<<4 | hexVal(lo)
}

func hexVal(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	}
	return 0
}
