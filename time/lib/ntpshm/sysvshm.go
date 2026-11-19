//go:build linux || darwin

package ntpshm

import "golang.org/x/sys/unix"

const ipcCreat = unix.IPC_CREAT

func sysvShmGet(key, size, flag int) (int, error) {
	return unix.SysvShmGet(key, size, flag)
}

func sysvShmAttach(id int) ([]byte, error) {
	return unix.SysvShmAttach(id, 0, 0)
}

func sysvShmDetach(data []byte) error {
	return unix.SysvShmDetach(data)
}

func sysvShmRemove(id int) error {
	_, err := unix.SysvShmCtl(id, unix.IPC_RMID, nil)
	return err
}
