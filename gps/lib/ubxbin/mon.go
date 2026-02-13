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
	Version  byte
	NPorts   byte
	TxErrors MonCommsTxErrors
	_        byte
	ProtIds  [4]byte
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
	PortID      MonCommsPortID
	TxPending   uint16
	TxBytes     uint32
	TxUsage     byte
	TxPeakUsage byte
	RxPending   uint16
	RxBytes     uint32
	RxUsage     byte
	RxPeakUsage byte
	OverrunErrs uint16
	Msgs        [4]uint16
	_           [8]byte
	Skipped     uint32
}

type MonComms struct {
	MonCommsFixed
	Ports []MonCommsPort
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
	regMsg[MonComms]("COMMS")
	regMsg[MonGnss]("GNSS")
	regMsg[MonHw]("HW")
	regMsg[MonMsgPP]("MSGPP")
	regMsg[MonVer]("VER")
}
