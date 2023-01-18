package ubx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
)

const (
	Sync1 = 0xB5
	Sync2 = 0x62
)

type MsgID uint16

const (
	clsNav = 0x01
	clsAck = 0x05
	clsCfg = 0x06
	clsMon = 0x0A
	clsTim = 0x0D
)

var clsMap = map[byte]string{
	clsNav: "nav",
	clsAck: "ack",
	clsCfg: "cfg",
	clsMon: "mon",
	clsTim: "tim",
}

func NavMsgID(id byte) MsgID { return makeMsgID(clsNav, id) }
func AckMsgID(id byte) MsgID { return makeMsgID(clsAck, id) }
func CfgMsgID(id byte) MsgID { return makeMsgID(clsCfg, id) }
func MonMsgID(id byte) MsgID { return makeMsgID(clsMon, id) }
func TimMsgID(id byte) MsgID { return makeMsgID(clsTim, id) }

func makeMsgID(cls byte, id byte) MsgID {
	return MsgID(uint16(cls) | (uint16(id) << 8))
}

func (mid MsgID) unpack() (byte, byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

type Msg interface {
	ID() MsgID
}

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)

func (mid MsgID) String() string {
	idName := idNameMap[mid]
	cls, id := mid.unpack()
	s := clsMap[cls]
	if s == "" {
		s = fmt.Sprintf("0x%02x", cls)
	}
	s += "-"
	if idName != "" {
		s += idName
	} else {
		s += fmt.Sprintf("0x%02x", id)
	}
	return s
}

type AckNak struct {
	MsgID MsgID
}

func (m *AckNak) ID() MsgID { return AckMsgID(0x00) }

type AckAck struct {
	MsgID MsgID
}

func (m *AckAck) ID() MsgID { return AckMsgID(0x01) }

type NavTimeGPS struct {
	ITOW  uint32
	FTOW  int32
	Week  int16
	LeapS byte
	Valid byte
	TAcc  uint32
}

func (m *NavTimeGPS) ID() MsgID { return NavMsgID(0x20) }

type NavTimeUTC struct {
	ITOW  uint32
	TAcc  uint32
	Nano  int32
	Year  uint16
	Month byte
	Day   byte
	Hour  byte
	Min   byte
	Sec   byte
	Valid byte
}

func (m *NavTimeUTC) ID() MsgID { return NavMsgID(0x21) }

type NavTimeBDS struct {
	ITOW  uint32
	SOW   uint32
	FSOW  int32
	Week  int16
	LeapS byte
	Valid byte
	TAcc  uint32
}

func (m *NavTimeBDS) ID() MsgID { return NavMsgID(0x24) }

type NavTimeLS struct {
	ITOW          uint32
	Version       byte
	_             [3]byte
	SrcOfCurrLS   byte
	CurrLS        byte
	SrcOfLSChange byte
	LSChange      int8
	TimeToLSEvent int32
	DateOfLSGPSWN uint16
	DateOfLSGPSDN uint16
	_             [3]byte
	Valid         byte
}

func (m *NavTimeLS) ID() MsgID { return NavMsgID(0x26) }

type TimTP struct {
	TowMS    uint32
	TowSubMS uint32
	QErr     int32
	Week     uint16
	Flags    byte
	RefInfo  byte
}

func (m *TimTP) ID() MsgID { return TimMsgID(0x01) }

const (
	TimTPFlagTimeBase = 1 << iota
	TimTPFlagUTC
	TimTPFlagRAIM
	TimTPFlagQErr
)

const (
	TimTPRefGPS = iota
	TimTPRefGLONASS
	TimTPRefBeiDou
	TimTPRefGalileo
	TimTPRefNavIC
	TimTPRefUnknown = 15
)

const (
	TimTPUTCRL = iota + 1
	TimTPUTCNIST
	TimTPUTCUSNO
	TimTPUTCBIPM
	TimTPUTCEU
	TimTPUTCSU
	TimTPUTCNTSC
	TimTPUTCNPLI
	TimTPUTCUnknown = 15
)

type TimSvIn struct {
	Dur    uint32
	MeanX  int32
	MeanY  int32
	MeanZ  int32
	MeanV  uint32
	Obs    uint32
	Valid  byte
	Active byte
	_      [2]byte
}

func (m *TimSvIn) ID() MsgID { return TimMsgID(0x04) }

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

func (m *MonHw) ID() MsgID { return MonMsgID(0x09) }

type MonVerFixed struct {
	SwVersion [30]byte
	HwVersion [10]byte
}

type MonVer struct {
	MonVerFixed
	Extension [][30]byte
}

func (m *MonVer) ID() MsgID { return MonMsgID(0x04) }

func (m *MonVer) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 30+10, 30)
	if err == nil {
		m.Extension = make([][30]byte, len)
	}
	return
}

func (m *MonVer) Parts() (fixed any, slice any) {
	fixed = &m.MonVerFixed
	slice = &m.Extension
	return
}

type VarLengthMsg interface {
	Msg
	InitForLen(payloadLen int) error
	Parts() (fixed any, slice any)
}

// Use VarLengthMsg here to help ensure that InitForLen is correctly declared for each type
func sliceLen(m VarLengthMsg, payloadLen, minLen, elemLen int) (int, error) {
	extraLen := payloadLen - minLen
	if extraLen < 0 || extraLen%elemLen != 0 {
		return 0, fmt.Errorf("bad %v payload length (%d bytes)", m.ID(), payloadLen)
	}
	return extraLen / elemLen, nil
}

func init() {
	regMsg[AckNak]("nak")
	regMsg[AckAck]("ack")
	regMsg[NavTimeGPS]("timegps")
	regMsg[NavTimeBDS]("timebds")
	regMsg[NavTimeUTC]("timeutc")
	regMsg[NavTimeLS]("timels")
	regMsg[TimTP]("tp")
	regMsg[TimSvIn]("svin")
	regMsg[MonHw]("hw")
	regMsg[MonVer]("ver")
}

func regMsg[T any, PT interface {
	ID() MsgID
	*T
}](idName string) {
	m := PT(new(T))
	mid := m.ID()
	msgMap[mid] = func() Msg { return PT(new(T)) }
	idNameMap[mid] = idName
}

// 2 bytes sync, 2 bytes clsid, 2 bytes length, 2 bytes checksum
const frameMinLength = 8

func ParseMsg(frame []byte) (Msg, error) {
	n := len(frame)
	if n < frameMinLength {
		return nil, fmt.Errorf("UBX message too short (length %d bytes)", n)
	}
	checksumIndex := n - 2
	trimmed := frame[2:checksumIndex]
	ckA, ckB := checksum(trimmed)
	if ckA != frame[checksumIndex] || ckB != frame[checksumIndex+1] {
		return nil, fmt.Errorf("ubx message: invalid checksum")
	}
	mid := makeMsgID(trimmed[0], trimmed[1])
	ctor := msgMap[mid]
	payload := trimmed[4:]
	if ctor == nil {
		// fmt.Printf("unknown UBX message %s\n", clsId)
		// XXX return a message with bytes as payload
		return nil, nil
	}
	msg := ctor()
	var fixed, slice any
	if vMsg, ok := msg.(VarLengthMsg); ok {
		err := vMsg.InitForLen(len(payload))
		if err != nil {
			return nil, err
		}
		fixed, slice = vMsg.Parts()
	} else {
		fixed = msg
		slice = nil
	}
	r := bytes.NewReader(payload)
	err := binary.Read(r, binary.LittleEndian, fixed)
	if err == nil && slice != nil {
		err = binary.Read(r, binary.LittleEndian, slice)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing ubx-%s: %v", mid.String(), err)
	}
	return msg, nil
}

func Serialize(msg Msg) ([]byte, error) {
	buf := new(bytes.Buffer)
	var v any
	if vMsg, ok := msg.(VarLengthMsg); ok {
		fixed, slice := vMsg.Parts()
		err := binary.Write(buf, binary.LittleEndian, fixed)
		if err != nil {
			return nil, err
		}
		v = slice
	} else {
		v = msg
	}
	err := binary.Write(buf, binary.LittleEndian, v)
	if err != nil {
		return nil, err
	}
	return packMsg(msg.ID(), buf.Bytes())
}

func Poll[T any, PT interface {
	ID() MsgID
	*T
}]() []byte {
	m := PT(new(T))
	mid := m.ID()
	frame, _ := packMsg(mid, []byte{})
	return frame
}

func SetRate[T any, PT interface {
	ID() MsgID
	*T
}](rate byte) []byte {
	m := PT(new(T))
	cfgMsgID := makeMsgID(clsCfg, 0x01)
	cls, id := m.ID().unpack()
	frame, _ := packMsg(cfgMsgID, []byte{cls, id, rate})
	return frame
}

func packMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("ubx-%s payload too long (%d bytes", mid.String(), len(payload))
	}
	cls, id := mid.unpack()
	frame := []byte{
		Sync1,
		Sync2,
		cls,
		id,
		byte(len(payload) & 0xFF),
		byte((len(payload) >> 8) & 0xFF),
	}
	frame = append(frame, payload...)
	ckA, ckB := checksum(frame[2:])
	frame = append(frame, ckA, ckB)
	return frame, nil
}

func checksum(bytes []byte) (ckA, ckB byte) {
	for _, b := range bytes {
		ckA += b
		ckB += ckA
	}
	return
}

// Latin1ZString create a string from a ISO Latin-1, nul-terminated byte slice.
// This can be used for the fields of MonVer
func Latin1ZToString(chars []byte) string {
	r := make([]rune, 0)
	for _, ch := range chars {
		if ch == 0 {
			break
		}
		r = append(r, rune(ch))
	}
	return string(r)
}

var protverRegexp = regexp.MustCompile(`^PROTVER[= ]([0-9][0-9]?)\.([0-9][0-9])$`)

func (m *MonVer) ProtVer() (int, int) {
	submatches := m.findExtension(protverRegexp)
	if submatches == nil {
		return -1, 0
	}
	return mustAtoi(submatches[1]), mustAtoi(submatches[2])
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(`could not convert UBX "` + s + `" to integer: ` + err.Error())
	}
	return n
}

func (m *MonVer) findExtension(re *regexp.Regexp) []string {
	for _, buf := range m.Extension {
		submatches := re.FindStringSubmatch(Latin1ZToString(buf[:]))
		if submatches != nil {
			return submatches
		}
	}
	return nil
}
