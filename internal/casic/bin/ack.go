package bin

const (
	AckNakID MsgID = clsAck | (0x00 << 8)
	AckAckID MsgID = clsAck | (0x01 << 8)
)

// AckPayload is the common payload for ACK-NAK and ACK-ACK messages.
// Payload: clsID (U1), msgID (U1), res (U2).
type AckPayload struct {
	ClsID uint8
	MsgID uint8
	_     uint16 // reserved
}

// AckNak is sent when a CFG message was not processed correctly.
type AckNak struct{ AckPayload }

func (m *AckNak) ID() MsgID { return AckNakID }

// AckAck is sent when a CFG message was processed correctly.
type AckAck struct{ AckPayload }

func (m *AckAck) ID() MsgID { return AckAckID }

func init() {
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
}
