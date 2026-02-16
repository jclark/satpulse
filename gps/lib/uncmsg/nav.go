package uncmsg

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
	PSolStatus    SolStatus  // position solution status
	PosType       PosVelType // position type
	Lat           float64    // latitude (degrees)
	Lon           float64    // longitude (degrees)
	Hgt           float64    // height above MSL (meters)
	Undulation    float32    // geoid undulation (meters)
	DatumID       DatumID    // datum ID (61 = WGS84)
	LatSigma      float32    // latitude std dev (meters)
	LonSigma      float32    // longitude std dev (meters)
	HgtSigma      float32    // height std dev (meters)
	StnID         StationID  // base station ID
	DiffAge       float32    // differential data age (seconds)
	SolAge        float32    // solution age (seconds)
	NumSVs        uint8      // satellites tracked
	NumSolnSVs    uint8      // satellites in solution
	NumGGL1       uint8      // satellites with L1/G1/B1 signals
	NumSolnMulti  uint8      // satellites with multi-frequency
	Reserved      uint8      // reserved
	ExtSolStat    HexByte    // extended solution status
	GalBDS3Sig    HexByte    // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig HexByte    // GPS/GLONASS/BDS2 signal mask
	VSolStatus    SolStatus  // velocity solution status
	VelType       PosVelType // velocity type
	VLatency      float32    // velocity latency (seconds)
	VDiffAge      float32    // velocity differential age (seconds)
	HorSpd        float64    // horizontal speed (m/s)
	TrkGnd        float64    // track over ground (degrees, clockwise from north)
	VertSpd       float64    // vertical speed (m/s), positive = up
	VertSpdSigma  float32    // vertical speed std dev (m/s)
	HorSpdSigma   float32    // horizontal speed std dev (m/s)
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
	PSolStatus    SolStatus  // position solution status
	PosType       PosVelType // position type
	PX            float64    // ECEF X (meters)
	PY            float64    // ECEF Y (meters)
	PZ            float64    // ECEF Z (meters)
	PXSigma       float32   // X std dev (meters)
	PYSigma       float32   // Y std dev (meters)
	PZSigma       float32   // Z std dev (meters)
	VSolStatus    SolStatus  // velocity solution status
	VelType       PosVelType // velocity type
	VX            float64    // ECEF VX (m/s)
	VY            float64    // ECEF VY (m/s)
	VZ            float64    // ECEF VZ (m/s)
	VXSigma       float32   // VX std dev (m/s)
	VYSigma       float32   // VY std dev (m/s)
	VZSigma       float32   // VZ std dev (m/s)
	StnID         StationID  // base station ID
	VLatency      float32    // velocity latency (seconds)
	DiffAge       float32    // differential age (seconds)
	SolAge        float32    // solution age (seconds)
	NumSVs        uint8      // satellites tracked
	NumSolnSVs    uint8      // satellites in solution
	NumGGL1       uint8      // satellites with L1/G1/B1 signals
	NumSolnMulti  uint8      // satellites with multi-frequency
	Reserved      uint8      // reserved
	ExtSolStat    HexByte    // extended solution status
	GalBDS3Sig    HexByte    // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig HexByte    // GPS/GLONASS/BDS2 signal mask
}

// ID returns the message ID for BESTNAVXYZ
func (m *BestNavXYZ) ID() (MsgID, string) {
	return BestNavXYZID, "BESTNAVXYZA"
}

func init() {
	regMsg[BestNav]("BESTNAV")
	regMsg[BestNavXYZ]("BESTNAVXYZ")
}
