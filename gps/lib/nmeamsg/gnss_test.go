package nmeamsg

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

// Helpers to build expected typed fields and to drive the parser the way
// nmeaDecode does (compute flags, pass the payload between $ and *).

func tod(h, m, s, cc int) opt.Val[todUTC] {
	return opt.Make(todUTC(h*360000 + m*6000 + s*100 + cc))
}
func lat(deg uint16, min int64) opt.Val[latCoord] { return opt.Make(latCoord{coord{deg, min}}) }
func lon(deg uint16, min int64) opt.Val[lonCoord] { return opt.Make(lonCoord{coord{deg, min}}) }
func f32(v float32) opt.Val[float32]              { return opt.Make(v) }
func f64(v float64) opt.Val[float64]              { return opt.Make(v) }
func u8(v uint8) opt.Val[uint8Dec2]               { return opt.Make(uint8Dec2(v)) }
func u16(v uint16) opt.Val[uint16Dec4]            { return opt.Make(uint16Dec4(v)) }

func frame(payload string) string {
	return fmt.Sprintf("$%s*%02X\r\n", payload, Checksum([]byte(payload)))
}

func parseSentence(s string) (GNSSTalkerIDMsg, error) {
	star := strings.IndexByte(s, '*')
	if star < 0 {
		return nil, fmt.Errorf("no checksum in %q", s)
	}
	return ParseGNSSTalkerPayload(s[1:star], CheckSyntax(s))
}

func tm(h, m, s, cc int) time.Time {
	return time.Date(2026, 6, 24, h, m, s, cc*10000000, time.UTC)
}
func pu8(v uint8) *uint8      { return &v }
func pf64(v float64) *float64 { return &v }

// TestParseGGA parses captured receiver GGA and checks the typed fields,
// including the no-fix and time-only cases, an 8-digit Unicore fix (minutes
// quantized to the 1e5 coord scale), and populated DGPS fields.
func TestParseGGA(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		talker string
		expect GGAFields
	}{
		{
			name:   "no fix, empty time",
			in:     "$GPGGA,,,,,,0,00,99.99,,,,,,*48\r\n",
			talker: "GP",
			expect: GGAFields{NumSats: u8(0), HDOP: f32(99.99)},
		},
		{
			name:   "time but no position",
			in:     "$GNGGA,060713.00,,,,,0,06,4.88,,,,,,*49\r\n",
			talker: "GN",
			expect: GGAFields{Time: tod(6, 7, 13, 0), NumSats: u8(6), HDOP: f32(4.88)},
		},
		{
			name:   "full u-blox fix, 5-digit minutes",
			in:     "$GAGGA,060810.00,1343.90957,N,10038.68814,E,1,05,2.50,-1.2,M,-27.4,M,,*4C\r\n",
			talker: "GA",
			expect: GGAFields{
				Time: tod(6, 8, 10, 0), Lat: lat(13, 4390957), LatSign: 1, Lon: lon(100, 3868814), LonSign: 1,
				Quality: 1, NumSats: u8(5), HDOP: f32(2.5), Alt: f64(-1.2), AltUnit: "M", GeoidSep: f64(-27.4), GeoidUnit: "M",
			},
		},
		{
			name:   "unicore 8-digit DGPS fix with age and station id",
			in:     "$GNGGA,081015.000,4911.69099090,N,00228.79721128,E,2,36,0.34,56.718,M,44.686,M,5.0,9002*69\r\n",
			talker: "GN",
			expect: GGAFields{
				Time: tod(8, 10, 15, 0), Lat: lat(49, 1169099), LatSign: 1, Lon: lon(2, 2879721), LonSign: 1,
				Quality: 2, NumSats: u8(36), HDOP: f32(0.34), Alt: f64(56.718), AltUnit: "M", GeoidSep: f64(44.686), GeoidUnit: "M",
				DGPSAge: f32(5.0), DGPSID: u16(9002),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseSentence(tc.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if m.TalkerID() != tc.talker {
				t.Errorf("talker = %q, want %q", m.TalkerID(), tc.talker)
			}
			got, ok := m.Body().(GGAFields)
			if !ok {
				t.Fatalf("body is %T, want GGAFields", m.Body())
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

// TestSerializeGGA checks MakeGGA + SerializeMsg produce exact golden bytes
// for known positions in each hemisphere, including the zero/no-fix case.
func TestSerializeGGA(t *testing.T) {
	tests := []struct {
		name   string
		in     GNSSTalkerIDMsg
		expect string
	}{
		{
			name:   "north-east",
			in:     MakeGGA("GP", tm(12, 34, 56, 0), &[2]float64{13.731826167, 100.644802333}, nil, nil, 1, pu8(8), pf64(1.2)),
			expect: "$GPGGA,123456.00,1343.90957,N,10038.68814,E,1,08,1.2,,,,,,*5E\r\n",
		},
		{
			name:   "south-west",
			in:     MakeGGA("GP", tm(23, 59, 59, 99), &[2]float64{-33.8688, -151.2093}, nil, nil, 4, pu8(12), pf64(0.8)),
			expect: "$GPGGA,235959.99,3352.12800,S,15112.55800,W,4,12,0.8,,,,,,*5E\r\n",
		},
		{
			name:   "msl height and geoid separation",
			in:     MakeGGA("GP", tm(12, 34, 56, 0), &[2]float64{13.731826167, 100.644802333}, pf64(100), pf64(75.5), 1, pu8(8), pf64(1.2)),
			expect: frame("GPGGA,123456.00,1343.90957,N,10038.68814,E,1,08,1.2,75.5,M,24.5,M,,"),
		},
		{
			name:   "ellipsoid height without msl height",
			in:     MakeGGA("GP", tm(12, 34, 56, 0), &[2]float64{13.731826167, 100.644802333}, pf64(100), nil, 1, pu8(8), pf64(1.2)),
			expect: "$GPGGA,123456.00,1343.90957,N,10038.68814,E,1,08,1.2,,,,,,*5E\r\n",
		},
		{
			name:   "zero position, no fix with metadata",
			in:     MakeGGA("GN", tm(0, 0, 1, 50), &[2]float64{0, 0}, nil, nil, 0, pu8(0), pf64(9.9)),
			expect: "$GNGGA,000001.50,0000.00000,N,00000.00000,E,0,00,9.9,,,,,,*47\r\n",
		},
		{
			name:   "zero position, no fix without metadata",
			in:     MakeGGA("GN", tm(0, 0, 1, 50), &[2]float64{0, 0}, nil, nil, 0, nil, nil),
			expect: "$GNGGA,000001.50,0000.00000,N,00000.00000,E,0,,,,,,,,*69\r\n",
		},
		{
			name:   "absent position",
			in:     MakeGGA("GN", tm(0, 0, 1, 50), nil, pf64(100), pf64(75.5), 0, nil, nil),
			expect: "$GNGGA,000001.50,,,,,0,,,,,,,,*52\r\n",
		},
		{
			name:   "zero time leaves the time field empty",
			in:     MakeGGA("GN", time.Time{}, nil, nil, nil, 0, nil, nil),
			expect: frame("GNGGA,,,,,,0,,,,,,,,"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SerializeMsg(tc.in)
			if err != nil {
				t.Fatalf("serialize: %v", err)
			}
			if string(got) != tc.expect {
				t.Errorf("got  %q\nwant %q", got, tc.expect)
			}
		})
	}
}

// TestRoundtripGGA checks parse -> serialize -> parse. The canonical
// reserialization normalizes decimal spelling and quantizes minutes to the
// 1e5 scale, but parsing it back yields the same typed fields (idempotence).
func TestRoundtripGGA(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		canon string
	}{
		{
			name:  "already canonical, byte stable",
			in:    "$GNGGA,060628.00,1343.91029,N,10038.68390,E,1,08,4.34,12.2,M,-27.4,M,,*56\r\n",
			canon: "$GNGGA,060628.00,1343.91029,N,10038.68390,E,1,08,4.34,12.2,M,-27.4,M,,*56\r\n",
		},
		{
			name:  "trailing-zero decimals normalized",
			in:    "$GAGGA,060810.00,1343.90957,N,10038.68814,E,1,05,2.50,-1.2,M,-27.4,M,,*4C\r\n",
			canon: "$GAGGA,060810.00,1343.90957,N,10038.68814,E,1,05,2.5,-1.2,M,-27.4,M,,*7C\r\n",
		},
		{
			name:  "8-digit minutes and 3-digit time quantized",
			in:    "$GNGGA,081015.000,4911.69099090,N,00228.79721128,E,2,36,0.34,56.718,M,44.686,M,5.0,9002*69\r\n",
			canon: "$GNGGA,081015.00,4911.69099,N,00228.79721,E,2,36,0.34,56.718,M,44.686,M,5,9002*45\r\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m1, err := parseSentence(tc.in)
			if err != nil {
				t.Fatalf("parse in: %v", err)
			}
			out, err := SerializeMsg(m1)
			if err != nil {
				t.Fatalf("serialize: %v", err)
			}
			if string(out) != tc.canon {
				t.Errorf("canonical:\n got  %q\n want %q", out, tc.canon)
			}
			m2, err := parseSentence(string(out))
			if err != nil {
				t.Fatalf("parse out: %v", err)
			}
			if !reflect.DeepEqual(m1.Body(), m2.Body()) {
				t.Errorf("not idempotent:\n b1 %+v\n b2 %+v", m1.Body(), m2.Body())
			}
		})
	}
}

// TestParseRMC parses an RMC fix and a no-fix RMC, checks the typed fields,
// and verifies the date+time combine.
func TestParseRMC(t *testing.T) {
	in := "$GNRMC,114650.00,A,1343.90931561,N,10038.68511804,E,0.005,221.7,040525,0.5,W,A,C*59\r\n"
	m, err := parseSentence(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := RMCFields{
		Time: tod(11, 46, 50, 0), Status: "A", Lat: lat(13, 4390931), LatSign: 1, Lon: lon(100, 3868511), LonSign: 1,
		Speed: f64(0.005), Course: f64(221.7), Date: opt.Make(dateDMY(time.Date(2025, 5, 4, 0, 0, 0, 0, time.UTC))),
		MagVar: f64(0.5), MagVarSign: -1, Mode: "A", NavStatus: "C",
	}
	got := m.Body().(RMCFields)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	ts, ok := got.Timestamp()
	if !ok || !ts.Equal(time.Date(2025, 5, 4, 11, 46, 50, 0, time.UTC)) {
		t.Errorf("Timestamp() = %v, %v; want 2025-05-04 11:46:50 UTC", ts, ok)
	}

	m12, err := parseSentence("$GNRMC,050400.00,A,1343.91095,N,10038.68425,E,0.028,,240625,,,A*64\r\n")
	if err != nil {
		t.Fatalf("parse 12-field RMC: %v", err)
	}
	want12 := RMCFields{
		Time: tod(5, 4, 0, 0), Status: "A", Lat: lat(13, 4391095), LatSign: 1, Lon: lon(100, 3868425), LonSign: 1,
		Speed: f64(0.028), Date: opt.Make(dateDMY(time.Date(2025, 6, 24, 0, 0, 0, 0, time.UTC))), Mode: "A",
	}
	r12 := m12.Body().(RMCFields)
	if !reflect.DeepEqual(r12, want12) {
		t.Errorf("12-field RMC\n got  %+v\n want %+v", r12, want12)
	}
	if ts, ok := r12.Timestamp(); !ok || !ts.Equal(time.Date(2025, 6, 24, 5, 4, 0, 0, time.UTC)) {
		t.Errorf("Timestamp() = %v, %v; want 2025-06-24 05:04:00 UTC", ts, ok)
	}

	// No-fix RMC: status V and no position, but time and date are present,
	// so a timestamp is still available.
	m2, err := parseSentence("$GARMC,060651.00,V,,,,,,,150626,,,N,V*14\r\n")
	if err != nil {
		t.Fatalf("parse no-fix: %v", err)
	}
	r2 := m2.Body().(RMCFields)
	if r2.Status != "V" || r2.NavStatus != "V" || r2.Lat.IsSet() {
		t.Errorf("no-fix RMC = %+v, want status V, navStatus V, no position", r2)
	}
	if ts, ok := r2.Timestamp(); !ok || !ts.Equal(time.Date(2026, 6, 15, 6, 6, 51, 0, time.UTC)) {
		t.Errorf("Timestamp() = %v, %v; want 2026-06-15 06:06:51 UTC", ts, ok)
	}

	// Without time and date, there is no timestamp.
	m3, err := parseSentence(frame("GPRMC,,V,,,,,,,,,,N,V"))
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if ts, ok := m3.Body().(RMCFields).Timestamp(); ok {
		t.Errorf("expected no timestamp when time and date are empty, got %v", ts)
	}
}

// TestParseDispatch checks the routing contract: GGA/RMC decode, an
// unsupported GNSS-talker sentence returns ErrUnsupportedSentence, and a
// non-GNSS-talker sentence returns (nil, nil).
func TestParseDispatch(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantBody any
		wantErr  error
	}{
		{"gga", frame("GNGGA,,,,,,0,00,99.99,,,,,,"), GGAFields{}, nil},
		{"rmc", frame("GNRMC,114650.00,A,1343.9,N,10038.6,E,0,,040525,,,A,V"), RMCFields{}, nil},
		{"unsupported gsv", frame("GPGSV,1,1,01,01,40,083,41"), nil, ErrUnsupportedSentence},
		{"proprietary pubx", frame("PUBX,41,1"), nil, nil},
		{"non-gnss talker", frame("XXTXT,01,01,02,hello"), nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseSentence(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.wantBody == nil {
				if m != nil {
					t.Errorf("expected nil Msg, got %T", m.Body())
				}
				return
			}
			if reflect.TypeOf(m.Body()) != reflect.TypeOf(tc.wantBody) {
				t.Errorf("body is %T, want %T", m.Body(), tc.wantBody)
			}
		})
	}
}

// TestParseFieldCount rejects wrong field counts for known sentences. fieldenc
// tolerates short input, so a truncated GGA/RMC must be a malformed known
// message, not a half-populated struct.
func TestParseFieldCount(t *testing.T) {
	bad := []string{
		frame("GPGGA"),                                                  // address only
		frame("GPGGA,123456.00,1343.90957,N"),                           // truncated GGA
		frame("GPGGA,,,,,,0,00,99.99,,,,,,,"),                           // one field too many
		frame("GNRMC,114650.00,A"),                                      // truncated RMC
		frame("GNRMC,114650.00,A,1343.9,N,10038.6,E,0,,040525,,,A,V,X"), // too many
	}
	for _, in := range bad {
		m, err := parseSentence(in)
		if err == nil {
			t.Errorf("%q: expected field-count error, got %T", in, m.Body())
		}
	}
}

// TestGGAJSON checks the decode-facing JSON: readable values, spec field
// names, and unset fields omitted (no nulls, no Fields wrapper).
func TestGGAJSON(t *testing.T) {
	nofix := GGAFields{NumSats: u8(0), HDOP: f32(99.99)}
	got, err := json.Marshal(nofix)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"quality":0,"numSats":0,"hdop":99.99}`
	if string(got) != want {
		t.Errorf("no-fix JSON\n got  %s\n want %s", got, want)
	}

	// A full fix: check readable values and that unset DGPS fields are omitted.
	m, err := parseSentence("$GNGGA,061655.00,1343.91072,N,10038.68507,E,2,12,0.92,3.5,M,-27.4,M,,*6C\r\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, _ := json.Marshal(m.Body())
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if obj["time"] != "06:16:55.00" || obj["latSign"] != 1.0 || obj["quality"] != 2.0 || obj["altUnit"] != "M" {
		t.Errorf("unexpected fields: %v", obj)
	}
	if _, ok := obj["dgpsAge"]; ok {
		t.Errorf("dgpsAge should be omitted when unset: %s", b)
	}
	latObj, ok := obj["lat"].(map[string]any)
	if !ok || latObj["deg"] != 13.0 || math.Abs(latObj["min"].(float64)-43.91072) > 1e-5 {
		t.Errorf("lat = %v, want {deg:13, min:~43.91072}", obj["lat"])
	}
}

// TestTodUTC checks centisecond-since-midnight handling: round-trip,
// fractional-digit normalization, the leap second, and the JSON form.
func TestTodUTC(t *testing.T) {
	tests := []struct {
		in       string
		wantText string
		wantJSON string
	}{
		{"060810.00", "060810.00", `"06:08:10.00"`},
		{"235959.99", "235959.99", `"23:59:59.99"`},
		{"000000.05", "000000.05", `"00:00:00.05"`},
		{"081015.000", "081015.00", `"08:10:15.00"`}, // 3-digit fraction -> centiseconds
		{"120000", "120000.00", `"12:00:00.00"`},     // no fraction -> .00
		{"235960.00", "235960.00", `"23:59:60.00"`},  // leap second
	}
	for _, tc := range tests {
		var v todUTC
		if err := v.UnmarshalText([]byte(tc.in)); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", tc.in, err)
		}
		if text, _ := v.MarshalText(); string(text) != tc.wantText {
			t.Errorf("%q text -> %q, want %q", tc.in, text, tc.wantText)
		}
		if j, _ := v.MarshalJSON(); string(j) != tc.wantJSON {
			t.Errorf("%q json -> %s, want %s", tc.in, j, tc.wantJSON)
		}
	}
	for _, in := range []string{"240000.00", "126000.00", "120060.00", "235961.00"} {
		var v todUTC
		if err := v.UnmarshalText([]byte(in)); err == nil {
			t.Errorf("UnmarshalText(%q) unexpectedly succeeded as %q", in, v)
		}
	}
}

// TestCoord checks the degree/minute wire form and the {deg, min} JSON form,
// including the carry when rounded minutes reach 60.
func TestCoord(t *testing.T) {
	marshal := []struct {
		c        coord
		degWidth int
		text     string
		js       string
	}{
		{coord{13, 4390957}, 2, "1343.90957", `{"deg":13,"min":43.90957}`},
		{coord{100, 3868814}, 3, "10038.68814", `{"deg":100,"min":38.68814}`},
		{coord{13, 500000}, 2, "1305.00000", `{"deg":13,"min":5}`},        // minutes 5, zero-padded
		{coord{2, 2879721}, 3, "00228.79721", `{"deg":2,"min":28.79721}`}, // small 3-digit degrees
	}
	for _, tc := range marshal {
		if got := string(tc.c.marshal(tc.degWidth)); got != tc.text {
			t.Errorf("marshal(%+v, %d) = %q, want %q", tc.c, tc.degWidth, got, tc.text)
		}
		if j, _ := tc.c.MarshalJSON(); string(j) != tc.js {
			t.Errorf("MarshalJSON(%+v) = %s, want %s", tc.c, j, tc.js)
		}
	}

	// makeCoord carries rounded 60.00000 minutes up into the degree field.
	if got := makeCoord(30.99999995); got != (coord{31, 0}) {
		t.Errorf("makeCoord carry: got %+v, want {31 0}", got)
	}

	parse := []struct {
		in       string
		degWidth int
		expect   coord
	}{
		{"1343.90957", 2, coord{13, 4390957}},
		{"4911.69099090", 2, coord{49, 1169099}}, // 8 digits truncated to 5
		{"00228.7972", 3, coord{2, 2879720}},     // 4 digits right-padded
	}
	for _, tc := range parse {
		var c coord
		if err := c.unmarshal(tc.in, tc.degWidth); err != nil {
			t.Fatalf("unmarshal(%q): %v", tc.in, err)
		}
		if c != tc.expect {
			t.Errorf("unmarshal(%q) = %+v, want %+v", tc.in, c, tc.expect)
		}
	}
}

// TestLatLon checks that LatLon recovers the position MakeGGA encodes, across
// hemispheres, within the coord quantization tolerance.
func TestLatLon(t *testing.T) {
	const tol = 1.0 / (60 * minScale)
	tests := []struct {
		name     string
		lat, lon float64
	}{
		{"north-east", 13.731826167, 100.644802333},
		{"south-west", -33.8688, -151.2093},
		{"south-east", -0.0001, 0.0001},
		{"equator prime-meridian", 0, 0},
		{"high lat west", 71.123456, -179.987654},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := MakeGGA("GP", time.Time{}, &[2]float64{tc.lat, tc.lon}, nil, nil, 1, pu8(10), pf64(1.0)).Fields
			lat, lon := g.LatLon()
			if math.Abs(lat-tc.lat) > tol || math.Abs(lon-tc.lon) > tol {
				t.Errorf("got (%v, %v) want (%v, %v)", lat, lon, tc.lat, tc.lon)
			}
		})
	}
}

// TestWireUints checks the zero-padded decimal integer types parse base-10
// (not octal), serialize to fixed widths on the wire, and emit plain numbers
// in JSON.
func TestWireUints(t *testing.T) {
	var n2 uint8Dec2
	if err := n2.UnmarshalText([]byte("08")); err != nil || n2 != 8 {
		t.Fatalf("uint8Dec2 %q -> %d, %v", "08", n2, err)
	}
	if text, _ := n2.MarshalText(); string(text) != "08" {
		t.Errorf("uint8Dec2 text = %q, want 08", text)
	}
	if j, _ := n2.MarshalJSON(); string(j) != "8" {
		t.Errorf("uint8Dec2 json = %s, want 8", j)
	}

	var n4 uint16Dec4
	if err := n4.UnmarshalText([]byte("0001")); err != nil || n4 != 1 {
		t.Fatalf("uint16Dec4 %q -> %d, %v", "0001", n4, err)
	}
	if text, _ := n4.MarshalText(); string(text) != "0001" {
		t.Errorf("uint16Dec4 text = %q, want 0001", text)
	}
	if j, _ := n4.MarshalJSON(); string(j) != "1" {
		t.Errorf("uint16Dec4 json = %s, want 1", j)
	}
}

// TestDateDMY checks the ddmmyy wire form, the two-digit-year century rule,
// and the ISO "yyyy-mm-dd" JSON form.
func TestDateDMY(t *testing.T) {
	var d dateDMY
	if err := d.UnmarshalText([]byte("040525")); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !time.Time(d).Equal(time.Date(2025, 5, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date = %v, want 2025-05-04", time.Time(d))
	}
	if text, _ := d.MarshalText(); string(text) != "040525" {
		t.Errorf("text = %q, want 040525", text)
	}
	if j, _ := d.MarshalJSON(); string(j) != `"2025-05-04"` {
		t.Errorf("json = %s, want \"2025-05-04\"", j)
	}
	// Two-digit year 99 maps to 1999 (time package 69-99 rule).
	_ = d.UnmarshalText([]byte("311299"))
	if y := time.Time(d).Year(); y != 1999 {
		t.Errorf("year = %d, want 1999", y)
	}
}
