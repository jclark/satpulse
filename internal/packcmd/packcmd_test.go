package packcmd

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

func TestRunPacketSelection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		cfg   runConfig
		want  []byte
	}{
		{
			name:  "binary packet emits decoded bytes",
			input: lines(`{"tag":"UBX","msg":"NAV-PVT","bin":"b5620107"}`),
			want:  []byte{0xb5, 0x62, 0x01, 0x07},
		},
		{
			name:  "ASCII packet emits exact bytes",
			input: lines(`{"tag":"NMEA","msg":"RMC","ascii":"$GPRMC,x\r\n"}`),
			want:  []byte("$GPRMC,x\r\n"),
		},
		{
			name:  "metadata records are skipped",
			input: lines(`{"event":"start"}`, `{"tag":"UBX","bin":"01"}`),
			want:  []byte{0x01},
		},
		{
			name:  "out records are skipped",
			input: lines(`{"tag":"UBX","bin":"01","out":true}`, `{"tag":"UBX","bin":"02"}`),
			want:  []byte{0x02},
		},
		{
			name:  "tag filter",
			input: lines(`{"tag":"UBX","bin":"01"}`, `{"tag":"RTCM","bin":"02"}`),
			cfg:   runConfig{tag: "RTCM"},
			want:  []byte{0x02},
		},
		{
			name:  "msg filter",
			input: lines(`{"tag":"UBX","msg":"TIM-TP","bin":"01"}`, `{"tag":"UBX","msg":"NAV-PVT","bin":"02"}`),
			cfg:   runConfig{tag: "UBX", msg: "NAV-PVT"},
			want:  []byte{0x02},
		},
		{
			name:  "filters are case-insensitive",
			input: lines(`{"tag":"UBX","msg":"NAV-PVT","bin":"01"}`),
			cfg:   runConfig{tag: "ubx", msg: "nav-pvt"},
			want:  []byte{0x01},
		},
		{
			name:  "no matches is empty output",
			input: lines(`{"tag":"UBX","msg":"NAV-PVT","bin":"01"}`),
			cfg:   runConfig{tag: "RTCM"},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := &recordingOutput{}
			if err := run(strings.NewReader(tc.input), out, tc.cfg); err != nil {
				t.Fatalf("run: %v", err)
			}
			if !bytes.Equal(out.Bytes(), tc.want) {
				t.Fatalf("output = %x, want %x", out.Bytes(), tc.want)
			}
		})
	}
}

func TestRunLineErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "invalid JSON",
			input:   lines(`{"tag":"UBX","bin":"01"`, `{"tag":"UBX","bin":"02"}`),
			wantErr: "line 1: invalid JSON",
		},
		{
			name:    "invalid bin hex",
			input:   lines(`{"event":"start"}`, `{"tag":"UBX","bin":"zz"}`),
			wantErr: "line 2: invalid bin hex",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := run(strings.NewReader(tc.input), &recordingOutput{}, runConfig{})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

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
			args: []string{"packets.jsonl"},
			want: &flagVars{filePath: "packets.jsonl"},
		},
		{
			name: "tag msg timing",
			args: []string{"--tag", "UBX", "--msg", "NAV-PVT", "--timing", "-"},
			want: &flagVars{tag: "UBX", msg: "NAV-PVT", timing: true, filePath: "-"},
		},
		{
			name:      "msg requires tag",
			args:      []string{"--msg", "NAV-PVT", "-"},
			expectErr: true,
		},
		{
			name:      "too many files",
			args:      []string{"a.jsonl", "b.jsonl"},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, usageFunc, err := parseFlags("pack", tc.args)
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

func TestTimingSleepsFromEmittedPacketDeltas(t *testing.T) {
	base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	fc := &fakeClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	out := &recordingOutput{clock: fc}
	input := lines(
		timedBin(base, "UBX", "NAV-PVT", "01"),
		timedBin(base.Add(150*time.Millisecond), "UBX", "NAV-PVT", "02"),
		timedBin(base.Add(350*time.Millisecond), "UBX", "NAV-PVT", "03"),
	)

	err := run(strings.NewReader(input), out, runConfig{timing: true, clock: fc})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !bytes.Equal(out.Bytes(), []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("output = %x", out.Bytes())
	}
	wantSleeps := []time.Duration{150 * time.Millisecond, 200 * time.Millisecond}
	if !reflect.DeepEqual(fc.sleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", fc.sleeps, wantSleeps)
	}
	if out.flushes != 3 {
		t.Fatalf("flushes = %d, want 3", out.flushes)
	}
}

func TestTimingUsesWriteStartBeforeWrite(t *testing.T) {
	base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	start := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	fc := &fakeClock{now: start}
	out := &recordingOutput{
		clock:          fc,
		advanceOnWrite: 200 * time.Millisecond,
	}
	input := lines(
		timedBin(base, "UBX", "NAV-PVT", "01"),
		timedBin(base.Add(time.Second), "UBX", "NAV-PVT", "02"),
		timedBin(base.Add(2*time.Second), "UBX", "NAV-PVT", "03"),
	)

	err := run(strings.NewReader(input), out, runConfig{timing: true, clock: fc})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantSleeps := []time.Duration{800 * time.Millisecond, 800 * time.Millisecond}
	if !reflect.DeepEqual(fc.sleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", fc.sleeps, wantSleeps)
	}
	wantWriteTimes := []time.Time{start, start.Add(time.Second), start.Add(2 * time.Second)}
	if !reflect.DeepEqual(out.writeTimes, wantWriteTimes) {
		t.Fatalf("writeTimes = %v, want %v", out.writeTimes, wantWriteTimes)
	}
}

func TestTimingUsesPreviousEmittedPacket(t *testing.T) {
	base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	fc := &fakeClock{now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)}
	out := &recordingOutput{clock: fc}
	input := lines(
		timedBin(base, "UBX", "NAV-PVT", "01"),
		timedBin(base.Add(100*time.Millisecond), "RTCM", "1077", "02"),
		timedBin(base.Add(300*time.Millisecond), "UBX", "NAV-PVT", "03"),
	)

	err := run(strings.NewReader(input), out, runConfig{tag: "UBX", timing: true, clock: fc})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !bytes.Equal(out.Bytes(), []byte{0x01, 0x03}) {
		t.Fatalf("output = %x", out.Bytes())
	}
	wantSleeps := []time.Duration{300 * time.Millisecond}
	if !reflect.DeepEqual(fc.sleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", fc.sleeps, wantSleeps)
	}
}

func TestTimingRequiresTimestampOnSelectedPackets(t *testing.T) {
	input := lines(`{"event":"metadata"}`, `{"tag":"UBX","bin":"01"}`)
	err := run(strings.NewReader(input), &recordingOutput{}, runConfig{timing: true, clock: &fakeClock{}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "line 2: --timing requires selected packets to have a non-zero t") {
		t.Fatalf("error = %q", err)
	}
}

func lines(s ...string) string {
	return strings.Join(s, "\n") + "\n"
}

func timedBin(t time.Time, tag, msg, bin string) string {
	return fmt.Sprintf(`{"t":%q,"tag":%q,"msg":%q,"bin":%q}`, t.UTC().Format(gpsio.RFC3339Micro), tag, msg, bin)
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
}

type recordingOutput struct {
	bytes.Buffer
	flushes        int
	clock          *fakeClock
	writeTimes     []time.Time
	advanceOnWrite time.Duration
}

func (w *recordingOutput) Write(p []byte) (int, error) {
	if w.clock != nil {
		w.writeTimes = append(w.writeTimes, w.clock.now)
	}
	n, err := w.Buffer.Write(p)
	if w.clock != nil {
		w.clock.now = w.clock.now.Add(w.advanceOnWrite)
	}
	return n, err
}

func (w *recordingOutput) Flush() error {
	w.flushes++
	return nil
}
