//go:build linux || darwin

package ntpshm

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

type shmWriter struct {
	t    *shmTime
	data []byte
}

func newShmWriter(segment uint8) (shmWriter, Attach, error) {
	key := shmKey(segment)
	id, err := unix.SysvShmGet(int(key), expectedSize, unix.IPC_CREAT|shmMode(segment))
	a := Attach{Segment: int(segment), Key: key}
	if err != nil {
		return shmWriter{}, a, fmt.Errorf("shmget: %w", err)
	}
	data, err := unix.SysvShmAttach(id, 0, 0)
	if err != nil {
		return shmWriter{}, a, fmt.Errorf("shmat: %w", err)
	}
	if len(data) < expectedSize {
		unix.SysvShmDetach(data)
		return shmWriter{}, a, fmt.Errorf("shmat: segment size %d is smaller than %d", len(data), expectedSize)
	}
	w := shmWriter{t: (*shmTime)(unsafe.Pointer(&data[0])), data: data}
	w.init()
	return w, a, nil
}

func shmMode(segment uint8) int {
	if segment < 2 {
		return 0o600
	}
	return 0o666
}

func (w shmWriter) close() error {
	if len(w.data) == 0 {
		return nil
	}
	return unix.SysvShmDetach(w.data)
}
