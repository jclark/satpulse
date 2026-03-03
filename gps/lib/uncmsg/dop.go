package uncmsg

import "github.com/jclark/satpulse/gps/lib/novmsg"

// StaDOPID is the message ID for STADOP
const StaDOPID MsgID = 954

// StaDOPFixed contains the fixed-length fields of the STADOP message
type StaDOPFixed struct {
	Reserved  uint32
	GDOP      float32
	PDOP      float32
	TDOP      float32
	VDOP      float32
	HDOP      float32
	NDOP      float32
	EDOP      float32
	Cutoff    float32
	Reserved2 float32
	PRNCount  uint16
}

// StaDOP represents DOP values for the BESTNAV solution (message ID 954)
type StaDOP struct {
	StaDOPFixed
	PRNs []uint16
}

var _ novmsg.Chunked = (*StaDOP)(nil)

// staDOPPRN wraps a single PRN for chunk-based reading/writing
type staDOPPRN struct {
	PRN uint16
}

// ID returns the message ID for STADOP
func (s *StaDOP) ID() (MsgID, string) {
	return StaDOPID, "STADOPA"
}

// Chunks implements the novmsg.Chunked interface for StaDOP
func (s *StaDOP) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		s.PRNCount = uint16(len(s.PRNs))
		if !yield(&s.StaDOPFixed) {
			return
		}
		if len(s.PRNs) == 0 && s.PRNCount > 0 {
			s.PRNs = make([]uint16, s.PRNCount)
		}
		for i := range s.PRNs {
			prn := staDOPPRN{PRN: s.PRNs[i]}
			if !yield(&prn) {
				return
			}
			s.PRNs[i] = prn.PRN
		}
	}
}

func init() {
	regMsg[StaDOP]("STADOP")
}
