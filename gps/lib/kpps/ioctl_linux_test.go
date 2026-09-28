//go:build linux

package kpps

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

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
