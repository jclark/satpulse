package sdbpbin

const (
	PubAckID MsgID = clsPUB | (0x01 << 8)
	PubNakID MsgID = clsPUB | (0x02 << 8)
)

// PubAck is PUB-ACK (class 0x01, id 0x01).
type PubAck struct {
	Class uint8 // class of responded command
	MsgID uint8 // id of responded command
}

func (m *PubAck) ID() MsgID { return PubAckID }

// PubNak is PUB-NAK (class 0x01, id 0x02).
type PubNak struct {
	Class uint8 // class of responded command
	MsgID uint8 // id of responded command
}

func (m *PubNak) ID() MsgID { return PubNakID }

func init() {
	regMsg[PubAck]("ACK")
	regMsg[PubNak]("NAK")
}
