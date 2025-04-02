package term

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func IoctlGetSerialICounter(fd int) (*SerialICounter, error) {
	var value SerialICounter
	err := ioctlPtr(fd, unix.TIOCGICOUNT, unsafe.Pointer(&value))
	return &value, err
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
