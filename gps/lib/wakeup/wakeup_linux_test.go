//go:build linux

package wakeup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRequestLatencyLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		max  time.Duration
		want uint32
	}{
		{name: "zero", max: 0, want: 0},
		{name: "sub-resolution", max: time.Nanosecond, want: 0},
		{name: "exact", max: 10 * time.Microsecond, want: 10},
		{name: "floor", max: 10*time.Microsecond + 999*time.Nanosecond, want: 10},
		{name: "maximum", max: maxLatencyMicros * time.Microsecond, want: maxLatencyMicros},
		{name: "maximum floor", max: maxLatencyMicros*time.Microsecond + 999*time.Nanosecond, want: maxLatencyMicros},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cpu_dma_latency")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			req, err := requestLatencyLimit(path, tc.max)
			if err != nil {
				t.Fatal(err)
			}
			if err := req.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 4 {
				t.Fatalf("request length = %d, want 4", len(got))
			}
			if value := binary.NativeEndian.Uint32(got); value != tc.want {
				t.Errorf("request = %d, want %d", value, tc.want)
			}
		})
	}
}

func TestRequestLatencyLimitRejectsOutOfRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cpu_dma_latency")
	for _, max := range []time.Duration{-1, (maxLatencyMicros + 1) * time.Microsecond} {
		if _, err := requestLatencyLimit(path, max); err == nil {
			t.Errorf("requestLatencyLimit(%v) returned nil error", max)
		}
	}
}

func TestRequestLatencyLimitOpenError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if _, err := requestLatencyLimit(path, time.Microsecond); err == nil {
		t.Fatal("requestLatencyLimit returned nil error for missing device")
	}
}
