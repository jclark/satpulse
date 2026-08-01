package gpsio

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
	"golang.org/x/sys/unix"
)

func openTestPTY(t *testing.T, speed int) (*os.File, *SerialConn) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	master := os.NewFile(uintptr(fd), "/dev/ptmx")
	t.Cleanup(func() { master.Close() })
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := OpenSerial(fmt.Sprintf("/dev/pts/%d", n), speed)
	if err != nil {
		t.Fatal(err)
	}
	return master, conn
}

func TestDetectSpeedPTY(t *testing.T) {
	master, conn := openTestPTY(t, 9600)
	formats := gpsreg.CreatePacketFormats(nil)
	packetCh := make(chan scan.Packet, 1)
	scanCtx, cancelScan := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	wg.Go(func() { Scan(scanCtx, lg, conn, packetCh, nil, formats) })
	wg.Go(func() {
		time.Sleep(stalePacketMargin + 20*time.Millisecond)
		_, _ = master.Write([]byte("$GPGGA,,,,,,0,00,99.99,,,,,,*48\r\n"))
	})

	detected, err := DetectSpeed(
		context.Background(),
		lg,
		packetCh,
		conn,
		gpsreg.CreatePacketProcessors(nil),
		[]int{38400, 9600},
		time.Second,
		nil,
	)
	if err != nil || detected != 38400 {
		t.Errorf("DetectSpeed() = %d, %v, want 38400, nil", detected, err)
	}
	if got := conn.Speed(); got != 38400 {
		t.Errorf("connection speed = %d, want 38400", got)
	}

	conn.Stop()
	cancelScan()
	for range packetCh {
	}
	wg.Wait()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

// A failed detection leaves the speed unspecified and reports only why it
// failed: cleanup is the caller's Close, which restores the port.
func TestDetectSpeedPTYFailure(t *testing.T) {
	_, conn := openTestPTY(t, 9600)
	_, err := DetectSpeed(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		make(chan scan.Packet),
		conn,
		nil,
		[]int{38400},
		time.Millisecond,
		nil,
	)
	if err != ErrSilent {
		t.Errorf("DetectSpeed() error = %v, want exactly ErrSilent", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}
