package pmc

import (
	"fmt"
	"net"
	"os"
)

type Transport struct {
	Conn       net.PacketConn
	RemoteAddr net.Addr
}

func NewUnixTransport(rPath string) (*Transport, error) {
	// Local abstract name: @satpulse-<pid>-<rand>
	// Abstract socket (starts with \x00): no filesystem, no cleanup, but repliable
	laddr := &net.UnixAddr{
		Name: fmt.Sprintf("\x00satpulse-%d", os.Getpid()),
		Net:  "unixgram",
	}
	conn, err := net.ListenUnixgram("unixgram", laddr)
	if err != nil {
		return nil, fmt.Errorf("could not create local unixgram socket: %w", err)
	}
	return &Transport{
		Conn: conn,
		RemoteAddr: &net.UnixAddr{
			Name: rPath,
			Net:  "unixgram",
		},
	}, nil
}
