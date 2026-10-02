package kpps

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func rawConn(t *testing.T, f *os.File) syscall.RawConn {
	t.Helper()
	raw, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// A pipe stands in for a PPS device: the ioctls never run, because both paths
// fail on the closed descriptor before reaching the callback.
func TestFetchAfterClose(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	s := &Source{file: r, raw: rawConn(t, r)}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []time.Duration{0, -1} {
		if _, err := s.Fetch(Info{}, timeout); !errors.Is(err, os.ErrClosed) {
			t.Errorf("Fetch with timeout %v: error = %v, want os.ErrClosed", timeout, err)
		}
	}
}
