package novmsg

// Message IDs for navigation messages
const (
	BestPosID     MsgID = 42
	BestVelID     MsgID = 99
	BestXYZID     MsgID = 241
	BestGNSSVelID MsgID = 1430
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

// BestVel represents geodetic velocity from a NovAtel-format receiver.
// Message ID: 99
type BestVel struct {
	Vel[PosType]
}

// ID returns the message ID for BESTVEL
func (m *BestVel) ID() (MsgID, string) {
	return BestVelID, "BESTVELA"
}

// BestGNSSVel represents GNSS-only velocity from a NovAtel OEM7/ByNav receiver.
// Message ID: 1430
type BestGNSSVel struct {
	Vel[PosType]
}

// ID returns the message ID for BESTGNSSVEL
func (m *BestGNSSVel) ID() (MsgID, string) {
	return BestGNSSVelID, "BESTGNSSVELA"
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
	regMsg[BestVel]("BESTVEL")
	regMsg[BestGNSSVel]("BESTGNSSVEL")
	regMsg[BestXYZ]("BESTXYZ")
}
