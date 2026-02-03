package asbin

const (
	MonVerID MsgID = clsMon | (0x04 << 8)
)

// MON-VER (0x0A 0x04)
type MonVer struct {
	SwVersion [16]byte // Software version string
	HwVersion [16]byte // Hardware version string
}

func (m *MonVer) ID() MsgID { return MonVerID }

func init() {
	regMsg[MonVer]("VER")
}
