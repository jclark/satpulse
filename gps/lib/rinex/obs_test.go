package rinex

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/pelletier/go-toml/v2"
)

func testPtr[T any](v T) *T {
	return &v
}

func TestSignalObservationJSON(t *testing.T) {
	obs := SignalObservation{
		T:   mustTime(t, "2025-06-30T23:59:59.1234567"),
		Sat: SatelliteID("G03"),
		Sig: SignalID("1C"),
		SignalValues: SignalValues{
			Frq: opt.Make(int8(-4)),
			PR:  opt.Make(22187868.655),
			CP:  opt.Make(116598092.035),
			CN0: opt.Make(float32(48.5)),
			LLI: opt.Make(LLILostLock),
		},
	}
	b, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"t":"2025-06-30T23:59:59.1234567","sat":"G03","sig":"1C","frq":-4,"pr":22187868.655,"cp":116598092.035,"cn0":48.5,"lli":1}`
	if string(b) != want {
		t.Errorf("Marshal = %s, want %s", string(b), want)
	}
	var got SignalObservation
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got != obs {
		t.Errorf("round trip = %#v, want %#v", got, obs)
	}
}

func TestSignalValuesIsZero(t *testing.T) {
	for _, tt := range []struct {
		name string
		v    SignalValues
		want bool
	}{
		{name: "zero", want: true},
		{name: "frq", v: SignalValues{Frq: opt.Make(int8(-4))}},
		{name: "pr", v: SignalValues{PR: opt.Make(1.0)}},
		{name: "cp", v: SignalValues{CP: opt.Make(1.0)}},
		{name: "do", v: SignalValues{Do: opt.Make(1.0)}},
		{name: "cn0", v: SignalValues{CN0: opt.Make(float32(1.0))}},
		{name: "lli zero", v: SignalValues{LLI: opt.Make(LLI(0))}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.IsZero(); got != tt.want {
				t.Errorf("IsZero = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetadataJSON(t *testing.T) {
	meta := Metadata{
		Receiver: Receiver{
			Number:  "4503038",
			Type:    "GNSS_RECEIVER",
			Version: "5.4.0",
		},
		Antenna: Antenna{
			Number: "5644",
			Type:   "GEOANTENNA NONE",
		},
		ApproxPosition: testPtr([3]float64{4091423.719, 368380.653, 4863179.994}),
		LeapSeconds:    testPtr(int16(18)),
	}
	meta.Marker.Name = "REDU00BEL"
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"marker":{"name":"REDU00BEL"},"receiver":{"number":"4503038","type":"GNSS_RECEIVER","version":"5.4.0"},"antenna":{"number":"5644","type":"GEOANTENNA NONE"},"approxPosition":[4091423.719,368380.653,4863179.994],"leapSeconds":18}`
	if string(b) != want {
		t.Errorf("Marshal = %s, want %s", string(b), want)
	}
	var got Metadata
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Marker.Name != meta.Marker.Name || got.Receiver.Number != meta.Receiver.Number ||
		got.Antenna.Type != meta.Antenna.Type || *got.ApproxPosition != *meta.ApproxPosition ||
		*got.LeapSeconds != *meta.LeapSeconds {
		t.Errorf("round trip = %#v, want %#v", got, meta)
	}
}

func TestMetadataTOML(t *testing.T) {
	data := []byte(`marker.name = "SITE"
observer = "JJ"
agency = "SatPulse"
comment = """
first
second
"""
approxPosition = [-1144697.9559, 6090335.6106, 1504171.2880]
antennaDelta = [1.2, 0.3, 0.4]
interval = 30.0
leapSeconds = 18

[receiver]
number = "RX001"
type = "ZED-X20P"
version = "HPG 2.02"

[antenna]
number = "ANT001"
type = "ANTMODEL NONE"
`)
	var got Metadata
	if err := toml.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Marker.Name != "SITE" || got.Observer != "JJ" || got.Agency != "SatPulse" {
		t.Errorf("metadata strings = %#v", got)
	}
	if len(got.Comment) != 2 || got.Comment[0] != "first" || got.Comment[1] != "second" {
		t.Errorf("comment = %#v", got.Comment)
	}
	if got.Receiver.Number != "RX001" || got.Receiver.Type != "ZED-X20P" || got.Receiver.Version != "HPG 2.02" {
		t.Errorf("receiver = %#v", got.Receiver)
	}
	if got.Antenna.Number != "ANT001" || got.Antenna.Type != "ANTMODEL NONE" {
		t.Errorf("antenna = %#v", got.Antenna)
	}
	pos := [3]float64{-1144697.9559, 6090335.6106, 1504171.2880}
	if got.ApproxPosition == nil || *got.ApproxPosition != pos {
		t.Errorf("approx position = %v, want %v", got.ApproxPosition, pos)
	}
	delta := [3]float64{1.2, 0.3, 0.4}
	if got.AntennaDelta == nil || *got.AntennaDelta != delta {
		t.Errorf("antenna delta = %v, want %v", got.AntennaDelta, delta)
	}
	if got.Interval == nil || *got.Interval != 30 {
		t.Errorf("interval = %v, want 30", got.Interval)
	}
	if got.LeapSeconds == nil || *got.LeapSeconds != 18 {
		t.Errorf("leap seconds = %v, want 18", got.LeapSeconds)
	}
}

func TestUnmarshalRecord(t *testing.T) {
	obs, meta, isObs, err := UnmarshalRecord([]byte(`{"t":"2025-06-30T23:59:59.0000000","sat":"R03","sig":"1C","frq":-4,"pr":22187868.655}`))
	if err != nil {
		t.Fatalf("UnmarshalRecord observation error: %v", err)
	}
	if !isObs {
		t.Fatal("UnmarshalRecord observation isObs = false, want true")
	}
	if obs.Sat != "R03" || obs.Frq.Get() != -4 || !obs.PR.IsSet() {
		t.Errorf("observation = %#v", obs)
	}
	if meta.Marker.Name != "" {
		t.Errorf("metadata for observation = %#v", meta)
	}
	obs, meta, isObs, err = UnmarshalRecord([]byte(`{"antenna":{"type":"ROVER"},"antennaDelta":[0.903,0,0]}`))
	if err != nil {
		t.Fatalf("UnmarshalRecord metadata error: %v", err)
	}
	if isObs {
		t.Fatal("UnmarshalRecord metadata isObs = true, want false")
	}
	if meta.Antenna.Type != "ROVER" || meta.AntennaDelta == nil || meta.AntennaDelta[0] != 0.903 {
		t.Errorf("metadata = %#v", meta)
	}
	if obs.T != 0 {
		t.Errorf("observation for metadata = %#v", obs)
	}
	_, _, _, err = UnmarshalRecord([]byte(`{"t":"2025-06-30T23:59:59.0000000","sat":"R03","sig":"1C","ssi":7}`))
	if err == nil {
		t.Fatal("UnmarshalRecord accepted unsupported ssi field")
	}
}

func TestObsJSONSinkAndReadObsJSON(t *testing.T) {
	var b bytes.Buffer
	sink := NewObsJSONSink(&b)
	if err := sink.Metadata(Metadata{Comment: Lines{"from first"}, Receiver: Receiver{Type: "RX"}}); err != nil {
		t.Fatalf("Metadata first: %v", err)
	}
	obs := SignalObservation{
		T:   mustTime(t, "2025-06-30T23:59:59.0000000"),
		Sat: "G03",
		Sig: "1C",
		SignalValues: SignalValues{
			PR:  opt.Make(22187868.655),
			LLI: opt.Make(LLI(0)),
		},
	}
	if err := sink.Observation(obs); err != nil {
		t.Fatalf("Observation: %v", err)
	}
	meta := Metadata{Comment: Lines{"from second"}}
	meta.Marker.Name = "MARK"
	if err := sink.Metadata(meta); err != nil {
		t.Fatalf("Metadata second: %v", err)
	}
	if err := sink.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	want := strings.Join([]string{
		`{"comment":["from first"],"receiver":{"type":"RX"}}`,
		`{"t":"2025-06-30T23:59:59.0000000","sat":"G03","sig":"1C","pr":22187868.655,"lli":0}`,
		`{"comment":["from second"],"marker":{"name":"MARK"}}`,
		"",
	}, "\n")
	if b.String() != want {
		t.Fatalf("obsj output = %q, want %q", b.String(), want)
	}
	meta, got, err := ReadObsJSON(strings.NewReader(" \n" + b.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v", err)
	}
	if meta.Marker.Name != "MARK" || meta.Receiver.Type != "RX" {
		t.Errorf("metadata = %#v", meta)
	}
	if len(meta.Comment) != 2 || meta.Comment[0] != "from first" || meta.Comment[1] != "from second" {
		t.Errorf("metadata comments = %#v", meta.Comment)
	}
	if len(got) != 1 || got[0] != obs {
		t.Errorf("observations = %#v, want %#v", got, []SignalObservation{obs})
	}
}

func TestReadObsJSONLineError(t *testing.T) {
	_, _, err := ReadObsJSON(strings.NewReader(`{"t":"2025-06-30T23:59:59.0000000","sat":"X03","sig":"1C"}`))
	if err == nil {
		t.Fatal("ReadObsJSON invalid observation succeeded")
	}
	if !strings.Contains(err.Error(), "obsj line 1") {
		t.Fatalf("error = %v, want line number", err)
	}
}

func TestObservationCodes(t *testing.T) {
	obs := SignalObservation{
		Sig: SignalID("5Q"),
		SignalValues: SignalValues{
			PR:  opt.Make(21474836.125),
			CP:  opt.Make(112859323.25),
			Do:  opt.Make(-1234.5),
			CN0: opt.Make(float32(42.0)),
		},
	}
	got := obs.ObservationCodes()
	want := []ObservationCode{"C5Q", "L5Q", "D5Q", "S5Q"}
	if !slices.Equal(got, want) {
		t.Errorf("ObservationCodes = %v, want %v", got, want)
	}
}

func TestObservationCodeSet(t *testing.T) {
	obs := []SignalObservation{
		{
			Sat: SatelliteID("G03"),
			Sig: SignalID("5Q"),
			SignalValues: SignalValues{
				PR: opt.Make(21474836.125),
				CP: opt.Make(112859323.25),
			},
		},
		{
			Sat:          SatelliteID("G03"),
			Sig:          SignalID("1C"),
			SignalValues: SignalValues{CN0: opt.Make(float32(48.5))},
		},
		{
			Sat:          SatelliteID("E11"),
			Sig:          SignalID("1C"),
			SignalValues: SignalValues{PR: opt.Make(24123456.0)},
		},
	}
	got := ObservationCodeSet(obs)
	wantGPS := []ObservationCode{"S1C", "C5Q", "L5Q"}
	wantGAL := []ObservationCode{"C1C"}
	if !slices.Equal(got["G"], wantGPS) {
		t.Errorf("ObservationCodeSet G = %v, want %v", got["G"], wantGPS)
	}
	if !slices.Equal(got["E"], wantGAL) {
		t.Errorf("ObservationCodeSet E = %v, want %v", got["E"], wantGAL)
	}
	if !IsMixed(obs) {
		t.Error("IsMixed = false, want true")
	}
}

func TestTimeText(t *testing.T) {
	testCases := []struct {
		name string
		time Time
		text string
	}{
		{"epoch", 0, "1980-01-06T00:00:00.0000000"},
		{"tick after epoch", 1, "1980-01-06T00:00:00.0000001"},
		{"tick before epoch", -1, "1980-01-05T23:59:59.9999999"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.time.String(); got != tc.text {
				t.Errorf("String = %s, want %s", got, tc.text)
			}
			var got Time
			if err := got.UnmarshalText([]byte(tc.text)); err != nil {
				t.Fatalf("UnmarshalText error: %v", err)
			}
			if got != tc.time {
				t.Errorf("UnmarshalText = %d, want %d", got, tc.time)
			}
			b, err := json.Marshal(tc.time)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			want := `"` + tc.text + `"`
			if string(b) != want {
				t.Errorf("Marshal = %s, want %s", string(b), want)
			}
		})
	}
	var got Time
	if err := got.UnmarshalText([]byte("1980-01-06T00:00:00.0000000Z")); err == nil {
		t.Fatal("UnmarshalText with timezone suffix succeeded")
	}
}

func TestGPSWeekMillis(t *testing.T) {
	tm := TimeFromGPSWeekMillis(1, 1234)
	if got := tm.String(); got != "1980-01-13T00:00:01.2340000" {
		t.Errorf("String = %s, want 1980-01-13T00:00:01.2340000", got)
	}
	week, towMS := tm.GPSWeekMillis()
	if week != 1 || towMS != 1234 {
		t.Errorf("GPSWeekMillis = %d, %d, want 1, 1234", week, towMS)
	}
	week, towMS = Time(10005).GPSWeekMillis()
	if week != 0 || towMS != 1 {
		t.Errorf("GPSWeekMillis floors = %d, %d, want 0, 1", week, towMS)
	}
}

func TestGPSTimeText(t *testing.T) {
	ls := ptime.LeapSecond2016()
	utc := ptime.UTC(2023, 10, 6, 23, 40, 9, 0)
	pt := ls.UTCtoTime(utc)
	if gps := ptime.GPS(2282, 517227*time.Second); gps != pt {
		t.Fatalf("ptime.GPS = %v, want %v", gps, pt)
	}
	rt := TimeFromGPSWeekMillis(2282, 517227000)
	if got := rt.String(); got != "2023-10-06T23:40:27.0000000" {
		t.Errorf("GPS String = %s, want 2023-10-06T23:40:27.0000000", got)
	}
}

func TestCivilTime(t *testing.T) {
	if got := Time(0).CivilTime(); !got.Equal(Epoch) {
		t.Errorf("Time(0).CivilTime = %v, want %v", got, Epoch)
	}
	if got := TimeFromCivilTime(Epoch); got != 0 {
		t.Errorf("TimeFromCivilTime(Epoch) = %d, want 0", got)
	}
	if got := TimeFromCivilTime(Epoch.Add(100 * time.Nanosecond)); got != 1 {
		t.Errorf("TimeFromCivilTime(Epoch + 100ns) = %d, want 1", got)
	}
	if got := TimeFromCivilTime(Epoch.Add(-1 * time.Nanosecond)); got != -1 {
		t.Errorf("TimeFromCivilTime(Epoch - 1ns) = %d, want -1", got)
	}
	if got := TimeFromCivilTime(Epoch.In(time.FixedZone("TEST", 7*3600))); got != 0 {
		t.Errorf("TimeFromCivilTime(Epoch in fixed zone) = %d, want 0", got)
	}
	now := FloorTime(time.Now())
	rt := TimeFromCivilTime(now)
	got, err := time.ParseInLocation(timeLayout, rt.String(), time.UTC)
	if err != nil {
		t.Fatalf("ParseInLocation error: %v", err)
	}
	if !got.Equal(now) {
		t.Errorf("civil time text round trip = %v, want %v", got, now)
	}
}

func TestIDsAreValid(t *testing.T) {
	satCases := []struct {
		id   SatelliteID
		want bool
	}{
		{"G03", true},
		{"E11", true},
		{"R00", false},
		{"X01", false},
		{"G3", false},
	}
	for _, tc := range satCases {
		if got := tc.id.IsValid(); got != tc.want {
			t.Errorf("%q IsValid = %v, want %v", tc.id, got, tc.want)
		}
	}
	sigCases := []struct {
		id   SignalID
		want bool
	}{
		{"1C", true},
		{"5Q", true},
		{"12", false},
		{"C1C", false},
		{"", false},
	}
	for _, tc := range sigCases {
		if got := tc.id.IsValid(); got != tc.want {
			t.Errorf("%q IsValid = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestInvalidIDUnmarshal(t *testing.T) {
	var obs SignalObservation
	if err := json.Unmarshal([]byte(`{"t":"2025-06-30T23:59:59.0000000","sat":"X01","sig":"1C"}`), &obs); err == nil {
		t.Fatal("Unmarshal invalid satellite succeeded")
	}
	if err := json.Unmarshal([]byte(`{"t":"2025-06-30T23:59:59.0000000","sat":"G01","sig":"C1C"}`), &obs); err == nil {
		t.Fatal("Unmarshal invalid signal succeeded")
	}
}

func mustTime(t *testing.T, s string) Time {
	t.Helper()
	var tm Time
	if err := tm.UnmarshalText([]byte(s)); err != nil {
		t.Fatalf("UnmarshalText(%q): %v", s, err)
	}
	return tm
}
