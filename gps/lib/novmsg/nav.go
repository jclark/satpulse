package novmsg

// Message IDs for navigation messages
const (
	BestPosID     MsgID = 42
	BestVelID     MsgID = 99
	PsrDopID      MsgID = 174 // OEM7, Bynav, SinoGNSS
	BestXYZID     MsgID = 241
	BestGNSSVelID MsgID = 1430
)

// PsrDopInitChunk is the initial chunk of a PSRDOP message.
type PsrDopInitChunk struct {
	GDOP    float32
	PDOP    float32
	HDOP    float32
	HTDOP   float32 // combined horizontal+time DOP (not stored in gpsprot.DOP)
	TDOP    float32
	Cutoff  float32 // elevation cutoff angle (degrees)
	NumPRNs uint32
}

// PsrDopPRN wraps a single PRN entry for binary/ASCII chunk serialization.
type PsrDopPRN struct {
	PRN uint32
}

// PsrDop represents pseudorange DOP from a NovAtel-format receiver.
// Message ID: 174
// Contains GDOP, PDOP, HDOP, TDOP, and a variable-length list of PRNs.
// VDOP is not provided; derive as sqrt(PDOP^2 - HDOP^2).
type PsrDop struct {
	PsrDopInitChunk
	PRNs []PsrDopPRN
}

// ID returns the message ID for PSRDOP.
func (m *PsrDop) ID() (MsgID, string) {
	return PsrDopID, "PSRDOPA"
}

// Chunks implements the novmsg.Chunked interface for PsrDop.
func (m *PsrDop) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		if !yield(&m.PsrDopInitChunk) {
			return
		}
		if len(m.PRNs) == 0 {
			m.PRNs = make([]PsrDopPRN, m.NumPRNs)
		}
		for i := range m.PRNs {
			if !yield(&m.PRNs[i]) {
				return
			}
		}
	}
}

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

// SinoBestPos represents geodetic position from a SinoGNSS receiver.
// Same binary layout as BestPos but with SinoGNSS-specific PosType enum values.
// NOT registered via init() -- shares message ID 42 with BestPos and is only
// used when the SinoGNSS variant builds its constructor map.
type SinoBestPos struct {
	Pos[SolStatus, SinoPosType]
}

// ID returns the message ID for BESTPOS.
func (m *SinoBestPos) ID() (MsgID, string) {
	return BestPosID, "BESTPOSA"
}

// SinoBestXYZ represents ECEF position and velocity from a SinoGNSS receiver.
// Same binary layout as BestXYZ but with SinoGNSS-specific PosType enum values.
// NOT registered via init() -- shares message ID 241 with BestXYZ and is only
// used when the SinoGNSS variant builds its constructor map.
type SinoBestXYZ struct {
	XYZ[SolStatus, SinoPosType]
}

// ID returns the message ID for BESTXYZ.
func (m *SinoBestXYZ) ID() (MsgID, string) {
	return BestXYZID, "BESTXYZA"
}

func init() {
	regMsg[BestPos]("BESTPOS")
	regMsg[BestVel]("BESTVEL")
	regMsg[BestGNSSVel]("BESTGNSSVEL")
	regMsg[PsrDop]("PSRDOP")
	regMsg[BestXYZ]("BESTXYZ")
}
