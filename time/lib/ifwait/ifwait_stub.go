//go:build !linux

package ifwait

import (
	"errors"
	"net"
)

var errIfWaitNotSupported = errors.New("interface waiting not supported on this platform")

// IfWaiter watches for changes in an interface's flags.
type IfWaiter struct{}

// NewIfWaiter returns a new IfWatcher for the interface with the given name.
func NewIfWaiter(ifname string) (*IfWaiter, error) {
	return nil, errIfWaitNotSupported
}

func (w *IfWaiter) Name() string {
	panic(errIfWaitNotSupported)
}

func (w *IfWaiter) Close() error {
	panic(errIfWaitNotSupported)
}

func (w *IfWaiter) Err() error {
	panic(errIfWaitNotSupported)
}

func (w *IfWaiter) SetCond(f func(net.Flags) bool) <-chan net.Flags {
	panic(errIfWaitNotSupported)
}