//go:build linux

package kpps

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func rawConn(t *testing.T, f *os.File) syscall.RawConn {
	t.Helper()
	raw, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestWaitReadable(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	firstCallback := make(chan struct{})
	want := byte(0x5a)
	go func() {
		<-firstCallback
		_, _ = w.Write([]byte{want})
	}()
	var got byte
	callbacks := 0
	err = waitReadable(rawConn(t, r), func(fd uintptr) (bool, error) {
		callbacks++
		if callbacks == 1 {
			close(firstCallback)
			return false, nil
		}
		var buf [1]byte
		if _, err := unix.Read(int(fd), buf[:]); err != nil {
			return true, err
		}
		got = buf[0]
		return true, nil
	})
	if err != nil {
		t.Fatalf("waitReadable: %v", err)
	}
	if got != want {
		t.Errorf("read %#x, want %#x", got, want)
	}
	if callbacks < 2 {
		t.Errorf("callbacks = %d, want at least 2", callbacks)
	}
}

func TestWaitReadableOperationCompletesBeforeReadiness(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	if err := r.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	called := false
	err = waitReadable(rawConn(t, r), func(uintptr) (bool, error) {
		called = true
		return true, nil
	})
	if err != nil {
		t.Fatalf("waitReadable: %v", err)
	}
	if !called {
		t.Error("operation was not called")
	}
}

func TestWaitReadableDeadline(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	if err := r.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	err = waitReadable(rawConn(t, r), func(uintptr) (bool, error) {
		callbacks++
		return false, nil
	})
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("waitReadable error = %v, want os.ErrDeadlineExceeded", err)
	}
	if callbacks != 1 {
		t.Errorf("callbacks = %d, want 1", callbacks)
	}
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
