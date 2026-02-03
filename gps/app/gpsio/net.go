package gpsio

import (
	"io"
	"net"
	"sync"
	"time"
)

type NetConn struct {
	conn     net.Conn
	mu       sync.Mutex
	closed   bool
	closeErr error
}

var _ Conn = (*NetConn)(nil)

func OpenSocket(path string) (*NetConn, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return &NetConn{conn: conn}, nil
}

func (c *NetConn) LocalAddr() string {
	return c.conn.LocalAddr().String()
}

func (c *NetConn) Read(p []byte) (int, error) {
	deadline := time.Now().Add(readTimeout)
	c.conn.SetReadDeadline(deadline)
	n, err := c.conn.Read(p)
	if err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		// scanner assumes we get io.EOF on end of stream
		if c.closed {
			err = io.EOF
		}
	}
	return n, err
}

func (c *NetConn) Write(p []byte) (int, error) {
	return c.conn.Write(p)
}

func (c *NetConn) Buffered() (int, error) {
	return 0, nil
}

func (c *NetConn) TransmitTime(nBytes int) time.Duration {
	return 0
}

func (c *NetConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		err := c.closeErr
		c.closeErr = nil
		return err
	}
	c.closed = true
	return c.conn.Close()
}

func (c *NetConn) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.closeErr = c.conn.Close()
}
