package bin

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
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
	GAL
	BDS
	IMES
	QZSS
	GLO
	NavIC
)

type PortID byte

const (
	PortI2C PortID = iota
	PortUART1
	PortUART2
	PortUSB
	PortSPI
)

const NPort = 6

const (
	clsNav  = 0x01
	clsRxm  = 0x02
	clsInf  = 0x04
	clsAck  = 0x05
	clsCfg  = 0x06
	clsMon  = 0x0A
	clsTim  = 0x0D
	clsMga  = 0x13
	clsSec  = 0x27
	clsNav2 = 0x29
	clsNmea = 0xF0
	clsRtcm = 0xF5
)

var clsMap = map[byte]string{
	clsNav:  "NAV",
	clsRxm:  "RXM",
	clsInf:  "INF",
	clsAck:  "ACK",
	clsCfg:  "CFG",
	clsMon:  "MON",
	clsTim:  "TIM",
	clsMga:  "MGA",
	clsSec:  "SEC",
	clsNav2: "NAV2",
	clsNmea: "NMEA",
	clsRtcm: "RTCM",
}

func makeMsgID(cls byte, id byte) MsgID {
	return MsgID(uint16(cls) | (uint16(id) << 8))
}

func (mid MsgID) unpack() (byte, byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

func (mid MsgID) Ackable() bool {
	return mid.CfgClass() && mid != CfgRstID
}

func (mid MsgID) CfgClass() bool {
	cls, _ := mid.unpack()
	return cls == clsCfg
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
		s = fmt.Sprintf("0x%02X", cls)
	}
	s += "-"
	if idName != "" {
		s += idName
	} else {
		s += fmt.Sprintf("0x%02X", id)
	}
	return s
}

type VaryingMsg interface {
	Msg
	InitVaryingPart(payloadLen int) error
	FixedPart() any
	VaryingPart() any
}

type PartiallyHandledMsg interface {
	Msg
	IsHandled() bool
}

// Use VaryingMsg here to help ensure that InitVaryingPart is correctly declared for each type
func sliceLen(m VaryingMsg, payloadLen, minLen, elemLen int) (int, error) {
	extraLen := payloadLen - minLen
	if extraLen < 0 || extraLen%elemLen != 0 {
		return 0, fmt.Errorf("bad UBX-%v payload length (%d bytes)", m.ID(), payloadLen)
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
const packetMinLength = 8

// ParseMsg parses a UBlox message from a string.
// It assumes the checksums were already verified.
func ParseMsg(packet string) (Msg, error) {
	n := len(packet)
	if n < packetMinLength {
		return nil, fmt.Errorf("UBX message too short (length %d bytes)", n)
	}
	checksumIndex := n - 2
	trimmed := packet[2:checksumIndex]
	mid := makeMsgID(trimmed[0], trimmed[1])
	ctor := msgMap[mid]
	payload := trimmed[4:]
	if ctor == nil {
		return &UnknownMsg{MsgID: mid, Payload: payload}, nil
	}
	msg := ctor()
	var fixed, vary any
	if vMsg, ok := msg.(VaryingMsg); ok {
		fixed = vMsg.FixedPart()
	} else {
		fixed = msg
		vary = nil
	}
	r := strings.NewReader(payload)
	var err error
	// For UBX-INF-* messages, the payload does not have a fixed part.
	if fixed != nil {
		err = binary.Read(r, binary.LittleEndian, fixed)
	}
	// Check if this is a partially handled message that we don't handle.
	if err == nil {
		if phMsg, ok := msg.(PartiallyHandledMsg); ok && !phMsg.IsHandled() {
			return &UnknownMsg{MsgID: mid, Payload: payload}, nil
		}
	}
	if vMsg, ok := msg.(VaryingMsg); ok && err == nil {
		err = vMsg.InitVaryingPart(len(payload))
		if err != nil {
			return nil, err
		}
		vary = vMsg.VaryingPart()
	}
	if err == nil && vary != nil {
		err = binary.Read(r, binary.LittleEndian, vary)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing UBX-%s: %v", mid.String(), err)
	}
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing UBX-%s: trailing bytes", mid.String())
	}
	return msg, nil
}

func Serialize(msg Msg) ([]byte, error) {
	if uMsg, ok := msg.(*UnknownMsg); ok {
		return packMsg(uMsg.MsgID, []byte(uMsg.Payload))
	}
	buf := new(bytes.Buffer)
	var v any
	if vMsg, ok := msg.(VaryingMsg); ok {
		fixed := vMsg.FixedPart()
		vary := vMsg.VaryingPart()
		if fixed != nil {
			err := binary.Write(buf, binary.LittleEndian, fixed)
			if err != nil {
				return nil, err
			}
		}
		v = vary
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
	packet, _ := packMsg(mid, []byte{})
	return packet
}

func packMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("ubx-%s payload too long (%d bytes", mid.String(), len(payload))
	}
	cls, id := mid.unpack()
	packet := []byte{
		Sync1,
		Sync2,
		cls,
		id,
		byte(len(payload) & 0xFF),
		byte((len(payload) >> 8) & 0xFF),
	}
	packet = append(packet, payload...)
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)
	return packet, nil
}

type Bytes interface {
	string | []byte
}

// PacketMsgId returns the MsgId of a packet.
// This assumes a minimally-valid packet
func PacketMsgId[B Bytes](packet B) MsgID {
	return makeMsgID(packet[2], packet[3])
}

func Checksum(bytes []byte) (ckA, ckB byte) {
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
