package packcmd

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
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
			name: "tag msg realtime",
			args: []string{"--tag", "UBX", "--msg", "NAV-PVT", "--realtime", "2.5", "-"},
			want: &flagVars{tag: "UBX", msg: "NAV-PVT", realtime: 2.5, filePath: "-"},
		},
		{
			name:      "msg requires tag",
			args:      []string{"--msg", "NAV-PVT", "-"},
			expectErr: true,
		},
		{
			name:      "realtime zero rejected",
			args:      []string{"--realtime", "0", "-"},
			expectErr: true,
		},
		{
			name:      "realtime negative rejected",
			args:      []string{"--realtime=-1", "-"},
			expectErr: true,
		},
		{
			name:      "realtime NaN rejected",
			args:      []string{"--realtime", "NaN", "-"},
			expectErr: true,
		},
		{
			name:      "realtime Inf rejected",
			args:      []string{"--realtime", "Inf", "-"},
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

// The realtime timing tests run inside a synctest bubble, so time.Sleep
// advances a virtual clock deterministically and the recorded write offsets are
// exact. synctest sleeps never overshoot, so these tests cannot reproduce the
// wakeup-overshoot drift that absolute-base scheduling guards against; that
// regression is covered by the macOS smoke test, which failed reliably under
// the old previous-packet-relative scheduling.

func TestTimingSleepsFromEmittedPacketDeltas(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
		out := &recordingOutput{start: time.Now()}
		input := lines(
			timedBin(base, "UBX", "NAV-PVT", "01"),
			timedBin(base.Add(150*time.Millisecond), "UBX", "NAV-PVT", "02"),
			timedBin(base.Add(350*time.Millisecond), "UBX", "NAV-PVT", "03"),
		)
		if err := run(strings.NewReader(input), out, runConfig{factor: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !bytes.Equal(out.Bytes(), []byte{0x01, 0x02, 0x03}) {
			t.Fatalf("output = %x", out.Bytes())
		}
		wantOffsets := []time.Duration{0, 150 * time.Millisecond, 350 * time.Millisecond}
		if !reflect.DeepEqual(out.writeOffsets, wantOffsets) {
			t.Fatalf("writeOffsets = %v, want %v", out.writeOffsets, wantOffsets)
		}
		if out.flushes != 3 {
			t.Fatalf("flushes = %d, want 3", out.flushes)
		}
	})
}

func TestTimingAnchorsFirstPacketAfterFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
		// The first Flush blocks until a FIFO reader attaches; later packets must
		// be scheduled from when packet 1 was actually emitted, not from start.
		out := &recordingOutput{start: time.Now(), firstFlushBlock: 200 * time.Millisecond}
		input := lines(
			timedBin(base, "UBX", "NAV-PVT", "01"),
			timedBin(base.Add(time.Second), "UBX", "NAV-PVT", "02"),
			timedBin(base.Add(2*time.Second), "UBX", "NAV-PVT", "03"),
		)
		if err := run(strings.NewReader(input), out, runConfig{factor: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}
		wantOffsets := []time.Duration{0, 1200 * time.Millisecond, 2200 * time.Millisecond}
		if !reflect.DeepEqual(out.writeOffsets, wantOffsets) {
			t.Fatalf("writeOffsets = %v, want %v", out.writeOffsets, wantOffsets)
		}
	})
}

func TestTimingFactorCompressesDelays(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
		out := &recordingOutput{start: time.Now()}
		input := lines(
			timedBin(base, "UBX", "NAV-PVT", "01"),
			timedBin(base.Add(200*time.Millisecond), "UBX", "NAV-PVT", "02"),
			timedBin(base.Add(600*time.Millisecond), "UBX", "NAV-PVT", "03"),
		)
		if err := run(strings.NewReader(input), out, runConfig{factor: 4}); err != nil {
			t.Fatalf("run: %v", err)
		}
		wantOffsets := []time.Duration{0, 50 * time.Millisecond, 150 * time.Millisecond}
		if !reflect.DeepEqual(out.writeOffsets, wantOffsets) {
			t.Fatalf("writeOffsets = %v, want %v", out.writeOffsets, wantOffsets)
		}
	})
}

func TestTimingIgnoresFilteredPackets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
		out := &recordingOutput{start: time.Now()}
		input := lines(
			timedBin(base, "UBX", "NAV-PVT", "01"),
			timedBin(base.Add(100*time.Millisecond), "RTCM", "1077", "02"),
			timedBin(base.Add(300*time.Millisecond), "UBX", "NAV-PVT", "03"),
		)
		if err := run(strings.NewReader(input), out, runConfig{tag: "UBX", factor: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}
		if !bytes.Equal(out.Bytes(), []byte{0x01, 0x03}) {
			t.Fatalf("output = %x", out.Bytes())
		}
		// Packet 3 is scheduled from its own timestamp relative to the base; the
		// filtered RTCM packet does not shift its slot.
		wantOffsets := []time.Duration{0, 300 * time.Millisecond}
		if !reflect.DeepEqual(out.writeOffsets, wantOffsets) {
			t.Fatalf("writeOffsets = %v, want %v", out.writeOffsets, wantOffsets)
		}
	})
}

func TestTimingRequiresTimestampOnSelectedPackets(t *testing.T) {
	input := lines(`{"event":"metadata"}`, `{"tag":"UBX","bin":"01"}`)
	err := run(strings.NewReader(input), &recordingOutput{}, runConfig{factor: 1})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "line 2: --realtime requires selected packets to have a non-zero t") {
		t.Fatalf("error = %q", err)
	}
}

func lines(s ...string) string {
	return strings.Join(s, "\n") + "\n"
}

func timedBin(t time.Time, tag, msg, bin string) string {
	return fmt.Sprintf(`{"t":%q,"tag":%q,"msg":%q,"bin":%q}`, t.UTC().Format(gpsio.RFC3339Micro), tag, msg, bin)
}

// recordingOutput captures emitted bytes and, when start is set, the virtual
// time of each Write relative to start. A non-zero firstFlushBlock makes the
// first Flush sleep, modelling a FIFO reader attaching late.
type recordingOutput struct {
	bytes.Buffer
	start           time.Time
	writeOffsets    []time.Duration
	flushes         int
	firstFlushBlock time.Duration
	flushed         bool
}

func (w *recordingOutput) Write(p []byte) (int, error) {
	if !w.start.IsZero() {
		w.writeOffsets = append(w.writeOffsets, time.Since(w.start))
	}
	return w.Buffer.Write(p)
}

func (w *recordingOutput) Flush() error {
	w.flushes++
	if w.firstFlushBlock > 0 && !w.flushed {
		time.Sleep(w.firstFlushBlock)
	}
	w.flushed = true
	return nil
}
