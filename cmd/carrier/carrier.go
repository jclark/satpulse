package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"
	"unsafe"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

var ifName string
var runTime = 30 * time.Second

func main() {
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface to monitor")
	flag.Parse()
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func run() error {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return err
	}
	index := iface.Index
	ch, closer, err := notifyInterface()
	if err != nil {
		return err
	}
	timeout := time.After(runTime)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if ev.Err != nil {
				fmt.Printf("error: %v\n", ev.Err)
			}
			if ev.Index == index {
				fmt.Printf("flags: %s\n", ev.Flags.String())
			}
		case <-timeout:
			fmt.Println("timeout: closing")
			closer.Close()
		}
	}
}

func notifyInterface() (<-chan InterfaceEvent, io.Closer, error) {
	const (
		// Speak to route netlink using netlink
		familyRoute = 0

		// Listen for events triggered by addition or deletion of
		// network interfaces
		rtmGroupLink = 0x1
	)

	conn, err := netlink.Dial(familyRoute, &netlink.Config{Groups: rtmGroupLink})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial netlink: %w", err)
	}
	ch := make(chan InterfaceEvent)
	go func() {
		defer close(ch)
		for {
			msgs, err := conn.Receive()
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
	}()
	return ch, conn, nil
}

type InterfaceEvent struct {
	Index int
	Flags net.Flags
	Err   error
}

type ptrIfInfoMsg = *[unix.SizeofIfInfomsg]byte

func unpackIfInfomsg(data []byte) (iFlags InterfaceEvent) {
	if len(data) < unix.SizeofIfInfomsg {
		iFlags.Err = errors.New("RTM_NEWLINK message too short")
		return
	}
	ifim := unix.IfInfomsg{}
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
