// Linux PTP Clock interface
package ptp

import (
	"fmt"
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

//const exttsEventSize = ((64 + 32 + 32) + 32 + 32 + 32*2)/8

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

func IoctlExttsRequest(fd int, value *ExttsRequest) error {
	return ioctlPtrErr(fd, reqExttsRequest, unsafe.Pointer(value))
}

func IoctlPinSetFunc(fd int, value *PinDesc) error {
	return ioctlPtrErr(fd, reqPinSetFunc, unsafe.Pointer(value))
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

func ExttsEventRead(fd int) (*ExttsEvent, error) {
	event := ExttsEvent{}
	// This use of unsafe.Pointer is following a pattern that is documented as supported
	// (1) Conversion of a *T1 to Pointer to *T2.
	bytePtr := (*[exttsEventSize]byte)(unsafe.Pointer(&event))
	b := bytePtr[:]
	n, err := unix.Read(fd, b)
	if err != nil {
		return nil, err
	}
	if uintptr(n) != exttsEventSize {
		fmt.Printf("unexpected read size")
		return nil, nil
	}
	return &event, nil
}

const pathPrefix = "/dev/ptp"

func ClockPath(phcIndex int) string {
	return pathPrefix + strconv.Itoa(phcIndex)
}
