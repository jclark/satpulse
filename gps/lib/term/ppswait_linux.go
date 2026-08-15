package term

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type waitPinWatch struct {
	fd          int
	mask        uint
	pin         ModemControlPin
	cancelled   atomic.Bool
	inWait      atomic.Bool
	baseline    int32
	haveCounter bool
	path        string
}

func newWaitPinWatch(t *unixTerm, pin ModemControlPin) (*waitPinWatch, error) {
	mask, err := tiocmPinMask(pin)
	if err != nil {
		return nil, err
	}
	fd, err := unix.FcntlInt(uintptr(t.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, t.wrapErr(err, "fcntl(F_DUPFD_CLOEXEC)")
	}
	w := &waitPinWatch{fd: fd, mask: mask, pin: pin, path: t.path}
	if count, err := w.readCounter(); err == nil {
		w.baseline = count
		w.haveCounter = true
	}
	return w, nil
}

// Wait blocks the calling goroutine's OS thread in TIOCMIWAIT. Linux cannot
// interrupt this ioctl, so cancellation becomes observable only after the pin
// next changes. time.Now is the best clock here, so one reading serves as both
// Wall and Mono.
func (w *waitPinWatch) Wait() (ModemControlPinChange, int, error) {
	w.inWait.Store(true)
	defer w.inWait.Store(false)
	if w.cancelled.Load() {
		return ModemControlPinChange{}, 0, ErrCancelled
	}
	missed := 0
	for {
		if w.haveCounter {
			count, err := w.readCounter()
			if err != nil {
				return ModemControlPinChange{}, missed, w.wrapErr(err, "ioctl(TIOCGICOUNT)")
			}
			missed += int(count - w.baseline)
			w.baseline = count
		}

		// An edge between the counter read above and TIOCMIWAIT's entry
		// snapshot inflates the post-wait delta. A clean wakeup can therefore
		// look like two transitions and be withheld; this fails safe and the
		// next loop iteration re-arms the wait.
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(w.fd), unix.TIOCMIWAIT, uintptr(w.mask))
		if errno == unix.EINTR {
			continue
		}
		if errno == unix.ENOTTY {
			err := fmt.Errorf("%w: %w", ErrUnavailable, errno)
			return ModemControlPinChange{}, missed, w.wrapErr(err, "ioctl(TIOCMIWAIT)")
		}
		if errno != 0 {
			return ModemControlPinChange{}, missed, w.wrapErr(errno, "ioctl(TIOCMIWAIT)")
		}
		at := time.Now()
		if w.cancelled.Load() {
			return ModemControlPinChange{}, missed, ErrCancelled
		}
		status, err := ioctlGetModemState(w.fd)
		if err != nil {
			return ModemControlPinChange{}, missed, w.wrapErr(err, "ioctl(TIOCMGET)")
		}
		asserted := uint(status)&w.mask != 0
		if w.haveCounter {
			count, err := w.readCounter()
			if err != nil {
				return ModemControlPinChange{}, missed, w.wrapErr(err, "ioctl(TIOCGICOUNT)")
			}
			delta := int(count - w.baseline)
			w.baseline = count
			if delta != 1 {
				missed += delta
				continue
			}
		}
		return ModemControlPinChange{Wall: at, Mono: at, Asserted: asserted}, missed, nil
	}
}

func (w *waitPinWatch) readCounter() (int32, error) {
	ic, err := ioctlGetSerialICounter(w.fd)
	if err != nil {
		return 0, err
	}
	switch w.pin {
	case ModemCTS:
		return ic.Cts, nil
	case ModemDCD:
		return ic.Dcd, nil
	case ModemDSR:
		return ic.Dsr, nil
	case ModemRI:
		return ic.Rng, nil
	default:
		panic("term: invalid pin in ModemControlPinWatch")
	}
}

func (w *waitPinWatch) Cancel() { w.cancelled.Store(true) }

func (w *waitPinWatch) Close() error {
	if w.inWait.Load() {
		panic("term: ModemControlPinWatch.Close during Wait")
	}
	fd := w.fd
	w.fd = -1
	return w.wrapErr(unix.Close(fd), "close")
}

func (w *waitPinWatch) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: op, Path: w.path, Err: err}
}

func ioctlGetModemState(fd int) (int, error) {
	for {
		status, err := unix.IoctlGetInt(fd, unix.TIOCMGET)
		if err != unix.EINTR {
			return status, err
		}
	}
}

// tiocmPinMask returns the TIOCM_* bit for pin, forming TIOCMIWAIT's mask
// of pins to watch.
func tiocmPinMask(pin ModemControlPin) (uint, error) {
	switch pin {
	case ModemCTS:
		return unix.TIOCM_CTS, nil
	case ModemDCD:
		return unix.TIOCM_CAR, nil
	case ModemDSR:
		return unix.TIOCM_DSR, nil
	case ModemRI:
		return unix.TIOCM_RNG, nil
	}
	return 0, fmt.Errorf("invalid modem control pin: %d", pin)
}
