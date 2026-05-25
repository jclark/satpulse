package term

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// OpenBT opens an RFCOMM SPP connection to addr on channel 1.
// The remote device must already be paired. The returned file is
// non-blocking so callers can use SetReadDeadline for timeouts.
func OpenBT(addr BDAddr) (*os.File, error) {
	fd, err := unix.Socket(unix.AF_BLUETOOTH, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.BTPROTO_RFCOMM)
	if err != nil {
		return nil, fmt.Errorf("bluetooth socket: %w", err)
	}
	// SockaddrRFCOMM.Addr is little-endian (LSB first); reverse from textual order.
	var sa unix.SockaddrRFCOMM
	sa.Channel = 1
	for i := 0; i < 6; i++ {
		sa.Addr[i] = addr[5-i]
	}
	if err := unix.Connect(fd, &sa); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("bluetooth connect %s: %w", addr, err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("bluetooth set nonblock: %w", err)
	}
	return os.NewFile(uintptr(fd), "bt:"+addr.String()), nil
}
