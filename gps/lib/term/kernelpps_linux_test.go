package term

import (
	"errors"
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

// A pty has no kernel PPS source to find: the source the line discipline
// registers records the TTY's name, giving /dev/ptsN, which is not the
// /dev/pts/N the descriptor resolves to. A kernel without N_PPS fails the
// attach instead. Both are unavailable rather than unsupported, so the caller
// warns and falls back rather than failing the run.
func TestNewKernelModemControlPinWatchUnavailable(t *testing.T) {
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
	w, err := watcher.NewKernelModemControlPinWatch(ModemDCD)
	if err == nil {
		_ = w.Close()
		t.Fatal("NewKernelModemControlPinWatch succeeded on a pty")
	}
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error = %v, want ErrUnavailable without errors.ErrUnsupported", err)
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
