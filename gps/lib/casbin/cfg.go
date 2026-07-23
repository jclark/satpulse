package casbin

// CfgTMode2Mode selects the timing mode.
type CfgTMode2Mode uint8

const (
	CfgTMode2Realtime CfgTMode2Mode = iota // normal positioning
	CfgTMode2Survey                        // auto-survey
	CfgTMode2Fixed                         // fixed position
)

// CfgTMode2Band selects the signal band.
type CfgTMode2Band uint8

const (
	CfgTMode2BandL1B1I CfgTMode2Band = iota
	CfgTMode2BandL1
	CfgTMode2BandL2
	CfgTMode2BandL5
	CfgTMode2BandMulti
)

// CfgTMode2AntDetMode selects how antenna detection is performed.
type CfgTMode2AntDetMode uint8

const (
	CfgTMode2AntDetInternal CfgTMode2AntDetMode = iota
	CfgTMode2AntDetExternalPin
)

// CfgTMode2TimeSource selects the GNSS used for timing. The priority
// modes fall back to another timing system when their preferred system
// is unavailable.
type CfgTMode2TimeSource uint8

const (
	CfgTMode2TimeSourceForceGPS CfgTMode2TimeSource = iota
	CfgTMode2TimeSourceForceBDS
	CfgTMode2TimeSourceForceGLN
	CfgTMode2TimeSourceForceGAL
	CfgTMode2TimeSourcePriorityBDS
	CfgTMode2TimeSourcePriorityGPS
	CfgTMode2TimeSourcePriorityGLN
	CfgTMode2TimeSourcePriorityGAL
)

const (
	// CfgTMode2FixedPositionScale is the number of wire units per metre.
	CfgTMode2FixedPositionScale float64 = 100
	// CfgTMode2PositionAccuracyScale is the number of wire units per metre.
	CfgTMode2PositionAccuracyScale float64 = 1000
)

// CfgTMode2 is CFG-TMODE2 (0x06 0x16) - timing mode configuration (28 bytes)
type CfgTMode2 struct {
	TimFixMode  CfgTMode2Mode       `json:"timFixMode"`  // 0=realtime, 1=survey, 2=fixed
	BandMode    CfgTMode2Band       `json:"bandMode"`    // signal band selection
	AntDetMode  CfgTMode2AntDetMode `json:"antDetMode"`  // antenna-detection source
	TSrcMode    CfgTMode2TimeSource `json:"tsrc_mode"`   // timing GNSS selection
	XFixed      int32               `json:"xFixed"`      // 0.01 m, ECEF X
	YFixed      int32               `json:"yFixed"`      // 0.01 m, ECEF Y
	ZFixed      int32               `json:"zFixed"`      // 0.01 m, ECEF Z
	FixedPacc   uint32              `json:"fixedPacc"`   // mm, position accuracy
	SvinMinDur  uint32              `json:"svinMinDur"`  // s, min survey-in duration
	SvinPaccLim uint32              `json:"svinPaccLim"` // mm, survey-in accuracy limit
}

func (m *CfgTMode2) ID() MsgID { return CfgTMode2ID }

// PollPayloadLen returns the maximum payload length that indicates
// a poll for mid.
func PollPayloadLen(mid MsgID) int {
	switch mid {
	case CfgMsgID:
		return 4 // class + ID + rate (0xFFFF)
	default:
		return 0
	}
}

func init() {
	regMsg[CfgTMode2]("TMODE2")
}
