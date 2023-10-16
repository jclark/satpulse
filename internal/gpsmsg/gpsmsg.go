package gpsmsg

import (
	"time"

	"github.com/jclark/gps4ptp/internal/ptime"
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
	SrcType       string         `json:"srcType"`
}

type LeapSecond struct {
	ptime.LeapSecond
	NavEpoch uint32 `json:"navEpoch,omitempty"`
}

type Config interface {
	TimeMode() *TimeMode
	SolutionPeriod() time.Duration
	EnabledGNSS() []MajorGNSS
}

type TimeMode struct {
	FixedPos *FixedPosMode `json:"fixedPos,omitempty"`
	SurveyIn *SurveyInMode `json:"surveyIn,omitempty"`
}

type SurveyInMode struct {
	MinDur   float64 // seconds
	AccLimit float64 // meters
}

type FixedPosMode struct {
	LLA  *LLA    `json:"lla,omitempty"`
	ECEF *ECEF   `json:"ecef,omitempty"`
	Acc  float64 `json:"acc,omitempty"`
}

type ECEF struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type LLA struct {
	Lat float64  `json:"lat"` // degrees
	Lon float64  `json:"lon"` // degrees
	Alt *float64 `json:"alt,omitempty"`
}
