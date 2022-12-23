// Low-level Linux PTP Clock interface
// This is a thin wrapper around linux/ptp_clock.h
package ptp

import (
	"fmt"
	"os"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	reqExttsRequest = 0x40103d02
	reqPinSetFunc   = 0x40603d07
	reqPinGetFunc   = 0xc0603d06
)

const (
	EnableFeature = 1 << iota
	RisingEdge
	FallingEdge
)

type ClockTime struct {
	Sec      int64  // seconds
	Nsec     uint32 // nanoseconds
	Reserved uint32
}

type ExttsEvent struct {
	T     ClockTime // Time event occured.
	Index uint32    // Which channel produced the event.
	Flags uint32    // Reserved for future use.
	Rsv   [2]uint32 // Reserved for future use.
}

type PinDesc struct {
	Name  [64]byte
	Index uint32    // Index of the pin
	Func  uint32    // Which of the PinFuncXyz functions to use on this pin.
	Chan  uint32    // The specific channel to use for this function.
	Rsv   [5]uint32 // Reserved for future use.
}

const (
	PinFuncNone = iota
	PinFuncExtts
	PinFuncPerout
	PinFuncPhysync
)

type ExttsRequest struct {
	Index uint32    // Which channel to configure.
	Flags uint32    // Bit field for _xxx flags.
	Rsv   [2]uint32 // Reserved for future use.
}

// IoCtlExttsRequest perform a PTP_EXTTS_REQUEST ioctl.
// The path argument is used for constructing the error return value,
// which will be os.PathError.
func IoctlExttsRequest(fd int, value *ExttsRequest, path string) error {
	return wrapErr(ioctlPtrErr(fd, reqExttsRequest, unsafe.Pointer(value)), "ioctl(PTP_EXTTS_REQUEST", path)
}

// IoCtlExttsRequest perform a PTP_PIN_SETFUNC ioctl.
// The path argument is used for constructing the error return value,
// which will be os.PathError.
func IoctlPinSetFunc(fd int, value *PinDesc, path string) error {
	return wrapErr(ioctlPtrErr(fd, reqPinSetFunc, unsafe.Pointer(value)), "ioctl(PTP_PIN_SETFUNC", path)
}

// Wrapper around ioctl syscall for case when argument is a pointer
// and return value just says whether there's an error
func ioctlPtrErr(fd int, req uint, arg unsafe.Pointer) (err error) {
	err = nil
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	// see docs for syscall.Errno
	if errno != 0 {
		err = errno
	}
	return
}

const exttsEventSize = unsafe.Sizeof(ExttsEvent{})

// ExttsEventRead reads an extts event, waiting for timeout microseconds.
// Returns (nil, nil) if no event is available within that time.
// The path argument is used for constructing the error return value,
// which will be os.PathError.
func ExttsEventRead(fd int, timeout int, path string) (*ExttsEvent, error) {
	ready, err := poll(fd, timeout, path)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, nil
	}
	var buf [exttsEventSize]byte
	b := buf[:]
	n, err := unix.Read(fd, b)
	if err != nil {
		return nil, wrapErr(err, "read", path)
	}
	if uintptr(n) != exttsEventSize {
		return nil, wrapErr(fmt.Errorf("unexpected number of bytes %d (expected %d)", n, exttsEventSize), "read", path)
	}
	event := ExttsEvent{}
	// This use of unsafe.Pointer is following a pattern that is documented as supported
	// (1) Conversion of a *T1 to Pointer to *T2.
	*(*[exttsEventSize]byte)(unsafe.Pointer(&event)) = buf
	return &event, nil
}

func poll(fd int, timeout int, path string) (bool, error) {
	pollFd := [1]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLPRI, Revents: 0}}
	nReady, err := pollRestart(pollFd[0:1], timeout)
	if err != nil {
		return false, wrapErr(err, "poll", path)
	}
	if nReady == 0 {
		return false, nil
	}
	return true, nil
}

func pollRestart(fds []unix.PollFd, timeout int) (int, error) {
	for {
		n, err := unix.Poll(fds, timeout)
		switch err {
		case unix.EINTR:
			// This seems to happen because of an SIGURG signal
			// (used internally for non-cooperative preemption).
			// We need to restart in this case.
		default:
			return n, err
		}

	}
}

func wrapErr(err error, op string, path string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{
		Path: path,
		Op:   op,
		Err:  err,
	}
}

const pathPrefix = "/dev/ptp"

func ClockPath(phcIndex int) string {
	return pathPrefix + strconv.Itoa(phcIndex)
}
