//go:build linux

package kpps

import (
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ioctlGetCap(fd uintptr) (Mode, error) {
	var mode int32
	err := ioctlPtr(fd, unix.PPS_GETCAP, unsafe.Pointer(&mode))
	runtime.KeepAlive(&mode)
	return Mode(uint32(mode)), err
}

func ioctlFetch(fd uintptr) (Info, error) {
	var data unix.PPSFData // A zero timeout makes PPS_FETCH return immediately.
	err := ioctlPtr(fd, unix.PPS_FETCH, unsafe.Pointer(&data))
	runtime.KeepAlive(&data)
	if err != nil {
		return Info{}, err
	}
	return infoFromKernel(data.Info), nil
}

func infoFromKernel(info unix.PPSKInfo) Info {
	return Info{
		Assert: Edge{
			T:        time.Unix(info.Assert_tu.Sec, int64(info.Assert_tu.Nsec)),
			Sequence: info.Assert_sequence,
		},
		Clear: Edge{
			T:        time.Unix(info.Clear_tu.Sec, int64(info.Clear_tu.Nsec)),
			Sequence: info.Clear_sequence,
		},
		Mode: Mode(uint32(info.Current_mode)),
	}
}

func ioctlPtr(fd uintptr, req uint, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}
