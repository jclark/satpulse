package main

import (
	"example/gps2phc/sys/ptp"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func StartExtts(phcIndex int, pin, channel uint32) (err error) {
	path := ptp.ClockPath(phcIndex)
	fd, err := unix.Open(path, unix.O_RDONLY, 0)
	if err != nil {
		return wrapErr(err, "open", path)
	}
	pinDesc := ptp.PinDesc{
		Chan:  channel,
		Index: pin,
		Func:  ptp.PinFuncExtts,
	}
	err = ptp.IoctlPinSetFunc(fd, &pinDesc)
	if err != nil {
		return wrapErr(err, "ioctl(PTP_PIN_SET_FUNC)", path)
	}
	exttsReq := ptp.ExttsRequest{
		Index: channel,
		Flags: ptp.EnableFeature,
	}
	err = ptp.IoctlExttsRequest(fd, &exttsReq)
	if err != nil {
		return wrapErr(err, "ioctl(PTP_EXTTS_REQUEST)", path)
	}
	for {
		event, err := ptp.ExttsEventRead(fd)
		if err != nil {
			return wrapErr(err, "read", path)
		}
		if event == nil {
			break
		}
		fmt.Printf("%d.%d\n", event.T.Sec, event.T.Nsec)
	}
	return nil
}

func wrapErr(err error, op string, path string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{
		Path: path,
		Op:   op,
		Err:  err,
	}
}
