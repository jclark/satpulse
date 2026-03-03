package gpsprot

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

// ErrNotHandled is returned by ExtSentenceHandler.HandleSentence when
// the handler does not recognize the sentence.
var ErrNotHandled = errors.New("sentence not handled")

// Msg is implemented by all protocol-agnostic message types.
type Msg interface {
	Dispatch(MsgHandler, time.Time)
}

// PVMsg is implemented by position/velocity message types
// that participate in per-epoch accumulation with priority-based merging.
type PVMsg interface {
	Msg
	SetPriority(MsgPriority)
}

func (m *PosGeoMsg) Dispatch(h MsgHandler, t time.Time)     { h.PosGeo(m, t) }
func (m *PosECEFMsg) Dispatch(h MsgHandler, t time.Time)    { h.PosECEF(m, t) }
func (m *VelGeoMsg) Dispatch(h MsgHandler, t time.Time)     { h.VelGeo(m, t) }
func (m *VelECEFMsg) Dispatch(h MsgHandler, t time.Time)    { h.VelECEF(m, t) }
func (m *TimeMsg) Dispatch(h MsgHandler, t time.Time)       { h.Time(m, t) }
func (m *LeapSecondMsg) Dispatch(h MsgHandler, t time.Time) { h.LeapSecond(m, t) }
func (m *SurveyMsg) Dispatch(h MsgHandler, t time.Time)     { h.Survey(m, t) }
func (m *SatellitesMsg) Dispatch(h MsgHandler, t time.Time) { h.Satellites(m, t) }

// SetPriority implements PVMsg for the four position/velocity types.
func (m *PosGeoMsg) SetPriority(pri MsgPriority)  { m.Priority = pri }
func (m *PosECEFMsg) SetPriority(pri MsgPriority) { m.Priority = pri }
func (m *VelGeoMsg) SetPriority(pri MsgPriority)  { m.Priority = pri }
func (m *VelECEFMsg) SetPriority(pri MsgPriority) { m.Priority = pri }

// DispatchMsgs dispatches each message to the handler.
func DispatchMsgs(msgs []Msg, h MsgHandler, tRead time.Time) {
	for _, m := range msgs {
		m.Dispatch(h, tRead)
	}
}

// SetMsgsPriority sets priority on all PVMsg messages in the slice.
func SetMsgsPriority(msgs []Msg, pri MsgPriority) {
	for _, m := range msgs {
		if pv, ok := m.(PVMsg); ok {
			pv.SetPriority(pri)
		}
	}
}

// MsgPriority indicates the trustworthiness of a message's fields
// for priority-based merging within a navigation epoch.
type MsgPriority uint8

const (
	PriGenericLow  MsgPriority = iota + 1 // NMEA, basic sentence
	PriGenericHigh                        // NMEA, richer sentence (e.g. GGA over RMC)
	PriVendorLow                          // vendor binary, standard message
	PriVendorHigh                         // vendor binary, high-precision message
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
	NativeMsg(tag Tag, msgID string, msg any, tRead time.Time) error
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

// PVMsgBundle holds the accumulated position/velocity messages
// for a single navigation epoch.
type PVMsgBundle struct {
	PosGeo  opt.Val[PosGeoMsg]  `json:"posGeo,omitzero"`
	PosECEF opt.Val[PosECEFMsg] `json:"posECEF,omitzero"`
	VelGeo  opt.Val[VelGeoMsg]  `json:"velGeo,omitzero"`
	VelECEF opt.Val[VelECEFMsg] `json:"velECEF,omitzero"`
}

// FillDerived fills in missing fields using cross-frame derivation.
func (b *PVMsgBundle) FillDerived() {
	var lat, lon float64
	havePos := false
	if pg := b.PosGeo.Ptr(); pg != nil {
		lat = pg.LatLon[0].Degrees()
		lon = pg.LatLon[1].Degrees()
		havePos = true
		if !b.PosECEF.IsSet() && pg.Height.IsSet() {
			ecef := geopos.WGS84.LLHtoECEF(geopos.LLH{
				Lat: lat, Lon: lon, Height: pg.Height.Get().Meters(),
			})
			b.PosECEF.Set(PosECEFMsg{
				Pos:         Point3D{Meters(ecef[0]), Meters(ecef[1]), Meters(ecef[2])},
				Priority:    pg.Priority,
				Tag:         pg.Tag,
				NativeMsgID: pg.NativeMsgID,
			})
		}
	}
	if pe := b.PosECEF.Ptr(); pe != nil && !b.PosGeo.IsSet() {
		ecef := geopos.ECEF{pe.Pos[0].Meters(), pe.Pos[1].Meters(), pe.Pos[2].Meters()}
		llh, err := geopos.WGS84.ECEFtoLLH(ecef)
		if err == nil {
			lat = llh.Lat
			lon = llh.Lon
			havePos = true
			b.PosGeo.Set(PosGeoMsg{
				LatLon:      [2]Angle{DegreesFromFloat(llh.Lat), DegreesFromFloat(llh.Lon)},
				Height:      opt.Make(Meters(llh.Height)),
				Priority:    pe.Priority,
				Tag:         pe.Tag,
				NativeMsgID: pe.NativeMsgID,
			})
		}
	}
	if vg := b.VelGeo.Ptr(); vg != nil {
		if ned := vg.VelNED.Ptr(); ned != nil {
			if !b.VelECEF.IsSet() && havePos {
				ve := geopos.WGS84.NEDtoECEF(geopos.VelNED{
					ned[0].MetersPerSecond(), ned[1].MetersPerSecond(), ned[2].MetersPerSecond(),
				}, lat, lon)
				b.VelECEF.Set(VelECEFMsg{
					Vel:         [3]Speed{MetersPerSecondFromFloat(ve[0]), MetersPerSecondFromFloat(ve[1]), MetersPerSecondFromFloat(ve[2])},
					Priority:    vg.Priority,
					Tag:         vg.Tag,
					NativeMsgID: vg.NativeMsgID,
				})
			}
			if !vg.Speed3D.IsSet() {
				n, e, d := ned[0].MetersPerSecond(), ned[1].MetersPerSecond(), ned[2].MetersPerSecond()
				vg.Speed3D.Set(MetersPerSecondFromFloat(math.Sqrt(n*n + e*e + d*d)))
			}
		}
	}
	if ve := b.VelECEF.Ptr(); ve != nil && havePos {
		nedV := geopos.WGS84.ECEFtoNED(geopos.VelECEF{
			ve.Vel[0].MetersPerSecond(), ve.Vel[1].MetersPerSecond(), ve.Vel[2].MetersPerSecond(),
		}, lat, lon)
		nedS := [3]Speed{MetersPerSecondFromFloat(nedV[0]), MetersPerSecondFromFloat(nedV[1]), MetersPerSecondFromFloat(nedV[2])}
		n, e, d := nedV[0], nedV[1], nedV[2]
		spd3D := MetersPerSecondFromFloat(math.Sqrt(n*n + e*e + d*d))
		if vg := b.VelGeo.Ptr(); vg != nil {
			// VelGeo exists (e.g. GroundSpeed/Course from NAV) but
			// may be missing VelNED/Speed3D -- fill from VelECEF.
			if !vg.VelNED.IsSet() {
				vg.VelNED.Set(nedS)
			}
			if !vg.Speed3D.IsSet() {
				vg.Speed3D.Set(spd3D)
			}
		} else {
			vg := VelGeoMsg{
				Priority:    ve.Priority,
				Tag:         ve.Tag,
				NativeMsgID: ve.NativeMsgID,
			}
			vg.VelNED.Set(nedS)
			vg.Speed3D.Set(spd3D)
			b.VelGeo.Set(vg)
		}
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
	ObsTime    Duration `json:"obsTime"`
	Valid      bool          `json:"valid"`
	InProgress bool          `json:"inProgress"`
}

// FixLevel is the primary ordered axis describing the technique used to compute
// the GNSS navigation solution, ordered by increasing quality. Higher values
// represent higher intrinsic precision. The zero value means "not provided"
// or "not applicable".
type FixLevel uint8

const (
	// FixLevelNone indicates that no valid GNSS solution is available.
	FixLevelNone FixLevel = iota + 1

	// FixLevelNotMeasured indicates that the receiver reports a position that
	// is not based on any measurement (e.g. manual input or simulated). No
	// component of the PVT solution is being computed from observations.
	// CorrKind and FixDim do not apply.
	FixLevelNotMeasured

	// FixLevelCode indicates an uncorrected code-based GNSS solution
	// (e.g. standalone SPS or single point positioning).
	FixLevelCode

	// FixLevelCodeCorrected indicates a code-based GNSS solution with
	// corrections applied (e.g. DGPS or SBAS). This improves accuracy but
	// remains limited by code measurement precision.
	FixLevelCodeCorrected

	// FixLevelCarrierFloat indicates a carrier-phase-based solution with
	// ambiguities estimated as float (non-integer) values. This includes
	// RTK float and classical PPP solutions prior to ambiguity fixing.
	FixLevelCarrierFloat

	// FixLevelCarrierFixed indicates a carrier-phase-based solution with
	// integer ambiguities resolved and constrained. This includes RTK fixed
	// and PPP-AR/PPP-RTK solutions.
	FixLevelCarrierFixed
)

var fixLevelName = [...]string{
	FixLevelNone:          "none",
	FixLevelNotMeasured:   "notMeasured",
	FixLevelCode:          "code",
	FixLevelCodeCorrected: "codeCorrected",
	FixLevelCarrierFloat:  "carrierFloat",
	FixLevelCarrierFixed:  "carrierFixed",
}

var fixLevelFromName = func() map[string]FixLevel {
	m := make(map[string]FixLevel, len(fixLevelName))
	for i, name := range fixLevelName {
		if name != "" {
			m[name] = FixLevel(i)
		}
	}
	return m
}()

func (f FixLevel) String() string {
	if int(f) < len(fixLevelName) && fixLevelName[f] != "" {
		return fixLevelName[f]
	}
	return fmt.Sprintf("FixLevel(%d)", f)
}

func (f FixLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

func (f FixLevel) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

func (f *FixLevel) UnmarshalText(text []byte) error {
	v, ok := fixLevelFromName[string(text)]
	if !ok {
		return fmt.Errorf("unknown FixLevel %q", text)
	}
	*f = v
	return nil
}

// FixDim describes the dimensionality of the GNSS navigation solution. This is
// primarily about position dimensionality (2D/3D), but includes "time-only" and
// "velocity-only" cases for receivers that report those explicitly.
// The zero value means "not provided" or "not applicable".
type FixDim uint8

const (
	// FixDim2D indicates a two-dimensional solution where height is fixed
	// or constrained and only horizontal position is solved.
	FixDim2D FixDim = iota + 1

	// FixDim3D indicates a full three-dimensional solution where
	// position and clock bias are fully estimated.
	FixDim3D

	// FixDimTimeOnly indicates a solution where only time (and possibly
	// clock bias) is solved, without a valid position.
	FixDimTimeOnly

	// FixDimVelocityOnly indicates a solution where only velocity is solved
	// (e.g. from Doppler), without a valid position or time.
	FixDimVelocityOnly
)

var fixDimName = [...]string{
	FixDim2D:           "2D",
	FixDim3D:           "3D",
	FixDimTimeOnly:     "timeOnly",
	FixDimVelocityOnly: "velocityOnly",
}

var fixDimFromName = func() map[string]FixDim {
	m := make(map[string]FixDim, len(fixDimName))
	for i, name := range fixDimName {
		if name != "" {
			m[name] = FixDim(i)
		}
	}
	return m
}()

func (d FixDim) String() string {
	if int(d) < len(fixDimName) && fixDimName[d] != "" {
		return fixDimName[d]
	}
	return fmt.Sprintf("FixDim(%d)", d)
}

func (d FixDim) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d FixDim) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *FixDim) UnmarshalText(text []byte) error {
	v, ok := fixDimFromName[string(text)]
	if !ok {
		return fmt.Errorf("unknown FixDim %q", text)
	}
	*d = v
	return nil
}

// CorrKind is a bitmask of assertions about corrections applied in the navigation
// solution. The zero value means "no correction assertions".
//
// The Corr* bits are not necessarily orthogonal; they are related by a partial
// order. When you assert a more specific fact, you must also assert the more
// general facts it implies.
//
// Partial order definition: A <= B means asserting A implies asserting B.
//
// Ordering (immediate implications):
//
//	CorrBaseStation <= CorrUsed
//	CorrWideArea <= CorrUsed
//	CorrRTCM <= CorrUsed
//	CorrPartialDualFreq <= CorrBaseStation
//	CorrFullDualFreq <= CorrPartialDualFreq
//	CorrSBAS <= CorrWideArea
//	CorrCLAS <= CorrWideArea
//	CorrSPARTN <= CorrWideArea
//	CorrPPP <= CorrWideArea
//	CorrPPPRTK <= CorrPPP
//	CorrPPPConverging <= CorrPPP
//	CorrPPPConverged <= CorrPPP
//	CorrPPPHAS <= CorrPPP
//	CorrPPPMDC <= CorrPPP
//	CorrPPPB2b <= CorrPPP
type CorrKind uint16

const (
	// CorrUsed asserts that external corrections are applied.
	CorrUsed CorrKind = 1 << iota

	// CorrBaseStation asserts that corrections are base/network referenced.
	// This corresponds to OSR (Observation-State Representation / observation-space)
	// corrections such as RTK/network RTK.
	// CorrBaseStation <= CorrUsed.
	CorrBaseStation

	// CorrWideArea asserts that corrections are wide-area/broadcast/service
	// corrections (not tied to a local base station).
	// This corresponds to SSR (State-Space Representation / state-space) corrections
	// such as SBAS/PPP/CLAS/SPARTN and RTCM-SSR streams.
	// CorrWideArea <= CorrUsed.
	CorrWideArea

	// CorrRTCM asserts that corrections being applied are delivered/packaged as RTCM.
	// CorrRTCM <= CorrUsed.
	CorrRTCM

	// CorrPartialDualFreq asserts that a base-station solution uses a dual-frequency
	// ambiguity strategy that partially exploits multiple frequencies (e.g. wide-lane
	// techniques) rather than fully resolving the full dual-frequency ambiguity set.
	// CorrPartialDualFreq <= CorrBaseStation.
	CorrPartialDualFreq

	// CorrFullDualFreq asserts that a base-station solution uses a dual-frequency
	// ambiguity strategy that fully exploits multiple frequencies (e.g. narrow-lane
	// or ionosphere-free dual-frequency models, as opposed to wide-lane-only strategies).
	// CorrFullDualFreq <= CorrPartialDualFreq.
	CorrFullDualFreq

	// CorrSBAS asserts that the wide-area corrections being applied are SBAS.
	// CorrSBAS <= CorrWideArea.
	CorrSBAS

	// CorrCLAS asserts that the wide-area corrections being applied are CLAS.
	// CorrCLAS <= CorrWideArea.
	CorrCLAS

	// CorrSPARTN asserts that the wide-area corrections being applied are SPARTN.
	// CorrSPARTN <= CorrWideArea.
	CorrSPARTN

	// CorrPPP asserts that the wide-area corrections being applied are PPP-class
	// (standalone absolute solution regime with convergence).
	// CorrPPP <= CorrWideArea.
	CorrPPP

	// CorrPPPRTK asserts that the PPP corrections being applied are PPP-RTK/SSR-RTK
	// class (PPP with additional information enabling rapid ambiguity resolution).
	// CorrPPPRTK <= CorrPPP.
	CorrPPPRTK

	// CorrPPPConverging asserts that the PPP solution is converging.
	// CorrPPPConverging <= CorrPPP.
	CorrPPPConverging

	// CorrPPPConverged asserts that the PPP solution is converged.
	// CorrPPPConverged <= CorrPPP.
	CorrPPPConverged

	// CorrPPPHAS asserts that the PPP corrections are Galileo HAS.
	// CorrPPPHAS <= CorrPPP.
	CorrPPPHAS

	// CorrPPPMDC asserts that the PPP corrections are QZSS MADOCA.
	// CorrPPPMDC <= CorrPPP.
	CorrPPPMDC

	// CorrPPPB2b asserts that the PPP corrections are BeiDou B2b.
	// CorrPPPB2b <= CorrPPP.
	CorrPPPB2b
)

type corrKindBit struct {
	mask    CorrKind
	name    string
	implies CorrKind // immediate parent in the partial order
}

// corrKindBits defines the bits in declaration order, with their immediate
// parent in the partial order. Expand computes the transitive closure.
var corrKindBits = [...]corrKindBit{
	{CorrUsed, "used", 0},
	{CorrBaseStation, "baseStation", CorrUsed},
	{CorrWideArea, "wideArea", CorrUsed},
	{CorrRTCM, "RTCM", CorrUsed},
	{CorrPartialDualFreq, "partialDualFreq", CorrBaseStation},
	{CorrFullDualFreq, "fullDualFreq", CorrPartialDualFreq},
	{CorrSBAS, "SBAS", CorrWideArea},
	{CorrCLAS, "CLAS", CorrWideArea},
	{CorrSPARTN, "SPARTN", CorrWideArea},
	{CorrPPP, "PPP", CorrWideArea},
	{CorrPPPRTK, "PPP-RTK", CorrPPP},
	{CorrPPPConverging, "PPPConverging", CorrPPP},
	{CorrPPPConverged, "PPPConverged", CorrPPP},
	{CorrPPPHAS, "PPP-HAS", CorrPPP},
	{CorrPPPMDC, "PPP-MDC", CorrPPP},
	{CorrPPPB2b, "PPP-B2b", CorrPPP},
}

var corrKindFromName = func() map[string]CorrKind {
	m := make(map[string]CorrKind, len(corrKindBits))
	for _, b := range corrKindBits {
		m[b.name] = b.mask
	}
	return m
}()

// corrKindClosure maps each single bit to the full set of bits it implies
// (including itself). Computed once at init.
var corrKindClosure = func() [16]CorrKind {
	var cl [16]CorrKind
	for _, b := range corrKindBits {
		bit := bitIndex(b.mask)
		cl[bit] = b.mask
		if b.implies != 0 {
			// Add the transitive closure of the parent.
			// This works because corrKindBits is in declaration order
			// and parents are declared before children.
			cl[bit] |= cl[bitIndex(b.implies)]
		}
	}
	return cl
}()

func bitIndex(mask CorrKind) int {
	for i := 0; i < 16; i++ {
		if mask == 1<<i {
			return i
		}
	}
	panic("not a single bit")
}

// Close returns c with all implied bits set according to the partial order.
// For example, CorrSBAS.Expand() returns CorrSBAS|CorrWideArea|CorrUsed.
func (c CorrKind) Expand() CorrKind {
	result := c
	for v := c; v != 0; v &= v - 1 {
		bit := v & -v // lowest set bit
		result |= corrKindClosure[bitIndex(bit)]
	}
	return result
}

// items returns the names of the set bits that are not implied by any other
// set bit. This gives the minimal representation.
func (c CorrKind) items() []string {
	var items []string
	for _, b := range corrKindBits {
		if c&b.mask == 0 {
			continue
		}
		// Omit this bit if it is implied by some other set bit.
		if c & ^b.mask != 0 && (c & ^b.mask).Expand()&b.mask != 0 {
			continue
		}
		items = append(items, b.name)
	}
	return items
}

func (c CorrKind) String() string {
	items := c.items()
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ",")
}

func (c CorrKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.items())
}

func (c *CorrKind) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	var result CorrKind
	for _, name := range names {
		bit, ok := corrKindFromName[name]
		if !ok {
			return fmt.Errorf("unknown CorrKind %q", name)
		}
		result |= bit
	}
	*c = result.Expand()
	return nil
}

// AuxSrc is a bitmask of additional data sources that contributed to the
// navigation solution. GNSS contribution is implicit when FixLevel indicates
// a GNSS-based fix.
type AuxSrc uint8

const (
	AuxSrcDR  AuxSrc = 1 << iota // dead reckoning (e.g. wheel ticks, motion model)
	AuxSrcINS                    // inertial navigation system
)

type auxSrcBit struct {
	mask AuxSrc
	name string
}

var auxSrcBits = [...]auxSrcBit{
	{AuxSrcDR, "DR"},
	{AuxSrcINS, "INS"},
}

// Items returns the names of the set bits as a string slice.
func (a AuxSrc) Items() []string {
	var items []string
	for _, b := range auxSrcBits {
		if a&b.mask != 0 {
			items = append(items, b.name)
		}
	}
	return items
}

func (a AuxSrc) String() string {
	items := a.Items()
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ",")
}

func (a AuxSrc) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.Items())
}

// DOP holds dilution of precision values for the navigation solution. Fields
// are opt.Val because different protocols provide different subsets. DOP may
// be synthesized from multiple messages within an epoch (e.g. UBX-NAV-DOP
// provides all five, while NMEA GSA provides only PDOP/HDOP/VDOP).
type DOP struct {
	Geom opt.Val[float64] `json:"geom,omitzero"` // geometric DOP
	Pos  opt.Val[float64] `json:"pos,omitzero"`  // position (3D) DOP
	Hor  opt.Val[float64] `json:"hor,omitzero"`  // horizontal DOP
	Vert opt.Val[float64] `json:"vert,omitzero"` // vertical DOP
	Time opt.Val[float64] `json:"time,omitzero"` // time DOP
}

// Fill sets any unset fields in d from the corresponding fields in other.
func (d *DOP) Fill(other *DOP) {
	d.Geom.Fill(other.Geom)
	d.Pos.Fill(other.Pos)
	d.Hor.Fill(other.Hor)
	d.Vert.Fill(other.Vert)
	d.Time.Fill(other.Time)
}

// NavEpochMsg is emitted once at the end of each navigation epoch, after
// all time/position/velocity messages for that epoch have been dispatched.
// GNSS is the implicit baseline source when FixLevel indicates a GNSS-based
// fix. AuxSrc captures additional sources (e.g. DR/INS).
// The zero value of each field means "not provided" or "not applicable".
// CorrKind is a bitmask (not an enum) and its bits are related by a partial
// order (see CorrKind docs).
type NavEpochMsg struct {
	// FixLevel is the primary technique used to compute the GNSS solution,
	// ordered by increasing quality (None < Code < CodeCorrected <
	// CarrierFloat < CarrierFixed).
	FixLevel FixLevel `json:"fixLevel,omitzero"`
	// FixDim is the dimensionality of the solution (2D, 3D, time-only,
	// velocity-only).
	FixDim FixDim `json:"fixDim,omitzero"`
	// Correction is a bitmask of assertions about corrections applied.
	// Meaningful when FixLevel >= FixLevelCodeCorrected.
	Correction CorrKind `json:"correction,omitzero"`
	// AuxSrc is a bitmask of additional sources (DR, INS) that contributed
	// to the solution. GNSS is implicit when FixLevel indicates a fix.
	AuxSrc AuxSrc `json:"auxSrc,omitzero"`
	// Acc holds estimated position, velocity, and course accuracy.
	Acc Accuracy `json:"acc,omitzero"`
	// DOP holds dilution of precision values (GDOP, PDOP, HDOP, VDOP, TDOP).
	DOP DOP `json:"dop,omitzero"`
	// DiffAge is the age of the differential corrections applied to the
	// current solution. Unset when no corrections are in use or the
	// protocol doesn't report it.
	DiffAge opt.Val[Duration] `json:"diffAge,omitzero"`
	// RTCMRefBaseID is the RTCM reference station ID (DF003, 0-4095) of
	// the base station whose corrections are applied to this solution.
	// Distinct from the RTCMBaseID config property, which is this
	// receiver's own base ID for RTCM output.
	RTCMRefBaseID opt.Val[uint16] `json:"rtcmRefBaseID,omitzero"`
	// NumSVUsed is the number of satellites used in the solution.
	NumSVUsed opt.Val[uint16] `json:"numSVUsed,omitzero"`
	// NumSVTracked is the number of satellites tracked by the receiver.
	NumSVTracked opt.Val[uint16] `json:"numSVTracked,omitzero"`
	// NumSVInView is the number of satellites in view of the receiver.
	NumSVInView opt.Val[uint16] `json:"numSVInView,omitzero"`
	// SignalsUsed is the set of GNSS signals used in the solution.
	SignalsUsed SignalSet `json:"signalsUsed,omitzero"`
	// Tag identifies the protocol source (e.g. UBX, NMEA, Unicore).
	Tag Tag `json:"tag,omitzero"`
	// StartTime is when the first message in this epoch was read.
	StartTime time.Time `json:"startTime"`
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

// Fill sets any unset fields in a from the corresponding fields in other.
func (a *Accuracy) Fill(other *Accuracy) {
	a.Pos.Fill(other.Pos)
	a.Hor.Fill(other.Hor)
	a.Vert.Fill(other.Vert)
	a.Speed.Fill(other.Speed)
	a.GroundSpeed.Fill(other.GroundSpeed)
	a.Course.Fill(other.Course)
}

// MergeNavEpoch merges multiple NavEpochMsg values. Priority is implicit in
// argument order: the first non-nil message wins for scalar fields (Tag,
// FixLevel, FixDim); optional fields use fill-if-unset semantics; bitmask
// fields (Correction, AuxSrc, SignalsUsed) are unioned; StartTime is the
// earliest. The first non-nil argument is mutated and returned.
func MergeNavEpoch(msgs ...*NavEpochMsg) *NavEpochMsg {
	var dst *NavEpochMsg
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if dst == nil {
			dst = m
			continue
		}
		if dst.Tag == "" {
			dst.Tag = m.Tag
		}
		if dst.FixLevel == 0 {
			dst.FixLevel = m.FixLevel
		}
		if dst.FixDim == 0 {
			dst.FixDim = m.FixDim
		}
		dst.Acc.Fill(&m.Acc)
		dst.DOP.Fill(&m.DOP)
		dst.DiffAge.Fill(m.DiffAge)
		dst.RTCMRefBaseID.Fill(m.RTCMRefBaseID)
		dst.NumSVUsed.Fill(m.NumSVUsed)
		dst.NumSVTracked.Fill(m.NumSVTracked)
		dst.NumSVInView.Fill(m.NumSVInView)
		dst.Correction |= m.Correction
		dst.AuxSrc |= m.AuxSrc
		dst.SignalsUsed |= m.SignalsUsed
		if !m.StartTime.IsZero() && (dst.StartTime.IsZero() || m.StartTime.Before(dst.StartTime)) {
			dst.StartTime = m.StartTime
		}
	}
	return dst
}

// EpochFlusher is implemented by PacketProcessors that participate in
// epoch coordination. The manager calls FlushNavEpoch when an epoch
// boundary is detected. The processor returns its accumulated NavEpochMsg
// (or nil if it has nothing to contribute), a MsgPriority indicating
// the protocol band, and its MsgHandler for emission.
type EpochFlusher interface {
	FlushNavEpoch(tRead time.Time) (*NavEpochMsg, MsgPriority, MsgHandler)
}

// NavEpochManager coordinates navigation epoch handling across multiple
// protocol processors. Each PacketProcessor receives a reference to the
// shared manager in its constructor instead of directly calling
// MsgHandler.NavEpoch.
type NavEpochManager struct {
	active map[EpochFlusher]struct{}
}

// NewNavEpochManager creates a new NavEpochManager.
func NewNavEpochManager() *NavEpochManager {
	return &NavEpochManager{}
}

// EpochStarted is called by a processor when it detects the start of a new
// epoch. If the caller is already in the active set, the manager flushes
// all active processors and emits the merged NavEpochMsg. The caller is
// then added to the (possibly cleared) active set.
func (m *NavEpochManager) EpochStarted(f EpochFlusher, tRead time.Time) {
	if _, ok := m.active[f]; ok {
		m.flush(tRead)
	}
	if m.active == nil {
		m.active = make(map[EpochFlusher]struct{})
	}
	m.active[f] = struct{}{}
}

// EndOfEpoch is called by a processor that received an explicit end-of-epoch
// signal (e.g. UBX NAV-EOE, Quectel PQTMEOE). These messages mark the end
// of the epoch for all protocols on the receiver, so all active processors
// are flushed unconditionally.
func (m *NavEpochManager) EndOfEpoch(tRead time.Time) {
	m.flush(tRead)
}

func (m *NavEpochManager) flush(tRead time.Time) {
	type flushed struct {
		msg *NavEpochMsg
		pri MsgPriority
		mh  MsgHandler
	}
	var items []flushed
	for f := range m.active {
		msg, pri, h := f.FlushNavEpoch(tRead)
		if msg != nil {
			items = append(items, flushed{msg, pri, h})
		}
	}
	clear(m.active)
	if len(items) == 0 {
		return
	}
	slices.SortFunc(items, func(a, b flushed) int {
		return int(b.pri) - int(a.pri)
	})
	msgs := make([]*NavEpochMsg, len(items))
	for i, it := range items {
		msgs[i] = it.msg
	}
	merged := MergeNavEpoch(msgs...)
	if merged == nil {
		return
	}
	for _, it := range items {
		if mh := it.mh; mh != nil {
			mh.NavEpoch(merged, tRead)
			return
		}
	}
}

// Merge incorporates fields from other into m based on priority.
// Higher priority overwrites everything. Equal priority fills only
// missing optional fields (first-wins). Lower priority does nothing.
func (m *PosGeoMsg) Merge(other *PosGeoMsg) {
	if other.Priority < m.Priority {
		return
	}
	if other.Priority > m.Priority {
		*m = *other
		return
	}
	m.Height.Fill(other.Height)
	m.HeightMSL.Fill(other.HeightMSL)
}

// Merge incorporates fields from other into m based on priority.
func (m *PosECEFMsg) Merge(other *PosECEFMsg) {
	if other.Priority > m.Priority {
		*m = *other
	}
}

// Merge incorporates fields from other into m based on priority.
func (m *VelGeoMsg) Merge(other *VelGeoMsg) {
	if other.Priority < m.Priority {
		return
	}
	if other.Priority > m.Priority {
		*m = *other
		return
	}
	m.VelNED.Fill(other.VelNED)
	m.GroundSpeed.Fill(other.GroundSpeed)
	m.Speed3D.Fill(other.Speed3D)
	m.Course.Fill(other.Course)
}

// Merge incorporates fields from other into m based on priority.
func (m *VelECEFMsg) Merge(other *VelECEFMsg) {
	if other.Priority > m.Priority {
		*m = *other
	}
}

// PVMsgAccum accumulates the best position/velocity message of each kind
// within a navigation epoch. It implements MsgHandler for the four PV
// methods, merging incoming messages by priority. NavEpoch clears the
// accumulated bundle.
type PVMsgAccum struct {
	DefaultHandler
	PVMsgBundle
}

func (a *PVMsgAccum) PosGeo(msg *PosGeoMsg, _ time.Time) {
	if p := a.PVMsgBundle.PosGeo.Ptr(); p != nil {
		p.Merge(msg)
	} else {
		a.PVMsgBundle.PosGeo.Set(*msg)
	}
}

func (a *PVMsgAccum) PosECEF(msg *PosECEFMsg, _ time.Time) {
	if p := a.PVMsgBundle.PosECEF.Ptr(); p != nil {
		p.Merge(msg)
	} else {
		a.PVMsgBundle.PosECEF.Set(*msg)
	}
}

func (a *PVMsgAccum) VelGeo(msg *VelGeoMsg, _ time.Time) {
	if p := a.PVMsgBundle.VelGeo.Ptr(); p != nil {
		p.Merge(msg)
	} else {
		a.PVMsgBundle.VelGeo.Set(*msg)
	}
}

func (a *PVMsgAccum) VelECEF(msg *VelECEFMsg, _ time.Time) {
	if p := a.PVMsgBundle.VelECEF.Ptr(); p != nil {
		p.Merge(msg)
	} else {
		a.PVMsgBundle.VelECEF.Set(*msg)
	}
}

// NavEpoch clears the accumulated bundle, preparing for the next epoch.
func (a *PVMsgAccum) NavEpoch(_ *NavEpochMsg, _ time.Time) {
	a.PVMsgBundle = PVMsgBundle{}
}

// TimeTicker produces one filled-in TimeMsg per navigation epoch.
// It stores the first non-PrePulse TimeMsg each epoch, fills in
// derived fields (TAITime, UTCTime, UTCOffset), and forwards the
// result immediately to a downstream MsgHandler. NavEpoch resets
// the state for the next epoch.
type TimeTicker struct {
	DefaultHandler
	h    MsgHandler
	time opt.Val[TimeMsg]
	ls   ptime.LeapSecond
}

// NewTimeTicker creates a TimeTicker that forwards filled TimeMsgs to h.
func NewTimeTicker(h MsgHandler, ls ptime.LeapSecond) *TimeTicker {
	return &TimeTicker{h: h, ls: ls}
}

// SetLeapSecond replaces the stored leap second.
func (t *TimeTicker) SetLeapSecond(ls ptime.LeapSecond) {
	t.ls = ls
}

// LeapSecond updates the stored leap second.
func (t *TimeTicker) LeapSecond(msg *LeapSecondMsg, _ time.Time) {
	msg.UpdateLeapSecond(&t.ls)
}

// Time filters, fills, and forwards one TimeMsg per epoch.
func (t *TimeTicker) Time(msg *TimeMsg, tRead time.Time) {
	if msg.Ref == PrePulse {
		return
	}
	if t.time.IsSet() {
		return
	}
	m := *msg
	if m.UTCTime != nil {
		ut := *m.UTCTime
		m.UTCTime = &ut
	}
	t.fill(&m)
	t.time.Set(m)
	t.h.Time(&m, tRead)
}

// NavEpoch clears the stored TimeMsg, preparing for the next epoch.
func (t *TimeTicker) NavEpoch(_ *NavEpochMsg, _ time.Time) {
	t.time = opt.Val[TimeMsg]{}
}

func (t *TimeTicker) fill(m *TimeMsg) {
	if !m.TAITime.IsZero() {
		m.TAITime = m.TAITime.Round(time.Millisecond)
	}
	if m.UTCTime != nil {
		m.UTCTime.TimeOfDay = m.UTCTime.TimeOfDay.Round(time.Millisecond)
	}
	if m.TAITime.IsZero() && m.UTCTime != nil {
		m.TAITime = t.ls.UTCtoTime(*m.UTCTime)
	}
	if m.UTCTime == nil && !m.TAITime.IsZero() {
		ut := t.ls.TimeToUTC(m.TAITime)
		m.UTCTime = &ut
	}
	if m.UTCOffset == 0 && !m.TAITime.IsZero() {
		state := t.ls.StateAt(m.TAITime)
		if state.UTCOffset > 0 {
			m.UTCOffset = uint8(state.UTCOffset)
		}
	}
}
