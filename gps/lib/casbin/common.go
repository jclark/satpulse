package casbin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	Sync1 = 0xBA
	Sync2 = 0xCE
)

// Endian is the byte order used for CASIC messages.
var Endian = binary.LittleEndian

// MsgID encodes class in low byte, id in high byte
type MsgID uint16

const (
	clsNav   = 0x01
	clsTim   = 0x02
	clsRxm   = 0x03
	clsAck   = 0x05
	clsCfg   = 0x06
	clsMsg   = 0x08
	clsMon   = 0x0A
	clsAid   = 0x0B
	// these are in ZKW F8 dual-band receivers
	clsNav2  = 0x11
	clsTim2  = 0x12
	clsRxm2  = 0x13
	clsIns2  = 0x14
	clsRtcm2 = 0x15
	clsNmea  = 0x4E
)

type GNSSID uint8

const (
	GPS   GNSSID = iota
	BDS
	GLN
	GAL
	QZSS
	SBAS
	NAVIC
)

var clsMap = map[byte]string{
	clsNav:  "NAV",
	clsTim:  "TIM",
	clsRxm:  "RXM",
	clsAck:  "ACK",
	clsCfg:  "CFG",
	clsMsg:  "MSG",
	clsMon:  "MON",
	clsAid:  "AID",
	clsNmea: "NMEA",
}

// MakeMsgID creates a MsgID from class and id bytes.
func MakeMsgID(cls, id byte) MsgID {
	return MsgID(uint16(cls) | (uint16(id) << 8))
}

func (mid MsgID) Unpack() (cls, id byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

// CfgClass reports whether mid is in the CFG class.
func (mid MsgID) CfgClass() bool {
	cls, _ := mid.Unpack()
	return cls == clsCfg
}

func (mid MsgID) String() string {
	cls, id := mid.Unpack()
	s := clsMap[cls]
	if s == "" {
		s = fmt.Sprintf("0x%02X", cls)
	}
	s += "-"
	idName := idNameMap[mid]
	if idName != "" {
		s += idName
	} else {
		s += fmt.Sprintf("0x%02X", id)
	}
	return s
}

type Msg interface {
	ID() MsgID
}

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)

type VaryingMsg interface {
	Msg
	InitVaryingPart(payloadLen int) error
	FixedPart() any
	VaryingPart() any
}

// AllowTrailing is implemented by messages that tolerate trailing bytes
// beyond the declared structure. ParseMsg skips the trailing-bytes check
// for these messages.
type AllowTrailing interface {
	AllowTrailingBytes()
}

func sliceLen(m VaryingMsg, payloadLen, minLen, elemLen int) (int, error) {
	extraLen := payloadLen - minLen
	if extraLen < 0 || extraLen%elemLen != 0 {
		return 0, fmt.Errorf("bad CASIC-%v payload length (%d bytes)", m.ID(), payloadLen)
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

// Packet header: sync(2) + len(2) + class(1) + id(1) + payload + checksum(4)
const packetMinLength = 10

// ParseMsg parses a CASIC message from a packet.
// It assumes the checksum was already verified.
func ParseMsg(packet string) (Msg, error) {
	n := len(packet)
	if n < packetMinLength {
		return nil, fmt.Errorf("CASIC message too short (length %d bytes)", n)
	}
	checksumIndex := n - 4
	// Header: sync(2) + len(2) + class(1) + id(1)
	cls := packet[4]
	id := packet[5]
	mid := MakeMsgID(cls, id)
	ctor := msgMap[mid]
	payload := packet[6:checksumIndex]
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
	if fixed != nil {
		err = binary.Read(r, binary.LittleEndian, fixed)
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
		return nil, fmt.Errorf("parsing CASIC-%s: %v", mid.String(), err)
	}
	if _, ok := msg.(AllowTrailing); !ok {
		_, err = r.ReadByte()
		if err != io.EOF {
			return nil, fmt.Errorf("parsing CASIC-%s: trailing bytes", mid.String())
		}
	}
	return msg, nil
}

// Serialize serializes a CASIC message to a packet.
func Serialize(msg Msg) ([]byte, error) {
	if uMsg, ok := msg.(*UnknownMsg); ok {
		return PackMsg(uMsg.MsgID, []byte(uMsg.Payload))
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
	return PackMsg(msg.ID(), buf.Bytes())
}

// Poll creates a poll packet for the given message ID.
func Poll(mid MsgID) []byte {
	packet, _ := PackMsg(mid, []byte{})
	return packet
}

// PackMsg creates a CASIC packet from a message ID and payload.
func PackMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("CASIC-%s payload too long (%d bytes)", mid.String(), len(payload))
	}
	cls, id := mid.Unpack()
	packet := []byte{
		Sync1,
		Sync2,
		byte(len(payload) & 0xFF),
		byte((len(payload) >> 8) & 0xFF),
		cls,
		id,
	}
	packet = append(packet, payload...)
	ck := Checksum(packet)
	packet = binary.LittleEndian.AppendUint32(packet, ck)
	return packet, nil
}

// PacketMsgID returns the MsgID of a packet.
func PacketMsgID[B ~string | ~[]byte](packet B) MsgID {
	return MakeMsgID(packet[4], packet[5])
}

// Latin1Z32 is a [32]byte that JSON-marshals as a Latin-1 nul-terminated string.
type Latin1Z32 [32]byte

func latin1ZToString(chars []byte) string {
	for i, ch := range chars {
		if ch == 0 {
			return string(chars[:i])
		}
	}
	return string(chars)
}

func (z Latin1Z32) String() string               { return latin1ZToString(z[:]) }
func (z Latin1Z32) MarshalJSON() ([]byte, error) { return json.Marshal(z.String()) }

// Checksum computes the CASIC checksum for a complete packet (without the checksum field).
func Checksum(pkt []byte) uint32 {
	if len(pkt) < 6 {
		return 0
	}
	payloadLen := int(binary.LittleEndian.Uint16(pkt[2:4]))
	cls := pkt[4]
	id := pkt[5]
	// Per errata: receivers use (id << 24) + (cls << 16) + len
	ck := (uint32(id) << 24) + (uint32(cls) << 16) + uint32(payloadLen)
	payload := pkt[6:]
	for i := 0; i < len(payload); i += 4 {
		remaining := len(payload) - i
		var word uint32
		switch {
		case remaining >= 4:
			word = binary.LittleEndian.Uint32(payload[i:])
		case remaining == 3:
			word = uint32(payload[i]) | uint32(payload[i+1])<<8 | uint32(payload[i+2])<<16
		case remaining == 2:
			word = uint32(payload[i]) | uint32(payload[i+1])<<8
		case remaining == 1:
			word = uint32(payload[i])
		}
		ck += word
	}
	return ck
}
