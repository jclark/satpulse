package netlink

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

type InterfaceEvent struct {
	Index int       // Interface index
	Flags net.Flags // Interface flags
	Err   error
}

func NotifyInterface() (*Notifier[InterfaceEvent], error) {
	return notify[InterfaceEvent](unix.NETLINK_ROUTE, unix.RTMGRP_LINK)
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
