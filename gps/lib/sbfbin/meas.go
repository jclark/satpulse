package sbfbin

import "encoding/binary"

type MeasType uint8
type CommonFlags uint8
type ObsInfo uint8
type OffsetsMSB uint8

const (
	MeasSigIdxExtension          = 31
	MeasType1CodeInvalidMSB      = 0
	MeasType1CodeInvalidLSB      = 0
	MeasType1DopplerDNU          = -0x80000000
	MeasType1CarrierMSBDNU       = -128
	MeasType1CN0DNU              = 0xFF
	MeasType1LockTimeDNU         = 0xFFFF
	MeasType1LockTimeClipped     = 0xFFFE
	MeasType2LockTimeDNU         = 0xFF
	MeasType2LockTimeClipped     = 0xFE
	MeasType2CarrierMSBDNU       = -128
	MeasType2CodeOffsetMSBDNU    = -4
	MeasType2DopplerOffsetMSBDNU = -16
	MeasExtraCodeVarDNU          = 0xFFFF
	MeasExtraCodeVarClipped      = 0xFFFE
	MeasExtraCarrierVarDNU       = 0xFFFF
	MeasExtraCarrierVarClipped   = 0xFFFE
	MeasExtraLockTimeDNU         = 0xFFFF
	MeasExtraLockTimeClipped     = 0xFFFE
)

// CommonFlagsE6BUsed is the MeasEpoch CommonFlags bit indicating that the
// Galileo E6 measurement (signal number 19) is the E6-B component; when clear
// it is E6-C.
const CommonFlagsE6BUsed CommonFlags = 1 << 6

// E6BUsed reports whether the E6-B-used bit is set.
func (f CommonFlags) E6BUsed() bool {
	return f&CommonFlagsE6BUsed != 0
}

// Observed-axis signal numbers, guide sec 4.1.10. These index both
// MeasEpoch's SigIdxLo field and the PVT SignalInfo bitmask.
const (
	SigNumGPSL1CA         uint8 = 0
	SigNumGPSL1P          uint8 = 1
	SigNumGPSL2P          uint8 = 2
	SigNumGPSL2C          uint8 = 3
	SigNumGPSL5           uint8 = 4
	SigNumGPSL1C          uint8 = 5
	SigNumQZSSL1CA        uint8 = 6
	SigNumQZSSL2C         uint8 = 7
	SigNumGLONASSL1CA     uint8 = 8
	SigNumGLONASSL1P      uint8 = 9
	SigNumGLONASSL2P      uint8 = 10
	SigNumGLONASSL2CA     uint8 = 11
	SigNumGLONASSL3       uint8 = 12
	SigNumBeiDouB1C       uint8 = 13
	SigNumBeiDouB2a       uint8 = 14
	SigNumNavICL5         uint8 = 15
	SigNumGalileoE1       uint8 = 17
	SigNumGalileoE6       uint8 = 19
	SigNumGalileoE5a      uint8 = 20
	SigNumGalileoE5b      uint8 = 21
	SigNumGalileoE5AltBOC uint8 = 22
	SigNumLBand           uint8 = 23
	SigNumSBASL1CA        uint8 = 24
	SigNumSBASL5          uint8 = 25
	SigNumQZSSL5          uint8 = 26
	SigNumQZSSL6          uint8 = 27
	SigNumBeiDouB1I       uint8 = 28
	SigNumBeiDouB2I       uint8 = 29
	SigNumBeiDouB3I       uint8 = 30
	SigNumQZSSL1C         uint8 = 32
	SigNumQZSSL1S         uint8 = 33
	SigNumBeiDouB2b       uint8 = 34
	SigNumNavICL1         uint8 = 37
	SigNumQZSSL1CB        uint8 = 38
	SigNumQZSSL5S         uint8 = 39
)

type measEpochHead struct {
	SB1Length   uint8
	SB2Length   uint8
	CommonFlags CommonFlags
	CumClkJumps uint8
	Reserved    uint8
}

// MeasEpochChannelType1 is a MeasEpoch master sub-block.
type MeasEpochChannelType1 struct {
	RxChannel  uint8
	Type       MeasType
	SVID       uint8
	Misc       uint8
	CodeLSB    uint32
	Doppler    int32
	CarrierLSB uint16
	CarrierMSB int8
	CN0        uint8
	LockTime   uint16
	ObsInfo    ObsInfo
}

// MeasEpochChannelType2 is a MeasEpoch slave sub-block.
type MeasEpochChannelType2 struct {
	Type             MeasType
	LockTime         uint8
	CN0              uint8
	OffsetsMSB       OffsetsMSB
	CarrierMSB       int8
	ObsInfo          ObsInfo
	CodeOffsetLSB    uint16
	CarrierLSB       uint16
	DopplerOffsetLSB uint16
}

// MeasEpoch is the SBF MeasEpoch block.
type MeasEpoch struct {
	measEpochHead
	Type1 []MeasEpochChannelType1
	Type2 [][]MeasEpochChannelType2
}

// BlockNumber returns the SBF block number.
func (*MeasEpoch) BlockNumber() uint16 { return MeasEpochID }

// Chunks returns the binary chunks for the block parameters.
func (m *MeasEpoch) Chunks() func(yield func(chunk any) bool) {
	return twoLevelChunks(&m.SB1Length, &m.SB2Length, &m.measEpochHead, &m.Type1, &m.Type2,
		func(t *MeasEpochChannelType1, n *uint8, yield func(any) bool) bool {
			return yield(t) && yield(n)
		})
}

// SignalNumber returns the observed signal number for a MeasEpoch master
// channel, resolving the SigIdxLo==31 extension via ObsInfo bits 3-7.
func (t *MeasEpochChannelType1) SignalNumber() uint8 {
	return measSignalNumber(t.Type, t.ObsInfo)
}

// SignalNumber returns the observed signal number for a MeasEpoch slave
// channel, resolving the SigIdxLo==31 extension via ObsInfo bits 3-7.
func (t *MeasEpochChannelType2) SignalNumber() uint8 {
	return measSignalNumber(t.Type, t.ObsInfo)
}

// AntennaID returns the AntennaID field of a MeasEpoch master channel.
func (t *MeasEpochChannelType1) AntennaID() uint8 {
	return measAntennaID(t.Type)
}

// AntennaID returns the AntennaID field of a MeasEpoch slave channel.
func (t *MeasEpochChannelType2) AntennaID() uint8 {
	return measAntennaID(t.Type)
}

func measSignalNumber(typ MeasType, obs ObsInfo) uint8 {
	n := uint8(typ) & 0x1F
	if n == MeasSigIdxExtension {
		return 32 + (uint8(obs)>>3)&0x1F
	}
	return n
}

func measAntennaID(typ MeasType) uint8 {
	return uint8(typ) >> 5
}

// CN0dBHz returns the C/N0 in dB-Hz for a MeasEpoch master channel, applying
// the +10 dB offset that all signal numbers except GPS L1P/L2P use. It reports
// false when the CN0 field is at its do-not-use sentinel.
func (t *MeasEpochChannelType1) CN0dBHz() (float64, bool) {
	return measCN0dBHz(t.CN0, t.SignalNumber())
}

// CN0dBHz returns the C/N0 in dB-Hz for a MeasEpoch slave channel.
func (t *MeasEpochChannelType2) CN0dBHz() (float64, bool) {
	return measCN0dBHz(t.CN0, t.SignalNumber())
}

func measCN0dBHz(cn0, sig uint8) (float64, bool) {
	if cn0 == MeasType1CN0DNU {
		return 0, false
	}
	v := float64(cn0) * 0.25
	if sig != SigNumGPSL1P && sig != SigNumGPSL2P {
		v += 10
	}
	return v, true
}

// Pseudorange returns the master pseudorange in meters. It reports false when
// the pseudorange is at its Do-Not-Use encoding (CodeMSB and CodeLSB both 0).
func (t *MeasEpochChannelType1) Pseudorange() (float64, bool) {
	msb := uint64(t.Misc & 0x0F)
	if msb == MeasType1CodeInvalidMSB && t.CodeLSB == MeasType1CodeInvalidLSB {
		return 0, false
	}
	return float64(msb*4294967296+uint64(t.CodeLSB)) * 0.001, true
}

// DopplerHz returns the master carrier Doppler in Hz, positive for
// approaching satellites. It reports false at the Do-Not-Use sentinel.
func (t *MeasEpochChannelType1) DopplerHz() (float64, bool) {
	if t.Doppler == MeasType1DopplerDNU {
		return 0, false
	}
	return float64(t.Doppler) * 0.0001, true
}

// CarrierOffsetCycles returns the master carrier phase relative to the
// pseudorange, in cycles: L = PR/lambda + CarrierOffsetCycles. It reports
// false when the carrier phase is at its Do-Not-Use encoding (CarrierMSB -128
// and CarrierLSB 0).
func (t *MeasEpochChannelType1) CarrierOffsetCycles() (float64, bool) {
	return carrierOffsetCycles(t.CarrierMSB, t.CarrierLSB, MeasType1CarrierMSBDNU)
}

// CarrierOffsetCycles returns the slave carrier phase relative to the slave
// pseudorange, in cycles: L = PR_type2/lambda + CarrierOffsetCycles. It
// reports false when the carrier phase is at its Do-Not-Use encoding.
func (t *MeasEpochChannelType2) CarrierOffsetCycles() (float64, bool) {
	return carrierOffsetCycles(t.CarrierMSB, t.CarrierLSB, MeasType2CarrierMSBDNU)
}

func carrierOffsetCycles(msb int8, lsb uint16, dnu int8) (float64, bool) {
	if msb == dnu && lsb == 0 {
		return 0, false
	}
	return (float64(msb)*65536 + float64(lsb)) * 0.001, true
}

// PseudorangeOffset returns the slave pseudorange offset in meters relative
// to the master: PR_type2 = PR_type1 + PseudorangeOffset. It reports false at
// the Do-Not-Use encoding (CodeOffsetMSB -4 and CodeOffsetLSB 0).
// CodeOffsetMSB is OffsetsMSB bits 0-2, sign-extended from its 3-bit two's
// complement encoding.
func (t *MeasEpochChannelType2) PseudorangeOffset() (float64, bool) {
	msb := int8(t.OffsetsMSB<<5) >> 5
	if msb == MeasType2CodeOffsetMSBDNU && t.CodeOffsetLSB == 0 {
		return 0, false
	}
	return (float64(msb)*65536 + float64(t.CodeOffsetLSB)) * 0.001, true
}

// DopplerOffsetHz returns the slave Doppler offset in Hz:
// D_type2 = D_type1*alpha + DopplerOffsetHz, with alpha the ratio of the
// slave and master carrier frequencies. It reports false at the Do-Not-Use
// encoding (DopplerOffsetMSB -16 and DopplerOffsetLSB 0). DopplerOffsetMSB is
// OffsetsMSB bits 3-7, sign-extended from its 5-bit two's complement encoding.
func (t *MeasEpochChannelType2) DopplerOffsetHz() (float64, bool) {
	msb := int8(t.OffsetsMSB) >> 3
	if msb == MeasType2DopplerOffsetMSBDNU && t.DopplerOffsetLSB == 0 {
		return 0, false
	}
	return (float64(msb)*65536 + float64(t.DopplerOffsetLSB)) * 0.0001, true
}

// HalfCycleAmbiguity reports whether bit 2 is set, meaning the carrier phase
// has a half-cycle ambiguity.
func (o ObsInfo) HalfCycleAmbiguity() bool {
	return o&0x4 != 0
}

// GLONASSFreqNr returns the GLONASS frequency number (-7..6) for a master
// channel whose signal is one of the GLONASS FDMA signal numbers 8-11.
// ObsInfo bits 3-7 encode the frequency number with an offset of 8; the wire
// value 0 does not encode a frequency number and reports false, as does any
// non-FDMA signal.
func (t *MeasEpochChannelType1) GLONASSFreqNr() (int8, bool) {
	return glonassFreqNr(t.Type, t.ObsInfo)
}

// GLONASSFreqNr returns the GLONASS frequency number for a slave channel.
// The reference guide spells out the ObsInfo frequency-number encoding only
// for the Type1 sub-block; applying it to Type2 follows plan/sbf-rinex.md.
func (t *MeasEpochChannelType2) GLONASSFreqNr() (int8, bool) {
	return glonassFreqNr(t.Type, t.ObsInfo)
}

func glonassFreqNr(typ MeasType, obs ObsInfo) (int8, bool) {
	n := uint8(typ) & 0x1F
	if n < SigNumGLONASSL1CA || n > SigNumGLONASSL2CA || obs>>3 == 0 {
		return 0, false
	}
	return int8(uint8(obs)>>3) - 8, true
}

type measExtraHead struct {
	SBLength         uint8
	DopplerVarFactor float32
}

type measExtraBase struct {
	RxChannel     uint8
	Type          MeasType
	MPCorrection  int16
	SmoothingCorr int16
	CodeVar       uint16
	CarrierVar    uint16
	LockTime      uint16
	CumLossCont   uint8
}

type measExtraRev1 struct {
	CarMPCorr int8
}

type measExtraRev2 struct {
	Info uint8
}

type measExtraRev3 struct {
	Misc uint8
}

// MeasExtraChannelSub is a MeasExtra channel sub-block.
type MeasExtraChannelSub struct {
	measExtraBase
	measExtraRev1
	measExtraRev2
	measExtraRev3
}

// MeasExtra is the SBF MeasExtra block.
type MeasExtra struct {
	measExtraHead
	Channels []MeasExtraChannelSub
	payloadSize
}

// BlockNumber returns the SBF block number.
func (*MeasExtra) BlockNumber() uint16 { return MeasExtraID }

// Chunks returns the binary chunks for the block parameters.
func (m *MeasExtra) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		// The wire N is len(Channels) modulo 256, so this conversion must wrap
		// instead of using setCount.
		n := uint8(len(m.Channels))
		if len(m.Channels) > 0 {
			if m.SBLength == 0 {
				m.SBLength = uint8(binary.Size(MeasExtraChannelSub{}))
			}
		}
		if !yield(&n) || !yield(&m.measExtraHead) {
			return
		}
		count := len(m.Channels)
		if payloadLen := m.payloadLen(); payloadLen > 0 && m.SBLength > 0 {
			// Recover the true count from the modulo-256 wire N with the guide's
			// own formula, NrSB = ((Length/SBLength-N)/256)*256+N. Dividing the
			// payload instead would count a future revision's trailing fields as
			// a further channel.
			length := payloadLen + HeaderLen + binary.Size(TimeStamp{})
			count = ((length/int(m.SBLength)-int(n))/256)*256 + int(n)
		}
		if len(m.Channels) != count {
			m.Channels = make([]MeasExtraChannelSub, count)
		}
		for i := range m.Channels {
			if !yield(&m.Channels[i].measExtraBase) {
				return
			}
			sbLen := int(m.SBLength)
			used := binary.Size(measExtraBase{})
			if sbLen >= used+binary.Size(measExtraRev1{}) {
				if !yield(&m.Channels[i].measExtraRev1) {
					return
				}
				used += binary.Size(measExtraRev1{})
			}
			if sbLen >= used+binary.Size(measExtraRev2{}) {
				if !yield(&m.Channels[i].measExtraRev2) {
					return
				}
				used += binary.Size(measExtraRev2{})
			}
			if sbLen >= used+binary.Size(measExtraRev3{}) {
				if !yield(&m.Channels[i].measExtraRev3) {
					return
				}
				used += binary.Size(measExtraRev3{})
			}
			if !yield(paddingChunk(sbLen - used)) {
				return
			}
		}
	}
}

// encodedRev returns the revision implied by the sub-block length Chunks
// writes: revisions 1 to 3 each appended one field to the channel sub-block. A
// block with no channels carries nothing revision-specific.
func (m *MeasExtra) encodedRev() (uint8, bool) {
	if len(m.Channels) == 0 {
		return 0, false
	}
	sbLen := int(m.SBLength)
	used := binary.Size(measExtraBase{})
	if sbLen < used+binary.Size(measExtraRev1{}) {
		return 0, true
	}
	used += binary.Size(measExtraRev1{})
	if sbLen < used+binary.Size(measExtraRev2{}) {
		return 1, true
	}
	used += binary.Size(measExtraRev2{})
	if sbLen < used+binary.Size(measExtraRev3{}) {
		return 2, true
	}
	return 3, true
}

// SignalNumber returns the observed signal number for a MeasExtra channel
// sub-block, resolving the SigIdxLo==31 extension via Misc bits 3-7. A
// SigIdxLo of 31 can only occur at block revision 3 or above, where the Misc
// field exists.
func (s *MeasExtraChannelSub) SignalNumber() uint8 {
	n := uint8(s.Type) & 0x1F
	if n == MeasSigIdxExtension {
		return 32 + (s.Misc>>3)&0x1F
	}
	return n
}

// CN0HighRes returns the high-resolution C/N0 refinement in dB-Hz to be added
// to the MeasEpoch C/N0 (Misc bits 0-2, 0.03125 dB-Hz units). It is only
// meaningful when the enclosing block's HasCN0HighRes reports true.
func (s *MeasExtraChannelSub) CN0HighRes() float64 {
	return float64(s.Misc&0x7) * 0.03125
}

// HasCN0HighRes reports whether the block's sub-block length includes the
// revision 3 Misc field that carries the CN0HighRes refinement.
func (m *MeasExtra) HasCN0HighRes() bool {
	return int(m.SBLength) >= binary.Size(measExtraBase{})+binary.Size(measExtraRev1{})+
		binary.Size(measExtraRev2{})+binary.Size(measExtraRev3{})
}

// EndOfMeas is the SBF EndOfMeas block.
type EndOfMeas struct{}

// BlockNumber returns the SBF block number.
func (*EndOfMeas) BlockNumber() uint16 { return EndOfMeasID }

func init() {
	regBlock[MeasEpoch]("MeasEpoch")
	regBlock[MeasExtra]("MeasExtra")
	regBlock[EndOfMeas]("EndOfMeas")
}
