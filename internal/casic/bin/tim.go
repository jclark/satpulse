package bin

const (
	TimTPID MsgID = clsTim | (0x00 << 8)
)

// TimTP is TIM-TP (0x02 0x00) - time pulse information (24 bytes)
type TimTP struct {
	RunTime  uint32  // ms since boot/reset
	QErr     float32 // s, quantization error of next time pulse
	TOW      float64 // s, time of week of next time pulse
	Wn       uint16  // week number of next time pulse
	RefTime  TimTPRefTime
	UTCValid TimTPUTCValid
	_        uint32 // reserved
}

func (m *TimTP) ID() MsgID { return TimTPID }

type TimTPRefTime uint8

// TimeRef returns the GNSS time reference (0=GPS, 1=BDS, 2=GLN)
func (r TimTPRefTime) TimeRef() NavTimeSrc {
	return NavTimeSrc(r & 0x0F)
}

// TimeBase returns true if the time base is GNSS, false if UTC
func (r TimTPRefTime) IsGNSSBase() bool {
	return (r >> 4) != 0
}

type TimTPUTCValid uint8

const (
	TimTPUTCMissing     TimTPUTCValid = 0
	TimTPUTCReserved    TimTPUTCValid = 1
	TimTPUTCExpired     TimTPUTCValid = 2
	TimTPUTCValidParams TimTPUTCValid = 3
)

func init() {
	regMsg[TimTP]("TP")
}
