package rtcmbin

import (
	"fmt"
	"iter"
	"math/bits"
	"strconv"

	"github.com/jclark/satpulse/gps/lib/bitsenc"
)

// Meter is the number of RTCM length units per meter (0.0001 m resolution).
const Meter = 10000

// MT1005 represents an RTCM 1005 Stationary RTK Reference Station ARP message.
// ECEF coordinates are in units of 0.0001 m.
type MT1005 struct {
	MsgHdr
	StationID    uint16 `bits:"12" json:"stationID"`
	ITRFYear     uint8  `bits:"6" json:"itrfYear"`
	GPS          bool   `bits:"1" json:"gps"`
	GLONASS      bool   `bits:"1" json:"glonass"`
	Galileo      bool   `bits:"1" json:"galileo"`
	RefStation   bool   `bits:"1" json:"refStation"`
	ECEFX        int64  `bits:"38" json:"ecefX"`
	SingleOsc    bool   `bits:"1" json:"singleOsc"`
	Reserved     uint8  `bits:"1" json:"-"`
	ECEFY        int64  `bits:"38" json:"ecefY"`
	QuarterCycle uint8  `bits:"2" json:"quarterCycle"`
	ECEFZ        int64  `bits:"38" json:"ecefZ"`
}

// ECEF returns the antenna reference point in meters.
func (m *MT1005) ECEF() [3]float64 {
	return [3]float64{float64(m.ECEFX) / Meter, float64(m.ECEFY) / Meter, float64(m.ECEFZ) / Meter}
}

// MT1006 represents an RTCM 1006 Stationary RTK Reference Station ARP
// with Antenna Height message. ECEF coordinates and antenna height are
// in units of 0.0001 m.
type MT1006 struct {
	MT1005
	AntennaHeight uint16 `bits:"16" json:"antennaHeight"`
}

// MT1230 represents a GLONASS L1 and L2 Code-Phase Biases message.
// The CodePhaseBias slice contains one int16 value per set bit in
// SignalMask, in order: L1 C/A (bit 3), L1 P (bit 2), L2 C/A (bit 1),
// L2 P (bit 0). Values are in units of 0.02 m.
type MT1230 struct {
	MsgHdr
	StationID     uint16 `bits:"12"`
	Indicator     bool
	Reserved      uint8 `bits:"3"`
	SignalMask    uint8 `bits:"4"`
	CodePhaseBias []int16
}

// SizeSlices allocates the CodePhaseBias slice based on SignalMask.
func (m *MT1230) SizeSlices() {
	m.CodePhaseBias = make([]int16, bits.OnesCount8(m.SignalMask))
}

// MSM is an MSM1-5 message (standard resolution signal data).
type MSM struct {
	MSMHeader
	CellMask uint64 `bits:"var"`
	Sat      MSMSatData
	Sig      MSMSigData
}

// MSMHiRes is an MSM6-7 message (high resolution signal data).
type MSMHiRes struct {
	MSMHeader
	CellMask uint64 `bits:"var"`
	Sat      MSMSatData
	Sig      MSMHiResSigData
}

// MSMHeader is the header common to all MSM messages (1071-1137).
type MSMHeader struct {
	MsgHdr
	StationID     uint16 `bits:"12"`
	EpochTime     uint32 `bits:"30"`
	MultipleMsg   bool
	IODS          uint8 `bits:"3"`
	Reserved      uint8 `bits:"7"`
	ClockSteering uint8 `bits:"2"`
	ExtClock      uint8 `bits:"2"`
	DivFree       bool
	Smoothing     uint8 `bits:"3"`
	SatMask       uint64
	SigMask       uint32
}

// MSMSatData holds satellite-level data common to all MSM levels.
type MSMSatData struct {
	RangeInt  Uint8Slice // DF397, MSM4-7
	ExtInfo   Uint8Slice `bits:"4"`  // MSM5/7
	RangeMod  []uint16   `bits:"10"` // DF398, all MSM
	PhaseRate []int16    `bits:"14"` // DF399, MSM5/7
}

// MSMSigData holds standard-resolution signal data (MSM1-5).
type MSMSigData struct {
	Pseudorange []int16    `bits:"15"` // DF400
	PhaseRange  []int32    `bits:"22"` // DF401
	LockTime    Uint8Slice `bits:"4"`  // DF402
	HalfCycle   []bool     // DF420
	CNR         Uint8Slice `bits:"6"`  // DF403
	PhaseRate   []int16    `bits:"15"` // DF404
}

// MSMHiResSigData holds high-resolution signal data (MSM6-7).
type MSMHiResSigData struct {
	Pseudorange []int32  `bits:"20"` // DF405
	PhaseRange  []int32  `bits:"24"` // DF406
	LockTime    []uint16 `bits:"10"` // DF407
	HalfCycle   []bool   // DF420
	CNR         []uint16 `bits:"10"` // DF408
	PhaseRate   []int16  `bits:"15"` // DF404
}

// GNSS identifies a GNSS constellation.
type GNSS string

const (
	GPS     GNSS = "GPS"
	GLONASS GNSS = "GLONASS"
	GALILEO GNSS = "GALILEO"
	SBAS    GNSS = "SBAS"
	QZSS    GNSS = "QZSS"
	BEIDOU  GNSS = "BEIDOU"
	IRNSS   GNSS = "IRNSS"
)

// GNSS returns the constellation for this message type.
func (h *MSMHeader) GNSS() GNSS {
	switch (h.MsgNum - 1) / 10 {
	case 107:
		return GPS
	case 108:
		return GLONASS
	case 109:
		return GALILEO
	case 110:
		return SBAS
	case 111:
		return QZSS
	case 112:
		return BEIDOU
	case 113:
		return IRNSS
	}
	return ""
}

// MSMLevel returns the MSM level (1-7) from the message type.
func (h *MSMHeader) MSMLevel() int {
	return int(h.MsgNum % 10)
}

// Nsat returns the number of satellites in the satellite mask.
func (h *MSMHeader) Nsat() int {
	return bits.OnesCount64(h.SatMask)
}

// Nsig returns the number of signal types in the signal mask.
func (h *MSMHeader) Nsig() int {
	return bits.OnesCount32(h.SigMask)
}

func (h *MSMHeader) cellMaskBits() int {
	return h.Nsat() * h.Nsig()
}

// Satellites returns the satellite IDs (1-based) from the satellite mask.
func (h *MSMHeader) Satellites() []uint8 {
	sats := make([]uint8, 0, h.Nsat())
	for i := range 64 {
		if h.SatMask>>(63-i)&1 != 0 {
			sats = append(sats, uint8(i+1))
		}
	}
	return sats
}

// Signals returns the signal IDs (1-based) from the signal mask.
func (h *MSMHeader) Signals() []uint8 {
	sigs := make([]uint8, 0, h.Nsig())
	for i := range 32 {
		if h.SigMask>>(31-i)&1 != 0 {
			sigs = append(sigs, uint8(i+1))
		}
	}
	return sigs
}

func (h *MSMHeader) sizeSatSlices(sat *MSMSatData) {
	nsat := h.Nsat()
	level := h.MSMLevel()
	sat.RangeInt = make(Uint8Slice, boolN(level >= 4, nsat))
	sat.ExtInfo = make(Uint8Slice, boolN(level == 5 || level == 7, nsat))
	sat.RangeMod = make([]uint16, nsat)
	sat.PhaseRate = make([]int16, boolN(level == 5 || level == 7, nsat))
}

// Uint8Slice is a []uint8 that serializes as a JSON array of numbers.
// encoding/json treats []uint8 (aka []byte) as binary data and base64-encodes
// it, which is wrong for numeric fields like lock times and CNR values.
type Uint8Slice []uint8

// MarshalJSON encodes the slice as a JSON array of numbers.
func (s Uint8Slice) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	buf := []byte{'['}
	for i, v := range s {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendUint(buf, uint64(v), 10)
	}
	return append(buf, ']'), nil
}
func (s *MSMSigData) sizeSlices(level, ncell int) {
	s.Pseudorange = make([]int16, boolN(level >= 1 && level != 2, ncell))
	s.PhaseRange = make([]int32, boolN(level >= 2, ncell))
	s.LockTime = make(Uint8Slice, boolN(level >= 2, ncell))
	s.HalfCycle = make([]bool, boolN(level >= 2, ncell))
	s.CNR = make(Uint8Slice, boolN(level >= 4, ncell))
	s.PhaseRate = make([]int16, boolN(level >= 5, ncell))
}

func (s *MSMHiResSigData) sizeSlices(level, ncell int) {
	s.Pseudorange = make([]int32, ncell)
	s.PhaseRange = make([]int32, ncell)
	s.LockTime = make([]uint16, ncell)
	s.HalfCycle = make([]bool, ncell)
	s.CNR = make([]uint16, ncell)
	s.PhaseRate = make([]int16, boolN(level >= 7, ncell))
}

// VarBits yields the bit width for each bits:"var" field.
func (m *MSM) VarBits() iter.Seq[int] {
	return func(yield func(int) bool) { yield(m.cellMaskBits()) }
}

// SizeSlices allocates all slices based on header masks and MSM level.
func (m *MSM) SizeSlices() {
	m.sizeSatSlices(&m.Sat)
	m.Sig.sizeSlices(m.MSMLevel(), bits.OnesCount64(m.CellMask))
}

// VarBits yields the bit width for each bits:"var" field.
func (m *MSMHiRes) VarBits() iter.Seq[int] {
	return func(yield func(int) bool) { yield(m.cellMaskBits()) }
}

// SizeSlices allocates all slices based on header masks and MSM level.
func (m *MSMHiRes) SizeSlices() {
	m.sizeSatSlices(&m.Sat)
	m.Sig.sizeSlices(m.MSMLevel(), bits.OnesCount64(m.CellMask))
}

// boolN returns n if cond is true, 0 otherwise.
func boolN(cond bool, n int) int {
	if cond {
		return n
	}
	return 0
}

func parseMSM(mt MsgType, payload string) (Msg, error) {
	r := bitsenc.NewReader([]byte(payload))
	if int(mt%10) >= 6 {
		var msg MSMHiRes
		if err := r.Read(&msg); err != nil {
			return nil, fmt.Errorf("rtcm MSM %d: %w", mt, err)
		}
		return &msg, nil
	}
	var msg MSM
	if err := r.Read(&msg); err != nil {
		return nil, fmt.Errorf("rtcm MSM %d: %w", mt, err)
	}
	return &msg, nil
}
