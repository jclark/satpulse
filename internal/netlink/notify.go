package netlink

import (
	"errors"
	"fmt"
	"net"

	"github.com/mdlayher/netlink"
)

type Notifier[E any] struct {
	Events <-chan E
	conn   *netlink.Conn
}

type setter interface {
	setFromMessage(msg *netlink.Message) bool
	setError(err error)
}

func notify[E any, PE interface {
	*E
	setter
}](family int, groups uint32) (*Notifier[E], error) {
	conn, err := netlink.Dial(family, &netlink.Config{Groups: groups})
	if err != nil {
		return nil, fmt.Errorf("failed to dial netlink: %w", err)
	}
	ch := make(chan E)
	n := &Notifier[E]{Events: ch, conn: conn}
	go func() {
		defer close(ch)
		for {
			var ev E
			var pev PE = &ev
			msgs, err := n.conn.Receive()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					break
				}
				pev.setError(fmt.Errorf("failed to receive messages: %w", err))
				ch <- ev
				continue
			}
			for i := range msgs {
				if pev.setFromMessage(&msgs[i]) {
					ch <- ev
				}
			}
		}
	}()
	return n, nil
}

func (n *Notifier[E]) Close() error {
	err := n.conn.Close()
	if err != nil {
		return err
	}
	for range n.Events {
		// Drain events channel
	}
	return nil
}
