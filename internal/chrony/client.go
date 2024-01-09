package chrony

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

type Client struct {
	cleanupMutex sync.Mutex
	conn         net.PacketConn
	remoteAddr   net.Addr
	localPath    string
}

func NewClient(lPathFormat, rPath string) (*Client, error) {
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
	return &Client{
		localPath: lPath,
		conn:      conn,
		remoteAddr: &net.UnixAddr{
			Name: rPath,
			Net:  "unixgram",
		},
	}, nil
}

func (c *Client) RemotePath() string {
	return c.remoteAddr.String()
}

// This is safe to call from multiple goroutines
func (c *Client) cleanup() error {
	c.cleanupMutex.Lock()
	defer c.cleanupMutex.Unlock()
	lPath := c.localPath
	if lPath == "" {
		return nil
	}
	c.localPath = ""
	return os.Remove(lPath)
}

func (c *Client) Send(sys time.Time, ref ptime.Time, ls ptime.LeapSecond, timeout time.Duration) error {
	pkt, err := RefclockSockPacket(sys, ref, ls)
	if err != nil {
		return err
	}
	tStart := time.Now()
	c.conn.SetWriteDeadline(tStart.Add(timeout))
	_, err = c.conn.WriteTo(pkt, c.remoteAddr)
	if err != nil {
		return fmt.Errorf("could not send chrony update message: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	closeErr := c.conn.Close()
	cleanErr := c.cleanup()
	if closeErr != nil {
		return closeErr
	}
	return cleanErr
}
