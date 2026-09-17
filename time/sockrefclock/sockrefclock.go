package sockrefclock

import (
	"fmt"
	"net"
	"time"

	"github.com/jclark/satpulse/time/lib/ntime"
)

// Leap is the leap-second status field of a SOCK sample, using the
// values defined by chrony's refclock SOCK protocol.
type Leap int32

const (
	LeapNone Leap = iota
	LeapInsert
	LeapDelete
)

type SockRefClock struct {
	conn       *net.UnixConn
	remoteAddr *net.UnixAddr
}

// New creates a new SockRefClock.
func New(rPath string) (*SockRefClock, error) {
	conn, err := listenUnixgramUnbound()
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

func (c *SockRefClock) Sample(sys ntime.Time, offset float64, leap Leap) error {
	pkt, err := sockPacket(sys, offset, leap)
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
