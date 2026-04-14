package asbin

import "github.com/jclark/satpulse/gps/lib/latin1z"

const (
	MonVerID MsgID = clsMon | (0x04 << 8)
)

// MON-VER (0x0A 0x04)
type MonVer struct {
	SwVersion latin1z.StringZ16 // Software version string
	HwVersion latin1z.StringZ16 // Hardware version string
}

func (m *MonVer) ID() MsgID { return MonVerID }

func init() {
	regMsg[MonVer]("VER")
}
