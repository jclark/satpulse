package term

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSpeedClearsInputBaud(t *testing.T) {
	var attr Attr
	attr.ts.Cflag = unix.BOTHER | unix.BOTHER<<16
	if err := Speed(9600)(&attr); err != nil {
		t.Fatalf("Speed(9600): %v", err)
	}
	if got := attr.ts.Cflag & unix.CBAUD; got != unix.B9600 {
		t.Errorf("output CBAUD = %#x, want B9600 (%#x)", got, unix.B9600)
	}
	if got := attr.ts.Cflag & unix.CIBAUD; got != 0 {
		t.Errorf("input CBAUD = %#x, want B0", got)
	}
}
