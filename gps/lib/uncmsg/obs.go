package uncmsg

import (
	"fmt"
	"strconv"

	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// TrackingStatus represents the OBSVM channel tracking status bitfield.
type TrackingStatus uint32

// String returns the OBSVM ASCII hex representation.
func (s TrackingStatus) String() string {
	return fmt.Sprintf("%08x", uint32(s))
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (s *TrackingStatus) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 16, 32)
	if err != nil {
		return err
	}
	*s = TrackingStatus(n)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (s TrackingStatus) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// ObsVMFixed contains the fixed-length fields of the OBSVM message.
type ObsVMFixed struct {
	ObsNumber uint32
}

// ObsVM represents raw observations from the master antenna.
type ObsVM struct {
	ObsVMFixed
	Obs []ObsVMObs
}

var _ novmsg.Chunked = (*ObsVM)(nil)

// ObsVMObs represents one frequency observation in an OBSVM epoch.
type ObsVMObs struct {
	SystemFreq uint16
	PRN        uint16
	PSR        float64
	ADR        float64
	PSRStd     uint16
	ADRStd     uint16
	Dopp       float32
	CN0        uint16
	Reserved   uint16
	LockTime   float32
	ChTrStatus TrackingStatus
}

// ID returns the message ID for OBSVM.
func (m *ObsVM) ID() (MsgID, string) {
	return ObsvMID, "OBSVMA"
}

// Chunks implements the novmsg.Chunked interface for ObsVM.
func (m *ObsVM) Chunks() func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		m.ObsNumber = uint32(len(m.Obs))
		if !yield(&m.ObsVMFixed) {
			return
		}
		if len(m.Obs) == 0 && m.ObsNumber > 0 {
			m.Obs = make([]ObsVMObs, m.ObsNumber)
		}
		for i := range m.Obs {
			if !yield(&m.Obs[i]) {
				return
			}
		}
	}
}

func init() {
	regMsg[ObsVM]("OBSVM")
}
