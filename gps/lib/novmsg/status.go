package novmsg

// ByCheckID is the binary message ID of the BYCHECK log.
// ByNav's manual does not give it; this is the ID used by the M10.
// BYCHECK is ByNav's own log, so it is not registered: only the ByNav
// variant of the NovAtel packet processors decodes it.
const ByCheckID MsgID = 42272 // Bynav

// ByCheckFlag is the value of a BYCHECK self-check item.
type ByCheckFlag int32

const (
	ByCheckFalse   ByCheckFlag = 0
	ByCheckTrue    ByCheckFlag = 1
	ByCheckUnknown ByCheckFlag = 2
)

// ByCheck represents the ByNav BYCHECK (GNSS self-check) log.
// Message ID: 42272
// The self-check items are mostly about antenna and RTK base/rover health.
// ByNav's manual gives the offset of BaseStationPosition as H+38, but the
// items are contiguous 4-byte fields, as M10 output confirms.
// The manual lists two further items, Valid Baseline and Differential Input,
// which only the X2 series outputs; those are not supported.
type ByCheck struct {
	Runtime             int32       // Receiver runtime (s)
	Week                int32       // GPS week
	Sow                 float32     // Second of week
	DualFrequency       ByCheckFlag // Antenna supports dual frequency
	AntennaVoltage      ByCheckFlag // Antenna is powered up
	GloFrequencyDiff    ByCheckFlag // GLONASS bias is corrected
	WorkFrequency       ByCheckFlag // Work frequencies of the base and the rover match
	BaseStationPosition ByCheckFlag // Base's position is received by the rover
	BaseAntennaBlock    ByCheckFlag // Base's antenna is not blocked
	DiffLink            ByCheckFlag // Correction data link is stable
	DualBase            ByCheckFlag // No multiple base RTCM is received
	BoardTemperature    ByCheckFlag // Board temperature is normal
	RoverAntennaBlock   ByCheckFlag // Rover antenna is not blocked
	DifferentialData    ByCheckFlag // Differential data is valid
	BasePositionError   ByCheckFlag // Base's position is OK
}

func (m *ByCheck) ID() (MsgID, string) {
	return ByCheckID, "BYCHECKA"
}
