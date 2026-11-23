package ntpshm

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type shmWriter struct {
	t    *shmTime
	h    windows.Handle // file-mapping object handle
	addr uintptr        // MapViewOfFile base, for UnmapViewOfFile
}

// newShmWriter attaches to the paging-file-backed named file mapping that
// ntpd built with SYS_WINNT reads: Local\NTPn for segments below 2 and
// world-accessible Global\NTPn for the rest, ntpd's analog of the Unix
// 0600/0666 mode split. Two differences from a SysV segment are worth
// knowing. The mapping is refcounted by its open handles and destroyed
// with the last close, so the last sample does not persist across a writer
// restart; the open-or-create call self-heals. And Local\ names are per
// logon session, so a writer and an ntpd running in different sessions
// (e.g. one of them a service) only meet on segments >= 2, whose Global\
// names require the create-global privilege.
func newShmWriter(segment uint8) (shmWriter, Attach, error) {
	a := Attach{Segment: int(segment), Key: shmKey(segment)}
	name := fmt.Sprintf(`Local\NTP%d`, segment)
	var sa *windows.SecurityAttributes
	if segment >= 2 {
		name = fmt.Sprintf(`Global\NTP%d`, segment)
		sd, err := windows.SecurityDescriptorFromString("D:(A;;GA;;;WD)")
		if err != nil {
			return shmWriter{}, a, fmt.Errorf("building world-access security descriptor: %w", err)
		}
		sa = &windows.SecurityAttributes{
			Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
			SecurityDescriptor: sd,
		}
	}
	namep, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return shmWriter{}, a, err
	}
	// The wrapper reports ERROR_ALREADY_EXISTS with a valid handle when the
	// mapping already exists; opening the existing mapping is exactly the
	// open-or-create semantics wanted here.
	h, err := windows.CreateFileMapping(windows.InvalidHandle, sa, windows.PAGE_READWRITE, 0, expectedSize, namep)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return shmWriter{}, a, fmt.Errorf("CreateFileMapping %s: %w", name, err)
	}
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_WRITE, 0, 0, expectedSize)
	if err != nil {
		windows.CloseHandle(h)
		return shmWriter{}, a, fmt.Errorf("MapViewOfFile %s: %w", name, err)
	}
	w := shmWriter{t: (*shmTime)(unsafe.Pointer(addr)), h: h, addr: addr}
	w.init()
	return w, a, nil
}

func (w shmWriter) close() error {
	if w.addr == 0 {
		return nil
	}
	err := windows.UnmapViewOfFile(w.addr)
	if cerr := windows.CloseHandle(w.h); err == nil {
		err = cerr
	}
	return err
}
