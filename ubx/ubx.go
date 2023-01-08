package ubx

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	Sync1 = 0xB5
	Sync2 = 0x62
)

const (
	clsNav ClsId = 0x0100
	clsAck ClsId = 0x0500
	clsCfg ClsId = 0x0600
	clsMon ClsId = 0x0A00
	clsTim ClsId = 0x0D00
)

var clsMap = map[ClsId]string{
	clsNav: "nav",
	clsAck: "ack",
	clsCfg: "cfg",
	clsMon: "mon",
	clsTim: "tim",
}

type ClsId uint16

type Msg interface {
	ClsId() ClsId
	Payload() any
}

var msgMap = make(map[ClsId]func() Msg)
var idNameMap = make(map[ClsId]string)

func (cid ClsId) String() string {
	idName := idNameMap[cid]
	if idName != "" {
		return clsMap[cid&0xFF00] + "-" + idName
	}
	return fmt.Sprintf("%04x", uint16(cid))
}

// Used for both AckAck and AckNak
type AckPayload struct {
	ClsId byte
	MsgId byte
}

type NavTimeGPSPayload struct {
	ITOW  uint32
	FTOW  int32
	Week  int16
	LeapS byte
	Valid byte
	TAcc  uint32
}

type NavTimeBDSPayload struct {
	ITOW  uint32
	SOW   uint32
	FSOW  int32
	Week  int16
	LeapS byte
	Valid byte
	TAcc  uint32
}

type NavTimeUTCPayload struct {
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

type NavTimeLSPayload struct {
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

type TimTPPayload struct {
	TowMS    uint32
	TowSubMS uint32
	QErr     int32
	Week     uint16
	Flags    byte
	RefInfo  byte
}

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

type TimSvInPayload struct {
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

const (
	AckNakId     ClsId = clsAck
	AckAckId     ClsId = clsAck | 0x01
	NavTimeGPSId ClsId = clsNav | 0x20
	NavTimeUTCId ClsId = clsNav | 0x21
	NavTimeBDSId ClsId = clsNav | 0x24
	NavTimeLSId  ClsId = clsNav | 0x26
	TimTPId      ClsId = clsTim | 0x01
	TimSvInId    ClsId = clsTim | 0x04
)

func init() {
	regMsg[AckPayload](AckAckId, "ack")
	regMsg[AckPayload](AckNakId, "nak")
	regMsg[NavTimeGPSPayload](NavTimeGPSId, "timegps")
	regMsg[NavTimeBDSPayload](NavTimeBDSId, "timebds")
	regMsg[NavTimeUTCPayload](NavTimeUTCId, "timeutc")
	regMsg[NavTimeLSPayload](NavTimeLSId, "timels")
	regMsg[TimTPPayload](TimTPId, "tp")
	regMsg[TimSvInPayload](TimSvInId, "svin")
}

type MsgData[P any] struct {
	clsId   ClsId
	payload P
}

func (msg *MsgData[P]) ClsId() ClsId {
	return msg.clsId
}

func (msg *MsgData[P]) Payload() any {
	return &msg.payload
}

func NewMsg[P any](clsId ClsId) Msg {
	m := new(MsgData[P])
	m.clsId = clsId
	return m
}

func regMsg[P any](clsId ClsId, idName string) {
	msgMap[clsId] = func() Msg { return NewMsg[P](clsId) }
	idNameMap[clsId] = idName
}

func ParseMsg(frame []byte) (Msg, error) {
	clsId := (ClsId(frame[2]) << 8) | ClsId(frame[3])
	ctor := msgMap[clsId]
	payload := frame[6 : len(frame)-2]
	if ctor == nil {
		// fmt.Printf("unknown UBX message %s\n", clsId)
		// XXX return a message with bytes as payload
		return nil, nil
	}
	msg := ctor()
	err := binary.Read(bytes.NewReader(payload), binary.LittleEndian, msg.Payload())
	if err != nil {
		return nil, fmt.Errorf("parsing ubx-%s: %v", clsId.String(), err)
	}
	return msg, nil
}
