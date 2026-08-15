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

func TestNewerThan(t *testing.T) {
	previous := Info{
		Assert: Edge{Sequence: 10},
		Clear:  Edge{Sequence: 20},
	}
	tests := []struct {
		name string
		info Info
		want bool
	}{
		{name: "same", info: Info{Assert: Edge{Sequence: 10}, Clear: Edge{Sequence: 20}}},
		{name: "assert", info: Info{Assert: Edge{Sequence: 11}, Clear: Edge{Sequence: 20}}, want: true},
		{name: "clear", info: Info{Assert: Edge{Sequence: 10}, Clear: Edge{Sequence: 21}}, want: true},
		{name: "both", info: Info{Assert: Edge{Sequence: 11}, Clear: Edge{Sequence: 21}}, want: true},
		{name: "assert wrap", info: Info{Assert: Edge{Sequence: 0}, Clear: Edge{Sequence: 20}}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := newerThan(tc.info, previous); got != tc.want {
				t.Errorf("newerThan = %v, want %v", got, tc.want)
			}
		})
	}
}
