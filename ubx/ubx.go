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

const (
	NavTimeGPSId ClsId = clsNav | 0x20
	NavTimeUTCId ClsId = clsNav | 0x21
	NavTimeBDSId ClsId = clsNav | 0x24
)

func init() {
	regMsg[NavTimeGPSPayload](NavTimeGPSId, "timegps")
	regMsg[NavTimeBDSPayload](NavTimeBDSId, "timebds")
	regMsg[NavTimeUTCPayload](NavTimeUTCId, "timeutc")
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
	fmt.Printf("initialized %04x\n", uint16(clsId))
	idNameMap[clsId] = idName
}

func ParseMsg(frame []byte) (Msg, error) {
	clsId := (ClsId(frame[2]) << 8) | ClsId(frame[3])
	ctor := msgMap[clsId]
	if ctor == nil {
		fmt.Printf("unknown UBX message %s\n", clsId)
		return nil, nil
	}
	msg := ctor()
	payload := frame[6 : len(frame)-2]
	err := binary.Read(bytes.NewReader(payload), binary.LittleEndian, msg.Payload())
	if err != nil {
		fmt.Println("parse failed")
		return nil, err
	}
	return msg, nil
}
