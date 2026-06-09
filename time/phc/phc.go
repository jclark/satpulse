//go:build linux

package phc

import (
	"bytes"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"strconv"
	"time"
	"unsafe"

	"github.com/jclark/satpulse/gps/ptime"
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
	return ptime.TimespecToTime(unix.Timespec{Sec: kernelLong(event.T.Sec), Nsec: kernelLong(event.T.Nsec)}), event.Index, nil
}

// This is only safe when any ReadWorker has closed its tsEvents channel
func (clk *Clock) Close() error {
	return clk.wrapErr(unix.Close(clk.fd), "close")
}

const (
	PinFuncNone    PinFunc = unix.PTP_PF_NONE
	PinFuncExtts   PinFunc = unix.PTP_PF_EXTTS
	PinFuncPerout  PinFunc = unix.PTP_PF_PEROUT
	PinFuncPhysync PinFunc = unix.PTP_PF_PHYSYNC
)

func (clk *Clock) PinSetFunc(pinIndex uint32, pinFunc PinFunc, chanIndex uint32) error {
	pd := unix.PtpPinDesc{Index: pinIndex, Func: uint32(pinFunc), Chan: chanIndex}
	return clk.wrapErr(unix.IoctlPtpPinSetfunc(clk.fd, &pd), "ioctl(PTP_PIN_SETFUNC)")
}

func (clk *Clock) PinGetFunc(pinIndex uint32) (*PinDesc, error) {
	pd, err := unix.IoctlPtpPinGetfunc(clk.fd, uint(pinIndex))
	if err != nil {
		return nil, clk.wrapErr(err, "ioctl(PTP_PIN_GETFUNC)")
	}

	// Convert C-style zero-terminated string to Go string
	name := pd.Name[:]
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}

	return &PinDesc{
		Name:  string(name),
		Index: pd.Index,
		Func:  PinFunc(pd.Func),
		Chan:  pd.Chan,
	}, nil
}

func (clk *Clock) PinCount() int {
	return int(clk.caps.N_pins)
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) (edges int, err error) {
	rq := unix.PtpExttsRequest{Index: chanIndex}
	if !enabled {
		err = clk.exttsRequest(clk.fd, &rq)
		return
	}
	// The list of flags to try to use when enabling external timestamping, in order of preference.
	var tryFlagsList = []uint32{
		// First preference is to timestamp just the rising edge
		unix.PTP_RISING_EDGE,
		// Next preference is to timestamp both edges
		unix.PTP_RISING_EDGE | unix.PTP_FALLING_EDGE,
		// Final preference is to not know what edges will be timestamped.
		// It will make exttsRequest use the older PTP_EXTTS_REQUEST ioctl.
		// It is only needed to deal with a kernel issue.
		0,
	}
	for _, flags := range tryFlagsList {
		rq.Flags = unix.PTP_ENABLE_FEATURE | flags
		err = clk.exttsRequest(clk.fd, &rq)
		if err == nil {
			edges = bits.OnesCount32(flags)
			break
		}
		if !errors.Is(err, unix.EOPNOTSUPP) {
			break
		}
	}
	return
}

// exttsRequest does either a PTP_EXTTS_REQUEST2 or a PTP_EXTTS_REQUEST ioctl, wrapping any error
// We are not trying to deal with older kernels that do not support PTP_EXTTS_REQUEST2 properly,
// (since that would require us not to use any of the unix.IoctlPtp functions).
// We are just trying to fix an issue with CM4/CM5 on some kernel versions (e.g. 6.16.7).
func (clk *Clock) exttsRequest(fd int, req *unix.PtpExttsRequest) error {
	// PTP_EXTTS_REQUEST2 is always strict: meaning that if we specify the edges we want to timestamp,
	// the driver will return EOPNOTSUPP if it cannot support that exactly.
	// PTP_EXTTS_REQUEST is not strict: the edges are just a hint.
	// Because of this:
	// https://lore.kernel.org/all/20250414-jk-supported-perout-flags-v2-1-f6b17d15475c@intel.com/
	// PTP_EXTTS_REQUEST2 will return EOPNOTSUPP on CM4/CM5 in some kernel versions e.g. 6.16.7
	// So only use PTP_EXTTS_REQUEST2 when we really need it - when we are specifying the edges we want to timestamp.
	otherFlags := req.Flags &^ unix.PTP_ENABLE_FEATURE
	if otherFlags != 0 {
		// In the latest kernel (6.16.7), PTP_EXTTS_REQUEST2 assumes PTP_STRICT_FLAGS,
		// but I am not sure if this has always been the case,
		// so add it in to be safe if we specify something other than just PTP_ENABLE_FLAGS
		req2 := *req
		req2.Flags |= unix.PTP_STRICT_FLAGS
		// use PTP_EXTTS_REQUEST2 ioctl
		return clk.wrapErr(unix.IoctlPtpExttsRequest(fd, &req2), "ioctl(PTP_EXTTS_REQUEST2)")
	} else {
		// use older PTP_EXTTS_REQUEST ioctl
		return clk.wrapErr(ioctlPtpExttsRequest(fd, req), "ioctl(PTP_EXTTS_REQUEST)")
	}
}

func (clk *Clock) ExttsChanCount() int {
	return int(clk.caps.N_ext_ts)
}

func (clk *Clock) PeroutChanCount() int {
	return int(clk.caps.N_per_out)
}

func (clk *Clock) GetTime() (ptime.Time, error) {
	var ts unix.Timespec
	err := unix.ClockGettime(clk.clockId(), &ts)
	if err != nil {
		return 0, clk.wrapErr(err, "clock_gettime")
	}
	return ptime.TimespecToTime(ts), nil
}

// PeroutEnable enables a periodic pulse on a channel.
// chanIndex is the index of the channel.
// period is the period of the pulse; 0 means disable the pulse
// width is the width of the pulse; 0 means use what driver provides
// startOffset is the offset from the top of the second to the start
func (clk *Clock) PeroutEnable(chanIndex uint32, period, width, startOffset time.Duration) error {
	req := unix.PtpPeroutRequest{
		Index:  chanIndex,
		Period: durationToPtpClockTime(period),
	}

	// Set duty cycle if width specified
	if width > 0 {
		req.Flags |= unix.PTP_PEROUT_DUTY_CYCLE
		req.On = durationToPtpClockTime(width)
	}

	// If period > 0, set start time to now + 2 seconds
	// Period of 0 disables the output
	if period > 0 {
		now, err := clk.GetTime()
		if err != nil {
			return err
		}
		startTime := now.Round(time.Second).Add(2*time.Second + startOffset)
		req.StartOrPhase = timespecToPtpClockTime(startTime.Timespec())
	}
	// Duty cycle won't work on the CM4/CM5 on a few kernels,
	// (similar problem with exttsRequest above)
	// but we cannot do anything about it in user space, since PTP_PEROUT_DUTY_CYCLE is
	// a PTP_PEROUT_REQUEST2 feature.
	return clk.wrapErr(unix.IoctlPtpPeroutRequest(clk.fd, &req), "ioctl(PTP_PEROUT_REQUEST2)")
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
	tx.Time.Sec = kernelLong(secs)
	tx.Time.Usec = kernelLong(nsecs)
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
	tx.Freq = kernelLong(newFreq)
	_, err := clk.adjtimex(&tx, "(ADJ_FREQUENCY)")
	if int64(tx.Freq) != newFreq {
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

// durationToPtpClockTime converts a time.Duration to unix.PtpClockTime
func durationToPtpClockTime(d time.Duration) unix.PtpClockTime {
	sec := int64(d / time.Second)
	nsec := uint32(d % time.Second)
	return unix.PtpClockTime{Sec: sec, Nsec: nsec}
}

// timespecToPtpClockTime converts a unix.Timespec to unix.PtpClockTime
func timespecToPtpClockTime(ts unix.Timespec) unix.PtpClockTime {
	return unix.PtpClockTime{
		Sec:  int64(ts.Sec),
		Nsec: uint32(ts.Nsec),
	}
}
