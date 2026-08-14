package serialcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"golang.org/x/sys/unix"
)

func TestSelectPortSymlink(t *testing.T) {
	dir := t.TempDir()
	node := filepath.Join(dir, "ttyACM0")
	if err := os.WriteFile(node, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "receiver")
	if err := os.Symlink(node, link); err != nil {
		t.Fatal(err)
	}
	ports := []serialenum.Port{{Device: node}}
	if port, ok := selectPort(ports, link); !ok || port.Device != node {
		t.Errorf("selectPort(%q) = %+v, %v, want the port for %s", link, port, ok, node)
	}
}

// TestDetectDevicePTY covers the detection lifecycle that the tests below
// the command layer do not reach: the scan worker, the packet log's two
// SemiClose paths, the drain of the packet channel, and the close.
func TestDetectDevicePTY(t *testing.T) {
	master, device := openTestPTY(t)
	logPath := filepath.Join(t.TempDir(), "capture.jsonl")
	writeCtx, cancelWrite := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-writeCtx.Done():
				return
			case <-ticker.C:
				if _, err := master.Write([]byte("$GPGGA,,,,,,0,00,99.99,,,,,,*48\r\n")); err != nil {
					return
				}
			}
		}
	})
	result := detectDevice(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), device, logPath)
	cancelWrite()
	wg.Wait()

	if result.failure != "" {
		t.Fatalf("detectDevice() failure = %q", result.failure)
	}
	if result.detection.Outcome != gpsio.DetectFound || result.detection.Speed == 0 {
		t.Fatalf("detectDevice() detection = %+v, want a detected speed", result.detection)
	}
	if result.device != device {
		t.Errorf("device = %q, want %q", result.device, device)
	}
	pktLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pktLog), "GPGGA") {
		t.Errorf("packet log does not hold the sentences sent:\n%s", pktLog)
	}
}

func TestCaptureDevicePTY(t *testing.T) {
	master, device := openTestPTY(t)
	logPath := filepath.Join(t.TempDir(), "capture.jsonl")
	writeCtx, cancelWrite := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-writeCtx.Done():
				return
			case <-ticker.C:
				if _, err := master.Write([]byte("$GPGGA,,,,,,0,00,99.99,,,,,,*48\r\n")); err != nil {
					return
				}
			}
		}
	})
	result := captureDevice(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		device,
		38400,
		logPath,
		100*time.Millisecond,
	)
	cancelWrite()
	wg.Wait()
	if result.failure != "" || result.packets == 0 {
		t.Fatalf("captureDevice() = %+v, want captured packets", result)
	}
	pktLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pktLog), "GPGGA") {
		t.Errorf("packet log does not hold the sentence sent:\n%s", pktLog)
	}
}

func TestCaptureDeviceNoPackets(t *testing.T) {
	_, device := openTestPTY(t)
	result := captureDevice(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		device,
		0,
		"",
		25*time.Millisecond,
	)
	if code := result.exitCode(); code != 2 {
		t.Fatalf("captureDevice() = %+v, want exit code 2", result)
	}
}

func openTestPTY(t *testing.T) (*os.File, string) {
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
	return master, fmt.Sprintf("/dev/pts/%d", n)
}
