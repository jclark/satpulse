package sdbpbin

func init() {
	regMsg[PubAck]("ACK")
	regMsg[PubNak]("NAK")
}

// PubAck is PUB-ACK (class 0x01, id 0x01).
type PubAck struct {
	AckedClass uint8 // type number of responded command
	AckedID    uint8 // function number of responded command
}

func (m *PubAck) ID() MsgID { return makeMsgID(clsPUB, 0x01) }

// PubNak is PUB-NAK (class 0x01, id 0x02).
type PubNak struct {
	NakedClass uint8 // type number of responded command
	NakedID    uint8 // function number of responded command
}

func (m *PubNak) ID() MsgID { return makeMsgID(clsPUB, 0x02) }
