package unix2

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func ClockAdjtime(clockid uint32, buf *unix.Timex) (state int, err error) {
	err = nil
	r0, _, e1 := unix.Syscall(unix.SYS_CLOCK_ADJTIME, uintptr(clockid), uintptr(unsafe.Pointer(buf)), 0)
	state = int(r0)
	if e1 != 0 {
		err = e1
	}
	return
}
