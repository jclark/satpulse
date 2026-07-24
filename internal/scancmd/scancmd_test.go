package scancmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsreg"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      *flagVars
		expectErr bool
	}{
		{
			name:      "missing file argument",
			expectErr: true,
		},
		{
			name: "stdin file argument",
			args: []string{"-"},
			want: &flagVars{filePath: "-"},
		},
		{
			name: "file argument",
			args: []string{"capture.bin"},
			want: &flagVars{filePath: "capture.bin"},
		},
		{
			name: "vendor",
			args: []string{"--vendor", "u-blox", "-"},
			want: &flagVars{vendor: gpsreg.VendorUblox, filePath: "-"},
		},
		{
			name:      "unknown vendor",
			args:      []string{"--vendor", "invalid", "-"},
			expectErr: true,
		},
		{
			name:      "too many files",
			args:      []string{"a.bin", "b.bin"},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, usageFunc, err := parseFlags("scan", tc.args)
			if usageFunc == nil {
				t.Fatalf("usageFunc is nil")
			}
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestRunScansPackets(t *testing.T) {
	input := append([]byte("$GPRMC,1,2,3*0F\r\n"), 0xb5, 0x62, 0x01, 0x07, 0x00, 0x00, 0x08, 0x19)
	out := &recordingOutput{}
	if err := run(bytes.NewReader(input), out, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	entries := parseEntries(t, out.String())
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if out.flushes != 2 {
		t.Fatalf("flushes = %d, want 2", out.flushes)
	}
	if entries[0].Tag != gpsreg.TagNMEA || entries[0].Msg != "GPRMC" || entries[0].Ascii != "$GPRMC,1,2,3*0F\r\n" {
		t.Fatalf("entry 0 = %+v", entries[0])
	}
	if time.Time(entries[0].T).IsZero() {
		t.Fatalf("entry 0 has zero timestamp")
	}
	if entries[1].Tag != gpsreg.TagUBX || entries[1].Msg != "NAV-PVT" {
		t.Fatalf("entry 1 = %+v", entries[1])
	}
	if !bytes.Equal(entries[1].Bin, []byte{0xb5, 0x62, 0x01, 0x07, 0x00, 0x00, 0x08, 0x19}) {
		t.Fatalf("entry 1 bin = %x", []byte(entries[1].Bin))
	}
	if time.Time(entries[1].T).IsZero() {
		t.Fatalf("entry 1 has zero timestamp")
	}
}

func TestRunEmitsInvalidBytesAtEOF(t *testing.T) {
	out := &recordingOutput{}
	if err := run(strings.NewReader("not a packet"), out, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	entries := parseEntries(t, out.String())
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Tag != "" || entries[0].Msg != "" || entries[0].Ascii != "not a packet" {
		t.Fatalf("entry = %+v", entries[0])
	}
	if time.Time(entries[0].T).IsZero() {
		t.Fatalf("entry has zero timestamp")
	}
}

func TestRunTimestampsFollowInputTiming(t *testing.T) {
	delay := 50 * time.Millisecond
	out := &recordingOutput{}
	input := &stagedReader{
		delay: delay,
		chunks: [][]byte{
			{0xb5, 0x62, 0x01, 0x07, 0x00, 0x00, 0x08, 0x19},
			{0xb5, 0x62, 0x01, 0x07, 0x00, 0x00, 0x08, 0x19},
		},
	}
	if err := run(input, out, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	entries := parseEntries(t, out.String())
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	got := time.Time(entries[1].T).Sub(time.Time(entries[0].T))
	if got < delay/2 {
		t.Fatalf("timestamp gap = %v, want at least %v", got, delay/2)
	}
	if out.flushes != 2 {
		t.Fatalf("flushes = %d, want 2", out.flushes)
	}
}

func parseEntries(t *testing.T, s string) []gpsio.PacketLogEntry {
	t.Helper()
	var entries []gpsio.PacketLogEntry
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		var entry gpsio.PacketLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("unmarshal entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	return entries
}

type recordingOutput struct {
	bytes.Buffer
	flushes int
}

func (w *recordingOutput) Flush() error {
	w.flushes++
	return nil
}

type stagedReader struct {
	chunks [][]byte
	delay  time.Duration
	i      int
}

func (r *stagedReader) Read(p []byte) (int, error) {
	if r.i >= len(r.chunks) {
		return 0, io.EOF
	}
	if r.i > 0 {
		time.Sleep(r.delay)
	}
	n := copy(p, r.chunks[r.i])
	r.i++
	return n, nil
}
