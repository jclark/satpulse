package netnotify

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

type Notifier[E any] struct {
	Events <-chan E
	conn   *netlink.Conn
}

type setter interface {
	setFromMessage(msg *netlink.Message) bool
	setError(err error)
}

type InterfaceEvent struct {
	Index int       // Interface index
	Flags net.Flags // Interface flags
	Err   error
}

func OpenNotifier() (*Notifier[InterfaceEvent], error) {
	return open[InterfaceEvent](unix.NETLINK_ROUTE, unix.RTMGRP_LINK)
}

func open[E any, PE interface {
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

func (ev *InterfaceEvent) setError(err error) {
	ev.Err = err
}

func (ev *InterfaceEvent) setFromMessage(msg *netlink.Message) bool {
	if msg.Header.Type != unix.RTM_NEWLINK {
		return false
	}
	data := msg.Data
	r := bytes.NewReader(data)
	ifim := unix.IfInfomsg{}
	err := binary.Read(r, binary.NativeEndian, &ifim)
	if err != nil {
		ev.Err = fmt.Errorf("failed to interpret IfInfomsg: %w", err)
		return true
	}
	ev.Index = int(ifim.Index)
	ev.Flags = netFlags(ifim.Flags)
	return true
}

func netFlags(rawFlags uint32) net.Flags {
	var f net.Flags
	if rawFlags&unix.IFF_UP != 0 {
		f |= net.FlagUp
	}
	if rawFlags&unix.IFF_RUNNING != 0 {
		f |= net.FlagRunning
	}
	if rawFlags&unix.IFF_BROADCAST != 0 {
		f |= net.FlagBroadcast
	}
	if rawFlags&unix.IFF_LOOPBACK != 0 {
		f |= net.FlagLoopback
	}
	if rawFlags&unix.IFF_POINTOPOINT != 0 {
		f |= net.FlagPointToPoint
	}
	if rawFlags&unix.IFF_MULTICAST != 0 {
		f |= net.FlagMulticast
	}
	return f
}
