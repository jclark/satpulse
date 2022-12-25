package unix2

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// IoctlPTPExttsRequest perform a PTP_EXTTS_REQUEST ioctl.
func IoctlPTPExttsRequest(fd int, value *PTPExttsRequest) error {
	return ioctlPtr(fd, PTP_EXTTS_REQUEST, unsafe.Pointer(value))
}

// IoctlPTPPinSetfunc perform a PTP_PIN_SETFUNC ioctl.
func IoctlPTPPinSetfunc(fd int, value *PTPPinDesc) error {
	return ioctlPtr(fd, PTP_PIN_SETFUNC, unsafe.Pointer(value))
}

// Wrapper around ioctl syscall for case when argument is a pointer
// and return value just says whether there's an error
func ioctlPtr(fd int, req uint, arg unsafe.Pointer) (err error) {
	err = nil
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	// see docs for syscall.Errno
	if errno != 0 {
		err = errno
	}
	return
}

func PTPExttsEventFromBytes(buf *[SizeofPTPExttsEvent]byte) *PTPExttsEvent {
	event := PTPExttsEvent{}
	// This use of unsafe.Pointer is following a pattern that is documented as supported
	// (1) Conversion of a *T1 to Pointer to *T2.
	*(*[SizeofPTPExttsEvent]byte)(unsafe.Pointer(&event)) = *buf
	return &event
}
