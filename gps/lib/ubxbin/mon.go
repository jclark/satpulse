package ubxbin

const (
	MonCommsID MsgID = clsMon | (0x36 << 8)
	MonGnssID  MsgID = clsMon | (0x28 << 8)
	MonHwID    MsgID = clsMon | (0x09 << 8)
	MonMsgPPID MsgID = clsMon | (0x06 << 8)
	MonVerID   MsgID = clsMon | (0x04 << 8)
)

type MonCommsTxErrors byte

const (
	MonCommsTxErrMem MonCommsTxErrors = 1 << iota
	MonCommsTxErrAlloc
)

// OutputPort returns the port from which this message was output.
// The bool is false when the port is N/A (value 0).
func (e MonCommsTxErrors) OutputPort() (PortID, bool) {
	v := (e >> 2) & 0x07
	if v == 0 {
		return 0, false
	}
	return PortID(v - 1), true
}

type MonCommsFixed struct {
	Version  byte             `json:"version"`
	NPorts   byte             `json:"nPorts"`
	TxErrors MonCommsTxErrors `json:"txErrors"`
	_        byte
	ProtIds  [4]byte          `json:"protIds"`
}

type MonCommsPortID uint16

const (
	MonCommsPortIDI2C   MonCommsPortID = iota << 8
	MonCommsPortIDUART1
	MonCommsPortIDUART2
	MonCommsPortIDUSB
	MonCommsPortIDSPI
)

func (pid MonCommsPortID) PortID() (PortID, bool) {
	if pid&0xFF != 0 || pid > MonCommsPortIDSPI {
		return 0, false
	}
	return PortID(pid >> 8), true
}

type MonCommsPort struct {
	PortID      MonCommsPortID `json:"portId"`
	TxPending   uint16         `json:"txPending"`
	TxBytes     uint32         `json:"txBytes"`
	TxUsage     byte           `json:"txUsage"`
	TxPeakUsage byte           `json:"txPeakUsage"`
	RxPending   uint16         `json:"rxPending"`
	RxBytes     uint32         `json:"rxBytes"`
	RxUsage     byte           `json:"rxUsage"`
	RxPeakUsage byte           `json:"rxPeakUsage"`
	OverrunErrs uint16         `json:"overrunErrs"`
	Msgs        [4]uint16      `json:"msgs"`
	_           [8]byte
	Skipped     uint32         `json:"skipped"`
}

type MonComms struct {
	MonCommsFixed
	Ports []MonCommsPort `json:"ports"`
}

var _ VaryingMsg = (*MonComms)(nil)
var _ PartiallyHandledMsg = (*MonComms)(nil)

func (m *MonComms) ID() MsgID { return MonCommsID }

func (m *MonComms) IsHandled() bool {
	return m.Version == 0
}

func (m *MonComms) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 40)
	if err == nil {
		m.Ports = make([]MonCommsPort, n)
	}
	return
}

func (m *MonComms) FixedPart() any {
	return &m.MonCommsFixed
}

func (m *MonComms) VaryingPart() any {
	return &m.Ports
}

type MonGnss struct {
	Version      byte             `json:"version"`
	Supported    MonGnssMajorGnss `json:"supported"`
	DefaultGnss  MonGnssMajorGnss `json:"defaultGnss"`
	Enabled      MonGnssMajorGnss `json:"enabled"`
	Simultaneous uint8            `json:"simultaneous"`
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
	PinSel        uint32   `json:"pinSel"`
	PinBank       uint32   `json:"pinBank"`
	PinDir        uint32   `json:"pinDir"`
	PinVal        uint32   `json:"pinVal"`
	NoisePerMS    uint16   `json:"noisePerMS"`
	AgcCnt        uint16   `json:"agcCnt"`
	AStatus       byte     `json:"aStatus"`
	APower        byte     `json:"aPower"`
	Flags         byte     `json:"flags"`
	_             byte
	UsedMask      uint32   `json:"usedMask"`
	VP            [17]byte `json:"VP"`
	CwSuppression byte     `json:"cwSuppression"`
	_             [2]byte
	PinIrq        uint32   `json:"pinIrq"`
	PullH         uint32   `json:"pullH"`
	PullL         uint32   `json:"pullL"`
}

func (m *MonHw) ID() MsgID { return MonHwID }

const nProtocol = 8

type MonMsgPP struct {
	Msg     [NPort][nProtocol]uint16 `json:"msg"`
	Skipped [NPort]uint32            `json:"skipped"`
}

func (m *MonMsgPP) ID() MsgID { return MonMsgPPID }

type MonVerFixed struct {
	SwVersion Latin1Z30 `json:"swVersion"`
	HwVersion Latin1Z10 `json:"hwVersion"`
}

type MonVer struct {
	MonVerFixed
	Extension []Latin1Z30 `json:"extension"`
}

func (m *MonVer) ID() MsgID { return MonVerID }

func (m *MonVer) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 30+10, 30)
	if err == nil {
		m.Extension = make([]Latin1Z30, len)
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
	regMsg[MonComms]("COMMS")
	regMsg[MonGnss]("GNSS")
	regMsg[MonHw]("HW")
	regMsg[MonMsgPP]("MSGPP")
	regMsg[MonVer]("VER")
}
