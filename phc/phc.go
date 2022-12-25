package phc

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jclark/gps2phc/unix2"
	"golang.org/x/sys/unix"
)

type Clock struct {
	fd       int
	path     string
	tsEvents chan TsEvent
	control  chan bool
}

type TsEvent struct {
	T         unix.Timespec
	ChanIndex uint32
	TRead     time.Time
	Err       error
}

func New(path string) (*Clock, error) {
	// clock_adjtime needs RDWR
	fd, err := unix.Open(path, unix.O_RDWR, 0)
	if err != nil {
		return nil, &os.PathError{
			Path: path,
			Op:   "open",
			Err:  err,
		}
	}
	return &Clock{
		fd:   fd,
		path: path,
	}, nil
}

func (clk *Clock) ReadTsEvents(enabled bool) {
	if clk.control == nil {
		clk.control = make(chan bool, 1)
		clk.tsEvents = make(chan TsEvent, 1)
		go clk.readTsEvents()
	}
	clk.control <- enabled
}

// this is responsible for reading extts events from the fd
// it runs until clk.control is closed
// a boolean sent to clk.control tells it to do reading
// It starts off not reading
func (clk *Clock) readTsEvents() {
	var reading bool
	var bytes [unix2.SizeofPTPExttsEvent]byte
	buf := bytes[:]
	for {
		if reading {
			select {
			case r, ok := <-clk.control:
				if !ok {
					break
				}
				reading = r
			default:
				// no control message
				// so read as normal
			}
		} else {
			// if we are not currently reading
			// then wait for a control message
			r, ok := <-clk.control
			if !ok {
				break
			}
			reading = r
		}
		if reading {
			event := TsEvent{}
			n, err := unix.Read(clk.fd, buf)
			if err != nil {
				event.Err = clk.wrapErr(err, "read")
			} else if n != unix2.SizeofPTPExttsEvent {
				event.Err = clk.wrapErr(fmt.Errorf("unexpected number of bytes %d (expected %d)", n, unix2.SizeofPTPExttsEvent), "read")
			} else {
				event.TRead = time.Now()
				ptpEv := unix2.PTPExttsEventFromBytes(&bytes)
				event.T = unix.Timespec{Sec: ptpEv.T.Sec, Nsec: int64(ptpEv.T.Nsec)}
			}
			clk.tsEvents <- event
		}
	}
	close(clk.tsEvents)
}

func (clk *Clock) TsChan() <-chan TsEvent {
	return clk.tsEvents
}

func (clk *Clock) Close() error {
	// XX close the control channel
	// cannot wait for tsEvents to be closed becuase the Read might be blocked
	// if we close fd here we have a race
	// leave it to the goroutine to close
	return clk.wrapErr(unix.Close(clk.fd), "close")
}

func (clk *Clock) PinSetfunc(pinIndex uint32, chanIndex uint32, pinFunc uint32) error {
	pd := unix2.PTPPinDesc{Index: pinIndex, Chan: chanIndex, Func: pinFunc}
	return clk.wrapErr(unix2.IoctlPTPPinSetfunc(clk.fd, &pd), "ioctl(PTP_PIN_SETFUNC)")
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) error {
	er := unix2.PTPExttsRequest{Index: chanIndex}
	if enabled {
		er.Flags = unix2.PTP_ENABLE_FEATURE
	}
	return clk.wrapErr(unix2.IoctlPTPExttsRequest(clk.fd, &er), "ioctl(PTP_EXTTS_REQUEST)")
}

func (clk *Clock) adjtime(timex *unix.Timex) (int, error) {
	state, err := unix2.ClockAdjtime(clk.clockId(), timex)
	return state, clk.wrapErr(err, "adjtime")
}

const clockfd = 3

func (clk *Clock) clockId() uint32 {
	return (uint32(^clk.fd) << 3) | clockfd
}

func (clk *Clock) wrapErr(err error, op string) error {
	if err == nil {
		return nil
	}
	return &os.PathError{
		Path: clk.path,
		Op:   op,
		Err:  err,
	}
}

const pathPrefix = "/dev/ptp"

func ClockPath(phcIndex int) string {
	return pathPrefix + strconv.Itoa(phcIndex)
}
