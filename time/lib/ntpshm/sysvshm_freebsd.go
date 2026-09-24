package ntpshm

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// x/sys/unix defines the SysV shared memory syscall numbers for freebsd but
// not the SysvShm* wrappers or the sys/ipc.h constants, so this file issues
// the syscalls directly.
const (
	ipcCreat = 0o1000 // IPC_CREAT from <sys/ipc.h>
	ipcRmid  = 0      // IPC_RMID from <sys/ipc.h>
)

func sysvShmGet(key, size, flag int) (int, error) {
	id, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(key), uintptr(size), uintptr(flag))
	if errno != 0 {
		return 0, errno
	}
	return int(id), nil
}

// sysvShmAttach maps the segment and returns a slice of the expectedSize
// bytes this package uses: shmget has already checked that the segment is at
// least that large.
func sysvShmAttach(id int) ([]byte, error) {
	addr, _, errno := unix.Syscall(unix.SYS_SHMAT, uintptr(id), 0, 0)
	if errno != 0 {
		return nil, errno
	}
	// vet's unsafeptr check flags converting the shmat result, but the
	// uintptr is a live mapping address, not a stale pointer; this is the
	// same pattern unix.SysvShmAttach uses on the platforms that have it.
	return unsafe.Slice((*byte)(unsafe.Pointer(addr)), expectedSize), nil
}

func sysvShmDetach(data []byte) error {
	if _, _, errno := unix.Syscall(unix.SYS_SHMDT, uintptr(unsafe.Pointer(&data[0])), 0, 0); errno != 0 {
		return errno
	}
	return nil
}

func sysvShmRemove(id int) error {
	if _, _, errno := unix.Syscall(unix.SYS_SHMCTL, uintptr(id), ipcRmid, 0); errno != 0 {
		return errno
	}
	return nil
}
