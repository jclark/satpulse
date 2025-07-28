package uncmsg

// Message IDs for time-related messages
const (
	PPSStatusID MsgID = 9000
)

// PPSStatus represents the PPS status information from a Unicore receiver.
// Message ID: 9000
// It only supports 1 Hz output.
type PPSStatus struct {
	Status       uint32 // PPS output status. 0: No PPS output, 1: Accuracy not guaranteed, 2: Accuracy ~100-1000 ns, 3: Accuracy < 100 ns
	Week         uint32 // PPS week (aligned with GPS Week)
	MsSecInWeek  uint32 // PPS milliseconds in the week (aligned with GPS MsCount)
	PPSPulseErr  int32  // PPS phase error (ns)
	OffsetTime   int32  // PPS offset time relative to observation time (0.01 ns)
	ConfigInfo   uint32 // PPS configuration information (Hex)
	Register     uint32 // Hardware register status
	TimeEstErr   int32  // DeltaT time estimated error
	InnerQuality uint32 // Inner positioning and time quality indicator
	PPSInnerSta  uint32 // PPS inner status
	PPSCfgSta    uint32 // PPS config status
	PPSStage     uint32 // PPS stage
	Reserved1    uint32 // Reserved
	Reserved2    uint32 // Reserved
	Reserved3    uint32 // Reserved
}

// ID returns the message ID for PPSSTATUS
func (p *PPSStatus) ID() MsgID {
	return PPSStatusID
}

func init() {
	// Register known message types
	regMsg[PPSStatus]("PPSSTATUS")
}
