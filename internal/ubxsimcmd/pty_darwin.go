package ubxsimcmd

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// Termios ioctl requests for raw mode on the pty slave.
const (
	reqGetTermios = unix.TIOCGETA
	reqSetTermios = unix.TIOCSETA
)

// ptySlavePath grants and unlocks the pty master's slave end (grantpt,
// unlockpt) and returns the slave path (ptsname). TIOCPTYGNAME fills a
// caller-supplied 128-byte buffer, which x/sys/unix has no wrapper for,
// so it goes through unix.Syscall (deprecated but libSystem-backed).
func ptySlavePath(fd uintptr) (string, error) {
	if err := unix.IoctlSetInt(int(fd), unix.TIOCPTYGRANT, 0); err != nil {
		return "", err
	}
	if err := unix.IoctlSetInt(int(fd), unix.TIOCPTYUNLK, 0); err != nil {
		return "", err
	}
	var buf [128]byte
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, fd, unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&buf[0]))); e != 0 {
		return "", e
	}
	return unix.ByteSliceToString(buf[:]), nil
}
