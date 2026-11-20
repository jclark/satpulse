package phc

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func ioctlPtpExttsRequest(fd int, r *unix.PtpExttsRequest) error {
	return ioctlPtr(fd, unix.PTP_EXTTS_REQUEST, unsafe.Pointer(r))
}

func ioctlPtpMaskClearAll(fd int) error {
	return ioctlPtr(fd, unix.PTP_MASK_CLEAR_ALL, nil)
}

func ioctlPtpMaskEnSingle(fd int, chanIndex uint32) error {
	return ioctlPtr(fd, unix.PTP_MASK_EN_SINGLE, unsafe.Pointer(&chanIndex))
}

func ioctlPtpSysOffset(fd int, nSamples uint32) (*unix.PtpSysOffset, error) {
	value := unix.PtpSysOffset{Samples: nSamples}
	return &value, ioctlPtr(fd, unix.PTP_SYS_OFFSET2, unsafe.Pointer(&value))
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
