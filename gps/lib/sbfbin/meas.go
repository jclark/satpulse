package sbfbin

import "encoding/binary"

type MeasType uint8
type CommonFlags uint8
type ObsInfo uint8
type OffsetsMSB uint8

const (
	MeasSigIdxExtension        = 31
	MeasType1CodeInvalidMSB    = 0
	MeasType1CodeInvalidLSB    = 0
	MeasType1DopplerDNU        = -0x80000000
	MeasType1CarrierMSBDNU     = -128
	MeasType1CN0DNU            = 255
	MeasType1LockTimeDNU       = 0xFFFF
	MeasType1LockTimeClipped   = 0xFFFE
	MeasType2LockTimeDNU       = 255
	MeasType2LockTimeClipped   = 254
	MeasType2CarrierMSBDNU     = -128
	MeasExtraCodeVarDNU        = 0xFFFF
	MeasExtraCodeVarClipped    = 0xFFFE
	MeasExtraCarrierVarDNU     = 0xFFFF
	MeasExtraCarrierVarClipped = 0xFFFE
	MeasExtraLockTimeDNU       = 0xFFFF
	MeasExtraLockTimeClipped   = 0xFFFE
)

type measEpochHead struct {
	N1          uint8
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
	N2         uint8
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
	return twoLevelChunks(&m.N1, &m.SB1Length, &m.SB2Length, &m.measEpochHead, &m.Type1, &m.Type2,
		func(t *MeasEpochChannelType1) uint8 { return t.N2 },
		func(t *MeasEpochChannelType1, n uint8) { t.N2 = n })
}

type measExtraHead struct {
	N                uint8
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
		if len(m.Channels) > 0 {
			// On write Channels is authoritative. N is only the count modulo
			// 256 (it wraps at 256), so neither N nor anything derived from it
			// may size the write; the true count is recovered on read from the
			// payload length and SBLength.
			m.N = uint8(len(m.Channels))
			if m.SBLength == 0 {
				m.SBLength = uint8(binary.Size(MeasExtraChannelSub{}))
			}
		}
		if !yield(&m.measExtraHead) {
			return
		}
		// Write path uses len(Channels); read path starts empty and recovers
		// the true count from the payload length below.
		n := len(m.Channels)
		payloadLen := m.payloadLen()
		if m.SBLength > 0 && payloadLen > binary.Size(measExtraHead{}) {
			n = (payloadLen - binary.Size(measExtraHead{})) / int(m.SBLength)
		}
		if len(m.Channels) != n {
			m.Channels = make([]MeasExtraChannelSub, n)
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

// EndOfMeas is the SBF EndOfMeas block.
type EndOfMeas struct{}

// BlockNumber returns the SBF block number.
func (*EndOfMeas) BlockNumber() uint16 { return EndOfMeasID }

func init() {
	regBlock[MeasEpoch]("MeasEpoch")
	regBlock[MeasExtra]("MeasExtra")
	regBlock[EndOfMeas]("EndOfMeas")
}
