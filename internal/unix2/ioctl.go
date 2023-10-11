package unix2

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// IoctlPTPExttsRequest perform a PTP_EXTTS_REQUEST ioctl.
func IoctlPTPExttsRequest(fd int, value *PTPExttsRequest) error {
	return ioctlPtr(fd, PTP_EXTTS_REQUEST, unsafe.Pointer(value))
}

// IoctlPTPExttsRequest2 perform a PTP_EXTTS_REQUEST2 ioctl.
func IoctlPTPExttsRequest2(fd int, value *PTPExttsRequest) error {
	return ioctlPtr(fd, PTP_EXTTS_REQUEST2, unsafe.Pointer(value))
}

// IoctlPTPPinSetFunc perform a PTP_PIN_SETFUNC ioctl.
// XXX should it be Setfunc instead of SetFunc since it is PTP_PIN_SETFUNC?
func IoctlPTPPinSetFunc(fd int, value *PTPPinDesc) error {
	return ioctlPtr(fd, PTP_PIN_SETFUNC, unsafe.Pointer(value))
}

func IoctlPTPClockGetCaps(fd int, value *PTPClockCaps) error {
	return ioctlPtr(fd, PTP_CLOCK_GETCAPS, unsafe.Pointer(value))
}

// Copied from unix pkg
type ifreqData struct {
	name [unix.IFNAMSIZ]byte
	data unsafe.Pointer
	_    [sizeofIFreq - unix.IFNAMSIZ - unix.SizeofPtr]byte
}

func IoctlGetEthtoolTsInfo(fd int, ifname string) (*EthtoolTsInfo, error) {
	if len(ifname) >= unix.IFNAMSIZ {
		return nil, unix.EINVAL
	}
	value := EthtoolTsInfo{Cmd: unix.ETHTOOL_GET_TS_INFO}
	ifrd := ifreqData{}
	copy(ifrd.name[:], ifname)
	ifrd.data = unsafe.Pointer(&value)
	err := ioctlPtr(fd, unix.SIOCETHTOOL, unsafe.Pointer(&ifrd))
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

// This works around bug in gopls
const eventSize = SizeofPTPExttsEvent

func PTPExttsEventFromBytes(buf *[eventSize]byte) *PTPExttsEvent {
	event := PTPExttsEvent{}
	// This use of unsafe.Pointer is following a pattern that is documented as supported
	// (1) Conversion of a *T1 to Pointer to *T2.
	*(*[SizeofPTPExttsEvent]byte)(unsafe.Pointer(&event)) = *buf
	return &event
}
