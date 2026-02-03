package sockrefclock

import (
	"fmt"
	"net"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

type SockRefClock struct {
	conn       *net.UnixConn
	remoteAddr *net.UnixAddr
}

// New creates a new SockRefClock.
func New(rPath string) (*SockRefClock, error) {
	// Anonymous unixgram socket: no filesystem path, AppArmor can't complain
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{
		Name: "",
		Net:  "unixgram",
	})
	if err != nil {
		return nil, fmt.Errorf("could not create local unixgram socket: %w", err)
	}

	return &SockRefClock{
		conn: conn,
		remoteAddr: &net.UnixAddr{
			Name: rPath,
			Net:  "unixgram",
		},
	}, nil
}

func (c *SockRefClock) RemotePath() string {
	return c.remoteAddr.String()
}

const sockRefClockTimeout = time.Second / 10

func (c *SockRefClock) Sample(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error {
	pkt, err := sockPacket(sys, ref, ls)
	if err != nil {
		return err
	}
	tStart := time.Now()
	c.conn.SetWriteDeadline(tStart.Add(sockRefClockTimeout))
	_, _, err = c.conn.WriteMsgUnix(pkt, nil, c.remoteAddr)
	if err != nil {
		return fmt.Errorf("could not send chrony update message: %w", err)
	}
	return nil
}

func (c *SockRefClock) Close() error {
	return c.conn.Close()
}
