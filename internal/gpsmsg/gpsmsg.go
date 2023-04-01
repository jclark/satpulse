package gpsmsg

import (
	"time"

	"github.com/jclark/gps4ptp/internal/ptime"
)

type Message interface {
	Time() *Time
	LeapSecond() *LeapSecond
}

//go:generate stringer -type=MajorGNSS
type MajorGNSS int

const (
	GPS MajorGNSS = iota
	GLONASS
	BeiDou
	Galileo
)

func (g MajorGNSS) MarshalText() ([]byte, error) {
	return []byte(g.String()), nil
}

type Time struct {
	TAITime       ptime.Time     `json:"taiTime,omitempty"`
	UTCTime       *ptime.UTCTime `json:"utcTime,omitempty"`
	Accuracy      time.Duration  `json:"accuracy,omitempty"`
	UTCOffset     uint8          `json:"utcOffset,omitempty"`
	PulseOffset   time.Duration  `json:"pulseOffset,omitempty"`
	GNSS          *MajorGNSS     `json:"gnss,omitempty"`
	PrecedesPulse bool           `json:"precedesPulse,omitempty"`
	NavEpoch      uint32         `json:"navEpoch,omitempty"`
}

type LeapSecond struct {
	ptime.LeapSecond
	NavEpoch uint32 `json:"navEpoch,omitempty"`
}
