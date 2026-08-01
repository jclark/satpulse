package term

import (
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

const testArbitrarySpeed = 123457

func TestArbitrarySpeed(t *testing.T) {
	path := newTestPTY(t)
	fd := openTestTTY(t, path)
	setTestArbitrarySpeed(t, fd, testArbitrarySpeed)
	if err := unix.Close(fd); err != nil {
		t.Fatalf("close setup fd: %v", err)
	}

	term, err := Open(path, RawMode)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := term.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if got := term.Speed(); got != testArbitrarySpeed {
		t.Errorf("Speed() = %d, want %d", got, testArbitrarySpeed)
	}
	checkTestArbitrarySpeed(t, term.fd, testArbitrarySpeed)

	if err := term.Change(Local); err != nil {
		t.Fatalf("Change: %v", err)
	}
	checkTestArbitrarySpeed(t, term.fd, testArbitrarySpeed)

	if err := term.Change(Speed(9600)); err != nil {
		t.Fatalf("Change Speed(9600): %v", err)
	}
	if got := term.Speed(); got != 9600 {
		t.Errorf("Speed() after Change = %d, want 9600", got)
	}
	checkTestSpeed(t, term.fd, unix.B9600, 0, 9600)

	if err := term.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	checkTestArbitrarySpeed(t, term.fd, testArbitrarySpeed)
}

func TestArbitrarySpeedCannotBeSet(t *testing.T) {
	if err := Speed(testArbitrarySpeed)(new(Attr)); err == nil {
		t.Errorf("Speed(%d) succeeded", testArbitrarySpeed)
	}
}

func newTestPTY(t *testing.T) string {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	t.Cleanup(func() {
		if err := unix.Close(fd); err != nil {
			t.Errorf("close PTY master: %v", err)
		}
	})
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatalf("unlock PTY: %v", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatalf("get PTY number: %v", err)
	}
	return fmt.Sprintf("/dev/pts/%d", n)
}

func openTestTTY(t *testing.T, path string) int {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	return fd
}

func setTestArbitrarySpeed(t *testing.T, fd int, speed uint32) {
	t.Helper()
	tp, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		t.Fatalf("ioctl(TCGETS2): %v", err)
	}
	tp.Cflag &^= unix.CBAUD | unix.CIBAUD
	tp.Cflag |= unix.BOTHER | unix.BOTHER<<16
	tp.Ispeed = speed
	tp.Ospeed = speed
	if err := unix.IoctlSetTermios(fd, unix.TCSETS2, tp); err != nil {
		t.Fatalf("ioctl(TCSETS2): %v", err)
	}
}

func checkTestArbitrarySpeed(t *testing.T, fd int, speed uint32) {
	checkTestSpeed(t, fd, unix.BOTHER, unix.BOTHER<<16, speed)
}

func checkTestSpeed(t *testing.T, fd int, ob, ib, speed uint32) {
	t.Helper()
	tp, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		t.Fatalf("ioctl(TCGETS2): %v", err)
	}
	if got := tp.Cflag & unix.CBAUD; got != ob {
		t.Errorf("output CBAUD = %#x, want %#x", got, ob)
	}
	if got := tp.Cflag & unix.CIBAUD; got != ib {
		t.Errorf("input CBAUD = %#x, want %#x", got, ib)
	}
	if tp.Ispeed != speed {
		t.Errorf("Ispeed = %d, want %d", tp.Ispeed, speed)
	}
	if tp.Ospeed != speed {
		t.Errorf("Ospeed = %d, want %d", tp.Ospeed, speed)
	}
}
