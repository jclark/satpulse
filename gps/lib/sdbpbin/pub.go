package sdbpbin

// PubAck is PUB-ACK (class 0x01, id 0x01).
type PubAck struct {
	Class uint8 // class of responded command
	MsgID uint8 // id of responded command
}

func (m *PubAck) ID() MsgID { return makeMsgID(clsPUB, 0x01) }

// PubNak is PUB-NAK (class 0x01, id 0x02).
type PubNak struct {
	Class uint8 // class of responded command
	MsgID uint8 // id of responded command
}

func (m *PubNak) ID() MsgID { return makeMsgID(clsPUB, 0x02) }

func init() {
	regMsg[PubAck]("ACK")
	regMsg[PubNak]("NAK")
}
