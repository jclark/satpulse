package asbin

const (
	AckNakID MsgID = clsAck | (0x00 << 8)
	AckAckID MsgID = clsAck | (0x01 << 8)
)

// ACK-NAK (0x05 0x00) and ACK-ACK (0x05 0x01)
type Ack struct {
	MsgClass uint8 `json:"groupID"` // Message class
	MsgID    uint8 `json:"subID"`   // Message ID
}

func (m *Ack) ID() MsgID { return AckNakID } // This would be overridden for AckAck

type AckAck Ack

func (m *AckAck) ID() MsgID { return AckAckID }

type AckNak Ack

func (m *AckNak) ID() MsgID { return AckNakID }

func init() {
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
}
