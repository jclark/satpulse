package ntptime

import (
	"errors"
	"time"
)

type LeapSecondStatus int

const (
	LeapSecondNormal     LeapSecondStatus = iota // Normal operation, no leap second
	LeapSecondInsTonight                         // Positive leap second at end of UTC day
	LeapSecondDelTonight                         // Negative leap second at end of UTC day
	LeapSecondInProgress                         // Currently in a leap second
	LeapSecondRecent                             // Leap second occurred in the last few seconds
)

// State represents the current NTP synchronization state of the system clock.
type State struct {
	Time             time.Time // current system time
	Synchronized     bool      // true if the system clock is synchronized with NTP
	LeapSecondStatus LeapSecondStatus
	EstError         time.Duration
	MaxError         time.Duration
	TAIOffset        int // current TAI-UTC offset in seconds, zero if not known
}

var ErrorNotSupported = errors.New("NTP kernel state not available on this platform")
