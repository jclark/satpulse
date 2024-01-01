package ifwait

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"unsafe"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// IfWaiter watches for changes in an interface's flags.
// Its public methods can be called from any goroutine.
type IfWaiter struct {
	ifname     string // never changes after New
	mu         sync.Mutex
	ch         chan<- net.Flags
	f          func(net.Flags) bool
	err        error
	conn       *netlink.Conn
	flags      net.Flags // only meaningful if ifindex > 0
	ifindex    int
	badIndexes map[int]struct{}
}

// NewIfWaiter returns a new IfWatcher for the interface with the given name.
func NewIfWaiter(ifname string) (*IfWaiter, error) {
	conn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{Groups: unix.RTMGRP_LINK})
	if err != nil {
		return nil, fmt.Errorf("failed to dial netlink: %w", err)
	}
	w := &IfWaiter{ifname: ifname, conn: conn}
	iface, err := net.InterfaceByName(ifname)
	if err == nil {
		w.ifindex = iface.Index
		w.flags = iface.Flags
	}
	// There's a race condition here (I think) in that the interface flag change might happen
	// between the call to net.InterfaceByName and the call to conn.Receive.
	go w.work(conn)
	return w, nil
}

func (w *IfWaiter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ch != nil {
		close(w.ch)
		w.ch = nil
	}
	return w.conn.Close()
}

func (w *IfWaiter) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

// SetCond returns a channel that can be used to wait for the interface flags to satisfy a condition.
// The f argument returns true when the flags satisfy the condition.
// If the interface doesn't exist yet, then it will wait for it to come into existence.
// The return channel has length 1. A single message will be sent to the channel when the condition is satisfied.
// If the condition is already satisfied, the message will be sent immediately.
// If there's an error, the channel will be closed without any message being sent.
// The error can be accessed with the Err method.
// SetCond will clear any existing error.
// A nil value for f means any flags satisfy the condition.
func (w *IfWaiter) SetCond(f func(net.Flags) bool) <-chan net.Flags {
	ch, flags, send := w.setCondLocked(f)
	if send {
		ch <- flags
		close(ch)
	}
	return ch
}

func (w *IfWaiter) setCondLocked(f func(net.Flags) bool) (chan net.Flags, net.Flags, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ch != nil {
		close(w.ch)
	}
	ch := make(chan net.Flags, 1)
	w.err = nil
	w.ch = ch
	w.f = f
	return ch, w.flags, w.ifindex > 0 && w.checkCond() != nil
}

func (w *IfWaiter) work(conn *netlink.Conn) {
	for {
		msgs, err := conn.Receive()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				break
			}
			w.setError(fmt.Errorf("failed to receive messages: %w", err))
			continue
		}
		for i := range msgs {
			msg := &msgs[i]
			if msg.Header.Type == unix.RTM_NEWLINK {
				ifindex, flags, err := unpackIfInfomsg(msg.Data)
				if err != nil {
					w.setError(err)
				} else if w.ifindexOK(ifindex) {
					ch := w.setFlagsCheckCond(flags)
					if ch != nil {
						ch <- flags
						close(ch)
					}
				}
			}
		}
	}
}

// setFlagsCheckCond sets the current flags and checks whether they satisfy the condition.
// If they do satisfy the condition, it returns the channel to send the flags to.
// In this case, the channel will be set to nil and the caller must send the flags to the channel and then close the channel.
// This is called only from work goroutine.
func (w *IfWaiter) setFlagsCheckCond(flags net.Flags) chan<- net.Flags {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flags = flags
	return w.checkCond()
}

// this should only be called while mu is locked
func (w *IfWaiter) checkCond() chan<- net.Flags {
	if w.f != nil && !w.f(w.flags) {
		return nil
	}
	ch := w.ch
	w.ch = nil
	w.f = nil
	return ch
}

// ifIndexOk returns true if the ifindex is the index of the interface being watched.
// This is called only from work goroutine.
func (w *IfWaiter) ifindexOK(ifindex int) bool {
	// we need a lock here because SetCond may indirectly access w.ifindex
	w.mu.Lock()
	defer w.mu.Unlock()
	if ifindex <= 0 {
		return false
	}
	if ifindex == w.ifindex {
		return true
	}
	if len(w.badIndexes) > 0 {
		_, found := w.badIndexes[ifindex]
		if found {
			return false
		}
	}
	ifname, err := IfName(ifindex)
	if err != nil {
		w.setError(err)
		return false
	}
	if ifname != w.ifname {
		if w.badIndexes == nil {
			w.badIndexes = make(map[int]struct{})
		}
		w.badIndexes[ifindex] = struct{}{}
		return false

	}
	w.ifindex = ifindex
	return true
}

func (w *IfWaiter) setError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = err
	if w.ch != nil {
		close(w.ch)
		w.ch = nil
		w.f = nil // for GC
	}
}

func unpackIfInfomsg(data []byte) (int, net.Flags, error) {
	if len(data) < unix.SizeofIfInfomsg {
		return 0, 0, errors.New("RTM_NEWLINK message too short")
	}
	ifim := unix.IfInfomsg{}
	type ptrIfInfoMsg = *[unix.SizeofIfInfomsg]byte
	// This avoids relying on alignment.
	*(ptrIfInfoMsg)(unsafe.Pointer(&ifim)) = *(ptrIfInfoMsg)(data)
	return int(ifim.Index), netFlags(ifim.Flags), nil
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

// IfName returns the name of the interface with the given index.
func IfName(ifindex int) (ifname string, err error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return "", fmt.Errorf("could not create a socket: %w", err)
	}
	defer func() {
		err2 := unix.Close(fd)
		if err == nil {
			err = err2
		}
	}()
	ifreq, _ := unix.NewIfreq("")
	ifreq.SetUint32(uint32(ifindex))
	err = unix.IoctlIfreq(fd, unix.SIOCGIFNAME, ifreq)
	if err != nil {
		err = fmt.Errorf("SIOCGIFNAME %d: %w", ifindex, err)
		return
	}
	ifname = ifreq.Name()
	return
}
