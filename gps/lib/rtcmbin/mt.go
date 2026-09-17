package rtcmbin

import (
	"bytes"
	"encoding/json"
	"iter"
	"math/bits"
	"strconv"
)

// Meter is the number of RTCM length units per meter (0.0001 m resolution).
const Meter = 10000

// MT1005 represents an RTCM 1005 Stationary RTK Reference Station ARP message.
// ECEF coordinates are in units of 0.0001 m.
type MT1005 struct {
	MsgHdrStationID
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
	MsgHdrStationID
	Indicator     bool
	Reserved      uint8 `bits:"3"`
	SignalMask    uint8 `bits:"4"`
	CodePhaseBias []int16
}

// SizeSlices allocates the CodePhaseBias slice based on SignalMask.
func (m *MT1230) SizeSlices() {
	m.CodePhaseBias = make([]int16, bits.OnesCount8(m.SignalMask))
}

// Char8 is a variable-length char8(n) field as defined in RTCM SC-104
// (e.g. DF030 antenna descriptor).  Per Table 3.3-1 char8 is the ISO
// 8859-1 character set, not limited to ASCII, with 0x00 as the reserved
// or unused character.  It shares the wire layout of []byte but marshals
// as a JSON string, not base64.
type Char8 []byte

// StripNul returns the used characters of c, dropping the unused ones.
func (c Char8) StripNul() Char8 {
	if bytes.IndexByte(c, 0) < 0 {
		return c
	}
	return bytes.ReplaceAll(c, []byte{0}, nil)
}

// String decodes the characters as a UTF-8 encoded string, mapping each
// to the code point with the same numeric value.
func (c Char8) String() string {
	r := make([]rune, len(c))
	for i, v := range c {
		r[i] = rune(v)
	}
	return string(r)
}

// MarshalJSON encodes the used characters as a JSON string.
func (c Char8) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.StripNul().String())
}

// MT1007 is an RTCM 1007 Antenna Descriptor message.
type MT1007 struct {
	MsgHdrStationID
	DescriptorCounterN uint8 `bits:"8" json:"descriptorCounterN"`
	AntennaDescriptor  Char8 `bits:"8" json:"antennaDescriptor"`
	AntennaSetupID     uint8 `bits:"8" json:"antennaSetupID"`
}

// SizeSlices allocates AntennaDescriptor from its length prefix.
func (m *MT1007) SizeSlices() {
	if m.AntennaDescriptor == nil {
		m.AntennaDescriptor = make(Char8, m.DescriptorCounterN)
	}
}

// MT1008 is an RTCM 1008 Antenna Descriptor with Serial Number message.
type MT1008 struct {
	MT1007
	SerialNumberCounterM uint8 `bits:"8" json:"serialNumberCounterM"`
	AntennaSerialNumber  Char8 `bits:"8" json:"antennaSerialNumber"`
}

// SizeSlices allocates the next currently-nil string slice using its
// length prefix.  bitsenc calls this for every nil slice encountered,
// so each call only needs to handle one.
func (m *MT1008) SizeSlices() {
	switch {
	case m.AntennaDescriptor == nil:
		m.AntennaDescriptor = make(Char8, m.DescriptorCounterN)
	case m.AntennaSerialNumber == nil:
		m.AntennaSerialNumber = make(Char8, m.SerialNumberCounterM)
	}
}

// MT1033 is an RTCM 1033 Receiver and Antenna Descriptors message.
type MT1033 struct {
	MT1008
	ReceiverTypeCounterI    uint8 `bits:"8" json:"receiverTypeCounterI"`
	ReceiverTypeDescriptor  Char8 `bits:"8" json:"receiverTypeDescriptor"`
	FirmwareCounterJ        uint8 `bits:"8" json:"firmwareCounterJ"`
	ReceiverFirmwareVersion Char8 `bits:"8" json:"receiverFirmwareVersion"`
	ReceiverSerialCounterK  uint8 `bits:"8" json:"receiverSerialCounterK"`
	ReceiverSerialNumber    Char8 `bits:"8" json:"receiverSerialNumber"`
}

// SizeSlices allocates the next currently-nil string slice using its
// length prefix.
func (m *MT1033) SizeSlices() {
	switch {
	case m.AntennaDescriptor == nil:
		m.AntennaDescriptor = make(Char8, m.DescriptorCounterN)
	case m.AntennaSerialNumber == nil:
		m.AntennaSerialNumber = make(Char8, m.SerialNumberCounterM)
	case m.ReceiverTypeDescriptor == nil:
		m.ReceiverTypeDescriptor = make(Char8, m.ReceiverTypeCounterI)
	case m.ReceiverFirmwareVersion == nil:
		m.ReceiverFirmwareVersion = make(Char8, m.FirmwareCounterJ)
	case m.ReceiverSerialNumber == nil:
		m.ReceiverSerialNumber = make(Char8, m.ReceiverSerialCounterK)
	}
}

// LegacyGPSHeader is the header common to MT1001-1004.
type LegacyGPSHeader struct {
	MsgHdrStationID
	Epoch       uint32 `bits:"30" json:"epoch"`
	MultipleMsg bool   `json:"multipleMsg"`
	Nsat        uint8  `bits:"5" json:"nsat"`
	DivFree     bool   `json:"divFree"`
	Smoothing   uint8  `bits:"3" json:"smoothing"`
}

// LegacyGLONASSHeader is the header common to MT1009-1012.
type LegacyGLONASSHeader struct {
	MsgHdrStationID
	Epoch       uint32 `bits:"27" json:"epoch"`
	MultipleMsg bool   `json:"multipleMsg"`
	Nsat        uint8  `bits:"5" json:"nsat"`
	DivFree     bool   `json:"divFree"`
	Smoothing   uint8  `bits:"3" json:"smoothing"`
}

// MT1001Sat is a per-satellite record for MT1001 (GPS L1 basic).
type MT1001Sat struct {
	SatID         uint8  `bits:"6" json:"satID"`
	L1Code        bool   `json:"l1Code"`
	L1Pseudorange uint32 `bits:"24" json:"l1Pseudorange"`
	L1PhaseRange  int32  `bits:"20" json:"l1PhaseRange"`
	L1LockTime    uint8  `bits:"7" json:"l1LockTime"`
}

// MT1002Sat is a per-satellite record for MT1002 (GPS L1 extended).
type MT1002Sat struct {
	MT1001Sat
	L1Ambiguity uint8 `bits:"8" json:"l1Ambiguity"`
	L1CNR       uint8 `bits:"8" json:"l1CNR"`
}

// LegacyL2 is the L1/L2 observable core shared by MT1003/1004 (GPS
// DF016-DF019) and MT1011/1012 (GLONASS DF046-DF049).  Both constellations
// use identical widths and signedness for these fields.  DF047/L2L1PRDiff
// is int14: the RTCM SC-104 v3.2 satellite tables mislabel it as uint14
// but the authoritative field definition is signed, with 0x2000 as the
// "invalid L2" sentinel (-163.84 m).
type LegacyL2 struct {
	L2Code       uint8 `bits:"2" json:"l2Code"`
	L2L1PRDiff   int16 `bits:"14" json:"l2L1PRDiff"`
	L2PhaseRange int32 `bits:"20" json:"l2PhaseRange"`
	L2LockTime   uint8 `bits:"7" json:"l2LockTime"`
}

// MT1003Sat is a per-satellite record for MT1003 (GPS L1+L2 basic).
type MT1003Sat struct {
	MT1001Sat
	LegacyL2
}

// MT1004Sat is a per-satellite record for MT1004 (GPS L1+L2 extended).
// Wire order is DF009..DF020, so L1 ambiguity + CNR precede L2 fields.
type MT1004Sat struct {
	MT1002Sat
	LegacyL2
	L2CNR uint8 `bits:"8" json:"l2CNR"`
}

// MT1009Sat is a per-satellite record for MT1009 (GLONASS L1 basic).
// FreqChan holds the raw 5-bit DF040 code (0..20). Per spec this encodes
// GLONASS frequency channel numbers -7..+13 (code 0 = -7, code 7 = 0,
// code 20 = +13); consumers handle the code-to-channel translation.
type MT1009Sat struct {
	SatID         uint8  `bits:"6" json:"satID"`
	L1Code        bool   `json:"l1Code"`
	FreqChan      uint8  `bits:"5" json:"freqChan"`
	L1Pseudorange uint32 `bits:"25" json:"l1Pseudorange"`
	L1PhaseRange  int32  `bits:"20" json:"l1PhaseRange"`
	L1LockTime    uint8  `bits:"7" json:"l1LockTime"`
}

// MT1010Sat is a per-satellite record for MT1010 (GLONASS L1 extended).
type MT1010Sat struct {
	MT1009Sat
	L1Ambiguity uint8 `bits:"7" json:"l1Ambiguity"`
	L1CNR       uint8 `bits:"8" json:"l1CNR"`
}

// MT1011Sat is a per-satellite record for MT1011 (GLONASS L1+L2 basic).
type MT1011Sat struct {
	MT1009Sat
	LegacyL2
}

// MT1012Sat is a per-satellite record for MT1012 (GLONASS L1+L2 extended).
// Wire order is DF038..DF050, so L1 ambiguity + CNR precede L2 fields.
type MT1012Sat struct {
	MT1010Sat
	LegacyL2
	L2CNR uint8 `bits:"8" json:"l2CNR"`
}

// MT1001 is an RTCM 1001 L1-only GPS RTK Observables message.
type MT1001 struct {
	LegacyGPSHeader
	Sat []MT1001Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1001) SizeSlices() { m.Sat = make([]MT1001Sat, m.Nsat) }

// MT1002 is an RTCM 1002 extended L1-only GPS RTK Observables message.
type MT1002 struct {
	LegacyGPSHeader
	Sat []MT1002Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1002) SizeSlices() { m.Sat = make([]MT1002Sat, m.Nsat) }

// MT1003 is an RTCM 1003 L1/L2 GPS RTK Observables message.
type MT1003 struct {
	LegacyGPSHeader
	Sat []MT1003Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1003) SizeSlices() { m.Sat = make([]MT1003Sat, m.Nsat) }

// MT1004 is an RTCM 1004 extended L1/L2 GPS RTK Observables message.
type MT1004 struct {
	LegacyGPSHeader
	Sat []MT1004Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1004) SizeSlices() { m.Sat = make([]MT1004Sat, m.Nsat) }

// MT1009 is an RTCM 1009 L1-only GLONASS RTK Observables message.
type MT1009 struct {
	LegacyGLONASSHeader
	Sat []MT1009Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1009) SizeSlices() { m.Sat = make([]MT1009Sat, m.Nsat) }

// MT1010 is an RTCM 1010 extended L1-only GLONASS RTK Observables message.
type MT1010 struct {
	LegacyGLONASSHeader
	Sat []MT1010Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1010) SizeSlices() { m.Sat = make([]MT1010Sat, m.Nsat) }

// MT1011 is an RTCM 1011 L1/L2 GLONASS RTK Observables message.
type MT1011 struct {
	LegacyGLONASSHeader
	Sat []MT1011Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1011) SizeSlices() { m.Sat = make([]MT1011Sat, m.Nsat) }

// MT1012 is an RTCM 1012 extended L1/L2 GLONASS RTK Observables message.
type MT1012 struct {
	LegacyGLONASSHeader
	Sat []MT1012Sat `json:"sat"`
}

// SizeSlices allocates Sat based on Nsat.
func (m *MT1012) SizeSlices() { m.Sat = make([]MT1012Sat, m.Nsat) }

// MT1013Announce is a single message-ID announcement record in MT1013.
// TxInterval is in units of 0.1 s.
type MT1013Announce struct {
	MsgID      uint16 `bits:"12" json:"msgID"`
	Sync       bool   `json:"sync"`
	TxInterval uint16 `bits:"16" json:"txInterval"`
}

// MT1013 represents an RTCM 1013 System Parameters message.
// It announces the messages this station transmits and their intervals.
type MT1013 struct {
	MsgHdrStationID
	MJD          uint16           `bits:"16" json:"mjd"`
	SecondsOfDay uint32           `bits:"17" json:"secondsOfDay"`
	Nm           uint8            `bits:"5" json:"nm"`
	LeapSeconds  uint8            `bits:"8" json:"leapSeconds"`
	Announce     []MT1013Announce `json:"announce"`
}

// SizeSlices allocates Announce based on Nm.
func (m *MT1013) SizeSlices() { m.Announce = make([]MT1013Announce, m.Nm) }

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
	MsgHdrStationID
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
