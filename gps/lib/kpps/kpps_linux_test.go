//go:build linux

package kpps

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestWaitReadable(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	want := byte(0x5a)
	go func() {
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte{want})
	}()
	var got byte
	err = waitReadable(r, func(fd uintptr) error {
		var buf [1]byte
		if _, err := unix.Read(int(fd), buf[:]); err != nil {
			return err
		}
		got = buf[0]
		return nil
	})
	if err != nil {
		t.Fatalf("waitReadable: %v", err)
	}
	if got != want {
		t.Errorf("read %#x, want %#x", got, want)
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
	err = waitReadable(r, func(uintptr) error {
		t.Fatal("operation called without readable data")
		return nil
	})
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("waitReadable error = %v, want os.ErrDeadlineExceeded", err)
	}
}

func TestInfoFromKernel(t *testing.T) {
	kinfo := unix.PPSKInfo{
		Assert_sequence: 17,
		Clear_sequence:  16,
		Assert_tu:       unix.PPSKTime{Sec: 100, Nsec: 123456789},
		Clear_tu:        unix.PPSKTime{Sec: 99, Nsec: 987654321},
		Current_mode:    int32(CaptureAssert | CaptureClear),
	}
	got := infoFromKernel(kinfo)
	if got.Assert.Sequence != 17 || !got.Assert.T.Equal(time.Unix(100, 123456789)) {
		t.Errorf("Assert = %+v", got.Assert)
	}
	if got.Clear.Sequence != 16 || !got.Clear.T.Equal(time.Unix(99, 987654321)) {
		t.Errorf("Clear = %+v", got.Clear)
	}
	if got.Mode != CaptureAssert|CaptureClear {
		t.Errorf("Mode = %#x, want %#x", got.Mode, CaptureAssert|CaptureClear)
	}
}
