package term

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBaudRates(t *testing.T) {
	for _, br := range baudRates {
		b, ok := speedToB(br.speed)
		if !ok {
			t.Errorf("speedToB(%d) returned false", br.speed)
		}
		if b != br.b {
			t.Errorf("speedToB(%d) = %d, want %d", br.speed, b, br.b)
		}
		speed := bToSpeed(br.b)
		if speed != br.speed {
			t.Errorf("bToSpeed(%d) = %d, want %d", br.b, speed, br.speed)
		}
	}
}

func TestBToSpeedBad(t *testing.T) {
	tests := []uint32{0, 1 << 31}
	for _, b := range tests {
		speed := bToSpeed(b)
		if speed != -1 {
			t.Errorf("bToSpeed(%d) = %d, want -1", b, speed)
		}
	}
}

func TestSpeedToBBad(t *testing.T) {
	tests := []int{-1, 0, 1 << 30}
	for _, speed := range tests {
		_, ok := speedToB(speed)
		if ok {
			t.Errorf("speedToB(%d) returned true, want false", speed)
		}
	}
}

func TestByteTransmitTime(t *testing.T) {
	// Set up Termios settings for 9600 baud 8N1
	var attr Attr
	ts := &attr.ts
	ts.Cflag = unix.CLOCAL | unix.CREAD | unix.CS8
	ts.Iflag = unix.IGNBRK
	ts.Oflag = 0
	ts.Lflag = 0
	ts.Cc[unix.VMIN] = 1
	ts.Cc[unix.VTIME] = 0
	Speed(9600)(&attr)

	// Calculate the expected duration to send a byte
	expected := (time.Second / 9600) * 10

	// Calculate the actual duration to send a byte
	actual := attr.byteTransmitTime()

	// Check that the actual duration matches the expected duration
	if actual != expected {
		t.Errorf("byteTransmitTime returned %v, want %v", actual, expected)
	}
}
