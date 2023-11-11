package gpsmsg

import (
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

type Handler interface {
	Time(msg *Time, tRead time.Time)
	LeapSecond(msg *LeapSecond, tRead time.Time)
}

type DefaultHandler struct{}

func (h *DefaultHandler) Time(msg *Time, tRead time.Time)             {}
func (h *DefaultHandler) LeapSecond(msg *LeapSecond, tRead time.Time) {}

//go:generate stringer -type=MajorGNSS
type MajorGNSS int

// Zero value means invalid/unknown/unspecified
const (
	GPS MajorGNSS = iota + 1
	GLONASS
	BeiDou
	Galileo
)

const NMajorGNSS = 4

func (g MajorGNSS) MarshalText() ([]byte, error) {
	return []byte(g.String()), nil
}

// MajorGNSSSet is a set of MajorGNSS values.
// It is comparable.
type MajorGNSSSet uint32

func MajorGNSSFlag(g MajorGNSS) MajorGNSSSet {
	return 1 << g
}

func (s MajorGNSSSet) Contains(g MajorGNSS) bool {
	return s&MajorGNSSFlag(g) != 0
}

func (s MajorGNSSSet) Items() []MajorGNSS {
	items := make([]MajorGNSS, 0, 4)
	for g := MajorGNSS(1); g <= NMajorGNSS; g++ {
		if s.Contains(g) {
			items = append(items, g)
		}
	}
	return items
}

type TimeRef int

const (
	NavSolution TimeRef = iota
	NextPulse
	LastPulse
)

type Time struct {
	TAITime     ptime.Time     `json:"taiTime,omitempty"`
	UTCTime     *ptime.UTCTime `json:"utcTime,omitempty"`
	Accuracy    time.Duration  `json:"accuracy,omitempty"`
	UTCOffset   uint8          `json:"utcOffset,omitempty"`
	PulseOffset time.Duration  `json:"pulseOffset,omitempty"`
	GNSS        MajorGNSS      `json:"gnss,omitempty"`
	Ref         TimeRef        `json:"ref,omitempty"`
	NavEpoch    uint32         `json:"navEpoch,omitempty"`
	SrcType     string         `json:"srcType"`
}

type LeapSecond struct {
	ptime.LeapSecond
	NavEpoch uint32 `json:"navEpoch,omitempty"`
}
