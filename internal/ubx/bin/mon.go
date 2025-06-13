package bin

const (
	MonGnssID  MsgID = clsMon | (0x28 << 8)
	MonHwID    MsgID = clsMon | (0x09 << 8)
	MonMsgPPID MsgID = clsMon | (0x06 << 8)
	MonVerID   MsgID = clsMon | (0x04 << 8)
)

type MonGnss struct {
	Version      byte
	Supported    MonGnssMajorGnss
	DefaultGnss  MonGnssMajorGnss
	Enabled      MonGnssMajorGnss
	Simultaneous uint8
	_            [3]byte
}

var _ PartiallyHandledMsg = (*MonGnss)(nil)

func (m *MonGnss) ID() MsgID { return MonGnssID }

func (m *MonGnss) IsHandled() bool {
	return m.Version == 0
}

type MonGnssMajorGnss uint8

const (
	MonGnssGPS MonGnssMajorGnss = 1 << iota
	MonGnssGlonass
	MonGnssBeidou
	MonGnssGalileo
)

type MonHw struct {
	PinSel        uint32
	PinBank       uint32
	PinDir        uint32
	PinVal        uint32
	NoisePerMS    uint16
	AgcCnt        uint16
	AStatus       byte
	APower        byte
	Flags         byte
	_             byte
	UsedMask      uint32
	VP            [17]byte
	CwSuppression byte
	_             [2]byte
	PinIrq        uint32
	PullH         uint32
	PullL         uint32
}

func (m *MonHw) ID() MsgID { return MonHwID }

const nProtocol = 8

type MonMsgPP struct {
	Msg     [NPort][nProtocol]uint16
	Skipped [NPort]uint32
}

func (m *MonMsgPP) ID() MsgID { return MonMsgPPID }

type MonVerFixed struct {
	SwVersion [30]byte
	HwVersion [10]byte
}

type MonVer struct {
	MonVerFixed
	Extension [][30]byte
}

func (m *MonVer) ID() MsgID { return MonVerID }

func (m *MonVer) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 30+10, 30)
	if err == nil {
		m.Extension = make([][30]byte, len)
	}
	return
}

func (m *MonVer) FixedPart() any {
	return &m.MonVerFixed
}

func (m *MonVer) VaryingPart() any {
	return &m.Extension
}

func init() {
	regMsg[MonGnss]("GNSS")
	regMsg[MonHw]("HW")
	regMsg[MonMsgPP]("MSGPP")
	regMsg[MonVer]("VER")
}
