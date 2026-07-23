package asbin

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Endian returns the byte order used by Allystar binary protocol.
func Endian() binary.ByteOrder { return binary.LittleEndian }

const (
	Sync1 = 0xF1
	Sync2 = 0xD9
)

type MsgID uint16

const (
	clsNav  = 0x01
	clsAck  = 0x05
	clsCfg  = 0x06
	clsMon  = 0x0A
	clsAid  = 0x0B
	clsNmea = 0xF0
)

var clsMap = map[byte]string{
	clsNav:  "NAV",
	clsAck:  "ACK",
	clsCfg:  "CFG",
	clsMon:  "MON",
	clsAid:  "AID",
	clsNmea: "NMEA",
}

func makeMsgID(cls byte, id byte) MsgID {
	return MsgID(uint16(cls) | (uint16(id) << 8))
}

// MakeMsgID creates a MsgID from class and id bytes.
func MakeMsgID(cls byte, id byte) MsgID { return makeMsgID(cls, id) }

// Unpack splits mid into its class and id bytes.
func (mid MsgID) Unpack() (byte, byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

// CfgClass reports whether mid is in the CFG class.
func (mid MsgID) CfgClass() bool {
	cls, _ := mid.Unpack()
	return cls == clsCfg
}

// Ackable reports whether mid expects an ACK/NAK response.
func (mid MsgID) Ackable() bool {
	return mid.CfgClass() && mid != CfgSimpleRstID
}

type Msg interface {
	ID() MsgID
}

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)

func (mid MsgID) String() string {
	idName := idNameMap[mid]
	cls, id := mid.Unpack()
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

const (
	HeaderLen    = 6 // sync(2) + class(1) + id(1) + length(2)
	TrailerLen   = 2 // checksum(2)
	PacketMinLen = HeaderLen + TrailerLen
)

// AckMsgID extracts the acknowledged MsgID from an ACK/NAK packet.
func AckMsgID[B Bytes](pkt B) MsgID {
	return makeMsgID(pkt[HeaderLen], pkt[HeaderLen+1])
}

// ParseMsg parses an Allystar binary message from a string.
// Unlike its ubxbin and casbin counterparts, it verifies the checksum itself.
func ParseMsg(packet string) (Msg, error) {
	n := len(packet)
	if n < PacketMinLen {
		return nil, fmt.Errorf("AS message too short (length %d bytes)", n)
	}
	checksumIndex := n - 2
	trimmed := packet[2:checksumIndex]
	ckA, ckB := Checksum(trimmed)
	if ckA != packet[checksumIndex] || ckB != packet[checksumIndex+1] {
		return nil, fmt.Errorf("AS message: checksum failed: checksum in message 0x%02x%02x; computed checksum 0x%02x%02x; data %x", packet[checksumIndex], packet[checksumIndex+1], ckA, ckB, []byte(trimmed))
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
	var err error
	// Parts may report no fixed part, for a payload that is all repeated block.
	if fixed != nil {
		err = binary.Read(r, binary.LittleEndian, fixed)
	}
	if err == nil && slice != nil {
		err = binary.Read(r, binary.LittleEndian, slice)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing as-%s: %v", mid.String(), err)
	}
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing as-%s: trailing bytes", mid.String())
	}
	return msg, nil
}

// Serialize serializes an Allystar binary message to a packet.
func Serialize(msg Msg) ([]byte, error) {
	if uMsg, ok := msg.(*UnknownMsg); ok {
		return packMsg(uMsg.MsgID, []byte(uMsg.Payload))
	}
	buf := new(bytes.Buffer)
	var v any
	if vMsg, ok := msg.(VarLengthMsg); ok {
		fixed, slice := vMsg.Parts()
		if fixed != nil {
			err := binary.Write(buf, binary.LittleEndian, fixed)
			if err != nil {
				return nil, err
			}
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

// Poll creates a poll packet for the given message ID.
func Poll(mid MsgID) []byte {
	packet, _ := packMsg(mid, []byte{})
	return packet
}

func packMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("as-%s payload too long (%d bytes)", mid.String(), len(payload))
	}
	cls, id := mid.Unpack()
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

// PackMsg creates a complete Allystar binary packet from a MsgID and payload.
func PackMsg(mid MsgID, payload []byte) ([]byte, error) { return packMsg(mid, payload) }

type Bytes interface {
	string | []byte
}

// PacketMsgId returns the MsgId of a packet.
// This assumes a minimally-valid packet
func PacketMsgId[B Bytes](packet B) MsgID {
	return makeMsgID(packet[2], packet[3])
}

// Checksum computes the Fletcher-8 checksum used in Allystar binary protocol.
func Checksum[B Bytes](bytes B) (ckA, ckB byte) {
	for i := 0; i < len(bytes); i++ {
		ckA += bytes[i]
		ckB += ckA
	}
	return
}
