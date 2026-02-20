package uncmsg

import "github.com/jclark/satpulse/gps/lib/novmsg"

// Message IDs for navigation messages
const (
	BestNavID    MsgID = 2118
	BestNavXYZID MsgID = 240
)

// BestNav represents geodetic position and velocity from a Unicore receiver.
// Message ID: 2118
// Contains both position (LLH) and velocity (ground speed + course) with
// independent solution status for each.
type BestNav struct {
	novmsg.Pos[SolStatus, PosVelType] // position fields (PSolStatus through GPSGLOBDS2Sig)
	VSolStatus   SolStatus            // velocity solution status
	VelType      PosVelType           // velocity type
	VLatency     float32        // velocity latency (seconds)
	VDiffAge     float32        // velocity differential age (seconds)
	HorSpd       float64        // horizontal speed (m/s)
	TrkGnd       float64        // track over ground (degrees, clockwise from north)
	VertSpd      float64        // vertical speed (m/s), positive = up
	VertSpdSigma float32        // vertical speed std dev (m/s)
	HorSpdSigma  float32        // horizontal speed std dev (m/s)
}

// ID returns the message ID for BESTNAV
func (m *BestNav) ID() (MsgID, string) {
	return BestNavID, "BESTNAVA"
}

// BestNavXYZ represents ECEF position and velocity from a Unicore receiver.
// Message ID: 240
// Contains both position (XYZ) and velocity (VX/VY/VZ) with
// independent solution status for each.
type BestNavXYZ struct {
	novmsg.XYZ[SolStatus, PosVelType]
}

// ID returns the message ID for BESTNAVXYZ
func (m *BestNavXYZ) ID() (MsgID, string) {
	return BestNavXYZID, "BESTNAVXYZA"
}

func init() {
	regMsg[BestNav]("BESTNAV")
	regMsg[BestNavXYZ]("BESTNAVXYZ")
}
