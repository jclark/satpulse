package casbin

import "github.com/jclark/satpulse/gps/lib/latin1z"

const (
	MonVerID MsgID = clsMon | (0x04 << 8)
)

// MonVer is MON-VER (0x0A 0x04) - receiver version information (64 bytes).
type MonVer struct {
	SwVersion latin1z.StringZ32 `json:"swVersion"` // software version string
	HwVersion latin1z.StringZ32 `json:"hwVersion"` // hardware version string
}

func (m *MonVer) ID() MsgID { return MonVerID }

func init() {
	regMsg[MonVer]("VER")
}
