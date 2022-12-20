package main

import (
	"example/gps2phc/sys/ptp"
	"fmt"

	"golang.org/x/sys/unix"
)

func StartExtts(phcIndex int, pin, channel uint32) error {
	fd, err := unix.Open(ptp.ClockPath(phcIndex), unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	pinDesc := ptp.PinDesc{
		Chan:  channel,
		Index: pin,
		Func:  ptp.PinFuncExtts,
	}
	err = ptp.IoctlPinSetFunc(fd, &pinDesc)
	if err != nil {
		fmt.Println("error from IoctlPinSetFunc")
		return err
	}
	exttsReq := ptp.ExttsRequest{
		Index: channel,
		Flags: ptp.EnableFeature,
	}
	err = ptp.IoctlExttsRequest(fd, &exttsReq)
	if err != nil {
		return err
	}
	for {
		event, err := ptp.ExttsEventRead(fd)
		if err != nil {
			return err
		}
		if event == nil {
			break
		}
		fmt.Printf("%d.%d\n", event.T.Sec, event.T.Nsec)
	}
	return nil
}
