package convobscmd

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rinex"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/scan"
)

func testPtr[T any](v T) *T {
	return &v
}

func testMetaMarker(name string) rinex.Metadata {
	var meta rinex.Metadata
	meta.Marker.Name = name
	return meta
}

func testWriteMeta(date time.Time) rinex.Metadata {
	return rinex.Metadata{Run: rinex.MetadataRun{Program: "test", Date: date}}
}

func testLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, nil))
}

type packetInputFunc func(scan.Packet, WeekConstraint) (bool, error)

func (f packetInputFunc) ConvertPacket(pkt scan.Packet, week WeekConstraint) (bool, error) {
	return f(pkt, week)
}

func runInputs(inputs iter.Seq2[string, inputReader], out io.Writer, from inputFormat, to outputFormat, packetLog bool, meta rinex.Metadata, interval time.Duration) error {
	cj := convJob{
		inputs: inputs,
		out:    out,
		opts: convertOptions{
			from:      from,
			to:        to,
			packetLog: packetLog,
			interval:  interval,
		},
		meta: meta,
	}
	return cj.run(testLogger(io.Discard), time.Now().UTC())
}

func TestMetadataPrecedence(t *testing.T) {
	path := "header.toml"
	data := `marker.name = "FILE"
receiver.type = "RX"
antenna.type = "HEADERANT"
comment = "from file"
interval = 60
`
	v, _, err := parseFlags("", []string{
		"--header-file", path,
		"--interval", "30",
		"--program", "flag",
		"--run-by", "",
		"--antenna", "FLAGANT",
		"--approx-pos", "1,2,3",
		"--comment", "from flag",
		"input.ubx",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if v.headerFile != path {
		t.Fatalf("headerFile = %q, want %q", v.headerFile, path)
	}
	header, err := readHeaderFile(path, strings.NewReader(data))
	if err != nil {
		t.Fatalf("readHeaderFile: %v", err)
	}
	now := time.Date(2026, time.May, 19, 1, 2, 3, 0, time.UTC)
	meta := header
	setMetadataDefaults(&meta, now, v.interval)
	if err := applyMetadataFlags(&meta, v.metadataFlags); err != nil {
		t.Fatalf("applyMetadataFlags: %v", err)
	}
	if meta.Marker.Name != "FILE" {
		t.Errorf("Marker.Name = %q, want FILE", meta.Marker.Name)
	}
	if meta.Receiver.Type != "RX" {
		t.Errorf("Receiver.Type = %q, want RX", meta.Receiver.Type)
	}
	if meta.Run.Program != "flag" || meta.Run.By != "" || !meta.Run.Date.Equal(now) {
		t.Errorf("Run = %#v", meta.Run)
	}
	if meta.Antenna.Type != "FLAGANT" {
		t.Errorf("Antenna.Type = %q, want FLAGANT", meta.Antenna.Type)
	}
	if meta.ApproxPosition == nil || *meta.ApproxPosition != [3]float64{1, 2, 3} {
		t.Errorf("ApproxPosition = %#v", meta.ApproxPosition)
	}
	if len(meta.Comment) != 1 || meta.Comment[0] != "from flag" {
		t.Errorf("Comment = %#v", meta.Comment)
	}
	if meta.Interval == nil || *meta.Interval != 60 {
		t.Errorf("Interval = %v, want 60", meta.Interval)
	}
}

func TestParseFlagsRequiresInput(t *testing.T) {
	_, _, err := parseFlags("", nil)
	if err == nil {
		t.Fatal("parseFlags succeeded without input")
	}
	v, _, err := parseFlags("", []string{"a.ubx", "-", "b.ubx"})
	if err != nil {
		t.Fatalf("parseFlags multiple inputs: %v", err)
	}
	if len(v.inputPaths) != 3 || v.inputPaths[0] != "a.ubx" || v.inputPaths[1] != "-" || v.inputPaths[2] != "b.ubx" {
		t.Fatalf("inputPaths = %#v", v.inputPaths)
	}
	v, _, err = parseFlags("", []string{"-"})
	if err != nil {
		t.Fatalf("parseFlags dash input: %v", err)
	}
	if len(v.inputPaths) != 1 || v.inputPaths[0] != "-" {
		t.Fatalf("inputPaths = %#v, want dash", v.inputPaths)
	}
}

func TestParseFlagsFormats(t *testing.T) {
	v, _, err := parseFlags("", []string{"--from", "obsj", "--to", "obsj", "input.obsj"})
	if err != nil {
		t.Fatalf("parseFlags obsj: %v", err)
	}
	if v.from != inputObsJSON || v.to != outputObsJSON {
		t.Fatalf("formats = %q, %q; want obsj, obsj", v.from, v.to)
	}
	if v.metadataFlags.Changed("comment") {
		t.Fatal("obsj comment flag changed, want false")
	}
	v, _, err = parseFlags("", []string{"--from", "rinex", "--to", "obsj", "input.obs"})
	if err != nil {
		t.Fatalf("parseFlags rinex: %v", err)
	}
	if v.from != inputRINEX || v.to != outputObsJSON {
		t.Fatalf("formats = %q, %q; want rinex, obsj", v.from, v.to)
	}
	if v.metadataFlags.Changed("comment") {
		t.Fatal("rinex comment flag changed, want false")
	}
	v, _, err = parseFlags("", []string{"--from", "rtcm", "--to", "obsj", "--date", "20251218", "input.rtcm"})
	if err != nil {
		t.Fatalf("parseFlags rtcm: %v", err)
	}
	if v.from != inputRTCM || v.to != outputObsJSON || v.week.mode != weekDate {
		t.Fatalf("rtcm flags = %#v", v)
	}
	v, _, err = parseFlags("", []string{"--from", "raw", "--rtcm-strict-prr", "--rtcm-omit-zero-do", "input.jsonl"})
	if err != nil {
		t.Fatalf("parseFlags raw RTCM options: %v", err)
	}
	if !v.format.rtcm.UseSpecPhaseRangeRateSign {
		t.Fatal("UseSpecPhaseRangeRateSign = false, want true")
	}
	if !v.format.rtcm.OmitZeroDo {
		t.Fatal("OmitZeroDo = false, want true")
	}
	v, _, err = parseFlags("", []string{"--from", "ubx", "--ubx-slip-threshold", "12", "input.ubx"})
	if err != nil {
		t.Fatalf("parseFlags ubx slip threshold: %v", err)
	}
	if v.format.ubx.SlipThreshold != 12 {
		t.Fatalf("UBX slip threshold = %d; want 12", v.format.ubx.SlipThreshold)
	}
	if _, _, err := parseFlags("", []string{"--from", "raw", "--ubx-slip-threshold", "15", "input.raw"}); err != nil {
		t.Fatalf("parseFlags raw UBX slip threshold: %v", err)
	}
	v, _, err = parseFlags("", []string{"--from", "ubx", "--ubx-bds-geo-half-cycle", "input.ubx"})
	if err != nil {
		t.Fatalf("parseFlags ubx BDS GEO half cycle: %v", err)
	}
	if !v.format.ubx.BDSGeoHalfCycle {
		t.Fatal("BDSGeoHalfCycle = false, want true")
	}
	if _, _, err := parseFlags("", []string{"--from", "raw", "--ubx-bds-geo-half-cycle", "input.raw"}); err != nil {
		t.Fatalf("parseFlags raw UBX BDS GEO half cycle: %v", err)
	}
	v, _, err = parseFlags("", []string{"--from", "uncb", "--unc-omit-do-without-cp", "input.uncb"})
	if err != nil {
		t.Fatalf("parseFlags uncb omit doppler without cp: %v", err)
	}
	if !v.format.unc.OmitDoWithoutCP {
		t.Fatal("OmitDoWithoutCP = false, want true")
	}
	if _, _, err := parseFlags("", []string{"--from", "raw", "--unc-omit-do-without-cp", "input.raw"}); err != nil {
		t.Fatalf("parseFlags raw UNC omit doppler without cp: %v", err)
	}
	if _, _, err := parseFlags("", []string{"--from", "unca", "--unc-omit-do-without-cp", "input.unca"}); err != nil {
		t.Fatalf("parseFlags unca UNC omit doppler without cp: %v", err)
	}
	for _, tt := range []struct {
		from string
		want inputFormat
	}{
		{from: "uncb", want: inputUNCB},
		{from: "unca", want: inputUNCA},
	} {
		v, _, err = parseFlags("", []string{"--packet-log", "--from", tt.from, "input.jsonl"})
		if err != nil {
			t.Fatalf("parseFlags %s packet log: %v", tt.from, err)
		}
		if v.from != tt.want || !v.packetLog {
			t.Fatalf("%s flags = %#v", tt.from, v)
		}
	}
	if _, _, err := parseFlags("", []string{"--to", "ubx", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted unsupported ubx output")
	}
	v, _, err = parseFlags("", []string{"--packet-log", "--from", "raw", "input.jsonl"})
	if err != nil {
		t.Fatalf("parseFlags packet log: %v", err)
	}
	if !v.packetLog {
		t.Fatal("packetLog = false, want true")
	}
	if _, _, err := parseFlags("", []string{"--packet-log", "--from", "obsj", "input.obsj"}); err == nil {
		t.Fatal("parseFlags accepted packet log with obsj input")
	}
	if _, _, err := parseFlags("", []string{"--packet-log", "--from", "rinex", "input.obs"}); err == nil {
		t.Fatal("parseFlags accepted packet log with rinex input")
	}
	if _, _, err := parseFlags("", []string{"--from", "rtcm", "--date", "20251218", "--recent", "input.rtcm"}); err == nil {
		t.Fatal("parseFlags accepted conflicting RTCM date options")
	}
	if _, _, err := parseFlags("", []string{"--from", "ubx", "--date", "20251218", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted RTCM date option with UBX input")
	}
	if _, _, err := parseFlags("", []string{"--from", "ubx", "--rtcm-strict-prr", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted RTCM PRR option with UBX input")
	}
	if _, _, err := parseFlags("", []string{"--from", "ubx", "--rtcm-omit-zero-do", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted RTCM zero Doppler option with UBX input")
	}
	if _, _, err := parseFlags("", []string{"--from", "unca", "--ubx-slip-threshold", "15", "input.unc"}); err == nil {
		t.Fatal("parseFlags accepted UBX slip threshold option with UNCA input")
	}
	if _, _, err := parseFlags("", []string{"--from", "unca", "--ubx-bds-geo-half-cycle", "input.unc"}); err == nil {
		t.Fatal("parseFlags accepted UBX BDS GEO half cycle option with UNCA input")
	}
	if _, _, err := parseFlags("", []string{"--from", "ubx", "--unc-omit-do-without-cp", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted UNC omit doppler option with UBX input")
	}
	if _, _, err := parseFlags("", []string{"--from", "rtcm", "--unc-omit-do-without-cp", "input.rtcm"}); err == nil {
		t.Fatal("parseFlags accepted UNC omit doppler option with RTCM input")
	}
	if _, _, err := parseFlags("", []string{"--packet-log", "--from", "rtcm", "--date", "20251218", "input.jsonl"}); err == nil {
		t.Fatal("parseFlags accepted RTCM date option with packet log")
	}
	if _, _, err := parseFlags("", []string{"--from", "rtcm", "-f", "-"}); err == nil {
		t.Fatal("parseFlags accepted date-from-filename with stdin")
	}
}

func TestParseFlagsInterval(t *testing.T) {
	v, _, err := parseFlags("", []string{"--interval", "30", "input.ubx"})
	if err != nil {
		t.Fatalf("parseFlags interval: %v", err)
	}
	if v.interval != 30*time.Second {
		t.Fatalf("interval = %s, want 30s", v.interval)
	}
	if _, _, err := parseFlags("", []string{"--interval", "0.5", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted subsecond interval")
	}
	if _, _, err := parseFlags("", []string{"--interval", "7", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted interval that does not divide a GPS day")
	}
	if _, _, err := parseFlags("", []string{"--interval", "-1", "input.ubx"}); err == nil {
		t.Fatal("parseFlags accepted negative interval")
	}
}

func TestRunObsJSONInput(t *testing.T) {
	data := strings.Join([]string{
		`{"marker":{"name":"FILE"},"comment":["from obsj"]}`,
		`{"t":"2025-06-30T23:59:59.0000000","sat":"G03","sig":"1C","pr":22187868.655,"cp":116598092.035,"cn0":48}`,
		"",
	}, "\n")
	var got bytes.Buffer
	meta := testMetaMarker("FLAG")
	if err := runInputs(testInputs(strings.NewReader(data)), &got, inputObsJSON, outputRINEX, false, meta, 0); err != nil {
		t.Fatalf("runInputs obsj to rinex: %v", err)
	}
	s := got.String()
	checks := []string{
		"from obsj                                                   COMMENT",
		"FLAG                                                        MARKER NAME",
		"G    3 C1C L1C S1C                                          SYS / # / OBS TYPES",
		"G03  22187868.655   116598092.035          48.000",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Fatalf("output does not contain %q\n%s", want, s)
		}
	}
}

func TestRunRINEXInput(t *testing.T) {
	obs := []rinex.SignalObservation{
		{
			T:   mustTime(t, "2025-12-17T08:14:06.0080000"),
			Sat: "R06",
			Sig: "1C",
			SignalValues: rinex.SignalValues{
				Frq: opt.Make(int8(-4)),
				PR:  opt.Make(24968868.310),
				CP:  opt.Make(133238671.287),
				Do:  opt.Make(-2790.338),
				CN0: opt.Make(float32(32)),
			},
		},
	}
	meta := testWriteMeta(time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC))
	meta.Marker.Name = "FILE"
	meta.Comment = rinex.Lines{"from rinex"}
	meta.LeapSeconds = testPtr(int16(18))
	meta.AntennaDelta = testPtr([3]float64{1.25, 0, 0})
	var in bytes.Buffer
	if err := rinex.WriteObservationFile(&in, meta, obs); err != nil {
		t.Fatalf("WriteObservationFile: %v", err)
	}
	var out bytes.Buffer
	if err := runInputs(testInputs(&in), &out, inputRINEX, outputObsJSON, false, testMetaMarker("FLAG"), 0); err != nil {
		t.Fatalf("runInputs rinex to obsj: %v", err)
	}
	gotMeta, gotObs, err := rinex.ReadObsJSON(strings.NewReader(out.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, out.String())
	}
	if gotMeta.Marker.Name != "FLAG" || gotMeta.LeapSeconds == nil || *gotMeta.LeapSeconds != 18 || gotMeta.AntennaDelta == nil || gotMeta.AntennaDelta[0] != 1.25 {
		t.Fatalf("metadata = %#v", gotMeta)
	}
	if len(gotMeta.Comment) != 1 || gotMeta.Comment[0] != "from rinex" {
		t.Fatalf("comments = %#v", gotMeta.Comment)
	}
	if len(gotObs) != 1 || gotObs[0].Sat != "R06" || gotObs[0].Sig != "1C" || gotObs[0].Frq.Get() != -4 || !gotObs[0].CP.IsSet() {
		t.Fatalf("observations = %#v", gotObs)
	}
}

func TestRunMultipleObsJSONInputs(t *testing.T) {
	inputs := testInputs(
		strings.NewReader(strings.Join([]string{
			`{"marker":{"name":"A"},"comment":["first"]}`,
			`{"t":"2025-06-30T23:59:59.0000000","sat":"G03","sig":"1C","pr":22187868.655,"cp":116598092.035}`,
			"",
		}, "\n")),
		strings.NewReader(strings.Join([]string{
			`{"marker":{"name":"B"},"comment":["second"]}`,
			`{"t":"2025-07-01T00:00:00.0000000","sat":"G04","sig":"1C","pr":22728612.336,"cp":119439644.226}`,
			"",
		}, "\n")),
	)
	var got bytes.Buffer
	if err := runInputs(inputs, &got, inputObsJSON, outputRINEX, false, rinex.Metadata{}, 0); err != nil {
		t.Fatalf("runInputs obsj to rinex: %v", err)
	}
	s := got.String()
	checks := []string{
		"first                                                       COMMENT",
		"second                                                      COMMENT",
		"B                                                           MARKER NAME",
		"G03  22187868.655   116598092.035",
		"G04  22728612.336   119439644.226",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Fatalf("output does not contain %q\n%s", want, s)
		}
	}
}

func TestRunDecimatesObsJSONInput(t *testing.T) {
	data := strings.Join([]string{
		`{"marker":{"name":"FILE"}}`,
		`{"t":"2025-07-01T00:00:00.0000000","sat":"G03","sig":"1C","pr":1}`,
		`{"t":"2025-07-01T00:00:05.0000000","sat":"G03","sig":"1C","arc":1}`,
		`{"t":"2025-07-01T00:00:10.0000000","sat":"G03","sig":"1C","pr":2,"arc":1}`,
		"",
	}, "\n")
	var got bytes.Buffer
	if err := runInputs(testInputs(strings.NewReader(data)), &got, inputObsJSON, outputObsJSON, false, rinex.Metadata{}, 10*time.Second); err != nil {
		t.Fatalf("runInputs obsj decimation: %v", err)
	}
	meta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if meta.Marker.Name != "FILE" {
		t.Fatalf("Marker.Name = %q, want FILE", meta.Marker.Name)
	}
	if len(obs) != 2 {
		t.Fatalf("len observations = %d, want 2\n%s", len(obs), got.String())
	}
	if obs[0].PR.Get() != 1 || obs[1].PR.Get() != 2 {
		t.Fatalf("observations = %#v", obs)
	}
	if obs[1].Arc != 1 || obs[1].HC || obs[1].BT {
		t.Fatalf("selected Arc/HC/BT = %v/%v/%v, want 1/false/false", obs[1].Arc, obs[1].HC, obs[1].BT)
	}
}

func TestRunObsJSONOutput(t *testing.T) {
	pkt := rawxPacket(t)
	var got bytes.Buffer
	meta := testMetaMarker("JSONL")
	if err := runInputs(testInputs(bytes.NewReader(pkt)), &got, inputRaw, outputObsJSON, false, meta, 0); err != nil {
		t.Fatalf("runInputs raw to obsj: %v", err)
	}
	fileMeta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if fileMeta.Marker.Name != "JSONL" {
		t.Fatalf("Marker.Name = %q, want JSONL", fileMeta.Marker.Name)
	}
	if len(fileMeta.Comment) != 0 {
		t.Fatalf("comments = %#v", fileMeta.Comment)
	}
	if len(obs) != 1 {
		t.Fatalf("len observations = %d, want 1\n%s", len(obs), got.String())
	}
	if obs[0].Sat != "G03" || obs[0].Sig != "1C" || !obs[0].PR.IsSet() || !obs[0].CP.IsSet() {
		t.Fatalf("observation = %#v", obs[0])
	}
}

func TestRunRawInputSkipsInvalidFragments(t *testing.T) {
	pkt := append([]byte("junk"), rawxPacket(t)...)
	for _, from := range []inputFormat{inputRaw, inputUBX} {
		t.Run(string(from), func(t *testing.T) {
			var got bytes.Buffer
			if err := runInputs(testInputs(bytes.NewReader(pkt)), &got, from, outputObsJSON, false, rinex.Metadata{}, 0); err != nil {
				t.Fatalf("runInputs raw to obsj: %v", err)
			}
			_, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
			if err != nil {
				t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
			}
			if len(obs) != 1 || obs[0].Sat != "G03" || obs[0].Sig != "1C" {
				t.Fatalf("observations = %#v", obs)
			}
		})
	}
}

func TestRunDecimatesRawInput(t *testing.T) {
	pkt := append(rawxPacketAt(t, 345601.0), rawxPacketAt(t, 345610.0)...)
	var got bytes.Buffer
	if err := runInputs(testInputs(bytes.NewReader(pkt)), &got, inputRaw, outputObsJSON, false, rinex.Metadata{}, 10*time.Second); err != nil {
		t.Fatalf("runInputs raw decimation: %v", err)
	}
	_, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if len(obs) != 1 {
		t.Fatalf("len observations = %d, want 1\n%s", len(obs), got.String())
	}
	if obs[0].T != rinex.TimeFromGPSWeekSeconds(2397, 345610.0) {
		t.Fatalf("observation time = %s, want 345610 TOW", obs[0].T)
	}
}

func TestRunMultipleRawInputs(t *testing.T) {
	pkt := rawxPacket(t)
	var got bytes.Buffer
	meta := testMetaMarker("MULTI")
	if err := runInputs(testInputs(bytes.NewReader(pkt), bytes.NewReader(pkt)), &got, inputRaw, outputObsJSON, false, meta, 0); err != nil {
		t.Fatalf("runInputs raw to obsj: %v", err)
	}
	fileMeta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if fileMeta.Marker.Name != "MULTI" {
		t.Fatalf("Marker.Name = %q, want MULTI", fileMeta.Marker.Name)
	}
	if len(obs) != 2 {
		t.Fatalf("len observations = %d, want 2\n%s", len(obs), got.String())
	}
}

func TestRunMultipleInputFiles(t *testing.T) {
	dir := t.TempDir()
	pkt := rawxPacket(t)
	paths := []string{
		fileWithData(t, dir, "a.ubx", pkt),
		fileWithData(t, dir, "b.ubx", pkt),
	}
	var got bytes.Buffer
	if err := runInputs(openInputs(paths), &got, inputRaw, outputObsJSON, false, rinex.Metadata{}, 0); err != nil {
		t.Fatalf("runInputs raw files to obsj: %v", err)
	}
	_, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if len(obs) != 2 {
		t.Fatalf("len observations = %d, want 2\n%s", len(obs), got.String())
	}
}

func TestRunPacketLogInput(t *testing.T) {
	rawx := rawxPacket(t)
	tow := uint32(345600000)
	rtcmT := rinex.TimeFromGPSWeekMillis(2397, tow)
	uncT := rinex.TimeFromGPSWeekMillis(2419, 522335000)
	uncb := packetLogEntryFromFixture(t, gpsreg.TagUnicoreBin, "OBSVM")
	unca := packetLogEntryFromFixture(t, gpsreg.TagUnicoreAscii, "OBSVMA")
	uncbUntagged := uncb
	uncbUntagged.Tag = ""
	uncbUntagged.Msg = ""
	tests := []struct {
		name     string
		from     inputFormat
		entries  []gpsio.PacketLogEntry
		comments []string
		sat      rinex.SatelliteID
		sig      rinex.SignalID
		t        rinex.Time
		err      string
		warn     string
	}{
		{
			name: "ubx_raw",
			from: inputRaw,
			entries: []gpsio.PacketLogEntry{
				{Out: true, Tag: gpsreg.TagUBX, Msg: "RXM-RAWX", Bin: gpsio.HexString(rawx)},
				{Tag: gpsreg.TagUBX, Msg: "RXM-RAWX", Bin: gpsio.HexString(rawx)},
			},
			sat: "G03",
			sig: "1C",
		},
		{
			name: "rtcm_raw",
			from: inputRaw,
			entries: []gpsio.PacketLogEntry{{
				T:   gpsio.TimeMicro(rtcmT.CivilTime()),
				Tag: gpsreg.TagRTCM,
				Msg: "1077",
				Bin: gpsio.HexString(rtcmMSM7Packet(t, tow)),
			}},
			sat: "G03",
			sig: "1C",
			t:   rtcmT,
		},
		{
			name:    "uncb_explicit",
			from:    inputUNCB,
			entries: []gpsio.PacketLogEntry{uncb},
			sat:     "G06",
			sig:     "1C",
			t:       uncT,
		},
		{
			name:    "unca_explicit",
			from:    inputUNCA,
			entries: []gpsio.PacketLogEntry{unca},
			sat:     "G06",
			sig:     "1C",
			t:       uncT,
		},
		{
			name:    "raw_selects_uncb",
			from:    inputRaw,
			entries: []gpsio.PacketLogEntry{uncb},
			sat:     "G06",
			sig:     "1C",
			t:       uncT,
		},
		{
			name:    "raw_ignores_untagged_uncb",
			from:    inputRaw,
			entries: []gpsio.PacketLogEntry{uncbUntagged},
			err:     "no raw observation packets found",
		},
		{
			name: "raw_ignores_untagged_fragments",
			from: inputRaw,
			entries: []gpsio.PacketLogEntry{
				{Bin: gpsio.HexString([]byte{0xd3})},
				uncb,
			},
			sat: "G06",
			sig: "1C",
			t:   uncT,
		},
		{
			name:    "raw_ignores_mixed_unicore",
			from:    inputRaw,
			entries: []gpsio.PacketLogEntry{unca, uncb},
			sat:     "G06",
			sig:     "1C",
			t:       uncT,
			warn:    "ignoring mixed raw observation input",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			var log bytes.Buffer
			cj := convJob{
				inputs: testInputs(strings.NewReader(packetLog(t, tt.entries...))),
				out:    &got,
				opts: convertOptions{
					from:      tt.from,
					to:        outputObsJSON,
					packetLog: true,
				},
				meta: testMetaMarker("PKTLOG"),
			}
			err := cj.run(testLogger(&log), time.Now().UTC())
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("runInputs error = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("runInputs packet log to obsj: %v", err)
			}
			if tt.warn != "" && !strings.Contains(log.String(), tt.warn) {
				t.Fatalf("log = %q, want %q", log.String(), tt.warn)
			}
			assertObsJSON(t, got.String(), "PKTLOG", tt.comments, tt.sat, tt.sig, tt.t)
		})
	}
}

func TestConvertPacketDataRecognizesRawFormats(t *testing.T) {
	rawx := string(rawxPacket(t))
	rtcm := string(rtcmMSM7Packet(t, 345600000))
	uncbEntry := packetLogEntryFromFixture(t, gpsreg.TagUnicoreBin, "OBSVM")
	uncaEntry := packetLogEntryFromFixture(t, gpsreg.TagUnicoreAscii, "OBSVMA")
	uncb := uncbEntry.Data()
	unca := uncaEntry.Data()
	tests := []struct {
		name string
		tag  gpsprot.Tag
		data string
	}{
		{name: "ubx", tag: gpsreg.TagUBX, data: rawx},
		{name: "rtcm", tag: gpsreg.TagRTCM, data: rtcm},
		{name: "uncb", tag: gpsreg.TagUnicoreBin, data: uncb},
		{name: "unca", tag: gpsreg.TagUnicoreAscii, data: unca},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := false
			ok, err := convertPacketData(tt.data, packetLogFormatsByTag[tt.tag], packetInputFunc(func(pkt scan.Packet, week WeekConstraint) (bool, error) {
				seen = true
				if !pkt.HasTag(tt.tag) {
					t.Fatalf("packet tag = %s, want %s", pkt.Tag(), tt.tag)
				}
				if pkt.Data != tt.data {
					t.Fatal("packet data changed")
				}
				if !pkt.ChecksumValid {
					t.Fatalf("packet checksum invalid: %v", pkt.ChecksumError())
				}
				return true, nil
			}), WeekConstraint{})
			if err != nil {
				t.Fatalf("convertPacketData: %v", err)
			}
			if !ok || !seen {
				t.Fatalf("convertPacketData ok = %v, seen = %v; want true, true", ok, seen)
			}
		})
	}
}

func TestConvertPacketDataChecksumError(t *testing.T) {
	data := rawxPacket(t)
	data[len(data)-1] ^= 0xff
	in := &tagInput{
		tag: gpsreg.TagUBX,
		convert: func(string, WeekConstraint) (bool, error) {
			t.Fatal("convert called for packet with bad checksum")
			return true, nil
		},
	}
	ok, err := convertPacketData(string(data), packetLogFormatsByTag[gpsreg.TagUBX], in, WeekConstraint{})
	if err == nil {
		t.Fatal("convertPacketData error = nil, want checksum error")
	}
	if ok {
		t.Fatal("convertPacketData ok = true, want false")
	}
}

func TestRunRTCMInput(t *testing.T) {
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	pkt := rtcmMSM7Packet(t, tow)
	var got bytes.Buffer
	cj := convJob{
		inputs: testInputs(bytes.NewReader(pkt)),
		out:    &got,
		opts: convertOptions{
			from: inputRTCM,
			to:   outputObsJSON,
			week: weekOptions{mode: weekDate, date: civilDate(wantT.CivilTime())},
		},
		meta: testMetaMarker("RTCM"),
	}
	if err := cj.run(testLogger(io.Discard), time.Now().UTC()); err != nil {
		t.Fatalf("runInputs rtcm to obsj: %v", err)
	}
	meta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if meta.Marker.Name != "RTCM" {
		t.Fatalf("Marker.Name = %q, want RTCM", meta.Marker.Name)
	}
	if len(meta.Comment) != 0 {
		t.Fatalf("comments = %#v", meta.Comment)
	}
	if len(obs) != 1 {
		t.Fatalf("len observations = %d, want 1\n%s", len(obs), got.String())
	}
	if obs[0].T != wantT || obs[0].Sat != "G03" || obs[0].Sig != "1C" || !obs[0].PR.IsSet() || !obs[0].CP.IsSet() {
		t.Fatalf("observation = %#v", obs[0])
	}
	if !obs[0].Do.IsSet() || obs[0].Do.Get() <= 0 {
		t.Fatalf("Do = %v, want positive default PRR polarity", obs[0].Do)
	}
	got.Reset()
	cj = convJob{
		inputs: testInputs(bytes.NewReader(pkt)),
		out:    &got,
		opts: convertOptions{
			from: inputRTCM,
			to:   outputObsJSON,
			week: weekOptions{mode: weekDate, date: civilDate(wantT.CivilTime())},
		},
		meta: testMetaMarker("RTCM"),
	}
	cj.opts.format.rtcm.UseSpecPhaseRangeRateSign = true
	if err := cj.run(testLogger(io.Discard), time.Now().UTC()); err != nil {
		t.Fatalf("runInputs rtcm strict prr to obsj: %v", err)
	}
	_, obs, err = rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON strict PRR: %v\n%s", err, got.String())
	}
	if len(obs) != 1 || !obs[0].Do.IsSet() || obs[0].Do.Get() >= 0 {
		t.Fatalf("observations = %#v, want negative strict PRR Doppler", obs)
	}
}

func TestRunRawRTCMBuffersMetadataBeforeSelection(t *testing.T) {
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	pkt := append(rtcm1005Packet(t), rtcmMSM7Packet(t, tow)...)
	var got bytes.Buffer
	cj := convJob{
		inputs: testInputs(bytes.NewReader(pkt)),
		out:    &got,
		opts: convertOptions{
			from: inputRaw,
			to:   outputObsJSON,
			week: weekOptions{mode: weekDate, date: civilDate(wantT.CivilTime())},
		},
	}
	if err := cj.run(testLogger(io.Discard), time.Now().UTC()); err != nil {
		t.Fatalf("runInputs raw rtcm to obsj: %v", err)
	}
	meta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if meta.Marker.Number != "100" || meta.ApproxPosition == nil || *meta.ApproxPosition != [3]float64{1, 2, 3} {
		t.Fatalf("metadata = %#v", meta)
	}
	if len(meta.Comment) != 0 {
		t.Fatalf("comments = %#v", meta.Comment)
	}
	if len(obs) != 1 || obs[0].T != wantT || obs[0].Sat != "G03" {
		t.Fatalf("observations = %#v", obs)
	}
}

func TestRunRawDropsTentativeRTCMMetadataBeforeUBX(t *testing.T) {
	pkt := append(rtcm1005Packet(t), rawxPacket(t)...)
	now := time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)
	inputs := func(yield func(string, inputReader) bool) {
		yield("old.raw", inputReader{
			r:          bytes.NewReader(pkt),
			modTime:    now.Add(-8 * 24 * time.Hour),
			hasModTime: true,
		})
	}
	var got bytes.Buffer
	cj := convJob{inputs: inputs, out: &got, opts: convertOptions{from: inputRaw, to: outputObsJSON}}
	if err := cj.run(testLogger(io.Discard), now); err != nil {
		t.Fatalf("runInputs raw with tentative rtcm metadata to obsj: %v", err)
	}
	meta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if meta.Marker.Number != "" || meta.ApproxPosition != nil {
		t.Fatalf("metadata = %#v, want RTCM metadata dropped", meta)
	}
	if len(meta.Comment) != 0 {
		t.Fatalf("comments = %#v", meta.Comment)
	}
	if len(obs) != 1 || obs[0].Sat != "G03" {
		t.Fatalf("observations = %#v", obs)
	}
}

func TestRunRTCMDateFromFilename(t *testing.T) {
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	pkt := rtcmMSM7Packet(t, tow)
	inputs := func(yield func(string, inputReader) bool) {
		yield("MARK00USA_202512180000_01D_30S_MO.rtcm", inputReader{r: bytes.NewReader(pkt)})
	}
	var got bytes.Buffer
	cj := convJob{inputs: inputs, out: &got, opts: convertOptions{from: inputRTCM, to: outputObsJSON, week: weekOptions{mode: weekFilename}}}
	if err := cj.run(testLogger(io.Discard), time.Now().UTC()); err != nil {
		t.Fatalf("runInputs rtcm date from filename: %v", err)
	}
	_, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if len(obs) != 1 || obs[0].T != wantT {
		t.Fatalf("observations = %#v", obs)
	}
}

func TestRunRTCMOldFileNeedsDate(t *testing.T) {
	tow := uint32(345600000)
	pkt := rtcmMSM7Packet(t, tow)
	now := rinex.TimeFromGPSWeekMillis(2397, tow).CivilTime().Add(24 * time.Hour)
	inputs := func(yield func(string, inputReader) bool) {
		yield("old.rtcm", inputReader{
			r:          bytes.NewReader(pkt),
			modTime:    now.Add(-8 * 24 * time.Hour),
			hasModTime: true,
		})
	}
	var got bytes.Buffer
	cj := convJob{inputs: inputs, out: &got, opts: convertOptions{from: inputRTCM, to: outputObsJSON}}
	err := cj.run(testLogger(io.Discard), now)
	if err == nil || !strings.Contains(err.Error(), "provide --date") {
		t.Fatalf("runInputs old rtcm error = %v, want date hint", err)
	}
}

func TestRunRawIgnoresMixedObservationFamilies(t *testing.T) {
	tow := uint32(345600000)
	wantT := rinex.TimeFromGPSWeekMillis(2397, tow)
	pkt := append(rawxPacket(t), rtcmMSM7Packet(t, tow)...)
	var got bytes.Buffer
	var log bytes.Buffer
	cj := convJob{
		inputs: testInputs(bytes.NewReader(pkt)),
		out:    &got,
		opts: convertOptions{
			from: inputRaw,
			to:   outputObsJSON,
			week: weekOptions{mode: weekDate, date: civilDate(wantT.CivilTime())},
		},
	}
	if err := cj.run(testLogger(&log), time.Now().UTC()); err != nil {
		t.Fatalf("runInputs raw mixed to obsj: %v", err)
	}
	if !strings.Contains(log.String(), "ignoring mixed raw observation input") {
		t.Fatalf("log = %q, want mixed raw warning", log.String())
	}
	meta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if len(meta.Comment) != 0 {
		t.Fatalf("comments = %#v", meta.Comment)
	}
	if len(obs) != 1 || obs[0].Sat != "G03" || obs[0].Sig != "1C" || !obs[0].PR.IsSet() || !obs[0].CP.IsSet() {
		t.Fatalf("observations = %#v", obs)
	}
}

func TestGoldenFiles(t *testing.T) {
	// The golden files are RTKLIB Explorer convbin output; testdata/Makefile is
	// the single source of truth for the convbin flags and regenerates them.
	// The notes below explain only the SatPulse-side flags each case passes.
	// The UBX cases use --ubx-bds-geo-half-cycle to match RTKLIB Explorer's BDS
	// GEO half-cycle phase correction.
	// RTCM defines MSM PhaseRangeRate as d(PhaseRange)/dt, with PhaseRange
	// having pseudorange sign, so a strict RTCM decode converts to RINEX
	// Doppler as -PRR/wavelength. The RTCM logs need RTKLIB Explorer's -INVPRR
	// option (set in the Makefile) because their encoded PRR has the opposite
	// polarity from that strict RTCM interpretation. SatPulse's default matches
	// this common receiver polarity; --rtcm-strict-prr selects the strict RTCM
	// sign. RTKLIB Explorer omits numeric zero Doppler values; the RTCM cases
	// use --rtcm-omit-zero-do to test that compatibility mode.
	// RTKLIB Explorer zeroes Doppler when carrier-phase tracking is lost, so
	// the UNCB case uses --unc-omit-do-without-cp to match. Three BDS signals
	// are ignored because SatPulse and RTKLIB Explorer disagree on them in
	// ways the protocol specs resolve in SatPulse's favour (see
	// um980-cross-test-findings.md): B3I (6I), which RTKLIB Explorer's
	// build-time frequency limit drops but SatPulse emits, and B2b, which
	// SatPulse labels 7D (the BDS-3 data component) per RINEX 4.02 and RTKLIB
	// Explorer's Unicore decoder mislabels 7P.
	now := time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)
	cleanCommon := func(meta *rinex.Metadata) {
		meta.Run = rinex.MetadataRun{}
		meta.Comment = nil
	}
	cleanRTCM := func(meta *rinex.Metadata) {
		cleanCommon(meta)
		meta.Marker.Name = ""
		meta.Marker.Number = ""
	}
	tol := goldenTolerances()
	tests := []struct {
		name          string
		args          []string
		obs           string
		cleanMetadata func(*rinex.Metadata)
		ignoreSignals []ignoredSignal
	}{
		{
			name:          "m8t_20251217",
			args:          []string{"--run-by", "", "--ubx-bds-geo-half-cycle", filepath.Join("testdata", "m8t-20251217.ubx")},
			obs:           filepath.Join("testdata", "m8t-20251217.obs.gz"),
			cleanMetadata: cleanCommon,
		},
		{
			name:          "f9t_20251217",
			args:          []string{"--run-by", "", "--ubx-bds-geo-half-cycle", filepath.Join("testdata", "f9t-20251217.ubx")},
			obs:           filepath.Join("testdata", "f9t-20251217.obs.gz"),
			cleanMetadata: cleanCommon,
		},
		{
			name:          "rtcm_20260519",
			args:          []string{"--from", "rtcm", "--run-by", "", "--date-from-filename", "--rtcm-omit-zero-do", filepath.Join("testdata", "packet-rtcm-20260519.rtcm")},
			obs:           filepath.Join("testdata", "packet-rtcm-20260519.obs.gz"),
			cleanMetadata: cleanRTCM,
		},
		{
			name:          "um980_rtcm_20260527",
			args:          []string{"--from", "rtcm", "--run-by", "", "--date-from-filename", "--rtcm-omit-zero-do", filepath.Join("testdata", "um980-rtcm-20260527.rtcm")},
			obs:           filepath.Join("testdata", "um980-rtcm-20260527.obs.gz"),
			cleanMetadata: cleanRTCM,
		},
		{
			name:          "um980_uncb_20260527",
			args:          []string{"--from", "uncb", "--run-by", "", "--unc-omit-do-without-cp", filepath.Join("testdata", "um980-uncb-20260527.uncb")},
			obs:           filepath.Join("testdata", "um980-uncb-20260527.obs.gz"),
			cleanMetadata: cleanCommon,
			ignoreSignals: []ignoredSignal{
				{sys: 'C', sig: "6I"},
				{sys: 'C', sig: "7D"},
				{sys: 'C', sig: "7P"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, _, err := parseFlags("", tt.args)
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			var meta rinex.Metadata
			setMetadataDefaults(&meta, now, v.interval)
			if err := applyMetadataFlags(&meta, v.metadataFlags); err != nil {
				t.Fatalf("applyMetadataFlags: %v", err)
			}
			var got bytes.Buffer
			cj := convJob{
				inputs: openInputs(v.inputPaths),
				out:    &got,
				opts:   v.convertOptions,
				meta:   meta,
			}
			if err := cj.run(testLogger(io.Discard), now); err != nil {
				t.Fatalf("run: %v", err)
			}
			want, err := readGzipFile(tt.obs)
			if err != nil {
				t.Fatalf("readGzipFile %s: %v", tt.obs, err)
			}
			assertNoObservationFileDiff(t, tt.name, got.Bytes(), want, tt.cleanMetadata, tt.ignoreSignals, tol)
		})
	}
}

type goldenObsDiff struct {
	T   rinex.Time        `json:"t"`
	Sat rinex.SatelliteID `json:"sat"`
	Sig rinex.SignalID    `json:"sig"`
	A   *rinex.SignalDiff `json:"a,omitempty"`
	B   *rinex.SignalDiff `json:"b,omitempty"`
}

type goldenMetadataDiff struct {
	A rinex.Metadata `json:"a"`
	B rinex.Metadata `json:"b"`
}

type goldenDiffReporter struct {
	enc *json.Encoder
}

func (r goldenDiffReporter) Metadata(a, b rinex.Metadata) error {
	return r.enc.Encode(goldenMetadataDiff{A: a, B: b})
}

func (r goldenDiffReporter) Diff(t rinex.Time, sat rinex.SatelliteID, sig rinex.SignalID, a, b *rinex.SignalDiff) error {
	return r.enc.Encode(goldenObsDiff{T: t, Sat: sat, Sig: sig, A: a, B: b})
}

type goldenDiffErrorReporter struct {
	a []rinex.SignalObservation
	b []rinex.SignalObservation
}

func (r goldenDiffErrorReporter) Duplicate(side int, index, prevIndex int) error {
	o := r.obs(side)[index]
	return fmt.Errorf("%s: duplicate observation at %s %s %s: observations %d and %d", goldenSideName(side), o.T, o.Sat, o.Sig, prevIndex, index)
}

func (r goldenDiffErrorReporter) Unordered(side int, index int) error {
	obs := r.obs(side)
	return fmt.Errorf("%s: epoch out of order at observation %d: %s follows %s", goldenSideName(side), index, obs[index].T, obs[index-1].T)
}

func (r goldenDiffErrorReporter) obs(side int) []rinex.SignalObservation {
	if side == 0 {
		return r.a
	}
	return r.b
}

func goldenSideName(side int) string {
	if side == 0 {
		return "got"
	}
	return "want"
}

func goldenTolerances() rinex.Tolerances {
	return rinex.Tolerances{
		Obs:      rinex.ObsTolerances{PR: 0.0005, CP: 0.0005, Do: 0.0005, CN0: 0.0005},
		Metadata: rinex.MetadataTolerances{ApproxPos: 0.00005, AntennaDelta: 0.00005},
	}
}

// ignoredSignal identifies a satellite-system signal to exclude from a golden
// comparison, used where SatPulse and the reference decoder differ on a signal
// in a way the protocol specs resolve in SatPulse's favour.
type ignoredSignal struct {
	sys byte
	sig rinex.SignalID
}

func assertNoObservationFileDiff(t *testing.T, name string, got, want []byte, clean func(*rinex.Metadata), ignore []ignoredSignal, tol rinex.Tolerances) {
	t.Helper()
	gotMeta, gotObs, err := rinex.ReadObservationFile(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("ReadObservationFile got: %v", err)
	}
	wantMeta, wantObs, err := rinex.ReadObservationFile(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("ReadObservationFile want: %v", err)
	}
	gotObs = filterIgnoredSignals(gotObs, ignore)
	wantObs = filterIgnoredSignals(wantObs, ignore)
	if clean != nil {
		clean(&gotMeta)
		clean(&wantMeta)
	}
	gotOnly, wantOnly := rinex.DiffMetadata(gotMeta, wantMeta, tol.Metadata)
	var diff bytes.Buffer
	rep := goldenDiffReporter{enc: json.NewEncoder(&diff)}
	metaFields := metadataDiffFields(gotOnly, wantOnly)
	if len(metaFields) != 0 {
		if err := rep.Metadata(gotOnly, wantOnly); err != nil {
			t.Fatalf("report metadata diff: %v", err)
		}
	}
	obsDiffs, err := rinex.DiffObservations(gotObs, wantObs, tol.Obs, rep, goldenDiffErrorReporter{a: gotObs, b: wantObs})
	if err != nil {
		t.Fatalf("DiffObservations: %v", err)
	}
	if len(metaFields) != 0 || obsDiffs != 0 {
		dir, err := os.MkdirTemp("", name+"-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		diffPath := filepath.Join(dir, name+"-diff.jsonl")
		if err := os.WriteFile(diffPath, diff.Bytes(), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", diffPath, err)
		}
		t.Fatalf("semantic diff: metadata fields=%s observation records=%d; full diff written to %s", metadataFieldList(metaFields), obsDiffs, diffPath)
	}
}

func filterIgnoredSignals(obs []rinex.SignalObservation, ignore []ignoredSignal) []rinex.SignalObservation {
	if len(ignore) == 0 {
		return obs
	}
	out := make([]rinex.SignalObservation, 0, len(obs))
	for _, o := range obs {
		if !signalIgnored(o.Sat, o.Sig, ignore) {
			out = append(out, o)
		}
	}
	return out
}

func signalIgnored(sat rinex.SatelliteID, sig rinex.SignalID, ignore []ignoredSignal) bool {
	for _, ig := range ignore {
		if sat[0] == ig.sys && sig == ig.sig {
			return true
		}
	}
	return false
}

func metadataDiffFields(a, b rinex.Metadata) []string {
	return diffMetadataValueFields("", reflect.ValueOf(a), reflect.ValueOf(b))
}

func diffMetadataValueFields(prefix string, a, b reflect.Value) []string {
	if metadataValueIsZero(a) && metadataValueIsZero(b) {
		return nil
	}
	if a.Kind() != reflect.Struct || a.Type() == reflect.TypeOf(time.Time{}) {
		return []string{prefix}
	}
	fields := make([]string, 0)
	t := a.Type()
	for i := 0; i < a.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := metadataJSONName(f)
		if name == "" {
			continue
		}
		if prefix != "" {
			name = prefix + "." + name
		}
		fields = append(fields, diffMetadataValueFields(name, a.Field(i), b.Field(i))...)
	}
	if len(fields) == 0 {
		if prefix == "" {
			return nil
		}
		return []string{prefix}
	}
	return fields
}

func metadataJSONName(f reflect.StructField) string {
	name := f.Tag.Get("json")
	if name == "-" {
		return ""
	}
	if i := strings.IndexByte(name, ','); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		name = f.Name
	}
	return name
}

func metadataValueIsZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

func metadataFieldList(fields []string) string {
	if len(fields) == 0 {
		return "none"
	}
	return strings.Join(fields, ", ")
}

func TestUBXObservationFilesRoundTripThroughObsJSON(t *testing.T) {
	now := time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "m8t_20251217",
			args: []string{"--run-by", "", filepath.Join("testdata", "m8t-20251217.ubx")},
		},
		{
			name: "f9t_20251217",
			args: []string{"--run-by", "", filepath.Join("testdata", "f9t-20251217.ubx")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, _, err := parseFlags("", tt.args)
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			var meta rinex.Metadata
			setMetadataDefaults(&meta, now, v.interval)
			if err := applyMetadataFlags(&meta, v.metadataFlags); err != nil {
				t.Fatalf("applyMetadataFlags: %v", err)
			}
			var in bytes.Buffer
			cj := convJob{
				inputs: openInputs(v.inputPaths),
				out:    &in,
				opts:   v.convertOptions,
				meta:   meta,
			}
			if err := cj.run(testLogger(io.Discard), now); err != nil {
				t.Fatalf("run ubx to rinex: %v", err)
			}
			var obsj bytes.Buffer
			if err := runInputs(testInputs(bytes.NewReader(in.Bytes())), &obsj, inputRINEX, outputObsJSON, false, rinex.Metadata{}, 0); err != nil {
				t.Fatalf("runInputs rinex to obsj: %v", err)
			}
			if _, obs, err := rinex.ReadObsJSON(bytes.NewReader(obsj.Bytes())); err != nil {
				t.Fatalf("ReadObsJSON: %v", err)
			} else if len(obs) == 0 {
				t.Fatal("ReadObsJSON returned no observations")
			}
			var got bytes.Buffer
			if err := runInputs(testInputs(bytes.NewReader(obsj.Bytes())), &got, inputObsJSON, outputRINEX, false, rinex.Metadata{}, 0); err != nil {
				t.Fatalf("runInputs obsj to rinex: %v", err)
			}
			if !bytes.Equal(got.Bytes(), in.Bytes()) {
				dir := t.TempDir()
				gotPath := filepath.Join(dir, "got.obs")
				wantPath := filepath.Join(dir, "want.obs")
				if err := os.WriteFile(gotPath, got.Bytes(), 0o644); err != nil {
					t.Fatalf("WriteFile %s: %v", gotPath, err)
				}
				if err := os.WriteFile(wantPath, in.Bytes(), 0o644); err != nil {
					t.Fatalf("WriteFile %s: %v", wantPath, err)
				}
				t.Fatalf("round trip mismatch; got written to %s; want written to %s; %s", gotPath, wantPath, firstDiff(got.Bytes(), in.Bytes()))
			}
		})
	}
}

func readGzipFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func packetLogLine(t *testing.T, entry gpsio.PacketLogEntry) string {
	t.Helper()
	if time.Time(entry.T).IsZero() {
		entry.T = gpsio.TimeMicro(time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC))
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal packet log entry: %v", err)
	}
	return string(b)
}

func packetLog(t *testing.T, entries ...gpsio.PacketLogEntry) string {
	t.Helper()
	lines := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		lines = append(lines, packetLogLine(t, entry))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func packetLogEntryFromFixture(t *testing.T, tag gpsprot.Tag, msg string) gpsio.PacketLogEntry {
	t.Helper()
	path := filepath.Join("..", "..", "gps", "testdata", "packets", "unicore", "UM980", "raw-obs-dual-460800.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var entry gpsio.PacketLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("Unmarshal fixture packet log line: %v", err)
		}
		if entry.Tag == tag && entry.Msg == msg {
			return entry
		}
	}
	t.Fatalf("fixture packet %s %s not found", tag, msg)
	return gpsio.PacketLogEntry{}
}

func assertObsJSON(t *testing.T, data, marker string, comments []string, sat rinex.SatelliteID, sig rinex.SignalID, wantT rinex.Time) {
	t.Helper()
	meta, obs, err := rinex.ReadObsJSON(strings.NewReader(data))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, data)
	}
	if meta.Marker.Name != marker {
		t.Fatalf("Marker.Name = %q, want %s", meta.Marker.Name, marker)
	}
	if strings.Join(meta.Comment, "\n") != strings.Join(comments, "\n") {
		t.Fatalf("comments = %#v, want %#v", meta.Comment, comments)
	}
	for _, o := range obs {
		if o.Sat == sat && o.Sig == sig && o.PR.IsSet() && o.CP.IsSet() {
			if wantT != 0 && o.T != wantT {
				t.Fatalf("observation time = %s, want %s", o.T, wantT)
			}
			return
		}
	}
	t.Fatalf("observation %s %s with PR and CP not found in %d observations", sat, sig, len(obs))
}

func fileWithData(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return path
}

func testInputs(readers ...io.Reader) iter.Seq2[string, inputReader] {
	return func(yield func(string, inputReader) bool) {
		for i, r := range readers {
			if !yield(fmt.Sprintf("input%d", i), inputReader{r: r}) {
				return
			}
		}
	}
}

func mustTime(t *testing.T, s string) rinex.Time {
	t.Helper()
	var tm rinex.Time
	if err := tm.UnmarshalText([]byte(s)); err != nil {
		t.Fatalf("UnmarshalText(%q): %v", s, err)
	}
	return tm
}

func rawxPacket(t *testing.T) []byte {
	t.Helper()
	return rawxPacketAt(t, 345600.0)
}

func rtcm1005Packet(t *testing.T) []byte {
	t.Helper()
	msg := &rtcmbin.MT1005{
		MsgHdrStationID: rtcmbin.MsgHdrStationID{
			MsgHdr:    rtcmbin.MsgHdr{MsgNum: 1005},
			StationID: 100,
		},
		GPS:     true,
		GLONASS: true,
		Galileo: true,
		ECEFX:   rtcmbin.Meter,
		ECEFY:   2 * rtcmbin.Meter,
		ECEFZ:   3 * rtcmbin.Meter,
	}
	return rtcmPacket(t, msg)
}

func rtcmMSM7Packet(t *testing.T, tow uint32) []byte {
	t.Helper()
	return rtcmPacket(t, rtcmMSM7GPS(tow))
}

func rtcmPacket(t *testing.T, msg rtcmbin.Msg) []byte {
	t.Helper()
	pkt, err := rtcmbin.SerializeMsg(msg)
	if err != nil {
		t.Fatalf("Serialize RTCM: %v", err)
	}
	return []byte(pkt)
}

func rtcmMSM7GPS(tow uint32) *rtcmbin.MSMHiRes {
	return &rtcmbin.MSMHiRes{
		MSMHeader: rtcmbin.MSMHeader{
			MsgHdrStationID: rtcmbin.MsgHdrStationID{
				MsgHdr:    rtcmbin.MsgHdr{MsgNum: 1077},
				StationID: 100,
			},
			EpochTime: tow,
			SatMask:   rtcmSatMask(3),
			SigMask:   rtcmSigMask(2),
		},
		CellMask: 1,
		Sat: rtcmbin.MSMSatData{
			RangeInt:  rtcmbin.Uint8Slice{70},
			RangeMod:  []uint16{512},
			PhaseRate: []int16{100},
		},
		Sig: rtcmbin.MSMHiResSigData{
			Pseudorange: []int32{0},
			PhaseRange:  []int32{0},
			LockTime:    []uint16{10},
			HalfCycle:   []bool{false},
			CNR:         []uint16{720},
			PhaseRate:   []int16{-2000},
		},
	}
}

func rtcmSatMask(ids ...uint8) uint64 {
	var m uint64
	for _, id := range ids {
		m |= uint64(1) << uint(64-id)
	}
	return m
}

func rtcmSigMask(ids ...uint8) uint32 {
	var m uint32
	for _, id := range ids {
		m |= uint32(1) << uint(32-id)
	}
	return m
}

func civilDate(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func rawxPacketAt(t *testing.T, tow float64) []byte {
	t.Helper()
	msg := &ubxbin.RxmRawx{
		RxmRawxFixed: ubxbin.RxmRawxFixed{
			RcvTow:  tow,
			Week:    2397,
			LeapS:   18,
			NumMeas: 1,
			RecStat: ubxbin.RxmRawxLeapSec,
			Version: 1,
		},
		Meas: []ubxbin.RxmRawxMeas{
			{
				PrMes:    22187868.655,
				CpMes:    116598092.035,
				DoMes:    -1234.5,
				GNSSID:   ubxbin.GPS,
				SVID:     3,
				SigID:    0,
				LockTime: 100,
				CNO:      48,
				CpStdev:  1,
				TrkStat:  ubxbin.RxmRawxPrValid | ubxbin.RxmRawxCpValid | ubxbin.RxmRawxHalfCyc,
			},
		},
	}
	pkt, err := ubxbin.Serialize(msg)
	if err != nil {
		t.Fatalf("Serialize RAWX: %v", err)
	}
	return pkt
}

func firstDiff(got, want []byte) string {
	gotLines := bytes.Split(got, []byte{'\n'})
	wantLines := bytes.Split(want, []byte{'\n'})
	n := min(len(gotLines), len(wantLines))
	for i := range n {
		if !bytes.Equal(gotLines[i], wantLines[i]) {
			return fmt.Sprintf("line %d differs\ngot:  %q\nwant: %q", i+1, gotLines[i], wantLines[i])
		}
	}
	return fmt.Sprintf("line count differs: got %d, want %d", len(gotLines), len(wantLines))
}
