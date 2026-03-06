package asbin

const (
	MonVerID MsgID = clsMon | (0x04 << 8)
)

// MON-VER (0x0A 0x04)
type MonVer struct {
	SwVersion Latin1Z16 // Software version string
	HwVersion Latin1Z16 // Hardware version string
}

func (m *MonVer) ID() MsgID { return MonVerID }

func init() {
	regMsg[MonVer]("VER")
}
