package novmsg

// Message IDs for navigation messages
const BestPosID MsgID = 42

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

func init() {
	regMsg[BestPos]("BESTPOS")
}
