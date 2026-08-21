package term

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jclark/satpulse/gps/lib/kpps"
	"golang.org/x/sys/unix"
)

// pps_mode signal values from <dev/uart/uart_ppstypes.h>, shared by uart(4)
// and ucom(4).
const (
	uartPPSSignalCTS = 0x01
	uartPPSSignalDCD = 0x02
)

// kernelPPSPinWatch reads pin edges from the kernel PPS source that FreeBSD
// serial drivers implement on the TTY itself. The kernel timestamps each edge
// when the driver processes the modem-status change; the independent assert
// and clear sequence counters make missed-edge accounting exact.
type kernelPPSPinWatch struct {
	source     *kpps.Source
	sourceOnce sync.Once
	sourceErr  error
	cancelled  atomic.Bool
	inWait     atomic.Bool
	seq        kernelPPSSeq
}

var _ KernelModemControlPinWatcher = (*unixTerm)(nil)

// NewKernelModemControlPinWatch creates a watch that uses kernel PPS to
// timestamp transitions of the pin the device's pps_mode sysctl captures.
func (t *unixTerm) NewKernelModemControlPinWatch(pin ModemControlPin) (ModemControlPinWatch, error) {
	return newKernelPPSPinWatch(t, pin)
}

// newKernelPPSPinWatch checks that the device captures the requested pin and
// creates a kernel PPS source on a duplicate of the TTY descriptor. The
// driver captures edges whether or not anyone fetches them, so there is
// nothing to attach and nothing to restore on Close.
func newKernelPPSPinWatch(t *unixTerm, pin ModemControlPin) (*kernelPPSPinWatch, error) {
	var want uint32
	var pinName string
	switch pin {
	case ModemCTS:
		want, pinName = uartPPSSignalCTS, "cts"
	case ModemDCD:
		want, pinName = uartPPSSignalDCD, "dcd"
	default:
		return nil, fmt.Errorf("%s: kernel PPS reports only CTS and DCD changes: %w", t.path, errors.ErrUnsupported)
	}
	if err := checkPPSMode(t.path, pinName, want); err != nil {
		return nil, err
	}
	fd, err := unix.FcntlInt(uintptr(t.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "fcntl(F_DUPFD_CLOEXEC)", Path: t.path, Err: err}
	}
	source, err := kpps.OpenFD(fd, t.path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	info, err := source.Fetch(kpps.Info{}, 0)
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	w := &kernelPPSPinWatch{source: source}
	w.seq.lastAssert = info.Assert.Sequence
	w.seq.lastClear = info.Clear.Sequence
	return w, nil
}

// checkPPSMode requires the sysctl that configures the device's PPS capture
// to select exactly the requested pin. The invert (0x10) and narrow-pulse
// (0x20) options change what the captured edges mean -- invert swaps them,
// and narrow-pulse synthesizes an assert/clear pair from every transition --
// so they must be clear; then kernel ASSERT means the pin became asserted in
// the TIOCM sense, exactly as the wait method reports it. The watch never
// changes the sysctl: the wrong value is fixable configuration, so a mismatch
// is ErrUnavailable and the error names the sysctl and the value to set.
func checkPPSMode(path, pinName string, want uint32) error {
	name, err := ppsModeSysctl(path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	mode, err := unix.SysctlUint32(name)
	if err != nil {
		return fmt.Errorf("%w: %s: sysctl %s: %w", ErrUnavailable, path, name, err)
	}
	if mode != want {
		return fmt.Errorf("%w: %s: sysctl %s=%#x does not capture %s alone; set it to %#x", ErrUnavailable, path, name, mode, pinName, want)
	}
	return nil
}

// ppsModeSysctl maps a serial device path to the sysctl that selects its PPS
// capture pin: dev.uart.N.pps_mode for a uart(4) port, or the global
// hw.usb.ucom.pps_mode shared by every ucom(4) USB port.
func ppsModeSysctl(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	name := strings.TrimPrefix(path, "/dev/")
	for _, prefix := range []string{"cuau", "ttyu"} {
		if unit, ok := strings.CutPrefix(name, prefix); ok {
			if _, err := strconv.Atoi(unit); err == nil {
				return "dev.uart." + unit + ".pps_mode", nil
			}
		}
	}
	if strings.HasPrefix(name, "cuaU") || strings.HasPrefix(name, "ttyU") {
		return "hw.usb.ucom.pps_mode", nil
	}
	return "", fmt.Errorf("%s: no pps_mode sysctl for this device", path)
}

// Wait fetches the next kernel PPS event. The sequence numbers the watch has
// already accounted for are the baseline Fetch waits past, so an edge that
// arrived between calls is returned without waiting. kpps waits in bounded
// slices, so a cancellation takes effect within one slice.
func (w *kernelPPSPinWatch) Wait() (ModemControlPinChange, int, error) {
	w.inWait.Store(true)
	defer w.inWait.Store(false)
	if w.cancelled.Load() {
		return ModemControlPinChange{}, 0, ErrCancelled
	}
	for {
		previous := kpps.Info{
			Assert: kpps.Edge{Sequence: w.seq.lastAssert},
			Clear:  kpps.Edge{Sequence: w.seq.lastClear},
		}
		info, err := w.source.Fetch(previous, -1)
		tRead := time.Now()
		if w.cancelled.Load() {
			return ModemControlPinChange{}, 0, ErrCancelled
		}
		if err != nil {
			return ModemControlPinChange{}, 0, err
		}
		if change, missed, ok := w.seq.update(info, tRead); ok {
			return change, missed, nil
		}
	}
}

// Cancel is sticky. Closing the source makes a pending Fetch fail when its
// current bounded slice ends; every current or subsequent Wait observes
// cancelled.
func (w *kernelPPSPinWatch) Cancel() {
	w.cancelled.Store(true)
	_ = w.closeSource()
}

func (w *kernelPPSPinWatch) Close() error {
	if w.inWait.Load() {
		panic("term: ModemControlPinWatch.Close during Wait")
	}
	return w.closeSource()
}

func (w *kernelPPSPinWatch) closeSource() error {
	w.sourceOnce.Do(func() {
		w.sourceErr = w.source.Close()
	})
	return w.sourceErr
}
