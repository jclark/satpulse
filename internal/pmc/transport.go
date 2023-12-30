package pmc

import (
	"fmt"
	"net"
	"os"
	"sync"
)

type Transport struct {
	cleanupMutex sync.Mutex
	cleanup      func() error

	Conn       net.PacketConn
	RemoteAddr net.Addr
}

// This is safe to call from multiple goroutines
func (t *Transport) Cleanup() error {
	t.cleanupMutex.Lock()
	defer t.cleanupMutex.Unlock()
	cleanup := t.cleanup
	if cleanup == nil {
		return nil
	}
	t.cleanup = nil
	return cleanup()
}

func NewUnixTransport(lPath, rPath string) (*Transport, error) {
	_ = os.Remove(lPath)
	conn, err := net.DialUnix("unixgram", &net.UnixAddr{
		Name: lPath,
		Net:  "unixgram",
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create connection: %w", err)
	}
	err = os.Chmod(lPath, 0660)
	if err != nil {
		return nil, fmt.Errorf("could not chmod: %s: %w", lPath, err)
	}
	return &Transport{
		cleanup: func() error { return os.Remove(lPath) },
		Conn:    conn,
		RemoteAddr: &net.UnixAddr{
			Name: rPath,
			Net:  "unixgram",
		},
	}, nil
}
