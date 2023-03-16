package pmc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type MgmtMsg[V any] struct {
	MgmtMsgFixed
	V V
}

type MgmtMsgFixed struct {
	Header
	TargetPortIdentity   PortIdentity
	StartingBoundaryHops uint8
	BoundaryHops         uint8
	ActionField          Action
	_                    uint8
	TLVType              TLVType
	TLVLength            uint16
}

type Header struct {
	MessageType         MessageType
	Version             uint8
	MessageLength       uint16
	DomainNumber        uint8
	MinorSdoID          uint8
	FlagField           uint16
	CorrectionField     uint64
	MessageTypeSpecific uint32
	SourcePortIdentity  PortIdentity
	SequenceID          uint16
	ControlField        Control
	LogMessageInterval  uint8
}

type MgmtMsgStarter interface {
	MgmtMsgStart() *MgmtMsgFixed
}

func (m *MgmtMsg[V]) MgmtMsgStart() *MgmtMsgFixed {
	return &m.MgmtMsgFixed
}

type MessageType uint8

const (
	MgmtMessageType MessageType = 0xD
)

type Control uint8

const (
	ControlMgmt Control = 0x4
)

type PortIdentity struct {
	ClockIdentity [8]byte
	PortNumber    uint16
}

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

// Offset of length field of TLV within MgmtMsg.
const OffsetofTLVLength = 50

type MgmtID uint16

const (
	IDGrandmasterSettingsNP MgmtID = 0xC001
)

type MgmtV[D MgmtData] struct {
	MgmtID MgmtID
	Data         D
}

type MgmtData interface {
	MgmtID() MgmtID
}

type MgmtErrorID uint16

const (
	_ MgmtErrorID = iota
	MgmtErrorResponseTooBig
	MgmtErrorNoSuchID
	MgmtErrorWrongLength
	MgmtErrorWrongValue
	MgmtErrorNotSetable
	MgmtErrorNotSupported
	MgmtErrorUnpopulated
	MgmtErrorGeneralError MgmtErrorID = 0xFFFE
)

type MgmtErrorStatusV struct {
	MgmtErrorID MgmtErrorID
	MgmtID      MgmtID
	_                 [4]byte
	DisplayData       string
}

type MgmtErrorStatusMsg = MgmtMsg[MgmtErrorStatusV]

type BinaryReaderFrom interface {
	ReadBinaryFrom(io.Reader) error
}

type BinaryWriterTo interface {
	WriteBinaryTo(io.Writer) error
}

func UnmarshalMgmtMsg(data []byte) (any, error) {
	var f MgmtMsgFixed
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.BigEndian, &f); err != nil {
		return nil, err
	}
	var msg MgmtMsgStarter
	switch f.TLVType {
	case TLVTypeMgmt:
		var mid MgmtID
		var err error
		if err = binary.Read(r, binary.BigEndian, &mid); err != nil {
			return nil, err
		}
		switch mid {
		case IDGrandmasterSettingsNP:
			msg, err = unmarshalMgmtV[GrandmasterSettingsNP](r)
		default:
			return nil, fmt.Errorf("unsupported management ID: 0x%04x", mid)
		}
		if err != nil {
			return nil, err
		}
	case TLVTypeMgmtErrorStatus:
		var v MgmtErrorStatusV
		if err := v.ReadBinaryFrom(r); err != nil {
			return nil, err
		}
		m := new(MgmtErrorStatusMsg)
		m.V = v
		msg = m
	default:
		return nil, fmt.Errorf("unsupported TLV type: 0x%04x", f.TLVType)
	}
	*msg.MgmtMsgStart() = f
	_, err := r.ReadByte()
	if err == nil {
		return nil, fmt.Errorf("trailing bytes")
	}
	if err != io.EOF {
		return nil, err
	}
	return msg, nil
}

func unmarshalMgmtV[D MgmtData](r io.Reader) (MgmtMsgStarter, error) {
	m := MgmtMsg[MgmtV[D]]{}
	if err := binary.Read(r, binary.BigEndian, &m.V.Data); err != nil {
		return nil, err
	}
	m.V.MgmtID = m.V.Data.MgmtID()
	return &m, nil
}

func (m *MgmtMsg[V]) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	var err error
	if err = binary.Write(buf, binary.BigEndian, &m.MgmtMsgFixed); err != nil {
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

func (m *MgmtMsg[V]) fixupBinaryLength(data []byte) error {
	l := len(data)
	if l > 0xffff {
		return fmt.Errorf("message too long")
	}
	// Write length field in Header
	lenBytes := data[2:4]
	binary.BigEndian.PutUint16(lenBytes, uint16(l))
	// Write length field in TLV
	lenBytes = data[OffsetofTLVLength : OffsetofTLVLength+2]
	binary.BigEndian.PutUint16(lenBytes, uint16(l-(OffsetofTLVLength+2)))
	return nil
}

func (m *MgmtMsg[T]) SetLength(l uint16) {
	m.Header.MessageLength = uint16(l)
	m.TLVLength = l - (OffsetofTLVLength + 2)
}

func (m *MgmtErrorStatusV) WriteBinaryTo(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, &m.MgmtErrorID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, &m.MgmtID); err != nil {
		return err
	}
	var reserved [4]byte
	if err := binary.Write(w, binary.BigEndian, &reserved); err != nil {
		return err
	}
	if err := writePTPText(w, m.DisplayData); err != nil {
		return err
	}
	if len(m.DisplayData)%2 == 0 {
		var padding uint8
		if err := binary.Write(w, binary.BigEndian, &padding); err != nil {
			return err
		}
	}
	return nil
}

func (m *MgmtErrorStatusV) ReadBinaryFrom(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.MgmtErrorID); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &m.MgmtID); err != nil {
		return err
	}
	var reserved [4]byte
	if err := binary.Read(r, binary.BigEndian, &reserved); err != nil {
		return err
	}
	if err := readPTPText(r, &m.DisplayData); err != nil {
		return err
	}
	// The padding needs to make the length of the TLV even.
	// Since there is a single byte of length, even lengths require a padding byte.
	if len(m.DisplayData)%2 == 0 {
		var padding uint8
		if err := binary.Read(r, binary.BigEndian, &padding); err != nil {
			return err
		}
	}
	return nil
}

func writePTPText(w io.Writer, s string) error {
	l := len(s)
	if l > 255 {
		return fmt.Errorf("PTPText too long: %d", len(s))
	}
	lengthField := uint8(l)
	if err := binary.Write(w, binary.BigEndian, &lengthField); err != nil {
		return err
	}
	textField := ([]byte)(s)
	return binary.Write(w, binary.BigEndian, &textField)
}

func readPTPText(r io.Reader, p *string) error {
	lengthField := uint8(0)
	if err := binary.Read(r, binary.BigEndian, &lengthField); err != nil {
		return err
	}
	textField := make([]byte, lengthField)
	if err := binary.Read(r, binary.BigEndian, &textField); err != nil {
		return err
	}
	*p = string(textField)
	return nil
}

var AnyPortIdentity = PortIdentity{[8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 0xffff}

type ClockQuality struct {
	ClockClass              uint8
	ClockAccuracy           uint8
	OffsetScaledLogVariance uint16
}

// Same as high bits of FlagField in the header
type TimeFlags uint8

const (
	Leap61 TimeFlags = 1 << iota
	Leap59
	CurrentUTCOffsetValid
	PTPTimescale
	TimeTraceable
	FrequencyTraceable
	SynchronizationUncertain
)

type GrandmasterSettingsNPMsg = MgmtMsg[MgmtV[GrandmasterSettingsNP]]

type GrandmasterSettingsNP struct {
	ClockQuality ClockQuality
	UTCOffset    int16
	TimeFlags    TimeFlags
	TimeSource   uint8
}

func (gsn GrandmasterSettingsNP) MgmtID() MgmtID {
	return IDGrandmasterSettingsNP
}

type MgmtClient struct {
	sequenceID uint16
	portNumber uint16
	domain     uint8
}

func NewMgmtClient() *MgmtClient {
	return &MgmtClient{
		portNumber: uint16(os.Getpid()),
	}
}

func (c *MgmtClient) getSequenceID() uint16 {
	id := c.sequenceID
	c.sequenceID++
	return id
}

func (c *MgmtClient) SetHeader(h *Header) {
	h.MessageType = MgmtMessageType // XXX this also has transport specific
	// MessageLength is filled in by MarshalBinary
	h.Version = 2
	h.DomainNumber = c.domain
	h.SequenceID = c.getSequenceID()
	h.ControlField = ControlMgmt
	h.SourcePortIdentity.PortNumber = c.portNumber
	h.LogMessageInterval = 0x7f // required by Table 42 of the standard
}

func MgmtSetBinaryMsg[D MgmtData](c *MgmtClient, data D) ([]byte, error) {
	msg := NewMgmtSetMsg(c, data)
	return msg.MarshalBinary()
}

func NewMgmtSetMsg[D MgmtData](c *MgmtClient, data D) *MgmtMsg[MgmtV[D]] {
	msg := new(MgmtMsg[MgmtV[D]])
	// Header
	c.SetHeader(&msg.Header)
	// MgmtBody
	msg.ActionField = ActionSet
	msg.TargetPortIdentity = AnyPortIdentity
	msg.TLVType = TLVTypeMgmt
	// length will be fixed by MarshalBinary
	// MgmtTLV
	SetMgmtV(&msg.V, data)
	return msg
}

func SetMgmtV[D MgmtData](mv *MgmtV[D], data D) {
	// tlv.LengthField is filled in by MarshalBinary
	mv.MgmtID = data.MgmtID()
	mv.Data = data
}

func NewGrandmasterSettingsNPMsg(c *MgmtClient, gsn GrandmasterSettingsNP) *GrandmasterSettingsNPMsg {
	return NewMgmtSetMsg(c, gsn)
}

func NewMgmtErrorStatusMsg(c *MgmtClient, eid MgmtErrorID, mid MgmtID, display string) *MgmtErrorStatusMsg {
	msg := new(MgmtErrorStatusMsg)
	// Header
	c.SetHeader(&msg.Header)
	// MgmtBody
	msg.ActionField = ActionResponse // or AcknowledgeAction?
	msg.TargetPortIdentity = AnyPortIdentity

	msg.TLVType = TLVTypeMgmtErrorStatus
	// LengthField is filled in by MarshalBinary
	msg.V.MgmtErrorID = eid
	msg.V.MgmtID = mid
	msg.V.DisplayData = display
	return msg
}
