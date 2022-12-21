package main

import (
	"example/gps2phc/sys/ptp"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func StartExtts(phcIndex int, pin, channel uint32) error {
	path := ptp.ClockPath(phcIndex)
	fd, err := unix.Open(path, unix.O_RDONLY, 0)
	if err != nil {
		return &os.PathError{
			Path: path,
			Op:   "open",
			Err:  err,
		}
	}
	pinDesc := ptp.PinDesc{
		Chan:  channel,
		Index: pin,
		Func:  ptp.PinFuncExtts,
	}
	err = ptp.IoctlPinSetFunc(fd, &pinDesc, path)
	if err != nil {
		return err
	}
	exttsReq := ptp.ExttsRequest{
		Index: channel,
		Flags: 0,
	}
	err = ptp.IoctlExttsRequest(fd, &exttsReq, path)
	if err != nil {
		return err
	}
	skipCount := 0
	for {
		event, err := ptp.ExttsEventRead(fd, 50, path)
		if err != nil {
			return err
		}
		if event == nil {
			// timeout
			break
		}
		skipCount++
	}
	fmt.Printf("Skipped %d stale timestamps\n", skipCount)

	exttsReq = ptp.ExttsRequest{
		Index: channel,
		Flags: ptp.EnableFeature,
	}
	err = ptp.IoctlExttsRequest(fd, &exttsReq, path)
	if err != nil {
		return err
	}
	for {
		event, err := ptp.ExttsEventRead(fd, 1500, path)
		if err != nil {
			return err
		}
		if event == nil {
			fmt.Println("missing PPS event")
			break
		}
		fmt.Printf("%d.%d\n", event.T.Sec, event.T.Nsec)
	}
	return nil
}
