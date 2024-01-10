package phc

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/unix2"
	"golang.org/x/sys/unix"
)

type Clock struct {
	fd         int
	path       string
	caps       *unix2.PTPClockCaps
	eraCounter ptime.AtomicEra
}

type TsEvent struct {
	Ts        ptime.ClockTime
	TRead     time.Time
	TReadPHC  ptime.ClockTime
	ChanIndex uint32
	Err       error
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
	// We start off with an era that is certain.
	// Zero era represent stale PHC clock readings.
	clk.eraCounter.Inc()
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

const StaleEra ptime.Era = ptime.Era(0)

func (clk *Clock) ReadWorker(done <-chan struct{}, tsEvents chan<- TsEvent, timeout time.Duration) {
	era := StaleEra
	var bytes [unix2.SizeofPTPExttsEvent]byte
	buf := bytes[:]
Loop:
	for {
		select {
		case <-done:
			break Loop
		default:
		}
		pollFds := make([]unix.PollFd, 1)
		pollFds[0].Fd = int32(clk.fd)
		pollFds[0].Events = unix.POLLIN | unix.POLLPRI
		nFds, _ := unix.Poll(pollFds, int(timeout.Milliseconds()))
		// The idea is that if we poll and there are no pending events, then any step to the clock
		// that we have made with adjtimex will be in effect for the next read.
		if nFds != 1 || (pollFds[0].Revents&unix.POLLIN) == 0 {
			era = clk.eraCounter.Load()
			continue
		}
		event := TsEvent{}
		n, err := unix.Read(clk.fd, buf)
		if err != nil {
			event.Err = clk.wrapErr(err, "read")
		} else if n != unix2.SizeofPTPExttsEvent {
			event.Err = clk.wrapErr(fmt.Errorf("unexpected number of bytes %d (expected %d)", n, unix2.SizeofPTPExttsEvent), "read")
		} else {
			tClock := ptime.ClockTime{}
			tClock.Era = clk.eraCounter.Load()
			if tClock.Era != era && !tClock.Era.Uncertain() {
				if era.Uncertain() {
					tClock.Era = era
				} else {
					// Make the era uncertain.
					// We cannot be sure that the adjtimex is in effect now.
					// We have to wait for a poll that does not return any events.
					tClock.Era = era + 1
				}
			}
			ptpEv := unix2.PTPExttsEventFromBytes(&bytes)
			tClock.T = ptime.TimespecToTime(unix.Timespec{Sec: ptpEv.T.Sec, Nsec: int64(ptpEv.T.Nsec)})
			event.TReadPHC, event.TRead, err = clk.sample()
			if err != nil {
				event.Err = err
				event.TRead = time.Now()
			}
			event.Ts = tClock
		}
		tsEvents <- event
	}
	close(tsEvents)
}

// sample reads the PHC and system clocks and returns the results.
// This can be called only from ReadWorker.
func (clk *Clock) sample() (tClock ptime.ClockTime, tSys time.Time, err error) {
	eraPre := clk.eraCounter.Load()
	ms, err := clk.SysOffsetExtended(6)
	if err != nil {
		return
	}
	eraPost := clk.eraCounter.Load()
	if eraPre == eraPost || eraPre.Uncertain() {
		tClock.Era = eraPre
	} else if eraPost.Uncertain() {
		tClock.Era = eraPost
	} else {
		tClock.Era = eraPre + 1
	}
	tClock.T, tSys = ms.Reduce()
	return
}

// This is only safe when any ReadWorker has closed its tsEvents channel
func (clk *Clock) Close() error {
	return clk.wrapErr(unix.Close(clk.fd), "close")
}

type PinFunc uint32

const (
	PinFuncNone   PinFunc = unix2.PTP_PF_NONE
	PinFuncExtts  PinFunc = unix2.PTP_PF_EXTTS
	PinFuncPerout PinFunc = unix2.PTP_PF_PEROUT
)

func (clk *Clock) PinSetFunc(pinIndex uint32, pinFunc PinFunc, chanIndex uint32) error {
	pd := unix2.PTPPinDesc{Index: pinIndex, Func: uint32(pinFunc), Chan: chanIndex}
	return clk.wrapErr(unix2.IoctlPTPPinSetFunc(clk.fd, &pd), "ioctl(PTP_PIN_SETFUNC)")
}

func (clk *Clock) PinCount() int {
	return int(clk.caps.N_pins)
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) (edges int, err error) {
	rq := unix2.PTPExttsRequest{Index: chanIndex}
	if enabled {
		// We want to know if possible how many edges of the pulse are getting timestamped.
		// We can do this by using the PTP_EXTTS_REQUEST2 ioctl with the PTP_STRICT_FLAGS set,
		// which will give an EOPNOTSUPP error if it can't give the edges we request.
		// This is only supported since kernel 5.4.
		rq.Flags = unix2.PTP_ENABLE_FEATURE | unix2.PTP_RISING_EDGE | unix2.PTP_STRICT_FLAGS
		err = unix2.IoctlPTPExttsRequest2(clk.fd, &rq)
		if err == nil {
			edges = 1
			return
		}
		if errors.Is(err, unix.EOPNOTSUPP) {
			rq.Flags |= unix2.PTP_FALLING_EDGE
			err = unix2.IoctlPTPExttsRequest2(clk.fd, &rq)
			if err == nil {
				edges = 2
				return
			}
		}
		// If we get ENOTTY here, it means the ioctl isn't recognized
		if !errors.Is(err, unix.ENOTTY) {
			err = clk.wrapErr(err, "ioctl(PTP_EXTTS_REQUEST2)")
			return
		}
		// We get here if the kernel is older than 5.4 and does understand PTP_EXTTS_REQUEST2
		rq.Flags = unix2.PTP_ENABLE_FEATURE
	}
	err = clk.wrapErr(unix2.IoctlPTPExttsRequest(clk.fd, &rq), "ioctl(PTP_EXTTS_REQUEST)")
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

func (clk *Clock) SysOffsetExtended(nSamples int) (MultiSample, error) {
	ms := MultiSample{}
	buf := unix2.PTPSysOffsetExtended{}
	if nSamples <= 0 || nSamples > unix2.PTP_MAX_SAMPLES {
		return ms, unix.EINVAL
	}
	buf.Samples = uint32(nSamples)
	err := clk.wrapErr(unix2.IoctlPTPSysOffsetExtended(clk.fd, &buf), "ioctl(PTP_SYS_OFFSET_EXTENDED)")
	if err != nil {
		return ms, err
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

func ptpClockTimeToTimeSys(t unix2.PTPClockTime) time.Time {
	return time.Unix(t.Sec, int64(t.Nsec))
}

func ptpClockTimeToTimePHC(t unix2.PTPClockTime) ptime.Time {
	return ptime.Unix(t.Sec, int64(t.Nsec))
}

func (clk *Clock) AdjTime(d time.Duration) (ptime.Era, error) {
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
	clk.eraCounter.Inc()
	_, err := clk.adjtimex(&tx, "(ADJ_SETOFFSET)")
	era := clk.eraCounter.Inc()
	if err != nil {
		era = 0
	}
	return era, err
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
	tx.Modes = unix.ADJ_FREQUENCY
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
	tsInfo, err := unix2.IoctlGetEthtoolTsInfo(fd, ifname)
	if err != nil {
		err = fmt.Errorf("ETHTOOL_GET_TS_INFO %s: %w", ifname, err)
		return
	}
	phcIndex = int(tsInfo.Phc_index)
	return
}
