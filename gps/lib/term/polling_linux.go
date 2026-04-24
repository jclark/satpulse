package term

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// OpenPolling opens path as an *os.File suitable for reading with
// SetReadDeadline-based timeouts. It is intended for GNSS devices that
// speak a GPS protocol over a non-termios interface (e.g. /dev/gnss0)
// and for FIFOs used as a replay sink.
//
// Character devices must have a driver that implements .poll; they are
// opened O_RDWR|O_NOCTTY|O_CLOEXEC|O_NONBLOCK and classified as
// DevUnknown (the UART/USB/BT classification applies only to TTYs).
// FIFOs are opened O_RDWR|O_NONBLOCK -- so that our own fd keeps the
// write side of the pipe open, avoiding EOF on every read before an
// external writer connects -- and classified as DevFIFO. Writes into a
// DevFIFO port are rejected at the gpsio layer to prevent self-feeding.
// The file descriptor is locked exclusively with flock.
func OpenPolling(path string) (*os.File, DevKind, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return nil, DevUnknown, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFCHR:
		return openPollingChar(path)
	case unix.S_IFIFO:
		return openPollingFIFO(path)
	case unix.S_IFREG:
		return nil, DevUnknown, fmt.Errorf("%s: regular files are not supported", path)
	case unix.S_IFBLK:
		return nil, DevUnknown, fmt.Errorf("%s: block devices are not supported", path)
	case unix.S_IFDIR:
		return nil, DevUnknown, fmt.Errorf("%s: is a directory", path)
	case unix.S_IFSOCK:
		return nil, DevUnknown, fmt.Errorf("%s: sockets are not supported", path)
	default:
		return nil, DevUnknown, fmt.Errorf("%s: unsupported file type", path)
	}
}

func openPollingChar(path string) (*os.File, DevKind, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := probePoll(fd); err != nil {
		unix.Close(fd)
		return nil, DevUnknown, fmt.Errorf("%s: device does not support polling: %w", path, err)
	}
	if err := lock(fd, path); err != nil {
		unix.Close(fd)
		return nil, DevUnknown, err
	}
	return os.NewFile(uintptr(fd), path), DevUnknown, nil
}

func openPollingFIFO(path string) (*os.File, DevKind, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, DevUnknown, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := lock(fd, path); err != nil {
		unix.Close(fd)
		return nil, DevUnknown, err
	}
	return os.NewFile(uintptr(fd), path), DevFIFO, nil
}

// probePoll verifies that the driver backing fd implements .poll by
// attempting to register the fd with a throwaway epoll instance.
func probePoll(fd int) error {
	ep, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return fmt.Errorf("epoll_create1: %w", err)
	}
	defer unix.Close(ep)
	ev := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(fd)}
	if err := unix.EpollCtl(ep, unix.EPOLL_CTL_ADD, fd, &ev); err != nil {
		return fmt.Errorf("epoll_ctl: %w", err)
	}
	return nil
}
