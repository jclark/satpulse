package ubxsimcmd

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Termios ioctl requests for raw mode on the pty slave.
const (
	reqGetTermios = unix.TCGETS
	reqSetTermios = unix.TCSETS
)

// ptySlavePath unlocks the pty master's slave end (unlockpt) and
// returns the slave path (ptsname).
func ptySlavePath(fd uintptr) (string, error) {
	n, err := unix.IoctlGetInt(int(fd), unix.TIOCGPTN)
	if err != nil {
		return "", err
	}
	if err := unix.IoctlSetPointerInt(int(fd), unix.TIOCSPTLCK, 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}
