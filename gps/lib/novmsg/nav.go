package novmsg

// Message IDs for navigation messages
const (
	BestPosID MsgID = 42
	BestXYZID MsgID = 241
)

// BestPos represents geodetic position from a NovAtel-format receiver.
// Message ID: 42
// Binary layout is identical across OEM7, ByNav, SinoGNSS, and Unicore;
// the SolStatus and PosType enum value sets vary by vendor.
type BestPos struct {
	Pos[SolStatus, PosType]
}

// ID returns the message ID for BESTPOS
func (m *BestPos) ID() (MsgID, string) {
	return BestPosID, "BESTPOSA"
}

// BestXYZ represents ECEF position and velocity from a NovAtel-format receiver.
// Message ID: 241
// Binary layout is identical across OEM7, ByNav, SinoGNSS, and Unicore;
// the SolStatus and PosType enum value sets vary by vendor.
type BestXYZ struct {
	XYZ[SolStatus, PosType]
}

// ID returns the message ID for BESTXYZ
func (m *BestXYZ) ID() (MsgID, string) {
	return BestXYZID, "BESTXYZA"
}

func init() {
	regMsg[BestPos]("BESTPOS")
	regMsg[BestXYZ]("BESTXYZ")
}
