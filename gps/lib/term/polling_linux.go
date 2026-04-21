package term

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// OpenPolling opens path as an *os.File suitable for reading with
// SetReadDeadline-based timeouts. It is intended for GNSS devices that
// speak a GPS protocol over a non-termios interface (e.g. /dev/gnss0).
//
// The device must be a character device whose driver implements .poll.
// The file descriptor is opened O_RDWR|O_NOCTTY|O_CLOEXEC|O_NONBLOCK
// and locked exclusively with flock.
func OpenPolling(path string) (*os.File, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return nil, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFCHR:
		// ok
	case unix.S_IFREG:
		return nil, fmt.Errorf("%s: regular files are not supported", path)
	case unix.S_IFBLK:
		return nil, fmt.Errorf("%s: block devices are not supported", path)
	case unix.S_IFDIR:
		return nil, fmt.Errorf("%s: is a directory", path)
	case unix.S_IFSOCK:
		return nil, fmt.Errorf("%s: sockets are not supported", path)
	case unix.S_IFIFO:
		return nil, fmt.Errorf("%s: FIFOs are not supported", path)
	default:
		return nil, fmt.Errorf("%s: unsupported file type", path)
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := probePoll(fd); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("%s: device does not support polling: %w", path, err)
	}
	if err := lock(fd, path); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
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
