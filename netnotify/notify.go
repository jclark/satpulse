package netnotify

import (
	"errors"
	"fmt"
	"net"
	"unsafe"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

type Notifier struct {
	Events <-chan InterfaceEvent
	conn   *netlink.Conn
}

type InterfaceEvent struct {
	Index int       // Interface index
	Flags net.Flags // Interface flags
	Err   error
}

func OpenNotifier() (*Notifier, error) {
	conn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{Groups: unix.RTMGRP_LINK})
	if err != nil {
		return nil, fmt.Errorf("failed to dial netlink: %w", err)
	}
	ch := make(chan InterfaceEvent)
	n := &Notifier{Events: ch, conn: conn}
	go n.recv(ch)
	return n, nil
}

func (n *Notifier) Close() error {
	err := n.conn.Close()
	if err != nil {
		return err
	}
	for range n.Events {
		// Drain events channel
	}
	return nil
}

func (n *Notifier) recv(ch chan<- InterfaceEvent) {
	defer close(ch)
	for {
		msgs, err := n.conn.Receive()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				break
			}
			ch <- InterfaceEvent{Err: fmt.Errorf("failed to receive messages: %w", err)}
			continue
		}
		for i := range msgs {
			msg := &msgs[i]
			if msg.Header.Type == unix.RTM_NEWLINK {
				ch <- unpackIfInfomsg(msg.Data)
			}
		}
	}
}

func unpackIfInfomsg(data []byte) (iFlags InterfaceEvent) {
	if len(data) < unix.SizeofIfInfomsg {
		iFlags.Err = errors.New("RTM_NEWLINK message too short")
		return
	}
	ifim := unix.IfInfomsg{}
	type ptrIfInfoMsg = *[unix.SizeofIfInfomsg]byte
	// This avoids relying on alignment.
	*(ptrIfInfoMsg)(unsafe.Pointer(&ifim)) = *(ptrIfInfoMsg)(data)
	iFlags.Index = int(ifim.Index)
	iFlags.Flags = netFlags(ifim.Flags)
	return
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
