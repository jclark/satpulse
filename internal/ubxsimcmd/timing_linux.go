package ubxsimcmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jclark/satpulse/gps/app/ubxsim"
	"golang.org/x/sys/unix"
)

func openTiming(v *flagVars) (*ubxsim.Timing, func(), error) {
	t := &ubxsim.Timing{ReadTime: systemTime, LeapSeconds: v.leapSeconds}
	var phc *os.File
	if v.phcPath != "" {
		var err error
		phc, err = os.Open(v.phcPath)
		if err != nil {
			return nil, nil, err
		}
		t.ReadTime = func() (int64, int64, error) {
			p, err := unix.IoctlPtpSysOffsetPrecise(int(phc.Fd()))
			if err != nil {
				return 0, 0, err
			}
			return p.Device.Sec*1e9 + int64(p.Device.Nsec), p.Monoraw.Sec*1e9 + int64(p.Monoraw.Nsec), nil
		}
	}
	closePHC := func() {
		if phc != nil {
			phc.Close()
		}
	}
	if _, _, err := t.ReadTime(); err != nil {
		closePHC()
		return nil, nil, fmt.Errorf("read timing reference: %w", err)
	}
	edge, err := os.OpenFile(v.edgePath, os.O_WRONLY, 0)
	if err != nil {
		closePHC()
		return nil, nil, err
	}
	t.ScheduleEdge = func(raw int64) error {
		// Each write replaces the schedule. The timing engine owns one
		// rising-edge stream and submits only after its previous edge passed.
		_, err := edge.WriteAt([]byte(strconv.FormatInt(raw, 10)+"\n"), 0)
		return err
	}
	return t, func() { edge.Close(); closePHC() }, nil
}

// systemTime brackets realtime with raw reads and uses the shortest of five
// samples to limit scheduler interference in the TCG fallback.
func systemTime() (utc, raw int64, err error) {
	var best int64 = 1<<63 - 1
	for range 5 {
		var a, b, c unix.Timespec
		if err = unix.ClockGettime(unix.CLOCK_MONOTONIC_RAW, &a); err != nil {
			return
		}
		if err = unix.ClockGettime(unix.CLOCK_REALTIME, &b); err != nil {
			return
		}
		if err = unix.ClockGettime(unix.CLOCK_MONOTONIC_RAW, &c); err != nil {
			return
		}
		d := c.Nano() - a.Nano()
		if d < best {
			best = d
			utc, raw = b.Nano(), a.Nano()+d/2
		}
	}
	return
}
