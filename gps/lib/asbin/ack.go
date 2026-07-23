package asbin

const (
	AckNakID MsgID = clsAck | (0x00 << 8)
	AckAckID MsgID = clsAck | (0x01 << 8)
)

// ACK-NAK (0x05 0x00)
type AckNak struct {
	MsgClass uint8 `json:"groupID"` // Message class
	MsgID    uint8 `json:"subID"`   // Message ID
}

func (m *AckNak) ID() MsgID { return AckNakID }

// ACK-ACK (0x05 0x01)
type AckAck struct {
	MsgClass uint8 `json:"groupID"` // Message class
	MsgID    uint8 `json:"subID"`   // Message ID
}

func (m *AckAck) ID() MsgID { return AckAckID }

func init() {
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
}
