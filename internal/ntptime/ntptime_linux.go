//go:build linux

package ntptime

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// Get returns the current NTP state of the system clock.
func Get() (*State, error) {
	var tx unix.Timex
	// If we use ADJ_NANOS here, then it would require root
	state, err := unix.Adjtimex(&tx)
	if err != nil {
		return nil, fmt.Errorf("adjtimex: %w", err)
	}
	var ls LeapSecondStatus
	switch state {
	case unix.TIME_INS:
		ls = LeapSecondInsTonight
	case unix.TIME_DEL:
		ls = LeapSecondDelTonight
	case unix.TIME_OOP:
		ls = LeapSecondInProgress
	case unix.TIME_OK:
		ls = LeapSecondNormal
	case unix.TIME_WAIT:
		ls = LeapSecondRecent
	}
	return &State{
		Time:             time.Unix(tx.Time.Sec, tx.Time.Usec*1000),
		EstError:         time.Duration(tx.Esterror) * time.Microsecond,
		MaxError:         time.Duration(tx.Maxerror) * time.Microsecond,
		TAIOffset:        int(tx.Tai),
		LeapSecondStatus: ls,
		Synchronized:     state != unix.TIME_ERROR,
	}, nil
}
