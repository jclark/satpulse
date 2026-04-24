package term

//go:generate sh -c "go tool cgo -godefs types_linux.go | gofmt > ztypes_linux.go && rm -rf _obj"

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func ioctlGetSerialICounter(fd int) (*serialICounter, error) {
	var value serialICounter
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
