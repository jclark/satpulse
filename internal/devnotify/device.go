package devnotify

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

type DeviceEvent struct {
	Properties map[string]string
	Err        error
}

type Notifier struct {
	Events <-chan DeviceEvent
	conn   *netlink.Conn
}

// This multicast group gets the events from udevd rather than the events from the kernel.
const groupUdev = 0x2

func Open() (*Notifier, error) {
	conn, err := netlink.Dial(unix.NETLINK_KOBJECT_UEVENT, &netlink.Config{Groups: groupUdev})
	if err != nil {
		return nil, fmt.Errorf("failed to dial netlink: %w", err)
	}
	conn.SetOption(netlink.NoENOBUFS, true)
	ch := make(chan DeviceEvent)
	n := &Notifier{Events: ch, conn: conn}
	go func() {
		defer close(ch)
		for {
			var ev DeviceEvent
			data, err := n.receive()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					break
				}
				ev.setError(fmt.Errorf("failed to receive messages: %w", err))
			} else {
				ev.setFromMessage(data)
			}
			ch <- ev
		}
	}()
	return n, nil
}

const bufSize = 64

func (n *Notifier) receive() ([]byte, error) {
	conn, err := n.conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, bufSize)
	var nRead int
	conn.Read(func(fd uintptr) bool {
		nRead, _, _, _, err = unix.Recvmsg(int(fd), buf, nil, unix.MSG_PEEK)
		if err != nil {
			return !errors.Is(err, unix.EAGAIN)
		}
		// might be more data than can fit in buf
		if nRead >= len(buf) {
			// try again with a bigger buffer
			buf = make([]byte, len(buf)*2)
			return false
		}
		nRead, err = unix.Read(int(fd), buf)
		if err != nil {
			return !errors.Is(err, unix.EAGAIN)
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return buf[:nRead], nil
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

func (ev *DeviceEvent) setError(err error) {
	ev.Err = err
}

func (ev *DeviceEvent) setFromMessage(data []byte) {
	props, err := makeUdevPropertiesMap(data)
	if err != nil {
		ev.Err = err
	} else {
		ev.Properties = props
	}
}

const udevMonitorMagic = 0xfeedcafe

func makeUdevPropertiesMap(data []byte) (map[string]string, error) {
	// See struct monitor_netlink_header in systemd
	if len(data) < 24 {
		return nil, errors.New("udev event too short")
	}
	if string(data[0:8]) != "libudev\x00" {
		return nil, fmt.Errorf("udev event does not start with %q", "libudev")
	}
	magic := binary.BigEndian.Uint32(data[8:12])
	if magic != udevMonitorMagic {
		return nil, fmt.Errorf("udev event has wrong magic number %x (expected %x)", magic, udevMonitorMagic)
	}
	propertiesOff := int64(binary.NativeEndian.Uint32(data[16:20]))
	propertiesLen := int64(binary.NativeEndian.Uint32(data[20:24]))
	if propertiesOff+propertiesLen > int64(len(data)) {
		return nil, fmt.Errorf("udev event properties out of bounds")
	}
	return decodeProperties(data[propertiesOff : propertiesOff+propertiesLen])
}

func decodeProperties(data []byte) (map[string]string, error) {
	m := make(map[string]string)
	for len(data) > 0 {
		i := bytes.IndexByte(data, 0)
		if i < 0 {
			return nil, errors.New("udev event property without nul byte")
		}
		kv := string(data[:i])
		data = data[i+1:]
		k, v, found := strings.Cut(kv, "=")
		if !found {
			return nil, errors.New("udev event property without equals sign")
		}
		m[k] = v
	}
	return m, nil
}
