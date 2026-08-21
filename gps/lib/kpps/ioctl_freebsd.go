package kpps

//go:generate sh -c "go tool cgo -godefs types_freebsd.go | gofmt > ztypes_freebsd.go && rm -rf _obj"

import (
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ioctlCreate(fd uintptr) error {
	return ioctlNone(fd, ppsIocCreate)
}

func ioctlDestroy(fd uintptr) error {
	return ioctlNone(fd, ppsIocDestroy)
}

func ioctlSetParams(fd uintptr, mode Mode) error {
	params := ppsParams{Api_version: ppsAPIVers1, Mode: int32(mode)}
	return ioctlPtr(fd, ppsIocSetParams, unsafe.Pointer(&params))
}

func ioctlGetCap(fd uintptr) (Mode, error) {
	var mode int32
	err := ioctlPtr(fd, ppsIocGetCap, unsafe.Pointer(&mode))
	return Mode(uint32(mode)), err
}

// ioctlFetch returns the most recently captured information. A zero timeout
// returns immediately. A positive timeout blocks until an edge of either
// polarity is captured after the call is entered, failing with ETIMEDOUT if
// the timeout expires first.
func ioctlFetch(fd uintptr, timeout time.Duration) (Info, error) {
	args := ppsFetchArgs{Tsformat: ppsTsfmtTspec}
	if timeout > 0 {
		args.Timeout = timespec{Sec: int64(timeout / time.Second), Nsec: int64(timeout % time.Second)}
	}
	if err := ioctlPtr(fd, ppsIocFetch, unsafe.Pointer(&args)); err != nil {
		return Info{}, err
	}
	return infoFromKernel(args.Info_buf), nil
}

func infoFromKernel(info ppsInfo) Info {
	// The timestamps are pps_timeu_t unions, which cgo -godefs renders as
	// byte arrays; the union's first member is the struct timespec view.
	assert := *(*timespec)(unsafe.Pointer(&info.Assert_tu))
	clear := *(*timespec)(unsafe.Pointer(&info.Clear_tu))
	return Info{
		Assert: Edge{
			T:        time.Unix(assert.Sec, assert.Nsec),
			Sequence: info.Assert_sequence,
		},
		Clear: Edge{
			T:        time.Unix(clear.Sec, clear.Nsec),
			Sequence: info.Clear_sequence,
		},
		Mode: Mode(uint32(info.Current_mode)),
	}
}

func ioctlNone(fd uintptr, req uint) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlPtr(fd uintptr, req uint, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}
