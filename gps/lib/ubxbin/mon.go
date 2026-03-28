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

// MonGnss1Fixed is the fixed part of MON-GNSS version 1 (signal plans).
type MonGnss1Fixed struct {
	Version        byte   `json:"version"`
	NumPlans       byte   `json:"numPlans"`
	ActivePlanInfo uint16 `json:"activePlanInfo"`
}

// MonGnss1Plan is one signal plan entry (28 bytes).
type MonGnss1Plan struct {
	ID       uint8    `json:"id"`
	Name     Latin1Z5 `json:"name"`
	GpsSup   uint16   `json:"gpsSup"`
	GalSup   uint16   `json:"galSup"`
	BdsSup   uint16   `json:"bdsSup"`
	GloSup   uint16   `json:"gloSup"`
	SbasSup  uint16   `json:"sbasSup"`
	QzssSup  uint16   `json:"qzssSup"`
	NavicSup uint16   `json:"navicSup"`
	LbandSup uint16   `json:"lbandSup"`
	_        [6]byte
}

// MonGnss1 represents UBX-MON-GNSS version 1, which reports signal plans.
type MonGnss1 struct {
	MonGnss1Fixed
	Plans []MonGnss1Plan `json:"plans"`
}

var _ VaryingMsg = (*MonGnss1)(nil)
var _ PartiallyHandledMsg = (*MonGnss1)(nil)

func (m *MonGnss1) ID() MsgID { return MonGnssID }

func (m *MonGnss1) IsHandled() bool {
	return m.Version == 1
}

func (m *MonGnss1) FixedPart() any { return &m.MonGnss1Fixed }

func (m *MonGnss1) VaryingPart() any { return &m.Plans }

func (m *MonGnss1) InitVaryingPart(payloadLen int) error {
	n, err := sliceLen(m, payloadLen, 4, 28)
	if err == nil {
		m.Plans = make([]MonGnss1Plan, n)
	}
	return err
}

// ActivePlanID returns the configuration ID of the active signal plan.
func (m *MonGnss1) ActivePlanID() byte {
	return byte(m.ActivePlanInfo & 0xFF)
}

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
	regMsg[MonGnss1]("GNSS")
	regMsg[MonHw]("HW")
	regMsg[MonMsgPP]("MSGPP")
	regMsg[MonVer]("VER")
}
