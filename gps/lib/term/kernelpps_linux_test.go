package term

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/kpps"
)

func TestNewKernelModemControlPinWatchWrongPin(t *testing.T) {
	term, err := Open(newTestPTY(t), RawMode)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := term.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	watcher, ok := term.(KernelModemControlPinWatcher)
	if !ok {
		t.Fatalf("%T does not implement KernelModemControlPinWatcher", term)
	}
	if _, err := watcher.NewKernelModemControlPinWatch(ModemCTS); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("NewKernelModemControlPinWatch(ModemCTS) error = %v, want errors.ErrUnsupported", err)
	}
}

func TestDevicePathForTTY(t *testing.T) {
	path := newTestPTY(t)
	alias := filepath.Join(t.TempDir(), "tty")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(alias, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	got, err := DevicePathForTTY(int(f.Fd()))
	if err != nil {
		t.Fatalf("DevicePathForTTY: %v", err)
	}
	if got != path {
		t.Errorf("DevicePathForTTY = %q, want %q", got, path)
	}
}

func TestKernelPPSSeqUpdate(t *testing.T) {
	mono := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	change := func(sec int64, nsec int, asserted bool) ModemControlPinChange {
		return ModemControlPinChange{Wall: time.Unix(sec, int64(nsec)), Mono: mono, Asserted: asserted}
	}
	edge := func(sec int64, nsec int, sequence uint32) kpps.Edge {
		return kpps.Edge{T: time.Unix(sec, int64(nsec)), Sequence: sequence}
	}
	type result struct {
		change  ModemControlPinChange
		missed  int
		ok      bool
		pending ModemControlPinChange
		hasPend bool
	}
	tests := []struct {
		name   string
		seq    kernelPPSSeq
		info   kpps.Info
		expect result
	}{
		{
			name: "no new event",
			seq:  kernelPPSSeq{lastAssert: 5, lastClear: 5},
			info: kpps.Info{Assert: kpps.Edge{Sequence: 5}, Clear: kpps.Edge{Sequence: 5}},
		},
		{
			name:   "assert only",
			seq:    kernelPPSSeq{lastAssert: 5, lastClear: 5},
			info:   kpps.Info{Assert: edge(100, 100000, 6), Clear: kpps.Edge{Sequence: 5}},
			expect: result{change: change(100, 100000, true), ok: true},
		},
		{
			name:   "clear only",
			seq:    kernelPPSSeq{lastAssert: 5, lastClear: 5},
			info:   kpps.Info{Assert: kpps.Edge{Sequence: 5}, Clear: edge(101, 40000, 6)},
			expect: result{change: change(101, 40000, false), ok: true},
		},
		{
			name: "both advanced, clear older",
			seq:  kernelPPSSeq{lastAssert: 5, lastClear: 5},
			info: kpps.Info{Assert: edge(100, 100000000, 6), Clear: edge(100, 50000, 6)},
			expect: result{change: change(100, 50000, false), ok: true,
				pending: change(100, 100000000, true), hasPend: true},
		},
		{
			name: "both advanced, assert older",
			seq:  kernelPPSSeq{lastAssert: 5, lastClear: 5},
			info: kpps.Info{Assert: edge(99, 900000000, 6), Clear: edge(100, 50000, 6)},
			expect: result{change: change(99, 900000000, true), ok: true,
				pending: change(100, 50000, false), hasPend: true},
		},
		{
			name: "counter jumps count missed edges",
			seq:  kernelPPSSeq{lastAssert: 5, lastClear: 5},
			info: kpps.Info{Assert: edge(103, 100000000, 8), Clear: edge(104, 40000, 7)},
			expect: result{change: change(103, 100000000, true), missed: 3, ok: true,
				pending: change(104, 40000, false), hasPend: true},
		},
		{
			name:   "sequence wraparound",
			seq:    kernelPPSSeq{lastAssert: 0xffffffff, lastClear: 5},
			info:   kpps.Info{Assert: edge(100, 100000, 0), Clear: kpps.Edge{Sequence: 5}},
			expect: result{change: change(100, 100000, true), ok: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got result
			got.change, got.missed, got.ok = tc.seq.update(tc.info, mono)
			got.pending, got.hasPend = tc.seq.pending, tc.seq.hasPending
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
			if tc.seq.lastAssert != tc.info.Assert.Sequence || tc.seq.lastClear != tc.info.Clear.Sequence {
				t.Errorf("counters = %d, %d; want %d, %d",
					tc.seq.lastAssert, tc.seq.lastClear, tc.info.Assert.Sequence, tc.info.Clear.Sequence)
			}
		})
	}
}

func TestKernelPPSSeqTakePending(t *testing.T) {
	change := ModemControlPinChange{Wall: time.Unix(100, 0), Asserted: true}
	seq := kernelPPSSeq{pending: change, hasPending: true}
	if got, ok := seq.takePending(); !ok || !reflect.DeepEqual(got, change) {
		t.Errorf("takePending = %+v, %v; want %+v, true", got, ok, change)
	}
	if _, ok := seq.takePending(); ok {
		t.Error("takePending returned a change twice")
	}
}

func TestFindKernelPPS(t *testing.T) {
	dir := t.TempDir()
	write := func(name, path string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "path"), []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pps0", "\n")
	write("pps1", "/dev/ttyS9\n")
	write("pps2", "/dev/ttyS0\n")
	if err := os.MkdirAll(filepath.Join(dir, "pps3"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		source    string
		expect    string
		expectErr bool
	}{
		{name: "match", source: "/dev/ttyS0", expect: "pps2"},
		{name: "other tty", source: "/dev/ttyS9", expect: "pps1"},
		{name: "no match", source: "/dev/ttyUSB0", expectErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := findKernelPPS(dir, tc.source)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Errorf("findKernelPPS = %q, want %q", got, tc.expect)
			}
		})
	}
}
