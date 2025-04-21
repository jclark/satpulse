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
	Satellites(msg *SatellitesMsg, tRead time.Time)
}

// NativeMsgHandler handles protocol-specific messages that don't map to standard messages.
type NativeMsgHandler interface {
	// NativeMsg processes a protocol-specific message.
	// tag: identifies the protocol (e.g., UBX, NMEA).
	// msgID: identifies the message type within the protocol (e.g., UBX-NAV-PVT, NMEA-GGA).
	// msg: the protocol-specific message object.
	// tRead: timestamp when the message was received.
	NativeMsg(tag Tag, msgID string, msg interface{}, tRead time.Time) error
}

type DefaultHandler struct{}

func (h *DefaultHandler) Time(msg *TimeMsg, tRead time.Time)             {}
func (h *DefaultHandler) LeapSecond(msg *LeapSecondMsg, tRead time.Time) {}
func (h *DefaultHandler) Survey(msg *SurveyMsg, tRead time.Time)         {}
func (h *DefaultHandler) Satellites(msg *SatellitesMsg, tRead time.Time) {}

type multiHandler struct {
	handlers []MsgHandler
}

func (h *multiHandler) Time(msg *TimeMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.Time(msg, tRead)
	}
}

func (h *multiHandler) LeapSecond(msg *LeapSecondMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.LeapSecond(msg, tRead)
	}
}

func (h *multiHandler) Survey(msg *SurveyMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.Survey(msg, tRead)
	}
}

func (h *multiHandler) Satellites(msg *SatellitesMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.Satellites(msg, tRead)
	}
}

func MultiHandler(handlers ...MsgHandler) MsgHandler {
	return &multiHandler{handlers: handlers}
}

//go:generate stringer -type=GNSS
type GNSS uint8

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

func (g GNSS) SVIDPrefix() string {
	switch g {
	case GPS:
		return "G"
	case GAL:
		return "E"
	case BDS:
		return "C"
	case GLO:
		return "R"
	case NAVIC:
		return "I"
	case QZSS:
		return "J"
	case SBAS:
		return "S"
	default:
		return ""
	}
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

func (gp *GNSS) UnmarshalText(text []byte) error {
	g, err := ParseGNSS(string(text))
	if err == nil {
		*gp = g
	}
	return err
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

// String returns a comma-separated list of the GNSS names in the set.
// Returns "(none)" if the set is empty.
func (s GNSSSet) String() string {
	if s == 0 {
		return "(none)"
	}

	items := s.Items()
	names := make([]string, len(items))
	for i, g := range items {
		names[i] = g.String()
	}

	return strings.Join(names, ",")
}

// SVID is an identifier of a space vehicle (satellite).
type SVID struct {
	GNSS GNSS
	// PRN code number used by the satellite, or orbital slot number for GLONASS FDMA
	// int16 because NMEA layer passes up non-standard three-digit SVIDs
	PRN int16
}

func (sv SVID) String() string {
	return fmt.Sprintf("%s%02d", sv.GNSS.SVIDPrefix(), sv.PRN)
}

func (sv SVID) MarshalJSON() ([]byte, error) {
	return json.Marshal(sv.String())
}

// SVInfo contains information about a space vehicle (satellite).
type SVInfo struct {
	SVID      SVID  `json:"svid"`
	Azimuth   int16 `json:"azimuth"`   // in degrees, 0 to 360
	Elevation int8  `json:"elevation"` // in degrees, -90 to +90
	CNO       uint8 `json:"cno"`       // C/NO signal to noise ratio
}

type SatellitesMsg struct {
	NavEpoch    uint32   `json:"navEpoch,omitempty"`
	Tag         Tag      `json:"tag,omitempty"`
	NativeMsgID string   `json:"nativeMsgID,omitempty"`
	Info        []SVInfo `json:"info"` // info about satellites being tracked or acquired
}

//go:generate stringer -type=TimeRef
type TimeRef int

const (
	NavSolution TimeRef = iota // a message provding part of the result of a navigation solution (e.g. UBX-NAV-TIMEGPS)
	PrePulse                   // a message that is emitted before a pulse (e.g. UBX-TIM-TP)
	PostPulse                  // a message that is emitted immediately after a pulse (e.g. UBX-TIM-TOS)
)

type TimeMsg struct {
	TAITime     ptime.Time     `json:"taiTime,omitempty"`
	UTCTime     *ptime.UTCTime `json:"utcTime,omitempty"`
	Accuracy    time.Duration  `json:"accuracy,omitempty"`
	UTCOffset   uint8          `json:"utcOffset,omitempty"`
	PulseOffset *time.Duration `json:"pulseOffset,omitempty"` // the time of the pulse minus the time of the top of the second
	GNSS        GNSS           `json:"gnss,omitempty"`
	Ref         TimeRef        `json:"ref,omitempty"`
	NavEpoch    uint32         `json:"navEpoch,omitempty"`
	Tag         Tag            `json:"tag,omitempty"`
	NativeMsgID string         `json:"nativeMsgID,omitempty"`
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
