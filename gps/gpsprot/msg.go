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

func (a AuxSrc) items() []string {
	var items []string
	for _, b := range auxSrcBits {
		if a&b.mask != 0 {
			items = append(items, b.name)
		}
	}
	return items
}

func (a AuxSrc) String() string {
	items := a.items()
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ",")
}

func (a AuxSrc) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.items())
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

// Merge incorporates fields from other into d based on priority.
func (d *DOP) Merge(other *DOP, dstPri, srcPri MsgPriority) {
	mergeOpt(&d.Geom, &other.Geom, dstPri, srcPri)
	mergeOpt(&d.Pos, &other.Pos, dstPri, srcPri)
	mergeOpt(&d.Hor, &other.Hor, dstPri, srcPri)
	mergeOpt(&d.Vert, &other.Vert, dstPri, srcPri)
	mergeOpt(&d.Time, &other.Time, dstPri, srcPri)
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
	DiffAge opt.Val[time.Duration] `json:"diffAge,omitzero"`
	// RTCMRefBaseID is the RTCM reference station ID (DF003, 0-4095) of
	// the base station whose corrections are applied to this solution.
	// Distinct from the RTCMBaseID config property, which is this
	// receiver's own base ID for RTCM output.
	RTCMRefBaseID opt.Val[uint16] `json:"rtcmRefBaseID,omitzero"`
	// NumSVUsed is the number of satellites used in the solution.
	NumSVUsed opt.Val[uint16] `json:"numSVUsed,omitzero"`
	// NumSVTracked is the number of satellites tracked by the receiver.
	NumSVTracked opt.Val[uint16] `json:"numSVTracked,omitzero"`
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

// Merge incorporates fields from other into a based on priority.
func (a *Accuracy) Merge(other *Accuracy, dstPri, srcPri MsgPriority) {
	mergeOpt(&a.Pos, &other.Pos, dstPri, srcPri)
	mergeOpt(&a.Hor, &other.Hor, dstPri, srcPri)
	mergeOpt(&a.Vert, &other.Vert, dstPri, srcPri)
	mergeOpt(&a.Speed, &other.Speed, dstPri, srcPri)
	mergeOpt(&a.GroundSpeed, &other.GroundSpeed, dstPri, srcPri)
	mergeOpt(&a.Course, &other.Course, dstPri, srcPri)
}

// MergeNavEpoch merges two NavEpochMsg values by priority. The higher-priority
// message provides Tag and scalar quality fields (FixLevel, FixDim);
// Accuracy and DOP fields are merged with mergeOpt semantics; bitmask fields
// (Correction, AuxSrc, SignalsUsed) are unioned; StartTime is the earliest.
// The returned priority is the higher of the two, for chaining in pairwise merge.
func MergeNavEpoch(a *NavEpochMsg, aPri MsgPriority, b *NavEpochMsg, bPri MsgPriority) (*NavEpochMsg, MsgPriority) {
	if a == nil {
		return b, bPri
	}
	if b == nil {
		return a, aPri
	}
	merged := *a
	merged.Acc.Merge(&b.Acc, aPri, bPri)
	merged.DOP.Merge(&b.DOP, aPri, bPri)
	mergeOpt(&merged.DiffAge, &b.DiffAge, aPri, bPri)
	mergeOpt(&merged.RTCMRefBaseID, &b.RTCMRefBaseID, aPri, bPri)
	mergeOpt(&merged.NumSVUsed, &b.NumSVUsed, aPri, bPri)
	mergeOpt(&merged.NumSVTracked, &b.NumSVTracked, aPri, bPri)
	merged.Correction |= b.Correction
	merged.AuxSrc |= b.AuxSrc
	merged.SignalsUsed |= b.SignalsUsed
	if bPri >= aPri {
		merged.Tag = b.Tag
		if b.FixLevel != 0 {
			merged.FixLevel = b.FixLevel
		}
		if b.FixDim != 0 {
			merged.FixDim = b.FixDim
		}
		aPri = bPri
	} else {
		if merged.FixLevel == 0 {
			merged.FixLevel = b.FixLevel
		}
		if merged.FixDim == 0 {
			merged.FixDim = b.FixDim
		}
	}
	if b.StartTime.Before(merged.StartTime) {
		merged.StartTime = b.StartTime
	}
	return &merged, aPri
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
	var merged *NavEpochMsg
	var mergedPri MsgPriority
	var mh MsgHandler
	for f := range m.active {
		msg, pri, h := f.FlushNavEpoch(tRead)
		if msg != nil && mh == nil {
			mh = h
		}
		merged, mergedPri = MergeNavEpoch(merged, mergedPri, msg, pri)
	}
	clear(m.active)
	if merged != nil && mh != nil {
		mh.NavEpoch(merged, tRead)
	}
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
