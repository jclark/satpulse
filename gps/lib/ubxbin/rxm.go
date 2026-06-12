package ubxbin

const (
	RxmCorID MsgID = clsRxm | (0x34 << 8)
)

// RxmCor is UBX-RXM-COR: differential correction input status.
// It is output upon successful parsing of a differential correction input
// message, irrespective of whether the parsed message is supported/used
// by the receiver.
type RxmCor struct {
	Version    byte             `json:"version"`
	EbNo       byte             `json:"ebno"`
	_          [2]byte
	StatusInfo RxmCorStatusInfo `json:"statusInfo"`
	MsgType    uint16           `json:"msgType"`
	MsgSubType uint16           `json:"msgSubType"`
}

var _ PartiallyHandledMsg = (*RxmCor)(nil)

func (m *RxmCor) ID() MsgID { return RxmCorID }

func (m *RxmCor) IsHandled() bool {
	return m.Version == 1
}

type RxmCorStatusInfo uint32

// Boolean flags in StatusInfo.
const (
	RxmCorMsgTypeValid    RxmCorStatusInfo = 1 << 25
	RxmCorMsgSubTypeValid RxmCorStatusInfo = 1 << 26
	RxmCorMsgInputHandle  RxmCorStatusInfo = 1 << 27
)

// Masks to extract multi-bit fields from StatusInfo.
const (
	RxmCorProtocol     RxmCorStatusInfo = 0b11111
	RxmCorErrStatus    RxmCorStatusInfo = 0b11 << 5
	RxmCorMsgUsed      RxmCorStatusInfo = 0b11 << 7
	RxmCorMsgEncrypted RxmCorStatusInfo = 0b11 << 28
	RxmCorMsgDecrypted RxmCorStatusInfo = 0b11 << 30
)

// Protocol values (StatusInfo & RxmCorProtocol).
const (
	RxmCorProtocolUnknown   RxmCorStatusInfo = 0
	RxmCorProtocolRTCM3     RxmCorStatusInfo = 1
	RxmCorProtocolSPARTN    RxmCorStatusInfo = 2
	RxmCorProtocolRxmPMP    RxmCorStatusInfo = 29
	RxmCorProtocolRxmQZSSL6 RxmCorStatusInfo = 30
)

// ErrStatus values (StatusInfo & RxmCorErrStatus).
const (
	RxmCorErrStatusUnknown RxmCorStatusInfo = iota << 5
	RxmCorErrStatusErrorFree
	RxmCorErrStatusErroneous
)

// MsgUsed values (StatusInfo & RxmCorMsgUsed).
const (
	RxmCorMsgUsedUnknown RxmCorStatusInfo = iota << 7
	RxmCorMsgUsedNotUsed
	RxmCorMsgUsedUsed
)

// MsgEncrypted values (StatusInfo & RxmCorMsgEncrypted).
const (
	RxmCorMsgEncryptedUnknown RxmCorStatusInfo = iota << 28
	RxmCorMsgEncryptedNotEncrypted
	RxmCorMsgEncryptedEncrypted
)

// MsgDecrypted values (StatusInfo & RxmCorMsgDecrypted).
const (
	RxmCorMsgDecryptedUnknown RxmCorStatusInfo = iota << 30
	RxmCorMsgDecryptedNotDecrypted
	RxmCorMsgDecryptedDecrypted
)

// CorrectionID extracts the correction stream identifier (bits 9..24).
// For RTCM 3 messages with a reference station ID (DF003), this is the
// station ID in range 0-4095; otherwise 0xFFFF.
func (si RxmCorStatusInfo) CorrectionID() uint16 {
	return uint16((si >> 9) & 0xFFFF)
}

// RxmRawx is UBX-RXM-RAWX: multi-GNSS raw measurements.
// It carries the pseudorange, carrier phase, Doppler, and signal quality
// information needed to generate a RINEX 3 multi-GNSS observation file.
type RxmRawx struct {
	RxmRawxFixed
	Meas []RxmRawxMeas `json:"meas"`
}

// RxmRawxFixed is the fixed part of UBX-RXM-RAWX.
type RxmRawxFixed struct {
	RcvTow  float64        `json:"rcvTow"`  // measurement time of week, s
	Week    uint16         `json:"week"`    // GPS week
	LeapS   int8           `json:"leapS"`   // GPS-UTC leap seconds
	NumMeas uint8          `json:"numMeas"` // number of measurements
	RecStat RxmRawxRecStat `json:"recStat"`
	Version byte           `json:"version"` // 0x01 for this version
	_       [2]byte
}

// RxmRawxMeas is one per-signal measurement in UBX-RXM-RAWX.
type RxmRawxMeas struct {
	PrMes    float64        `json:"prMes"`    // pseudorange, m
	CpMes    float64        `json:"cpMes"`    // carrier phase, cycles
	DoMes    float32        `json:"doMes"`    // Doppler, Hz
	GNSSID   GNSSID         `json:"gnssId"`
	SVID     byte           `json:"svId"`
	SigID    byte           `json:"sigId"`
	FreqID   byte           `json:"freqId"`   // GLONASS only: slot + 7
	LockTime uint16         `json:"locktime"` // carrier phase locktime, ms (max 64500)
	CNO      byte           `json:"cno"`      // carrier-to-noise density, dB-Hz
	PrStdev  byte           `json:"prStdev"`  // bits 3..0 = prStd (0.01*2^n m)
	CpStdev  byte           `json:"cpStdev"`  // bits 3..0 = cpStd (0.004 cycles; 0x0F = invalid)
	DoStdev  byte           `json:"doStdev"`  // bits 3..0 = doStd (0.002*2^n Hz)
	TrkStat  RxmRawxTrkStat `json:"trkStat"`
	_        byte
}

// Masks to extract multi-bit fields from RAWX measurement fields.
const (
	RxmRawxPrStdMask byte = 0b1111 // bits 3..0 of prStdev
	RxmRawxCpStdMask byte = 0b1111 // bits 3..0 of cpStdev
	RxmRawxDoStdMask byte = 0b1111 // bits 3..0 of doStdev
)

var _ VaryingMsg = (*RxmRawx)(nil)
var _ PartiallyHandledMsg = (*RxmRawx)(nil)

func (m *RxmRawx) ID() MsgID { return RxmRawxID }

func (m *RxmRawx) IsHandled() bool {
	return m.Version == 1
}

func (m *RxmRawx) InitVaryingPart(payloadLen int) error {
	n, err := sliceLen(m, payloadLen, 16, 32)
	if err == nil {
		m.Meas = make([]RxmRawxMeas, n)
	}
	return err
}

func (m *RxmRawx) FixedPart() any   { return &m.RxmRawxFixed }
func (m *RxmRawx) VaryingPart() any { return &m.Meas }

// RxmRawxRecStat is the receiver tracking status bitfield in UBX-RXM-RAWX.
type RxmRawxRecStat byte

// Boolean flags in RecStat.
const (
	RxmRawxLeapSec  RxmRawxRecStat = 1 << 0 // leap seconds have been determined
	RxmRawxClkReset RxmRawxRecStat = 1 << 1 // clock reset applied
)

// RxmRawxTrkStat is the per-measurement tracking status bitfield in UBX-RXM-RAWX.
type RxmRawxTrkStat byte

// Boolean flags in TrkStat.
const (
	RxmRawxPrValid    RxmRawxTrkStat = 1 << 0 // pseudorange valid
	RxmRawxCpValid    RxmRawxTrkStat = 1 << 1 // carrier phase valid
	RxmRawxHalfCyc    RxmRawxTrkStat = 1 << 2 // half cycle valid
	RxmRawxSubHalfCyc RxmRawxTrkStat = 1 << 3 // half cycle subtracted from phase
)

func init() {
	regMsg[RxmCor]("COR")
	regMsg[RxmRawx]("RAWX")
}
