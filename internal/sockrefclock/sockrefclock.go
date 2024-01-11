package sockrefclock

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

const defaultLocalPathFormat = "/var/run/satpulse-chrony%d.sock"

type SockRefClock struct {
	cleanupMutex sync.Mutex
	conn         net.PacketConn
	remoteAddr   net.Addr
	localPath    string
}

// New creates a new SockRefClock.
// lPathFormat is a format string for the local path;
// it must contain a single format verb for the process ID.
// If lPathFormat is empty, a default is used.
// The local path is created with permissions 0660.
func New(lPathFormat, rPath string) (*SockRefClock, error) {
	if lPathFormat == "" {
		lPathFormat = defaultLocalPathFormat
	}
	pid := os.Getpid()
	lPath := fmt.Sprintf(lPathFormat, pid)
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
	return &SockRefClock{
		localPath: lPath,
		conn:      conn,
		remoteAddr: &net.UnixAddr{
			Name: rPath,
			Net:  "unixgram",
		},
	}, nil
}

func (c *SockRefClock) RemotePath() string {
	return c.remoteAddr.String()
}

// This is safe to call from multiple goroutines
func (c *SockRefClock) cleanup() error {
	c.cleanupMutex.Lock()
	defer c.cleanupMutex.Unlock()
	lPath := c.localPath
	if lPath == "" {
		return nil
	}
	c.localPath = ""
	return os.Remove(lPath)
}

const sockRefClockTimeout = time.Second / 10

func (c *SockRefClock) Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error {
	pkt, err := sockPacket(sys, ref, ls)
	if err != nil {
		return err
	}
	tStart := time.Now()
	c.conn.SetWriteDeadline(tStart.Add(sockRefClockTimeout))	
	_, err = c.conn.WriteTo(pkt, c.remoteAddr)
	if err != nil {
		return fmt.Errorf("could not send chrony update message: %w", err)
	}
	return nil
}

func (c *SockRefClock) Close() error {
	closeErr := c.conn.Close()
	cleanErr := c.cleanup()
	if closeErr != nil {
		return closeErr
	}
	return cleanErr
}
