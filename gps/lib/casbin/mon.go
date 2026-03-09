package casbin

const (
	MonVerID MsgID = clsMon | (0x04 << 8)
)

// MonVer is MON-VER (0x0A 0x04) - receiver version information (64 bytes).
type MonVer struct {
	SwVersion Latin1Z32 // software version string
	HwVersion Latin1Z32 // hardware version string
}

func (m *MonVer) ID() MsgID { return MonVerID }

func init() {
	regMsg[MonVer]("VER")
}
