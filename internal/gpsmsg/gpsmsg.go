package gpsmsg

import (
	"time"

	"github.com/jclark/gps2phc/internal/ptime"
)

type Message interface {
	Time() *Time
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
	TAIMinusUTC   uint8          `json:"tai-utc,omitempty"`
	PulseOffset   time.Duration  `json:"pulseOffset,omitempty"`
	GNSS          *MajorGNSS     `json:"gnss,omitempty"`
	PrecedesPulse bool           `json:"precedesPulse,omitempty"`
	NavEpoch      uint32         `json:"navEpoch,omitempty"`
}
