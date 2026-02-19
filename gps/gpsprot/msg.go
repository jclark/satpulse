package gpsprot

import (
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

// MsgPriority indicates the trustworthiness of a message's fields
// for priority-based merging within a navigation epoch.
type MsgPriority uint8

const (
	PriGenericLow  MsgPriority = iota + 1 // NMEA, basic sentence
	PriGenericHigh                         // NMEA, richer sentence (e.g. GGA over RMC)
	PriVendorLow                           // vendor binary, standard message
	PriVendorHigh                          // vendor binary, high-precision message
)

type MsgHandler interface {
	Time(msg *TimeMsg, tRead time.Time)
	PosGeo(msg *PosGeoMsg, tRead time.Time)
	PosECEF(msg *PosECEFMsg, tRead time.Time)
	VelGeo(msg *VelGeoMsg, tRead time.Time)
	VelECEF(msg *VelECEFMsg, tRead time.Time)
	LeapSecond(msg *LeapSecondMsg, tRead time.Time)
	Survey(msg *SurveyMsg, tRead time.Time)
	Satellites(msg *SatellitesMsg, tRead time.Time)
	NavEpoch(msg *NavEpochMsg, tRead time.Time)
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
func (h *DefaultHandler) PosGeo(msg *PosGeoMsg, tRead time.Time)         {}
func (h *DefaultHandler) PosECEF(msg *PosECEFMsg, tRead time.Time)       {}
func (h *DefaultHandler) VelGeo(msg *VelGeoMsg, tRead time.Time)         {}
func (h *DefaultHandler) VelECEF(msg *VelECEFMsg, tRead time.Time)       {}
func (h *DefaultHandler) LeapSecond(msg *LeapSecondMsg, tRead time.Time) {}
func (h *DefaultHandler) Survey(msg *SurveyMsg, tRead time.Time)         {}
func (h *DefaultHandler) Satellites(msg *SatellitesMsg, tRead time.Time) {}
func (h *DefaultHandler) NavEpoch(msg *NavEpochMsg, tRead time.Time)     {}

type MultiHandler struct {
	handlers []MsgHandler
}

func (h *MultiHandler) Time(msg *TimeMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.Time(msg, tRead)
	}
}

func (h *MultiHandler) LeapSecond(msg *LeapSecondMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.LeapSecond(msg, tRead)
	}
}

func (h *MultiHandler) Survey(msg *SurveyMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.Survey(msg, tRead)
	}
}

func (h *MultiHandler) Satellites(msg *SatellitesMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.Satellites(msg, tRead)
	}
}

func (h *MultiHandler) PosGeo(msg *PosGeoMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.PosGeo(msg, tRead)
	}
}

func (h *MultiHandler) PosECEF(msg *PosECEFMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.PosECEF(msg, tRead)
	}
}

func (h *MultiHandler) VelGeo(msg *VelGeoMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.VelGeo(msg, tRead)
	}
}

func (h *MultiHandler) VelECEF(msg *VelECEFMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.VelECEF(msg, tRead)
	}
}

func (h *MultiHandler) NavEpoch(msg *NavEpochMsg, tRead time.Time) {
	for _, handler := range h.handlers {
		handler.NavEpoch(msg, tRead)
	}
}

func NewMultiHandler(handlers ...MsgHandler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Handlers returns an iterator over the message handlers
func (h *MultiHandler) Handlers() iter.Seq[MsgHandler] {
	return func(yield func(MsgHandler) bool) {
		for _, handler := range h.handlers {
			if !yield(handler) {
				return
			}
		}
	}
}

// MultiNativeMsgHandler fans out NativeMsg calls to multiple handlers
type MultiNativeMsgHandler struct {
	handlers []NativeMsgHandler
}

func NewMultiNativeMsgHandler(handlers ...NativeMsgHandler) *MultiNativeMsgHandler {
	return &MultiNativeMsgHandler{handlers: handlers}
}

func (m *MultiNativeMsgHandler) Reset(handlers ...NativeMsgHandler) {
	m.handlers = handlers
}

func (m *MultiNativeMsgHandler) NativeMsg(tag Tag, msgID string, msg any, tRead time.Time) error {
	var firstErr error
	for _, h := range m.handlers {
		if err := h.NativeMsg(tag, msgID, msg, tRead); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// MsgBundle holds a set of protocol-agnostic messages produced from a
// single packet or sentence. Any combination of fields may be non-nil.
// NavEpochMsg is excluded: it is accumulated across multiple messages
// within an epoch and has a different lifecycle.
type MsgBundle struct {
	Time       *TimeMsg
	PosGeo     *PosGeoMsg
	PosECEF    *PosECEFMsg
	VelGeo     *VelGeoMsg
	VelECEF    *VelECEFMsg
	LeapSecond *LeapSecondMsg
	Survey     *SurveyMsg
}

// Dispatch calls the corresponding MsgHandler methods for each non-nil field.
func (b *MsgBundle) Dispatch(h MsgHandler, tRead time.Time) {
	if b.Time != nil {
		h.Time(b.Time, tRead)
	}
	if b.PosGeo != nil {
		h.PosGeo(b.PosGeo, tRead)
	}
	if b.PosECEF != nil {
		h.PosECEF(b.PosECEF, tRead)
	}
	if b.VelGeo != nil {
		h.VelGeo(b.VelGeo, tRead)
	}
	if b.VelECEF != nil {
		h.VelECEF(b.VelECEF, tRead)
	}
	if b.LeapSecond != nil {
		h.LeapSecond(b.LeapSecond, tRead)
	}
	if b.Survey != nil {
		h.Survey(b.Survey, tRead)
	}
}

// SetPriority sets Priority on all non-nil Pos/Vel/Time messages in the bundle.
func (b *MsgBundle) SetPriority(pri MsgPriority) {
	if b.Time != nil {
		b.Time.Priority = pri
	}
	if b.PosGeo != nil {
		b.PosGeo.Priority = pri
	}
	if b.PosECEF != nil {
		b.PosECEF.Priority = pri
	}
	if b.VelGeo != nil {
		b.VelGeo.Priority = pri
	}
	if b.VelECEF != nil {
		b.VelECEF.Priority = pri
	}
}

// GNSSSet is a set of GNSS values.
// It is comparable.
type GNSSSet uint32

func GNSSSetOf(gs ...GNSS) GNSSSet {
	var flags GNSSSet
	for _, g := range gs {
		if g != 0 {
			flags |= 1 << g
		}
	}
	return flags
}

const MajorGNSSSet GNSSSet = 1<<GPS | 1<<GAL | 1<<BDS | 1<<GLO

func (s GNSSSet) Contains(g GNSS) bool {
	return s&GNSSSetOf(g) != 0
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

const GLOUnknown uint8 = 0 // with GLONASS FDMA, it is possible to be tracking a satellite but not know its PRN

// SVID is an identifier of a space vehicle (satellite).
type SVID struct {
	GNSS GNSS
	// Num is a number identifying the SV within a specific GNSS.
	// Numbering is the same as in RINEX 3.04.
	// For GPS, GAL, BDS, NAVIC, this is the PRN (pseudo-random noise) number.
	// For GLONASS, this is the orbital slot number.
	// For SBAS, this is the PRN number minus 100.
	// For QZSS, this is the PRN number minus 192.
	// This gives two-digit, non-zero numbers.
	// In addition, this can be GLOUnknown, when the GNSS is GLONASS
	Num uint8
}

func (sv SVID) String() string {
	if sv.Num == GLOUnknown {
		return fmt.Sprintf("%s?", sv.GNSS.SVIDPrefix())
	}
	return fmt.Sprintf("%s%02d", sv.GNSS.SVIDPrefix(), sv.Num)
}

func (sv SVID) MarshalJSON() ([]byte, error) {
	return json.Marshal(sv.String())
}

func (sv SVID) IsZero() bool {
	return sv.GNSS == 0 && sv.Num == 0
}

// IsValid checks if the SVID has a valid Num for its GNSS type
func (sv SVID) IsValid() bool {
	return sv.GNSS.IsValidSVNum(int(sv.Num))
}

type SVInfo struct {
	ID         SVID         `json:"id"`
	LookAngles *LookAngles  `json:"lookAngles,omitempty"` // look angle of the satellite
	Signals    []SignalInfo `json:"signals"`              // signals being transmitted by a satellite
	Used       bool         `json:"used,omitempty"`       // true if the satellite is used in the navigation solution
}

type LookAngles struct {
	Azimuth   int16 `json:"azimuth"`   // in degrees, 0 to 360
	Elevation int8  `json:"elevation"` // in degrees, -
}

// SignalInfo contains information about a single signal transmitted by a satellite.
type SignalInfo struct {
	ID   SignalID `json:"id,omitempty"`   // human-readable label of the signal e.g. "L5"
	CN0  uint8    `json:"cn0"`            // C/NO signal to noise ratio
	Used bool     `json:"used,omitempty"` // true if the signal is used in the navigation solution
}

type SatelliteUsedValidity int

const (
	SatelliteUsedInvalid SatelliteUsedValidity = iota // Used is not specified in both SVInfo and SignalInfo
	SatelliteUsedSV                                   // Used is specified in SVInfo, but not in SignalInfo
	SatelliteUsedSignal                               // Used is specified in SignalInfo and in SVInfo
)

type SatellitesMsg struct {
	Tag          Tag                   `json:"tag,omitempty"`
	NativeMsgID  string                `json:"nativeMsgID,omitempty"`
	SVs          []SVInfo              `json:"info"`                   // satellites being tracked
	UsedValidity SatelliteUsedValidity `json:"usedValidity,omitempty"` // says whether Used fields in SVInfo and SignalInfo are valid
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
	PulseOffset *float64       `json:"pulseOffset,omitempty"` // the true time of the top of second is the time of the pulse plus the PulseOffset
	GNSS        GNSS           `json:"gnss,omitempty"`
	Ref         TimeRef        `json:"ref,omitempty"`
	Priority    MsgPriority    `json:"-"`
	Tag         Tag            `json:"tag,omitempty"`
	NativeMsgID string         `json:"nativeMsgID,omitempty"`
}

// ComputeTAITime computes the TAI time from this message, using the leap second for UTC conversion if needed
func (msg *TimeMsg) ComputeTAITime(ls ptime.LeapSecond) (ptime.Time, bool) {
	if !msg.TAITime.IsZero() {
		return msg.TAITime, true
	}
	if msg.UTCTime == nil {
		return 0, false
	}
	return ls.UTCtoTime(*msg.UTCTime), true
}

// PosGeoMsg is a geodetic position (latitude, longitude, height above WGS-84 ellipsoid).
type PosGeoMsg struct {
	LatLon      [2]Angle        `json:"latLon"`             // [lat, lon]; lat positive north, lon positive east
	Height      opt.Val[Length] `json:"height,omitzero"`    // above WGS-84 ellipsoid
	HeightMSL   opt.Val[Length] `json:"heightMSL,omitzero"` // above mean sea level
	Priority    MsgPriority     `json:"-"`
	Tag         Tag             `json:"tag"`
	NativeMsgID string          `json:"nativeMsgID"`
}

// PosECEFMsg is an Earth-Centered, Earth-Fixed position.
type PosECEFMsg struct {
	Pos         Point3D     `json:"pos"` // ECEF X, Y, Z
	Priority    MsgPriority `json:"-"`
	Tag         Tag         `json:"tag"`
	NativeMsgID string      `json:"nativeMsgID"`
}

// VelGeoMsg is velocity in the local geodetic frame.
type VelGeoMsg struct {
	VelNED      opt.Val[[3]Speed] `json:"velNED,omitzero"`      // north, east, down
	GroundSpeed opt.Val[Speed]    `json:"groundSpeed,omitzero"` // 2D ground speed
	Speed3D     opt.Val[Speed]    `json:"speed3D,omitzero"`     // 3D speed
	Course      opt.Val[Angle]    `json:"course,omitzero"`      // course over ground, true north
	Priority    MsgPriority       `json:"-"`
	Tag         Tag               `json:"tag"`
	NativeMsgID string            `json:"nativeMsgID"`
}

// VelECEFMsg is velocity in the ECEF frame.
type VelECEFMsg struct {
	Vel         [3]Speed    `json:"vel"` // ECEF VX, VY, VZ
	Priority    MsgPriority `json:"-"`
	Tag         Tag         `json:"tag"`
	NativeMsgID string      `json:"nativeMsgID"`
}

type LeapSecondMsg struct {
	ptime.LeapSecond
	GNSS GNSS `json:"gnss,omitempty"` // the GNSS that is the source of this leap second
}

// UpdateLeapSecond updates the target leap second if this message contains newer information
// Returns true if the target was updated
func (msg *LeapSecondMsg) UpdateLeapSecond(target *ptime.LeapSecond) bool {
	if msg.OffChangeTime <= target.OffChangeTime {
		return false
	}
	*target = msg.LeapSecond
	return true
}

type SurveyMsg struct {
	Position   Point3D       `json:"position,omitzero"`
	Accuracy   Length        `json:"accuracy"`
	ObsCount   uint32        `json:"obsCount"`
	ObsTime    time.Duration `json:"obsTime"`
	Valid      bool          `json:"valid"`
	InProgress bool          `json:"inProgress"`
}

// NavEpochMsg is emitted once at the end of each navigation epoch, after
// all time/position/velocity messages for that epoch have been dispatched.
// Future fields will carry solution metadata (fix quality, corrections, DOPs).
type NavEpochMsg struct {
	Acc       Accuracy  `json:"acc,omitzero"`
	Tag       Tag       `json:"tag,omitzero"`
	StartTime time.Time `json:"startTime"` // when the first message in this epoch was read
}

// Accuracy holds estimated accuracy of the navigation solution. Fields are
// opt.Val because different protocols provide different subsets. Accuracy
// may be synthesized from multiple messages within an epoch.
type Accuracy struct {
	Pos         opt.Val[Length] `json:"pos,omitzero"`         // 3D position accuracy
	Hor         opt.Val[Length] `json:"hor,omitzero"`         // horizontal position accuracy
	Vert        opt.Val[Length] `json:"vert,omitzero"`        // vertical position accuracy
	Speed       opt.Val[Speed]  `json:"speed,omitzero"`       // 3D speed accuracy
	GroundSpeed opt.Val[Speed]  `json:"groundSpeed,omitzero"` // 2D ground speed accuracy
	Course      opt.Val[Angle]  `json:"course,omitzero"`      // course/heading accuracy
}

// mergeOpt merges a single opt.Val field from src into dst based on priority.
// If src is unset, dst is unchanged.
// If srcPri >= dstPri, dst is overwritten with src.
// If srcPri < dstPri, dst is filled from src only if dst is unset.
func mergeOpt[T any](dst, src *opt.Val[T], dstPri, srcPri MsgPriority) {
	if !src.IsSet() {
		return
	}
	if srcPri >= dstPri || !dst.IsSet() {
		*dst = *src
	}
}

// Merge incorporates fields from other into m based on priority.
func (m *TimeMsg) Merge(other *TimeMsg) {
	dp, sp := m.Priority, other.Priority
	if sp >= dp {
		m.TAITime = other.TAITime
		m.Accuracy = other.Accuracy
		m.UTCOffset = other.UTCOffset
		m.GNSS = other.GNSS
		m.Ref = other.Ref
		m.Priority = sp
		m.Tag = other.Tag
		m.NativeMsgID = other.NativeMsgID
		if other.UTCTime != nil {
			m.UTCTime = other.UTCTime
		}
		if other.PulseOffset != nil {
			m.PulseOffset = other.PulseOffset
		}
	} else {
		if m.UTCTime == nil {
			m.UTCTime = other.UTCTime
		}
		if m.PulseOffset == nil {
			m.PulseOffset = other.PulseOffset
		}
	}
}

// Merge incorporates fields from other into m based on priority.
func (m *PosGeoMsg) Merge(other *PosGeoMsg) {
	dp, sp := m.Priority, other.Priority
	if sp >= dp {
		m.LatLon = other.LatLon
		m.Priority = sp
		m.Tag = other.Tag
		m.NativeMsgID = other.NativeMsgID
	}
	mergeOpt(&m.Height, &other.Height, dp, sp)
	mergeOpt(&m.HeightMSL, &other.HeightMSL, dp, sp)
}

// Merge incorporates fields from other into m based on priority.
func (m *PosECEFMsg) Merge(other *PosECEFMsg) {
	if other.Priority >= m.Priority {
		m.Pos = other.Pos
		m.Priority = other.Priority
		m.Tag = other.Tag
		m.NativeMsgID = other.NativeMsgID
	}
}

// Merge incorporates fields from other into m based on priority.
func (m *VelGeoMsg) Merge(other *VelGeoMsg) {
	dp, sp := m.Priority, other.Priority
	if sp >= dp {
		m.Priority = sp
		m.Tag = other.Tag
		m.NativeMsgID = other.NativeMsgID
	}
	mergeOpt(&m.VelNED, &other.VelNED, dp, sp)
	mergeOpt(&m.GroundSpeed, &other.GroundSpeed, dp, sp)
	mergeOpt(&m.Speed3D, &other.Speed3D, dp, sp)
	mergeOpt(&m.Course, &other.Course, dp, sp)
}

// Merge incorporates fields from other into m based on priority.
func (m *VelECEFMsg) Merge(other *VelECEFMsg) {
	if other.Priority >= m.Priority {
		m.Vel = other.Vel
		m.Priority = other.Priority
		m.Tag = other.Tag
		m.NativeMsgID = other.NativeMsgID
	}
}

// NavEpochAccum accumulates the best message of each kind within a
// navigation epoch. It implements MsgHandler for the Pos/Vel/Time
// methods, merging incoming messages into its Bundle by priority.
// NavEpoch clears the accumulated Bundle.
type NavEpochAccum struct {
	DefaultHandler
	Bundle MsgBundle
}

func (a *NavEpochAccum) Time(msg *TimeMsg, tRead time.Time) {
	if a.Bundle.Time == nil {
		a.Bundle.Time = msg
		return
	}
	a.Bundle.Time.Merge(msg)
}

func (a *NavEpochAccum) PosGeo(msg *PosGeoMsg, tRead time.Time) {
	if a.Bundle.PosGeo == nil {
		a.Bundle.PosGeo = msg
		return
	}
	a.Bundle.PosGeo.Merge(msg)
}

func (a *NavEpochAccum) PosECEF(msg *PosECEFMsg, tRead time.Time) {
	if a.Bundle.PosECEF == nil {
		a.Bundle.PosECEF = msg
		return
	}
	a.Bundle.PosECEF.Merge(msg)
}

func (a *NavEpochAccum) VelGeo(msg *VelGeoMsg, tRead time.Time) {
	if a.Bundle.VelGeo == nil {
		a.Bundle.VelGeo = msg
		return
	}
	a.Bundle.VelGeo.Merge(msg)
}

func (a *NavEpochAccum) VelECEF(msg *VelECEFMsg, tRead time.Time) {
	if a.Bundle.VelECEF == nil {
		a.Bundle.VelECEF = msg
		return
	}
	a.Bundle.VelECEF.Merge(msg)
}

// NavEpoch clears the accumulated Bundle, preparing for the next epoch.
func (a *NavEpochAccum) NavEpoch(msg *NavEpochMsg, tRead time.Time) {
	a.Bundle = MsgBundle{}
}
