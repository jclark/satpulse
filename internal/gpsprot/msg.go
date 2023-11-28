package gpsprot

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

type MsgHandler interface {
	Time(msg *TimeMsg, tRead time.Time)
	LeapSecond(msg *LeapSecondMsg, tRead time.Time)
	Survey(msg *SurveyMsg, tRead time.Time)
}

type DefaultHandler struct{}

func (h *DefaultHandler) Time(msg *TimeMsg, tRead time.Time)             {}
func (h *DefaultHandler) LeapSecond(msg *LeapSecondMsg, tRead time.Time) {}
func (h *DefaultHandler) Survey(msg *SurveyMsg, tRead time.Time)         {}

//go:generate stringer -type=GNSS
type GNSS int

// Constants for GNSS type.
// Zero value means invalid/unknown/unspecified.
// The major GNSS systems are first, in order of preference.
// We prefer Galileo to BeiDou, because of close synchronization of Galileo and GPS.
// GLONASS is least preferred, because its unusual handling of leap seconds fits poorly with PTP.
// NavIC comes next because althought it is regional, it is standalone, not just an augmentation system
// QZSS started off as an augmentation system, but is getting the capability to be a standalone system.
// SBAS is an augmentation system, and not a standalone GNSS system.
const (
	GPS      GNSS = iota + 1 // GPS (USA)
	GAL                      // Galileo (Europe)
	BDS                      // BeiDou (China)
	GLO                      // GLONASS (Russia)
	NAVIC                    // NavIC (India)
	QZSS                     // QZSS (Japan)
	SBAS                     // Satellite-Based Augmentation System (e.g. WAAS, EGNOS, GAGAN, MSAS)
	GNSSLast GNSS = SBAS
)

func ParseGNSS(s string) (GNSS, error) {
	switch strings.ToUpper(s) {
	case "GPS":
		return GPS, nil
	case "GAL", "GALILEO":
		return GAL, nil
	case "BDS", "BEIDOU":
		return BDS, nil
	case "GLO", "GLONASS":
		return GLO, nil
	case "NAVIC":
		return NAVIC, nil
	case "QZSS":
		return QZSS, nil
	case "SBAS":
		return SBAS, nil
	}
	if s == "" {
		return 0, errors.New("invalid GNSS name: empty string")
	}
	return 0, fmt.Errorf("%s: invalid GNSS name", s)
}

func (g GNSS) IsMajor() bool {
	return g >= GPS && g <= GLO
}

func (g GNSS) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.String())
}

func (g GNSS) MarshalText() ([]byte, error) {
	return []byte(g.String()), nil
}

// GNSSSet is a set of GNSS values.
// It is comparable.
type GNSSSet uint32

func GNSSFlag(gs ...GNSS) GNSSSet {
	var flags GNSSSet
	for _, g := range gs {
		flags |= 1 << g
	}
	return flags
}

const MajorGNSSSet GNSSSet = 1<<GPS | 1<<GAL | 1<<BDS | 1<<GLO

func (s GNSSSet) Contains(g GNSS) bool {
	return s&GNSSFlag(g) != 0
}

func (s GNSSSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Items())
}

func (s GNSSSet) Items() []GNSS {
	items := make([]GNSS, 0, 4)
	for g := GNSS(1); g <= GNSSLast; g++ {
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

type TimeMsg struct {
	TAITime     ptime.Time     `json:"taiTime,omitempty"`
	UTCTime     *ptime.UTCTime `json:"utcTime,omitempty"`
	Accuracy    time.Duration  `json:"accuracy,omitempty"`
	UTCOffset   uint8          `json:"utcOffset,omitempty"`
	PulseOffset time.Duration  `json:"pulseOffset,omitempty"`
	GNSS        GNSS           `json:"gnss,omitempty"`
	Ref         TimeRef        `json:"ref,omitempty"`
	NavEpoch    uint32         `json:"navEpoch,omitempty"`
	SrcType     string         `json:"srcType"`
}

type LeapSecondMsg struct {
	ptime.LeapSecond
	NavEpoch uint32 `json:"navEpoch,omitempty"`
}

type SurveyMsg struct {
	Position   Point3D       `json:"position"`
	Accuracy   Length        `json:"accuracy"`
	NavEpoch   uint32        `json:"navEpoch,omitempty"`
	ObsCount   uint32        `json:"obsCount"`
	ObsTime    time.Duration `json:"obsTime"`
	Valid      bool          `json:"valid"`
	InProgress bool          `json:"inProgress"`
}
