package ubx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	Sync1 = 0xB5
	Sync2 = 0x62
)

type MsgID uint16

type GNSSID byte

const (
	GPS GNSSID = iota
	SBAS
	Galileo
	BeiDou
	IMES
	QZSS
	GLONASS
	NavIC
)

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

const (
	AckNakID     MsgID = clsAck | (0x00 << 8)
	AckAckID     MsgID = clsAck | (0x01 << 8)
	CfgGNSSID    MsgID = clsCfg | (0x3E << 8)
	CfgMsgID     MsgID = clsCfg | (0x01 << 8)
	CfgTmode2ID  MsgID = clsCfg | (0x3D << 8)
	CfgTp5ID     MsgID = clsCfg | (0x31 << 8)
	MonHwID      MsgID = clsMon | (0x09 << 8)
	MonVerID     MsgID = clsMon | (0x04 << 8)
	NavTimeGPSID MsgID = clsNav | (0x20 << 8)
	NavTimeUTCID MsgID = clsNav | (0x21 << 8)
	NavTimeBDSID MsgID = clsNav | (0x24 << 8)
	NavTimeLSID  MsgID = clsNav | (0x26 << 8)
	TimSvinID    MsgID = clsTim | (0x04 << 8)
	TimTPID      MsgID = clsTim | (0x01 << 8)
)

func init() {
	regMsg[AckNak]("nak")
	regMsg[AckAck]("ack")
	regMsg[CfgGNSS]("gnss")
	regMsg[CfgMsg]("msg")
	regMsg[CfgTmode2]("tmode2")
	regMsg[CfgTp5]("tp5")
	regMsg[MonHw]("hw")
	regMsg[MonVer]("ver")
	regMsg[NavTimeGPS]("timegps")
	regMsg[NavTimeBDS]("timebds")
	regMsg[NavTimeUTC]("timeutc")
	regMsg[NavTimeLS]("timels")
	regMsg[TimSvin]("svin")
	regMsg[TimTP]("tp")
}

type AckNak struct {
	MsgID MsgID
}

func (m *AckNak) ID() MsgID { return AckNakID }

type AckAck struct {
	MsgID MsgID
}

func (m *AckAck) ID() MsgID { return AckAckID }

type CfgMsg struct {
	MsgID MsgID
	Rate  [6]byte
}

func (m *CfgMsg) ID() MsgID { return CfgMsgID }

type CfgTp5 struct {
	TpIdx             byte
	Version           byte
	_                 [2]byte
	AntCableDelay     int16
	RfGroupDelay      int16
	FreqPeriod        uint32
	FreqPeriodLock    uint32
	PulseLenRatio     uint32
	PulseLenRatioLock uint32
	UserConfigDelay   int32
	Flags             uint32
}

func (m *CfgTp5) ID() MsgID { return CfgTp5ID }

type CfgTmode2 struct {
	TimeMode     byte
	_            byte
	Flags        uint16
	EcefXOrLat   int32
	EcefYOrLon   int32
	EcefZOrAlt   int32
	FixedPosAcc  uint32
	SvinMinDur   uint32
	SvinAccLimit uint32
}

func (m *CfgTmode2) ID() MsgID { return CfgTmode2ID }

type NavTimeGPS struct {
	ITOW  uint32
	FTOW  int32
	Week  int16
	LeapS byte
	Valid NavTimeGPSValid
	TAcc  uint32
}

type NavTimeGPSValid byte

const (
	NavTimeGPSTOWValid NavTimeGPSValid = 1 << iota
	NavTimeGPSWeekValid
	NavTimeGPSLeapSValid
)

func (m *NavTimeGPS) ID() MsgID { return NavTimeGPSID }

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

func (m *NavTimeUTC) ID() MsgID { return NavTimeUTCID }

type NavTimeBDS struct {
	ITOW  uint32
	SOW   uint32
	FSOW  int32
	Week  int16
	LeapS byte
	Valid byte
	TAcc  uint32
}

func (m *NavTimeBDS) ID() MsgID { return NavTimeBDSID }

type NavTimeLS struct {
	ITOW          uint32
	Version       byte
	_             [3]byte
	SrcOfCurrLS   NavTimeSrcOfCurrLS
	CurrLS        int8
	SrcOfLSChange NavTimeLSSrcOfLSChange
	LSChange      NavTimeLSChange
	TimeToLSEvent int32
	DateOfLSGPSWN uint16
	DateOfLSGPSDN uint16
	_             [3]byte
	Valid         NavTimeLSValid
}

type NavTimeSrcOfCurrLS byte

const (
	NavTimeSrcOfCurrLSFirmware NavTimeSrcOfCurrLS = iota
	NavTimeSrcOfCurrLSGPSDiffGLONASS
	NavTimeSrcOfCurrLSGPS
	NavTimeSrcOfCurrLSSBAS
	NavTimeSrcOfCurrLSBeiDou
	NavTimeSrcOfCurrLSGalileo
	NavTimeSrcOfCurrLSAided
	NavTimeSrcOfCurrLSConfigured
	NavTimeSrcOfCurrLSNavIC
	NavTimeSrcOfCurrLSUnknown NavTimeSrcOfCurrLS = 255
)

type NavTimeLSSrcOfLSChange byte

const (
	NavTimeLSSrcOfLSChangeNone NavTimeLSSrcOfLSChange = iota
	_
	NavTimeLSSrcOfLSChangeGPS
	NavTimeLSSrcOfLSChangeSBAS
	NavTimeLSSrcOfLSChangeBeiDou
	NavTimeLSSrcOfLSChangeGalileo
	NavTimeLSSrcOfLSChangeGLONASS
	NavTimeLSSrcOfLSChangeNavIC
)

type NavTimeLSChange int8

const NavTimeLSChangePositive NavTimeLSChange = +1
const NavTimeLSChangeNone NavTimeLSChange = 0
const NavTimeLSChangeNegative NavTimeLSChange = -1

type NavTimeLSValid byte

const (
	NavTimeLSValidCurrLS NavTimeLSValid = 1 << iota
	NavTimeLSValidTimeToLSEvent
)

func (m *NavTimeLS) ID() MsgID { return NavTimeLSID }

type TimTP struct {
	TOWMS    uint32
	TOWSubMS uint32
	QErr     int32
	Week     uint16
	Flags    TimTPFlags
	RefInfo  TimTPRefInfo
}

func (m *TimTP) ID() MsgID { return TimTPID }

type TimTPFlags byte

const (
	TimTPTimeBase TimTPFlags = 1 << iota
	TimTPUTC
	timTPRAIM1
	timTPRAIM2
	TimTPQErrInvalid
	TimTPRAIM TimTPFlags = timTPRAIM1 | timTPRAIM2
)

const TimTPTimeBaseUTC TimTPFlags = TimTPTimeBase
const TimTPUTCAvailable TimTPFlags = TimTPUTC

// Values for TimTPFlags & TimTPRAIM
const (
	TimTPRAIMNotActive TimTPFlags = timTPRAIM1
	TimTPRAIMActive    TimTPFlags = timTPRAIM2
)

type TimTPRefInfo byte

// These are bitwise-ANDed with TimTPRefInfo
const (
	TimTPTimeRefGNSS TimTPRefInfo = 0x0F
	TimTPUTCStandard TimTPRefInfo = 0xF0
)

// Values for TimTPRefInfo & TimTPTimeRefGNSS
const (
	TimTPTimeRefGNSSGPS TimTPRefInfo = iota
	TimTPTimeRefGNSSGLONASS
	TimTPTimeRefGNSSBeiDou
	TimTPTimeRefGNSSGalileo
	TimTPTimeRefGNSSNavIC
	TimTPTimeRefGNSSUnknown TimTPRefInfo = 15
)

// Values for TimTPRefInfo & TimTPUTCStandard
const (
	TimTPUTCStandardNotAvailable TimTPRefInfo = 0
	TimTPUTCStandardCRL          TimTPRefInfo = iota << 4
	TimTPUTCStandardNIST
	TimTPUTCStandardUSNO
	TimTPUTCStandardBIPM
	TimTPUTCStandardEU
	TimTPUTCStandardSU
	TimTPUTCStandardNTSC
	TimTPUTCStandardNPLI
	TimTPUTCStandardUnknown = TimTPUTCStandard
)

type TimSvin struct {
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

func (m *TimSvin) ID() MsgID { return TimSvinID }

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

type MonVerFixed struct {
	SwVersion [30]byte
	HwVersion [10]byte
}

type MonVer struct {
	MonVerFixed
	Extension [][30]byte
}

func (m *MonVer) ID() MsgID { return MonVerID }

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

type CfgGNSS struct {
	CfgGNSSFixed
	Blocks []CfgGNNSSBlock
}

type CfgGNSSFixed struct {
	MsgVer          byte
	NumTrkChHw      byte
	NumTrkChUse     byte
	NumConfigBlocks byte
}

type CfgGNNSSBlock struct {
	GNSSID     GNSSID
	ResTrkCh   byte
	MaxTrkCh   byte
	_          byte
	Enable     byte
	_          byte
	SigCfgMask byte
	_          byte
}

func (m *CfgGNSS) ID() MsgID { return CfgGNSSID }

func (m *CfgGNSS) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 8)
	if err == nil {
		m.Blocks = make([]CfgGNNSSBlock, len)
	}
	return
}

func (m *CfgGNSS) Parts() (fixed any, slice any) {
	fixed = &m.CfgGNSSFixed
	slice = &m.Blocks
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

type UnknownMsg struct {
	MsgID   MsgID
	Payload string
}

func (m *UnknownMsg) ID() MsgID { return m.MsgID }

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

func ParseMsg(frame string) (Msg, error) {
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
		return &UnknownMsg{MsgID: mid, Payload: payload}, nil
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
	r := strings.NewReader(payload)
	err := binary.Read(r, binary.LittleEndian, fixed)
	if err == nil && slice != nil {
		err = binary.Read(r, binary.LittleEndian, slice)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing ubx-%s: %v", mid.String(), err)
	}
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing ubx-%s: trailing bytes", mid.String())
	}
	return msg, nil
}

func Serialize(msg Msg) ([]byte, error) {
	if uMsg, ok := msg.(*UnknownMsg); ok {
		return packMsg(uMsg.MsgID, []byte(uMsg.Payload))
	}
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

func Poll(mid MsgID) []byte {
	frame, _ := packMsg(mid, []byte{})
	return frame
}

func SetRate(mid MsgID, rate byte) []byte {
	cls, id := mid.unpack()
	frame, _ := packMsg(CfgMsgID, []byte{cls, id, rate})
	return frame
}

func PollRate(mid MsgID) []byte {
	cls, id := mid.unpack()
	frame, _ := packMsg(CfgMsgID, []byte{cls, id})
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

type Bytes interface {
	string | []byte
}

func checksum[B Bytes](bytes B) (ckA, ckB byte) {
	for i := 0; i < len(bytes); i++ {
		ckA += bytes[i]
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

type ProtVer struct {
	major, minor byte
}

func (v ProtVer) String() string {
	return fmt.Sprintf("%d.%0d", v.major, v.minor)
}

var protverRegexp = regexp.MustCompile(`^PROTVER[= ]([1-9][0-9]?)\.([0-9][0-9])$`)

func (m *MonVer) ProtVer() ProtVer {
	submatches := m.findExtension(protverRegexp)
	if submatches == nil {
		return ProtVer{}
	}
	return ProtVer{mustAtob(submatches[1]), mustAtob(submatches[2])}
}

func mustAtob(s string) byte {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(`could not convert UBX "` + s + `" to integer: ` + err.Error())
	}
	return byte(n)
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
