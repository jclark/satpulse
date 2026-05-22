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

func init() {
	regMsg[RxmCor]("COR")
}
