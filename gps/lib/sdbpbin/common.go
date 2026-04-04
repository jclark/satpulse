package sdbpbin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Endian is the byte order used by SDBP.
var Endian = binary.LittleEndian

const (
	Sync1 = 0x23
	Sync2 = 0x3E
)

// MsgID identifies an SDBP message by class and ID.
type MsgID uint16

const (
	clsPUB = 0x01
	clsCTL = 0x02
	clsCFG = 0x03
	clsAST = 0x04
	clsQUE = 0x05
	clsDAT = 0x06
)

var clsMap = map[byte]string{
	clsPUB: "PUB",
	clsCTL: "CTL",
	clsCFG: "CFG",
	clsAST: "AST",
	clsQUE: "QUE",
	clsDAT: "DAT",
}

func makeMsgID(cls, id byte) MsgID {
	return MsgID(uint16(cls) | uint16(id)<<8)
}

// MakeMsgID creates a MsgID from class and id bytes.
func MakeMsgID(cls, id byte) MsgID { return makeMsgID(cls, id) }

func (mid MsgID) unpack() (cls, id byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

// CfgClass reports whether mid is in the CFG class.
func (mid MsgID) CfgClass() bool {
	cls, _ := mid.unpack()
	return cls == clsCFG
}

// Ackable reports whether mid expects an ACK/NAK response.
func (mid MsgID) Ackable() bool {
	return mid != CtlRestartID && mid != CtlStandbyID
}

func (mid MsgID) String() string {
	cls, id := mid.unpack()
	s := clsMap[cls]
	if s == "" {
		s = fmt.Sprintf("0x%02X", cls)
	}
	s += "-"
	if name := idNameMap[mid]; name != "" {
		s += name
	} else {
		s += fmt.Sprintf("0x%02X", id)
	}
	return s
}

// Msg is the interface implemented by all parsed SDBP messages.
type Msg interface {
	ID() MsgID
}

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)

func regMsg[T any, PT interface {
	ID() MsgID
	*T
}](idName string) {
	m := PT(new(T))
	mid := m.ID()
	msgMap[mid] = func() Msg { return PT(new(T)) }
	idNameMap[mid] = idName
}

// VaryingMsg is implemented by messages with a variable-length part.
type VaryingMsg interface {
	Msg
	InitVaryingPart(payloadLen int) error
	FixedPart() any
	VaryingPart() any
}

// TimeRef identifies a time reference used by CFG-PPS and DAT-TPPS.
type TimeRef uint8

const (
	TimeRefUTC TimeRef = 0
	TimeRefBDS TimeRef = 1
	TimeRefGPS TimeRef = 2
	TimeRefGLO TimeRef = 3
	TimeRefGAL TimeRef = 4
)

// AsciiSlice is a []byte that JSON-marshals as a string.
type AsciiSlice []byte

func (s AsciiSlice) String() string            { return string(s) }
func (s AsciiSlice) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

// UnknownMsg represents an SDBP message with no registered parser.
type UnknownMsg struct {
	MsgID_   MsgID
	Payload string
}

func (m *UnknownMsg) ID() MsgID { return m.MsgID_ }

// 2 sync + class + id + 2 length + 2 checksum
const packetMinLength = 8

// ParseMsg parses a complete SDBP packet (including sync and checksum) into a Msg.
func ParseMsg(packet string) (Msg, error) {
	n := len(packet)
	if n < packetMinLength {
		return nil, fmt.Errorf("SDBP message too short (%d bytes)", n)
	}
	checksumIndex := n - 2
	trimmed := packet[2:checksumIndex]
	ckA, ckB := Checksum(trimmed)
	if ckA != packet[checksumIndex] || ckB != packet[checksumIndex+1] {
		return nil, fmt.Errorf("SDBP checksum failed: got %02x%02x, want %02x%02x",
			packet[checksumIndex], packet[checksumIndex+1], ckA, ckB)
	}
	mid := makeMsgID(trimmed[0], trimmed[1])
	payload := trimmed[4:]
	ctor := msgMap[mid]
	if ctor == nil {
		return &UnknownMsg{MsgID_: mid, Payload: payload}, nil
	}
	msg := ctor()
	var fixed any
	var vary any
	if vMsg, ok := msg.(VaryingMsg); ok {
		fixed = vMsg.FixedPart()
	} else {
		fixed = msg
	}
	r := strings.NewReader(payload)
	var err error
	if fixed != nil {
		err = binary.Read(r, Endian, fixed)
	}
	if vMsg, ok := msg.(VaryingMsg); ok && err == nil {
		err = vMsg.InitVaryingPart(len(payload))
		if err != nil {
			return nil, err
		}
		vary = vMsg.VaryingPart()
	}
	if err == nil && vary != nil {
		err = binary.Read(r, Endian, vary)
	}
	if err != nil {
		return nil, fmt.Errorf("parsing SDBP-%s: %v", mid, err)
	}
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing SDBP-%s: trailing bytes", mid)
	}
	return msg, nil
}

// Serialize converts an Msg back to a complete SDBP packet.
func Serialize(msg Msg) ([]byte, error) {
	if u, ok := msg.(*UnknownMsg); ok {
		return PackMsg(u.MsgID_, []byte(u.Payload))
	}
	buf := new(bytes.Buffer)
	if vMsg, ok := msg.(VaryingMsg); ok {
		if fixed := vMsg.FixedPart(); fixed != nil {
			if err := binary.Write(buf, Endian, fixed); err != nil {
				return nil, err
			}
		}
		if err := binary.Write(buf, Endian, vMsg.VaryingPart()); err != nil {
			return nil, err
		}
	} else {
		if err := binary.Write(buf, Endian, msg); err != nil {
			return nil, err
		}
	}
	return PackMsg(msg.ID(), buf.Bytes())
}

// PackMsg creates a complete SDBP packet from a MsgID and payload.
func PackMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("SDBP-%s payload too long (%d bytes)", mid, len(payload))
	}
	cls, id := mid.unpack()
	pkt := []byte{
		Sync1, Sync2,
		cls, id,
		byte(len(payload)), byte(len(payload) >> 8),
	}
	pkt = append(pkt, payload...)
	ckA, ckB := Checksum(pkt[2:])
	pkt = append(pkt, ckA, ckB)
	return pkt, nil
}

type Bytes interface {
	string | []byte
}

// PacketMsgId returns the MsgID of a packet.
func PacketMsgId[B Bytes](packet B) MsgID {
	return makeMsgID(packet[2], packet[3])
}

// Checksum computes the UBX-style Fletcher checksum used by SDBP.
func Checksum[B Bytes](data B) (ckA, ckB byte) {
	for i := 0; i < len(data); i++ {
		ckA += data[i]
		ckB += ckA
	}
	return
}
