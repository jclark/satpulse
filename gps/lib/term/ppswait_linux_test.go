package term

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// TestModemControlPinWatchUnsupported checks that a tty whose driver
// lacks TIOCMIWAIT (a pty) reports that the wait facility is unavailable for
// this driver, preserving ENOTTY as the underlying cause. It deliberately
// does not report errors.ErrUnsupported, which is reserved for a backend that
// has no wait facility at all.
func TestModemControlPinWatchUnsupported(t *testing.T) {
	w := openTestWatcher(t)
	watch, err := w.NewModemControlPinWatch(ModemCTS)
	if err != nil {
		t.Fatalf("NewModemControlPinWatch: %v", err)
	}
	t.Cleanup(func() {
		if err := watch.Close(); err != nil {
			t.Errorf("ModemControlPinWatch.Close: %v", err)
		}
	})
	_, _, err = watch.Wait()
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, unix.ENOTTY) || errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("ModemControlPinWatch.Wait error = %v, want ErrUnavailable wrapping ENOTTY without errors.ErrUnsupported", err)
	}
}

// TestModemControlPinWatchCancel checks that cancellation is sticky and is observed before
// the ioctl: on a pty a non-cancelled wait fails with ErrUnavailable, so
// ErrCancelled proves the ioctl was never entered.
func TestModemControlPinWatchCancel(t *testing.T) {
	w := openTestWatcher(t)
	watch, err := w.NewModemControlPinWatch(ModemCTS)
	if err != nil {
		t.Fatalf("NewModemControlPinWatch: %v", err)
	}
	t.Cleanup(func() {
		if err := watch.Close(); err != nil {
			t.Errorf("ModemControlPinWatch.Close: %v", err)
		}
	})
	watch.Cancel()
	if c, missed, err := watch.Wait(); !errors.Is(err, ErrCancelled) || c != (ModemControlPinChange{}) || missed != 0 {
		t.Fatalf("ModemControlPinWatch.Wait after cancel = %+v, %d, %v; want zero change, 0, ErrCancelled", c, missed, err)
	}
}

func TestNewModemControlPinWatchInvalidLine(t *testing.T) {
	w := openTestWatcher(t)
	if _, err := w.NewModemControlPinWatch(ModemControlPin(99)); err == nil {
		t.Fatal("NewModemControlPinWatch accepted an invalid line")
	}
}

func openTestWatcher(t *testing.T) ModemControlPinWatcher {
	t.Helper()
	term, err := Open(newTestPTY(t), RawMode)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := term.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	w, ok := term.(ModemControlPinWatcher)
	if !ok {
		t.Fatalf("%T does not implement ModemControlPinWatcher", term)
	}
	return w
}
