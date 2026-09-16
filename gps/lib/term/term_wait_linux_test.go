package term

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFrameDuration(t *testing.T) {
	tests := []struct {
		name string
		attr Attr
		want time.Duration
	}{
		{"9600 8N1", Attr{ts: unix.Termios{Cflag: unix.B9600 | unix.CS8}}, 1041667 * time.Nanosecond},
		{"4800 7E2", Attr{ts: unix.Termios{Cflag: unix.B4800 | unix.CS7 | unix.PARENB | unix.CSTOPB}}, 2291667 * time.Nanosecond},
		{"arbitrary speed", Attr{ts: unix.Termios{Cflag: unix.BOTHER | unix.CS8, Ospeed: 10000}}, time.Millisecond},
		{"hung up", Attr{ts: unix.Termios{Cflag: unix.B0 | unix.CS8}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameDuration(tt.attr); got != tt.want {
				t.Errorf("frameDuration = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafeWriteTimePTY(t *testing.T) {
	path := newTestPTY(t)
	port, safe, err := Open(path, RawMode, NoFlowControl, Speed(115200), ReadTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer port.Close()
	if !safe.IsZero() {
		t.Errorf("PTY Open safe write time = %v, want zero", safe)
	}
	if safe, err = port.Change(Speed(38400)); err != nil || !safe.IsZero() {
		t.Fatalf("PTY Change = %v, %v, want zero, nil", safe, err)
	}
}
