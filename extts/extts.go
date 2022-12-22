// Higher level interface to Linux PTP extts
package extts

import (
	"fmt"
	"os"

	"github.com/jclark/gps2phc/sys/ptp"

	"golang.org/x/sys/unix"
)

type Extts struct {
	path    string
	fd      int
	pin     uint32
	channel uint32
}

func Start(phcIndex int, pin, channel uint32) error {
	e, err := Open(ptp.ClockPath(phcIndex), pin, channel)
	if err != nil {
		return err
	}
	err = SkipStale(e)
	if err != nil {
		return err
	}
	err = e.SetEnabled(true)
	if err != nil {
		return err
	}
	for {
		event, err := e.Read(1500)
		if err != nil {
			return err
		}
		if event == nil {
			fmt.Println("missing PPS event")
			break
		}
		fmt.Printf("%d.%09d\n", event.T.Sec, event.T.Nsec)
	}
	return nil
}

func SkipStale(e *Extts) error {
	err := e.SetEnabled(false)
	if err != nil {
		return err
	}
	skipCount := 0
	for {
		event, err := e.Read(50)
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
	return nil
}

func Open(path string, pin, channel uint32) (*Extts, error) {
	fd, err := unix.Open(path, unix.O_RDONLY, 0)
	if err != nil {
		return nil, &os.PathError{
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
		unix.Close(fd)
		return nil, err
	}
	return &Extts{
		path:    path,
		fd:      fd,
		pin:     pin,
		channel: channel,
	}, nil
}

func (e *Extts) SetEnabled(enabled bool) error {
	var flags uint32
	if enabled {
		flags = ptp.EnableFeature
	}
	exttsReq := ptp.ExttsRequest{
		Index: e.channel,
		Flags: flags,
	}
	return ptp.IoctlExttsRequest(e.fd, &exttsReq, e.path)
}

func (e *Extts) Read(timeout int) (*ptp.ExttsEvent, error) {
	return ptp.ExttsEventRead(e.fd, timeout, e.path)
}
