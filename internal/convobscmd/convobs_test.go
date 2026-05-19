package convobscmd

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rinex"
	rinexubx "github.com/jclark/satpulse/gps/lib/rinex/ubx"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

var updateGolden = flag.Bool("update", false, "update golden test data files")

func TestLoadMetadata(t *testing.T) {
	path := "meta.json"
	data := `{"markerName":"FILE","receiver":{"type":"RX"},"comments":["from file"]}`
	v, _, err := parseFlags("", []string{
		"--metadata", path,
		"--marker-name", "FLAG",
		"input.ubx",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if err := loadMetadata(&v.meta, v.metaPath, strings.NewReader(data)); err != nil {
		t.Fatalf("loadMetadata: %v", err)
	}
	meta := v.meta
	if meta.MarkerName != "FLAG" {
		t.Errorf("MarkerName = %q, want FLAG", meta.MarkerName)
	}
	if meta.Receiver.Type != "RX" {
		t.Errorf("Receiver.Type = %q, want RX", meta.Receiver.Type)
	}
	if len(meta.Comments) != 4 || meta.Comments[0] != "from file" || meta.Comments[1] != "format: u-blox UBX" || meta.Comments[2] != "options: -MULTICODE" || meta.Comments[3] != "log: input.ubx" {
		t.Errorf("Comments = %#v", meta.Comments)
	}
}

func TestParseFlagsRequiresInput(t *testing.T) {
	_, _, err := parseFlags("", nil)
	if err == nil {
		t.Fatal("parseFlags succeeded without input")
	}
	v, _, err := parseFlags("", []string{"-", "-"})
	if err == nil {
		t.Fatal("parseFlags succeeded with positional output")
	}
	v, _, err = parseFlags("", []string{"-"})
	if err != nil {
		t.Fatalf("parseFlags dash input: %v", err)
	}
	if v.inputPath != "-" {
		t.Fatalf("inputPath = %q, want dash", v.inputPath)
	}
}

func TestParseFlagsFormats(t *testing.T) {
	v, _, err := parseFlags("", []string{"--from", "obsj", "--to", "obsj", "input.obsj"})
	if err != nil {
		t.Fatalf("parseFlags obsj: %v", err)
	}
	if v.inputFormat != inputObsJSON || v.outputFormat != outputObsJSON {
		t.Fatalf("formats = %q, %q; want obsj, obsj", v.inputFormat, v.outputFormat)
	}
	if len(v.meta.Comments) != 0 {
		t.Fatalf("obsj comments = %#v, want none", v.meta.Comments)
	}
	if _, _, err := parseFlags("", []string{"--from", "rinex", "input.obs"}); err == nil {
		t.Fatal("parseFlags accepted unsupported rinex input")
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
}

func TestRunObsJSONInput(t *testing.T) {
	data := strings.Join([]string{
		`{"markerName":"FILE","comments":["from obsj"]}`,
		`{"t":"2025-06-30T23:59:59.0000000","sat":"G03","sig":"1C","pr":22187868.655,"cp":116598092.035,"cn0":48}`,
		"",
	}, "\n")
	var got bytes.Buffer
	meta := rinex.Metadata{MarkerName: "FLAG"}
	wopts := rinex.WriterOptions{Program: "test", Date: time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)}
	if err := runFormat(strings.NewReader(data), &got, inputObsJSON, outputRINEX, false, meta, wopts, rinexubx.Options{}); err != nil {
		t.Fatalf("runFormat obsj to rinex: %v", err)
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

func TestRunObsJSONOutput(t *testing.T) {
	pkt := rawxPacket(t)
	var got bytes.Buffer
	meta := rinex.Metadata{MarkerName: "JSONL"}
	if err := runFormat(bytes.NewReader(pkt), &got, inputRaw, outputObsJSON, false, meta, rinex.WriterOptions{}, rinexubx.Options{}); err != nil {
		t.Fatalf("runFormat raw to obsj: %v", err)
	}
	fileMeta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if fileMeta.MarkerName != "JSONL" {
		t.Fatalf("MarkerName = %q, want JSONL", fileMeta.MarkerName)
	}
	if len(obs) != 1 {
		t.Fatalf("len observations = %d, want 1\n%s", len(obs), got.String())
	}
	if obs[0].Sat != "G03" || obs[0].Sig != "1C" || !obs[0].PR.IsSet() || !obs[0].CP.IsSet() {
		t.Fatalf("observation = %#v", obs[0])
	}
}

func TestRunPacketLogInput(t *testing.T) {
	pkt := rawxPacket(t)
	log := strings.Join([]string{
		packetLogLine(t, gpsio.PacketLogEntry{Out: true, Tag: gpsreg.TagUBX, Msg: "RXM-RAWX", Bin: gpsio.HexString(pkt)}),
		packetLogLine(t, gpsio.PacketLogEntry{Tag: gpsreg.TagUBX, Msg: "RXM-RAWX", Bin: gpsio.HexString(pkt)}),
		"",
	}, "\n")
	var got bytes.Buffer
	meta := rinex.Metadata{MarkerName: "PKTLOG"}
	if err := runFormat(strings.NewReader(log), &got, inputRaw, outputObsJSON, true, meta, rinex.WriterOptions{}, rinexubx.Options{}); err != nil {
		t.Fatalf("runFormat packet log to obsj: %v", err)
	}
	fileMeta, obs, err := rinex.ReadObsJSON(strings.NewReader(got.String()))
	if err != nil {
		t.Fatalf("ReadObsJSON: %v\n%s", err, got.String())
	}
	if fileMeta.MarkerName != "PKTLOG" {
		t.Fatalf("MarkerName = %q, want PKTLOG", fileMeta.MarkerName)
	}
	if len(obs) != 1 {
		t.Fatalf("len observations = %d, want 1\n%s", len(obs), got.String())
	}
	if obs[0].Sat != "G03" || obs[0].Sig != "1C" || !obs[0].PR.IsSet() || !obs[0].CP.IsSet() {
		t.Fatalf("observation = %#v", obs[0])
	}
}

func TestGoldenFiles(t *testing.T) {
	// The golden files were checked against RTKLIB Explorer with:
	// convbin -r ubx -v 3.04 -od -os -ro -MULTICODE.
	// After normalizing PGM/RUN BY/DATE and the log path comment, the only
	// remaining header difference is that RTKLIB Explorer emits
	// SYS / PHASE SHIFT records. With those lines ignored, the output is
	// byte-identical to RTKLIB Explorer.
	now := time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		ubx  string
		obs  string
	}{
		{
			name: "m8t_20251217_4h",
			ubx:  filepath.Join("testdata", "m8t-20251217-4h.ubx"),
			obs:  filepath.Join("testdata", "m8t-20251217-4h.obs"),
		},
		{
			name: "f9t_20251217_3h",
			ubx:  filepath.Join("testdata", "f9t-20251217-3h.ubx"),
			obs:  filepath.Join("testdata", "f9t-20251217-3h.obs"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, _, err := parseFlags("", []string{tt.ubx})
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			v.wopts.Date = now
			in, err := os.Open(v.inputPath)
			if err != nil {
				t.Fatalf("Open %s: %v", v.inputPath, err)
			}
			defer in.Close()
			var got bytes.Buffer
			if err := run(in, &got, v.meta, v.wopts, v.uopts); err != nil {
				t.Fatalf("run: %v", err)
			}
			if *updateGolden {
				if err := os.WriteFile(tt.obs, got.Bytes(), 0o644); err != nil {
					t.Fatalf("WriteFile %s: %v", tt.obs, err)
				}
			}
			want, err := os.ReadFile(tt.obs)
			if err != nil {
				t.Fatalf("ReadFile %s: %v", tt.obs, err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				gotPath := filepath.Join(t.TempDir(), "got.obs")
				if err := os.WriteFile(gotPath, got.Bytes(), 0o644); err != nil {
					t.Fatalf("WriteFile %s: %v", gotPath, err)
				}
				t.Fatalf("output mismatch; got written to %s; %s", gotPath, firstDiff(got.Bytes(), want))
			}
		})
	}
}

func packetLogLine(t *testing.T, entry gpsio.PacketLogEntry) string {
	t.Helper()
	entry.T = gpsio.TimeMicro(time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC))
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal packet log entry: %v", err)
	}
	return string(b)
}

func rawxPacket(t *testing.T) []byte {
	t.Helper()
	msg := &ubxbin.RxmRawx{
		RxmRawxFixed: ubxbin.RxmRawxFixed{
			RcvTow:  345600.0,
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
