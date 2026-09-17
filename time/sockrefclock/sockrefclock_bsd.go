//go:build darwin || freebsd

package sockrefclock

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// listenUnixgramUnbound returns a unixgram socket with no local filesystem path.
// The BSDs have no autobind, so net.ListenUnixgram with an empty name fails;
// create the socket directly and leave it unbound. chrony never replies, so the
// socket only needs to send, and an unbound socket has no inode to create,
// secure, or remove.
func listenUnixgramUnbound() (*net.UnixConn, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "unixgram")
	defer f.Close() // FileConn dups the fd; drop our copy
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("FileConn returned %T, not *net.UnixConn", conn)
	}
	return uc, nil
}
