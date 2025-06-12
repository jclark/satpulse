package bin

const (
	AckNakID MsgID = clsAck | (0x00 << 8)
	AckAckID MsgID = clsAck | (0x01 << 8)
)

type AckNak struct {
	MsgID MsgID
}

func (m *AckNak) ID() MsgID { return AckNakID }

type AckAck struct {
	MsgID MsgID
}

func (m *AckAck) ID() MsgID { return AckAckID }

func init() {
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
}
