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
	writeCtx, cancelWrite := context.WithCancel(context.Background())
	writeErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	wg.Go(func() { Scan(scanCtx, lg, conn, packetCh, nil, formats) })
	wg.Go(func() {
		ticker := time.NewTicker(stalePacketMargin / 2)
		defer ticker.Stop()
		for {
			select {
			case <-writeCtx.Done():
				return
			case <-ticker.C:
				if _, err := master.Write([]byte("$GPGGA,,,,,,0,00,99.99,,,,,,*48\r\n")); err != nil {
					writeErrCh <- err
					return
				}
			}
		}
	})

	got, err := DetectSpeed(
		context.Background(),
		lg,
		packetCh,
		conn,
		gpsreg.CreatePacketProcessors(nil),
		[]int{38400, 9600},
		time.Second,
		nil,
	)
	cancelWrite()
	if err != nil || got != (DetectResult{Outcome: DetectFound, Speed: 38400}) {
		t.Errorf("DetectSpeed() = %+v, %v, want found at 38400", got, err)
	}
	if got := conn.Speed(); got != 38400 {
		t.Errorf("connection speed = %d, want 38400", got)
	}

	conn.Stop()
	cancelScan()
	for range packetCh {
	}
	wg.Wait()
	select {
	case err := <-writeErrCh:
		t.Errorf("writing test sentence: %v", err)
	default:
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

// A device that says nothing is an outcome, not an error, and leaves the
// speed unspecified: cleanup is the caller's Close, which restores the port.
func TestDetectSpeedPTYSilent(t *testing.T) {
	_, conn := openTestPTY(t, 9600)
	got, err := DetectSpeed(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		make(chan scan.Packet),
		conn,
		nil,
		[]int{38400},
		time.Millisecond,
		nil,
	)
	if err != nil || got.Outcome != DetectSilent {
		t.Errorf("DetectSpeed() = %+v, %v, want silent", got, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}
