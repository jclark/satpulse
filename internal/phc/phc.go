package phc

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
	"unsafe"

	"github.com/jclark/satpulse/internal/ptime"
	"golang.org/x/sys/unix"
)

type Clock struct {
	fd            int
	path          string
	caps          *unix.PtpClockCaps
	sysOffsetFunc func(*Clock, int) (MultiSample, error)
}

func Open(path string) (*Clock, error) {
	// clock_adjtime needs RDWR
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK, 0)
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

func (clk *Clock) Path() string {
	return clk.path
}

func (clk *Clock) ExttsAvailable(timeout time.Duration) bool {
	pollFds := make([]unix.PollFd, 1)
	pollFds[0].Fd = int32(clk.fd)
	pollFds[0].Events = unix.POLLIN | unix.POLLPRI
	nFds, _ := unix.Poll(pollFds, int(timeout.Milliseconds()))
	return nFds == 1 && (pollFds[0].Revents&unix.POLLIN) != 0
}

func (clk *Clock) ReadExtts() (ptime.Time, uint32, error) {
	event := unix.PtpExttsEvent{}
	size := int(unsafe.Sizeof(event))
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&event)), size)
	n, err := unix.Read(clk.fd, buf)
	if err != nil {
		return 0, 0, clk.wrapErr(err, "read")
	}
	if n != size {
		return 0, 0, clk.wrapErr(fmt.Errorf("unexpected number of bytes %d (expected %d)", n, size), "read")
	}
	return ptime.TimespecToTime(unix.Timespec{Sec: event.T.Sec, Nsec: int64(event.T.Nsec)}), event.Index, nil
}

// This is only safe when any ReadWorker has closed its tsEvents channel
func (clk *Clock) Close() error {
	return clk.wrapErr(unix.Close(clk.fd), "close")
}

type PinFunc uint32

const (
	PinFuncNone   PinFunc = unix.PTP_PF_NONE
	PinFuncExtts  PinFunc = unix.PTP_PF_EXTTS
	PinFuncPerout PinFunc = unix.PTP_PF_PEROUT
)

func (clk *Clock) PinSetFunc(pinIndex uint32, pinFunc PinFunc, chanIndex uint32) error {
	pd := unix.PtpPinDesc{Index: pinIndex, Func: uint32(pinFunc), Chan: chanIndex}
	return clk.wrapErr(unix.IoctlPtpPinSetfunc(clk.fd, &pd), "ioctl(PTP_PIN_SETFUNC)")
}

func (clk *Clock) PinCount() int {
	return int(clk.caps.N_pins)
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) (edges int, err error) {
	rq := unix.PtpExttsRequest{Index: chanIndex}
	// We want to know how many edges of the pulse are getting timestamped.
	// We can do this by using the PTP_EXTTS_REQUEST2 ioctl with the PTP_STRICT_FLAGS set,
	// which will give an EOPNOTSUPP error if it can't give the edges we request.
	// Go's IoctlPtpExttsRequest in fact wraps PTP_EXTTS_REQUEST2, so we can use that.
	// This is only supported since kernel 5.4.
	const enableFlags = unix.PTP_ENABLE_FEATURE | unix.PTP_STRICT_FLAGS | unix.PTP_RISING_EDGE
	if enabled {
		rq.Flags = enableFlags
		edges = 1
	}
	err = unix.IoctlPtpExttsRequest(clk.fd, &rq)
	if err != nil && enabled && errors.Is(err, unix.EOPNOTSUPP) {
		rq.Flags = enableFlags | unix.PTP_FALLING_EDGE
		edges = 2
		err = unix.IoctlPtpExttsRequest(clk.fd, &rq)
	}
	if err != nil {
		err = clk.wrapErr(err, "ioctl(PTP_EXTTS_REQUEST2)")
		edges = 0
		return
	}
	return
}

func (clk *Clock) ExttsChanCount() int {
	return int(clk.caps.N_ext_ts)
}

// A MultiSample is a series of samples of the PHC and system clocks.
// In the case of a MultiSample returned by SysOffset, the number
// of PHC samples is the one more than the number of system clock samples.
// In the case of a MultiSample returned by SysOffsetExtended,
// the number of system clock samples is twice the number of PHC samples;
// the system clock samples are the system clock times immediately before and after the PHC clock time.
// In the case of a MultiSample returns by SysOffsetPrecise, the number of PHC samples
// and of system clocks samples is both one.
type MultiSample struct {
	PHC []ptime.Time
	Sys []time.Time
}

func (clk *Clock) SysOffset(nSamples int) (MultiSample, error) {
	if clk.sysOffsetFunc != nil {
		return clk.sysOffsetFunc(clk, nSamples)
	}
	ms, err := clk.SysOffsetPrecise(nSamples)
	if err == nil {
		clk.sysOffsetFunc = (*Clock).SysOffsetPrecise
		return ms, nil
	}
	// According to the ioctl man page, I think it should be returning ENOTTY in this case, but it actually returns EOPNOTSUPP.
	if errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.EOPNOTSUPP) {
		clk.sysOffsetFunc = (*Clock).SysOffsetExtended
	}
	return clk.SysOffsetExtended(nSamples)
}

func (clk *Clock) SysOffsetExtended(nSamples int) (MultiSample, error) {
	ms := MultiSample{}
	if nSamples <= 0 || nSamples > unix.PTP_MAX_SAMPLES {
		return ms, unix.EINVAL
	}
	buf, err := unix.IoctlPtpSysOffsetExtended(clk.fd, uint(nSamples))
	if err != nil {
		return ms, clk.wrapErr(err, "ioctl(PTP_SYS_OFFSET_EXTENDED)")
	}
	ms.PHC = make([]ptime.Time, nSamples)
	ms.Sys = make([]time.Time, nSamples*2)
	for i := 0; i < nSamples; i++ {
		ts := &buf.Ts[i]
		ms.Sys[i*2] = ptpClockTimeToTimeSys(ts[0])
		ms.PHC[i] = ptpClockTimeToTimePHC(ts[1])
		ms.Sys[i*2+1] = ptpClockTimeToTimeSys(ts[2])
	}
	return ms, nil
}

func (clk *Clock) SysOffsetPrecise(_ int) (MultiSample, error) {
	ms := MultiSample{}
	buf, err := unix.IoctlPtpSysOffsetPrecise(clk.fd) 
	if err != nil {
		return ms, clk.wrapErr(err, "ioctl(PTP_SYS_OFFSET_PRECISE)")
	}
	ms.PHC = []ptime.Time{ptpClockTimeToTimePHC(buf.Device)}
	ms.Sys = []time.Time{ptpClockTimeToTimeSys(buf.Realtime)}
	return ms, nil
}

func (ms MultiSample) Reduce() (ptime.Time, time.Time) {
	return ms.Extract(ms.Select())
}

func (ms MultiSample) Extract(i int) (ptime.Time, time.Time) {
	return ms.PHC[i], ms.SysAverage(i)
}

func (ms MultiSample) Select() int {
	best := 0
	minInterval := ms.SysInterval(0)
	for i := 1; i < len(ms.PHC); i++ {
		if interval := ms.SysInterval(i); interval < minInterval {
			minInterval = interval
			best = i
		}
	}
	return best
}

func (ms MultiSample) SysInterval(i int) time.Duration {
	pre, post := ms.SysPrePost(i)
	return post.Sub(pre)
}

// SysPrePost returns the system times immediately before and after the i-th PHC sample.
func (ms MultiSample) SysPrePost(i int) (pre, post time.Time) {
	phcLen := len(ms.PHC)
	switch len(ms.Sys) {
	case phcLen * 2:
		return ms.Sys[i*2], ms.Sys[i*2+1]
	case phcLen + 1:
		return ms.Sys[i], ms.Sys[i+1]
	case 1:
		return ms.Sys[0], ms.Sys[0]
	default:
		panic("invalid MultiSample sys length")
	}
}

// SysAverage returns the average of the system times for the i-th PHC sample.
func (ms MultiSample) SysAverage(i int) time.Time {
	pre, post := ms.SysPrePost(i)
	return pre.Add(post.Sub(pre) / 2)
}

func ptpClockTimeToTimeSys(t unix.PtpClockTime) time.Time {
	return time.Unix(t.Sec, int64(t.Nsec))
}

func ptpClockTimeToTimePHC(t unix.PtpClockTime) ptime.Time {
	return ptime.Unix(t.Sec, int64(t.Nsec))
}

func (clk *Clock) AdjTime(d time.Duration) error {
	secs := int64(d) / 1e9
	nsecs := int64(d) % 1e9
	if nsecs < 0 {
		nsecs += 1e9
		secs -= 1
	}
	tx := unix.Timex{}
	tx.Modes = unix.ADJ_SETOFFSET | unix.ADJ_NANO
	tx.Time.Sec = secs
	tx.Time.Usec = nsecs
	_, err := clk.adjtimex(&tx, "(ADJ_SETOFFSET)")
	return err
}

func (clk *Clock) FreqOffset() (float64, error) {
	tx, err := clk.timexRead()
	if err != nil {
		return 0, err
	}
	// A tx.Freq of 1 means 2^-16 ppm
	return float64(tx.Freq) / 65.536, nil
}

func (clk *Clock) SetFreqOffset(fo float64) error {
	tx := unix.Timex{}
	tx.Modes = unix.ADJ_FREQUENCY
	newFreq := int64(fo * 65.536)
	tx.Freq = newFreq
	_, err := clk.adjtimex(&tx, "(ADJ_FREQUENCY)")
	if tx.Freq != newFreq {
		return fmt.Errorf("error setting freq offset to %vppb (got %v)", fo, float64(tx.Freq)/65.536)
	}
	return err
}

// Maximum supported frequency offset adjustment in parts per billion
func (clk *Clock) MaxFreqOffset() float64 {
	return float64(clk.caps.Max_adj)
}

func (clk *Clock) getCaps() error {
	var err error
	clk.caps, err = unix.IoctlPtpClockGetcaps(clk.fd)
	return clk.wrapErr(err, "ioctl(PTP_CLOCK_GETCAPS)")
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
	state, err := unix.ClockAdjtime(clk.clockId(), timex)
	return state, clk.wrapErr(err, "clock_adjtime"+opSuffix)
}

const clockfd = 3

func (clk *Clock) clockId() int32 {
	return (int32(^clk.fd) << 3) | clockfd
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

func IfPhcIndex(ifname string) (phcIndex int, err error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return 0, fmt.Errorf("could not create a socket: %w", err)
	}
	defer func() {
		err2 := unix.Close(fd)
		if err == nil {
			err = err2
		}
	}()
	tsInfo, err := unix.IoctlGetEthtoolTsInfo(fd, ifname)
	if err != nil {
		err = fmt.Errorf("ETHTOOL_GET_TS_INFO %s: %w", ifname, err)
		return
	}
	phcIndex = int(tsInfo.Phc_index)
	return
}
