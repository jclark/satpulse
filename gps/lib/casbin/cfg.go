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

// CfgTMode2 is CFG-TMODE2 (0x06 0x16) - timing mode configuration (28 bytes)
type CfgTMode2 struct {
	TimFixMode CfgTMode2Mode // 0=realtime, 1=survey, 2=fixed
	BandMode   CfgTMode2Band // signal band selection
	AntDetMode uint8         // 0=internal, 1=external pin
	TSrcMode   uint8         // 0-3=force single sys, 4-7=priority sys
	XFixed     int32         // 0.01 m, ECEF X
	YFixed     int32         // 0.01 m, ECEF Y
	ZFixed     int32         // 0.01 m, ECEF Z
	FixedPacc  uint32        // mm, position accuracy
	SvinMinDur uint32        // s, min survey-in duration
	SvinPaccLim uint32       // mm, survey-in accuracy limit
}

func (m *CfgTMode2) ID() MsgID { return CfgTMode2ID }

func init() {
	regMsg[CfgTMode2]("TMODE2")
}
