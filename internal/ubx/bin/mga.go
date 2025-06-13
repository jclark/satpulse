package bin

import (
	"fmt"
)

const (
	MgaIniID MsgID = clsMga | (0x40 << 8)
	MgaGalID MsgID = clsMga | (0x02 << 8)
)

type MgaIni struct {
	MgaIniFixed
	payload MgaIniPayload
}

var _ VaryingMsg = (*MgaIni)(nil)
var _ PartiallyHandledMsg = (*MgaIni)(nil)

type MgaIniFixed struct {
	Type    MgaIniType
	Version byte
}

type MgaIniType byte

const (
	MgaIniTypePosXYZ   MgaIniType = 0x00
	MgaIniTypePosLLH   MgaIniType = 0x01
	MgaIniTypeTimeUTC  MgaIniType = 0x10
	MgaIniTypeTimeGNSS MgaIniType = 0x11
	MgaIniTypeClkD     MgaIniType = 0x20
	MgaIniTypeFreq     MgaIniType = 0x21
)

type MgaIniPayload interface {
	mgaIniPayload()
}

func (m *MgaIni) ID() MsgID { return MgaIniID }

func (m *MgaIni) FixedPart() any {
	return &m.MgaIniFixed
}

func (m *MgaIni) VaryingPart() any {
	return m.payload
}

func (m *MgaIni) IsHandled() bool {
	switch m.Type {
	case MgaIniTypePosXYZ, MgaIniTypePosLLH, MgaIniTypeTimeUTC, MgaIniTypeTimeGNSS, MgaIniTypeClkD, MgaIniTypeFreq:
		// The only version we know about for these message types is 0
		return m.Version == 0
	}
	// Unknown types are not handled
	return false
}

func (m *MgaIni) InitVaryingPart(payloadLen int) error {
	var expectedPayloadLen int

	switch m.Type {
	case MgaIniTypePosXYZ:
		m.payload = &MgaIniPosXYZ{}
		expectedPayloadLen = 20
	case MgaIniTypePosLLH:
		m.payload = &MgaIniPosLLH{}
		expectedPayloadLen = 20
	case MgaIniTypeTimeUTC:
		m.payload = &MgaIniTimeUTC{}
		expectedPayloadLen = 24
	case MgaIniTypeTimeGNSS:
		m.payload = &MgaIniTimeGNSS{}
		expectedPayloadLen = 24
	case MgaIniTypeClkD:
		m.payload = &MgaIniClkD{}
		expectedPayloadLen = 12
	case MgaIniTypeFreq:
		m.payload = &MgaIniFreq{}
		expectedPayloadLen = 12
	}

	if expectedPayloadLen != 0 {
		if expectedPayloadLen == payloadLen {
			return nil
		}
		return fmt.Errorf("bad UBX-MGA-INI payload length (%d bytes); expected %d", payloadLen, expectedPayloadLen)
	}

	if payloadLen < 2 {
		return fmt.Errorf("bad UBX-MGA-INI payload length (%d bytes); expected at least 2", payloadLen)
	}
	unknown := make(MgaIniUnknown, payloadLen-2)
	m.payload = &unknown
	return nil
}

type MgaIniPosXYZ struct {
	_      [2]byte
	ECEF   [3]int32
	PosAcc uint32
}

func (m *MgaIniPosXYZ) mgaIniPayload() {}

type MgaIniPosLLH struct {
	_      [2]byte
	Lat    int32
	Lon    int32
	Alt    int32
	PosAcc uint32
}

func (m *MgaIniPosLLH) mgaIniPayload() {}

type MgaIniTimeUTC struct {
	Ref       MgaIniTimeRef
	LeapSecs  int8
	Year      uint16
	Month     byte
	Day       byte
	Hour      byte
	Minute    byte
	Second    byte
	Bitfield0 MgaIniTimeBitfield0
	Ns        uint32
	TAccS     uint16
	_         [2]byte
	TAccNs    uint32
}

func (m *MgaIniTimeUTC) mgaIniPayload() {}

type MgaIniTimeGNSS struct {
	Ref       MgaIniTimeRef
	GnssId    byte
	Bitfield0 MgaIniTimeBitfield0
	_         byte
	Week      uint16
	Tow       uint32
	Ns        uint32
	TAccS     uint16
	_         [2]byte
	TAccNs    uint32
}

func (m *MgaIniTimeGNSS) mgaIniPayload() {}

type MgaIniTimeRef byte

const (
	MgaIniTimeRefSourceNone MgaIniTimeRef = iota
	MgaIniTimeRefSourceEXTINT0
	MgaIniTimeRefSourceEXTINT1
	MgaIniTimeSourceMask MgaIniTimeRef = 0b00001111
	MgaIniTimeRefFall    MgaIniTimeRef = 1 << 4
	MgaIniTimeRefLast    MgaIniTimeRef = 1 << 5
)

type MgaIniTimeBitfield0 byte

const (
	MgaIniTimeTrustedSource MgaIniTimeBitfield0 = 1 << iota
)

type MgaIniClkD struct {
	_       [2]byte
	ClkD    int32
	ClkDAcc uint32
}

func (m *MgaIniClkD) mgaIniPayload() {}

type MgaIniFreq struct {
	_       byte
	Flags   byte
	Freq    int32
	FreqAcc uint32
}

func (m *MgaIniFreq) mgaIniPayload() {}

type MgaIniUnknown []byte

func (m *MgaIniUnknown) mgaIniPayload() {}

type MgaGal struct {
	MgaGalFixed
	payload MgaGalPayload
}

var _ VaryingMsg = (*MgaGal)(nil)
var _ PartiallyHandledMsg = (*MgaGal)(nil)

type MgaGalFixed struct {
	Type    MgaGalType
	Version byte
}

type MgaGalType byte

const (
	MgaGalTypeEph         MgaGalType = 0x01
	MgaGalTypeAlm         MgaGalType = 0x02
	MgaGalTypeTimeOffset  MgaGalType = 0x03
	MgaGalTypeUTC         MgaGalType = 0x05
	MgaGalTypeOSNMAPubkey MgaGalType = 0x07
	MgaGalTypeOSNMAMerkle MgaGalType = 0x08
)

type MgaGalPayload interface {
	mgaGalPayload()
}

func (m *MgaGal) ID() MsgID { return MgaGalID }

func (m *MgaGal) FixedPart() any {
	return &m.MgaGalFixed
}

func (m *MgaGal) VaryingPart() any {
	return m.payload
}

func (m *MgaGal) IsHandled() bool {
	// Only version 0 is supported for known message types
	// Unknown message types should return false to become UnknownMsg
	switch m.Type {
	case MgaGalTypeOSNMAPubkey, MgaGalTypeOSNMAMerkle:
		return m.Version == 0
	default:
		return false // Unknown types become UnknownMsg
	}
}

func (m *MgaGal) InitVaryingPart(payloadLen int) error {
	var expectedPayloadLen int

	switch m.Type {
	case MgaGalTypeOSNMAPubkey:
		m.payload = &MgaGalOSNMAPubkey{}
		expectedPayloadLen = 72
	case MgaGalTypeOSNMAMerkle:
		m.payload = &MgaGalOSNMAMerkle{}
		expectedPayloadLen = 36
	}

	if expectedPayloadLen != 0 {
		if expectedPayloadLen == payloadLen {
			return nil
		}
		return fmt.Errorf("bad UBX-MGA-GAL payload length (%d bytes); expected %d", payloadLen, expectedPayloadLen)
	}

	if payloadLen < 2 {
		return fmt.Errorf("bad UBX-MGA-GAL payload length (%d bytes); expected at least 2", payloadLen)
	}
	unknown := make(MgaGalUnknown, payloadLen-2)
	m.payload = &unknown
	return nil
}

type MgaGalOSNMAPubkey struct {
	Bitfield0   MgaGalOSNMAPubkeyBitfield0
	_           byte
	PubKeyPoint [67]byte
	_           byte
}

func (m *MgaGalOSNMAPubkey) mgaGalPayload() {}

type MgaGalOSNMAPubkeyBitfield0 byte

const (
	MgaGalOSNMAPubkeyIDMask   MgaGalOSNMAPubkeyBitfield0 = 0x0F
	MgaGalOSNMAPubkeyTypeMask MgaGalOSNMAPubkeyBitfield0 = 0xF0
)

func (b MgaGalOSNMAPubkeyBitfield0) PubKeyID() byte {
	return byte(b & MgaGalOSNMAPubkeyIDMask)
}

func (b MgaGalOSNMAPubkeyBitfield0) PubKeyType() byte {
	return byte((b & MgaGalOSNMAPubkeyTypeMask) >> 4)
}

type MgaGalOSNMAMerkle struct {
	Bitfield0 MgaGalOSNMAMerkleBitfield0
	_         byte
	TreeNode  [32]byte
}

func (m *MgaGalOSNMAMerkle) mgaGalPayload() {}

type MgaGalOSNMAMerkleBitfield0 byte

const (
	MgaGalOSNMAMerkleApplicabilityTime MgaGalOSNMAMerkleBitfield0 = 0x01
)

func (b MgaGalOSNMAMerkleBitfield0) ApplicabilityTime() bool {
	return (b & MgaGalOSNMAMerkleApplicabilityTime) != 0
}

type MgaGalUnknown []byte

func (m *MgaGalUnknown) mgaGalPayload() {}

func init() {
	regMsg[MgaIni]("INI")
	regMsg[MgaGal]("GAL")
}
