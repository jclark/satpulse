package pmc

// This file knows about the structure of a managment message.

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"
)

type MgmtMsg interface {
	encoding.BinaryMarshaler
	Prefix() *MgmtMsgPrefix
	Value() any
}

type MgmtMsgWithValue[V any] struct {
	MgmtMsgPrefix
	V V
}

func (m *MgmtMsgWithValue[V]) Prefix() *MgmtMsgPrefix {
	return &m.MgmtMsgPrefix
}

func (m *MgmtMsgWithValue[V]) Value() any {
	return &m.V
}

type MgmtMsgPrefix struct {
	Header
	TargetPortIdentity   PortIdentity
	StartingBoundaryHops uint8
	BoundaryHops         uint8
	ActionField          Action
	_                    uint8
	TLVType              TLVType
	TLVLength            uint16
}

const SizeofMgmtMsgPrefix = 52

type Header struct {
	MajorSdoIDMessageType uint8
	Version               uint8
	MessageLength         uint16
	DomainNumber          uint8
	MinorSdoID            uint8
	FlagField             uint16
	CorrectionField       uint64
	MessageTypeSpecific   uint32
	SourcePortIdentity    PortIdentity
	SequenceID            uint16
	ControlField          Control
	LogMessageInterval    uint8
}

func (h *Header) SetMessageType(mt MessageType) {
	h.MajorSdoIDMessageType &^= 0x0f
	h.MajorSdoIDMessageType |= uint8(mt) & 0x0f
}

func (h *Header) SetMajorSdoID(id uint8) {
	h.MajorSdoIDMessageType &^= 0xf0
	h.MajorSdoIDMessageType |= id << 4
}

type MessageType uint8

const (
	MessageTypeMgmt MessageType = 0xD
)

type Control uint8

const (
	ControlMgmt Control = 0x4
)

type Action uint8

const (
	ActionGet Action = iota
	ActionSet
	ActionResponse
	ActionCommand
	ActionAcknowledge
)

type TLVType uint16

const (
	TLVTypeMgmt            TLVType = 0x0001
	TLVTypeMgmtErrorStatus TLVType = 0x0002
)

type MgmtID uint16

type MgmtValue[D MgmtData] struct {
	MgmtID MgmtID
	Data   D
}

type MgmtData interface {
	MgmtID() MgmtID
}

/* We don't need this currently.
type BinaryReaderFrom interface {
	ReadBinaryFrom(io.Reader) error
}
*/

type BinaryWriterTo interface {
	WriteBinaryTo(io.Writer) error
}

func UnmarshalMgmtMsg(data []byte) (MgmtMsg, error) {
	var f MgmtMsgPrefix
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.BigEndian, &f); err != nil {
		return nil, err
	}
	var msg MgmtMsg
	switch f.TLVType {
	case TLVTypeMgmt:
		var mid MgmtID
		var err error
		if err = binary.Read(r, binary.BigEndian, &mid); err != nil {
			return nil, err
		}
		msg, err = unmarshalMID(r, mid)
		if err != nil {
			return nil, err
		}
	case TLVTypeMgmtErrorStatus:
		var v MgmtErrorStatus
		if err := unmarshalMgmtErrorStatus(r, &v); err != nil {
			return nil, fmt.Errorf("unmarshaling MgmtErrorStatus failed: %w", err)
		}
		m := new(MgmtErrorStatusMsg)
		m.V = v
		msg = m
	default:
		return nil, fmt.Errorf("unsupported TLV type: 0x%04x", f.TLVType)
	}
	*msg.Prefix() = f
	_, err := r.ReadByte()
	if err == nil {
		return nil, fmt.Errorf("trailing bytes")
	}
	if err != io.EOF {
		return nil, err
	}
	return msg, nil
}

func unmarshalMgmtValue[D MgmtData](r io.Reader) (MgmtMsg, error) {
	m := MgmtMsgWithValue[MgmtValue[D]]{}
	if err := binary.Read(r, binary.BigEndian, &m.V.Data); err != nil {
		return nil, err
	}
	m.V.MgmtID = m.V.Data.MgmtID()
	return &m, nil
}

func (m *MgmtMsgWithValue[V]) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	var err error
	if err = binary.Write(buf, binary.BigEndian, &m.MgmtMsgPrefix); err != nil {
		return nil, err
	}
	bw, ok := any(&m.V).(BinaryWriterTo)
	if ok {
		err = bw.WriteBinaryTo(buf)
	} else {
		err = binary.Write(buf, binary.BigEndian, &m.V)
	}
	if err != nil {
		return nil, err
	}
	data := buf.Bytes()
	if err := m.fixupBinaryLength(data); err != nil {
		return nil, err
	}
	return data, nil
}

func (m *MgmtMsgWithValue[V]) fixupBinaryLength(data []byte) error {
	l := len(data)
	if l > 0xffff {
		return fmt.Errorf("message too long")
	}
	// Write length field in Header
	lenBytes := data[2:4]
	binary.BigEndian.PutUint16(lenBytes, uint16(l))
	// Write length field in TLV
	lenBytes = data[SizeofMgmtMsgPrefix-2 : SizeofMgmtMsgPrefix]
	binary.BigEndian.PutUint16(lenBytes, uint16(l-SizeofMgmtMsgPrefix))
	return nil
}

func (m *MgmtMsgWithValue[T]) SetLength(l uint16) {
	m.Header.MessageLength = uint16(l)
	m.TLVLength = l - SizeofMgmtMsgPrefix
}

func NewMgmtSetMsg[D MgmtData](data D) *MgmtMsgWithValue[MgmtValue[D]] {
	msg := new(MgmtMsgWithValue[MgmtValue[D]])
	initPrefix(&msg.MgmtMsgPrefix, ActionSet)
	msg.V.MgmtID = data.MgmtID()
	msg.V.Data = data
	return msg
}

func NewMgmtGetMsg(mid MgmtID) *MgmtMsgWithValue[MgmtID] {
	msg := new(MgmtMsgWithValue[MgmtID])
	initPrefix(&msg.MgmtMsgPrefix, ActionGet)
	msg.V = mid
	return msg
}

func initPrefix(p *MgmtMsgPrefix, action Action) {
	// Header
	initHeader(&p.Header)
	// MgmtBody
	p.ActionField = action
	p.TargetPortIdentity = AnyPortIdentity()
	p.TLVType = TLVTypeMgmt
	// length will be fixed by MarshalBinary
}

func initHeader(h *Header) {
	h.SetMessageType(MessageTypeMgmt) // We can add transport specific stuff in MgmtClient.PrepareMsg
	// MessageLength is filled in by MarshalBinary
	h.Version = 0x12 // PTPv2.1
	// Using non-zero value for ControlField is a legacy thing
	// Standard says it should be 0, unless supporting some PTPv1 hardware compatibility and using IPv4
	h.ControlField = 0
	h.LogMessageInterval = 0x7f // required by Table 42 of the standard
}

func NewMgmtErrorStatusMsg(mes MgmtErrorStatus) *MgmtErrorStatusMsg {
	msg := new(MgmtErrorStatusMsg)
	// Header
	initHeader(&msg.Header)
	// MgmtBody
	msg.ActionField = ActionResponse // or AcknowledgeAction?
	msg.TargetPortIdentity = AnyPortIdentity()
	msg.TLVType = TLVTypeMgmtErrorStatus
	// LengthField is filled in by MarshalBinary
	msg.V = mes
	return msg
}

type MsgPreparer struct {
	sequenceID   atomic.Uint32
	PortNumber   uint16
	DomainNumber uint8
	MajorSdoID   uint8
	MinorSdoID   uint8
}

func (c *MsgPreparer) PrepareMsg(m MgmtMsg) {
	h := &m.Prefix().Header
	h.SetMajorSdoID(c.MajorSdoID)
	h.DomainNumber = c.DomainNumber
	h.SourcePortIdentity.PortNumber = c.PortNumber
	h.MinorSdoID = c.MinorSdoID
	h.SequenceID = c.AllocSequenceID()
}

func (c *MsgPreparer) AllocSequenceID() uint16 {
	n := c.sequenceID.Add(1)
	return uint16(n - 1)
}

func MgmtSetBinaryMsg[D MgmtData](c *MsgPreparer, data D) ([]byte, error) {
	msg := NewMgmtSetMsg(data)
	c.PrepareMsg(msg)
	return msg.MarshalBinary()
}
