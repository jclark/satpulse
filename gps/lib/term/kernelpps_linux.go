package term

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jclark/satpulse/gps/lib/kpps"
	"golang.org/x/sys/unix"
)

// nPPS is the N_PPS line discipline number from <linux/tty.h>;
// golang.org/x/sys/unix does not define it.
const nPPS = 18

const (
	ppsOpenRetryTotal    = 2 * time.Second
	ppsOpenRetryInterval = 100 * time.Millisecond
)

// kernelPPSPinWatch reads DCD edges from the kernel PPS source created by
// attaching the N_PPS line discipline to a TTY. The kernel timestamps each
// edge when the driver processes the modem-status change; the independent
// assert and clear sequence counters make missed-edge accounting exact.
type kernelPPSPinWatch struct {
	fd         int // duplicate TTY fd; holds the port claim and restores the ldisc
	source     *kpps.Source
	sourceOnce sync.Once
	sourceErr  error
	savedLdisc int
	ldiscSet   bool
	cancelled  atomic.Bool
	inWait     atomic.Bool
	path       string
	seq        kernelPPSSeq
}

// newKernelPPSPinWatch attaches N_PPS to the TTY and opens the source it
// creates. Any setup failure restores the saved line discipline. The line
// discipline is a property of the TTY, not the descriptor; N_PPS chains to
// N_TTY, so normal data I/O continues while the watch is active.
func newKernelPPSPinWatch(t *unixTerm, pin ModemControlPin) (*kernelPPSPinWatch, error) {
	if pin != ModemDCD {
		return nil, fmt.Errorf("%s: kernel PPS reports only DCD changes: %w", t.path, errors.ErrUnsupported)
	}
	fd, err := unix.FcntlInt(uintptr(t.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, t.wrapErr(err, "fcntl(F_DUPFD_CLOEXEC)")
	}
	w := &kernelPPSPinWatch{fd: fd, path: t.path}
	if err := w.setup(); err != nil {
		_ = w.teardown()
		return nil, err
	}
	return w, nil
}

func (w *kernelPPSPinWatch) setup() error {
	ldisc, err := unix.IoctlGetInt(w.fd, unix.TIOCGETD)
	if err != nil {
		return w.wrapErr(err, "ioctl(TIOCGETD)")
	}
	w.savedLdisc = ldisc
	// A kernel built without N_PPS, a system that will not autoload N_PPS, a
	// TTY with no source of its own, and a /dev/ppsN we may not open are all
	// systems where this method cannot run but another can, so they are
	// unavailable rather than unsupported: the caller warns and falls back
	// instead of failing the run.
	if err := unix.IoctlSetPointerInt(w.fd, unix.TIOCSETD, nPPS); err != nil {
		return w.wrapErr(kernelPPSAttachError(err), "ioctl(TIOCSETD)")
	}
	w.ldiscSet = true

	path, err := kpps.DevicePathForTTY(w.fd)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	w.source, err = openKernelPPSRetry(path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	info, err := w.source.Fetch(kpps.Info{}, 0)
	if err != nil {
		return err
	}
	w.seq.lastAssert = info.Assert.Sequence
	w.seq.lastClear = info.Clear.Sequence
	return nil
}

func kernelPPSAttachError(err error) error {
	switch {
	case errors.Is(err, unix.EINVAL):
		return fmt.Errorf("%w: no N_PPS line discipline: %w", ErrUnavailable, err)
	case errors.Is(err, unix.EPERM):
		return fmt.Errorf("%w: cannot attach N_PPS line discipline: %w", ErrUnavailable, err)
	default:
		return err
	}
}

// openKernelPPSRetry allows time for udev to change the permissions on a new
// /dev/ppsN node after TIOCSETD has synchronously created it.
func openKernelPPSRetry(path string) (*kpps.Source, error) {
	deadline := time.Now().Add(ppsOpenRetryTotal)
	for {
		source, err := kpps.Open(path)
		if err == nil {
			return source, nil
		}
		if !errors.Is(err, unix.EACCES) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(ppsOpenRetryInterval)
	}
}

// Wait fetches the next kernel PPS event. The sequence numbers the watch has
// already accounted for are the baseline Fetch waits past, so an edge that
// arrived between calls is returned without waiting. kpps uses the Go runtime
// poller, so closing the source in Cancel promptly wakes a pending Fetch.
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

// kernelPPSSeq turns the independently sequenced assert and clear edges in a
// kpps.Info into pin changes. A fetch reports the most recent edge of each
// polarity, but a Wait returns one change, so only the newest edge is
// reported; anything else the fetch found is counted as missed, along with
// the edges whose timestamps the latches had already overwritten.
type kernelPPSSeq struct {
	lastAssert uint32
	lastClear  uint32
}

func (s *kernelPPSSeq) update(info kpps.Info, tRead time.Time) (ModemControlPinChange, int, bool) {
	// The counters only ever advance, so the unsigned difference is the number
	// of captures since the last fetch however the 32-bit counter wraps.
	assertDelta := info.Assert.Sequence - s.lastAssert
	clearDelta := info.Clear.Sequence - s.lastClear
	s.lastAssert = info.Assert.Sequence
	s.lastClear = info.Clear.Sequence
	if assertDelta == 0 && clearDelta == 0 {
		return ModemControlPinChange{}, 0, false
	}
	// One edge is reported and the rest are missed: the ones whose timestamps
	// the latches overwrote, and, when both polarities are new, the older of
	// the two, since a wait reports one change.
	missed := int(assertDelta) + int(clearDelta) - 1
	assert := ModemControlPinChange{Timestamp: info.Assert.T, TRead: tRead, Asserted: true}
	clear := ModemControlPinChange{Timestamp: info.Clear.T, TRead: tRead, Asserted: false}
	if clearDelta == 0 {
		return assert, missed, true // only the assert is new
	}
	if assertDelta == 0 {
		return clear, missed, true // only the clear is new
	}
	if info.Clear.T.After(info.Assert.T) {
		return clear, missed, true // both are new; the clear is the later one
	}
	return assert, missed, true // both are new; the assert is the later one
}

// Cancel is sticky. Closing a kpps source interrupts a Fetch waiting in the
// runtime poller; every current or subsequent Wait observes cancelled.
func (w *kernelPPSPinWatch) Cancel() {
	w.cancelled.Store(true)
	_ = w.closeSource()
}

func (w *kernelPPSPinWatch) Close() error {
	if w.inWait.Load() {
		panic("term: ModemControlPinWatch.Close during Wait")
	}
	return w.teardown()
}

// teardown restores the saved line discipline and then releases the PPS and
// TTY descriptors, reporting the first error.
func (w *kernelPPSPinWatch) teardown() error {
	var firstErr error
	if w.ldiscSet {
		w.ldiscSet = false
		if err := unix.IoctlSetPointerInt(w.fd, unix.TIOCSETD, w.savedLdisc); err != nil {
			firstErr = w.wrapErr(err, "ioctl(TIOCSETD)")
		}
	}
	if err := w.closeSource(); err != nil && firstErr == nil {
		firstErr = err
	}
	if w.fd >= 0 {
		if err := unix.Close(w.fd); err != nil && firstErr == nil {
			firstErr = w.wrapErr(err, "close")
		}
		w.fd = -1
	}
	return firstErr
}

func (w *kernelPPSPinWatch) closeSource() error {
	w.sourceOnce.Do(func() {
		if w.source != nil {
			w.sourceErr = w.source.Close()
		}
	})
	return w.sourceErr
}

func (w *kernelPPSPinWatch) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: op, Path: w.path, Err: err}
}
