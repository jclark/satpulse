package novmsg

import (
	"fmt"
	"strconv"
)

// RangeID is the message ID of the RANGE log.
const RangeID MsgID = 43 // OEM7, SinoGNSS, RTKLIB

// RangeFixed contains the fixed-length fields of the RANGE log.
type RangeFixed struct {
	NumObs uint32
}

// Range represents the RANGE log: one record per tracked signal.
// The layout is the same for OEM7 and SinoGNSS.
type Range struct {
	RangeFixed
	Obs []RangeObs
}

var _ Chunked = (*Range)(nil)

// RangeObs is one signal observation in a RANGE log.
type RangeObs struct {
	PRN      uint16
	GloFreq  uint16 // GLONASS frequency channel + 7
	PSR      float64
	PSRStd   float32
	ADR      float64
	ADRStd   float32
	Dopp     float32
	CN0      float32
	LockTime float32
	Status   ChTrStatus
}

// ID returns the message ID for RANGE.
func (m *Range) ID() (MsgID, string) {
	return RangeID, "RANGEA"
}

// Chunks implements the Chunked interface for Range.
func (m *Range) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		m.NumObs = uint32(len(m.Obs))
		if !yield(&m.RangeFixed) {
			return
		}
		if len(m.Obs) == 0 && m.NumObs > 0 {
			m.Obs = make([]RangeObs, ClampLen[RangeObs](m.NumObs))
		}
		for i := range m.Obs {
			if !yield(&m.Obs[i]) {
				return
			}
		}
	}
}

// ChTrStatus is the channel tracking status word of a RANGE record
// (OEM7 Table 164).
type ChTrStatus uint32

// SatSystem is the satellite system field of ChTrStatus.
type SatSystem uint8

// Satellite systems in ChTrStatus.
const (
	SatSystemGPS     SatSystem = 0
	SatSystemGLONASS SatSystem = 1
	SatSystemSBAS    SatSystem = 2
	SatSystemGalileo SatSystem = 3
	SatSystemBeiDou  SatSystem = 4
	SatSystemQZSS    SatSystem = 5
	SatSystemNavIC   SatSystem = 6
	SatSystemOther   SatSystem = 7
)

// TrackingState returns the tracking state (OEM7 Table 165).
func (s ChTrStatus) TrackingState() uint8 {
	return uint8(s & 0x1f)
}

// PhaseLocked reports whether the phase lock flag is set.
func (s ChTrStatus) PhaseLocked() bool {
	return s&(1<<10) != 0
}

// ParityKnown reports whether the parity known flag is set; until it is,
// the carrier phase may have a half cycle ambiguity.
func (s ChTrStatus) ParityKnown() bool {
	return s&(1<<11) != 0
}

// CodeLocked reports whether the code locked flag is set.
func (s ChTrStatus) CodeLocked() bool {
	return s&(1<<12) != 0
}

// SatSystem returns the satellite system.
func (s ChTrStatus) SatSystem() SatSystem {
	return SatSystem((s >> 16) & 0x7)
}

// SignalType returns the signal type, whose meaning depends on the
// satellite system.
func (s ChTrStatus) SignalType() uint8 {
	return uint8((s >> 21) & 0x1f)
}

// HalfCycleAdded reports whether a half cycle was added to the carrier
// phase to correct an inverted phase.
func (s ChTrStatus) HalfCycleAdded() bool {
	return s&(1<<28) != 0
}

// String returns the ASCII hex representation.
func (s ChTrStatus) String() string {
	return fmt.Sprintf("%08x", uint32(s))
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (s *ChTrStatus) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 16, 32)
	if err != nil {
		return err
	}
	*s = ChTrStatus(n)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (s ChTrStatus) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func init() {
	regMsg[Range]("RANGE")
}
