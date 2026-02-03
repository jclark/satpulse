package casbin

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

func (m *TimTP) ID() MsgID                    { return TimTPID }
func (m *TimTP) RefTimeGNSS() GNSSID          { return GNSSID(m.RefTime & 0x0F) }
func (m *TimTP) RefTimeBase() TimTPTimeBase   { return TimTPTimeBase(m.RefTime >> 4) }
func (m *TimTP) SetRefTime(gnss GNSSID, base TimTPTimeBase) {
	m.RefTime = TimTPRefTime(gnss) | TimTPRefTime(base<<4)
}

type TimTPRefTime uint8

type TimTPTimeBase uint8

const (
	TimTPTimeBaseUTC  TimTPTimeBase = 0
	TimTPTimeBaseGNSS TimTPTimeBase = 1
)

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
