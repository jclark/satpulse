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
	fd   int
	path string
	caps *unix2.PTPClockCaps
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
	clk := &Clock{
		fd:   fd,
		path: path,
	}
	err = clk.getCaps()
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return clk, nil
}

func (clk *Clock) ReadWorker(done <-chan struct{}, tsEvents chan<- TsEvent) {
	var bytes [unix2.SizeofPTPExttsEvent]byte
	buf := bytes[:]
Loop:
	for {
		select {
		case <-done:
			break Loop
		default:
		}
		event := TsEvent{}
		// XXX think we need to do a poll here, so we don't end up blocking forever
		// if timestamps stop happening
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
		tsEvents <- event
	}
	close(tsEvents)
}

func (clk *Clock) Close() error {
	return clk.wrapErr(unix.Close(clk.fd), "close")
}

func (clk *Clock) PinSetfunc(pinIndex uint32, chanIndex uint32, pinFunc uint32) error {
	pd := unix2.PTPPinDesc{Index: pinIndex, Chan: chanIndex, Func: pinFunc}
	return clk.wrapErr(unix2.IoctlPTPPinSetFunc(clk.fd, &pd), "ioctl(PTP_PIN_SETFUNC)")
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) error {
	er := unix2.PTPExttsRequest{Index: chanIndex}
	if enabled {
		er.Flags = unix2.PTP_ENABLE_FEATURE
	}
	return clk.wrapErr(unix2.IoctlPTPExttsRequest(clk.fd, &er), "ioctl(PTP_EXTTS_REQUEST)")
}

const i210SetOffsetFudge = time.Nanosecond * 4600

func (clk *Clock) AdjTime(d time.Duration) error {
	d += i210SetOffsetFudge
	secs := int64(d) / 1e9
	nsecs := int64(d) % 1e9
	if nsecs < 0 {
		nsecs += 1e9
		secs -= 1
	}
	tx := unix.Timex{}
	tx.Modes = unix2.ADJ_SETOFFSET | unix2.ADJ_NANO
	tx.Time.Sec = secs
	tx.Time.Usec = nsecs
	_, err := clk.adjtimex(&tx, "(ADJ_SETOFFSET)")
	return err
}

func (clk *Clock) FreqAdj() (float64, error) {
	tx, err := clk.timexRead()
	if err != nil {
		return 0, err
	}
	// A tx.Freq of 1 means 2^-16 ppm
	return float64(tx.Freq) / 65.536, nil
}

func (clk *Clock) SetFreqAdj(fa float64) error {
	tx := unix.Timex{}
	tx.Modes = unix2.ADJ_FREQUENCY
	newFreq := int64(fa * 65.536)
	tx.Freq = newFreq
	_, err := clk.adjtimex(&tx, "(ADJ_FREQUENCY)")
	if tx.Freq != newFreq {
		return fmt.Errorf("error freq setting freq to %vppb (got %v)", fa, float64(tx.Freq)/65.536)
	}
	return err
}

// Maximum supported frequency adjustment in parts per billion
func (clk *Clock) MaxFreqAdj() float64 {
	return float64(clk.caps.Max_adj)
}

func (clk *Clock) getCaps() error {
	clk.caps = new(unix2.PTPClockCaps)
	return clk.wrapErr(unix2.IoctlPTPClockGetCaps(clk.fd, clk.caps), "ioctl(PTP_CLOCK_GETCAPS)")
}

func (clk *Clock) timexRead() (*unix.Timex, error) {
	tx := unix.Timex{}
	_, err := clk.adjtimex(&tx, "")
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (clk *Clock) adjtimex(timex *unix.Timex, opSuffix string) (int, error) {
	state, err := unix2.ClockAdjtime(clk.clockId(), timex)
	return state, clk.wrapErr(err, "clock_adjtime"+opSuffix)
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
