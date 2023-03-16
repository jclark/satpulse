package pmc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type ManagementMsg[V any] struct {
	ManagementMsgFixed
	V V
}

type ManagementMsgFixed struct {
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

type ManagementMsgStarter interface {
	ManagementMsgStart() *ManagementMsgFixed
}

func (m *ManagementMsg[V]) ManagementMsgStart() *ManagementMsgFixed {
	return &m.ManagementMsgFixed
}

type MessageType uint8

const (
	ManagementMessageType MessageType = 0xD
)

type Control uint8

const (
	ControlManagement Control = 0x4
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
	TLVTypeManagement            TLVType = 0x0001
	TLVTypeManagementErrorStatus TLVType = 0x0002
)

// Offset of length field of TLV within ManagementMsg.
const OffsetofTLVLength = 50

type ManagementID uint16

const (
	IDGrandmasterSettingsNP ManagementID = 0xC001
)

type ManagementV[D ManagementData] struct {
	ManagementID ManagementID
	Data         D
}

type ManagementData interface {
	ManagementID() ManagementID
}

type ManagementErrorID uint16

const (
	_ ManagementErrorID = iota
	ManagementErrorResponseTooBig
	ManagementErrorNoSuchID
	ManagementErrorWrongLength
	ManagementErrorWrongValue
	ManagementErrorNotSetable
	ManagementErrorNotSupported
	ManagementErrorUnpopulated
	ManagementErrorGeneralError ManagementErrorID = 0xFFFE
)

type ManagementErrorStatusV struct {
	ManagementErrorID ManagementErrorID
	ManagementID      ManagementID
	_                 [4]byte
	DisplayData       string
}

type ManagementErrorStatusMsg = ManagementMsg[ManagementErrorStatusV]

type BinaryReaderFrom interface {
	ReadBinaryFrom(io.Reader) error
}

type BinaryWriterTo interface {
	WriteBinaryTo(io.Writer) error
}

func UnmarshalManagementMsg(data []byte) (any, error) {
	var f ManagementMsgFixed
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.BigEndian, &f); err != nil {
		return nil, err
	}
	var msg ManagementMsgStarter
	switch f.TLVType {
	case TLVTypeManagement:
		var mid ManagementID
		var err error
		if err = binary.Read(r, binary.BigEndian, &mid); err != nil {
			return nil, err
		}
		switch mid {
		case IDGrandmasterSettingsNP:
			msg, err = unmarshalManagementV[GrandmasterSettingsNP](r)
		default:
			return nil, fmt.Errorf("unsupported management ID: 0x%04x", mid)
		}
		if err != nil {
			return nil, err
		}
	case TLVTypeManagementErrorStatus:
		var v ManagementErrorStatusV
		if err := v.ReadBinaryFrom(r); err != nil {
			return nil, err
		}
		m := new(ManagementErrorStatusMsg)
		m.V = v
		msg = m
	default:
		return nil, fmt.Errorf("unsupported TLV type: 0x%04x", f.TLVType)
	}
	*msg.ManagementMsgStart() = f
	_, err := r.ReadByte()
	if err == nil {
		return nil, fmt.Errorf("trailing bytes")
	}
	if err != io.EOF {
		return nil, err
	}
	return msg, nil
}

func unmarshalManagementV[D ManagementData](r io.Reader) (ManagementMsgStarter, error) {
	m := ManagementMsg[ManagementV[D]]{}
	if err := binary.Read(r, binary.BigEndian, &m.V.Data); err != nil {
		return nil, err
	}
	m.V.ManagementID = m.V.Data.ManagementID()
	return &m, nil
}

func (m *ManagementMsg[V]) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	var err error
	if err = binary.Write(buf, binary.BigEndian, &m.ManagementMsgFixed); err != nil {
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

func (m *ManagementMsg[V]) fixupBinaryLength(data []byte) error {
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

func (m *ManagementMsg[T]) SetLength(l uint16) {
	m.Header.MessageLength = uint16(l)
	m.TLVLength = l - (OffsetofTLVLength + 2)
}

func (m *ManagementErrorStatusV) WriteBinaryTo(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, &m.ManagementErrorID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, &m.ManagementID); err != nil {
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

func (m *ManagementErrorStatusV) ReadBinaryFrom(r io.Reader) error {
	if err := binary.Read(r, binary.BigEndian, &m.ManagementErrorID); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &m.ManagementID); err != nil {
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

type GrandmasterSettingsNPMsg = ManagementMsg[ManagementV[GrandmasterSettingsNP]]

type GrandmasterSettingsNP struct {
	ClockQuality ClockQuality
	UTCOffset    int16
	TimeFlags    TimeFlags
	TimeSource   uint8
}

func (gsn GrandmasterSettingsNP) ManagementID() ManagementID {
	return IDGrandmasterSettingsNP
}

type ManagementClient struct {
	sequenceID uint16
	portNumber uint16
	domain     uint8
}

func NewManagementClient() *ManagementClient {
	return &ManagementClient{
		portNumber: uint16(os.Getpid()),
	}
}

func (c *ManagementClient) getSequenceID() uint16 {
	id := c.sequenceID
	c.sequenceID++
	return id
}

func (c *ManagementClient) SetHeader(h *Header) {
	h.MessageType = ManagementMessageType // XXX this also has transport specific
	// MessageLength is filled in by MarshalBinary
	h.Version = 2
	h.DomainNumber = c.domain
	h.SequenceID = c.getSequenceID()
	h.ControlField = ControlManagement
	h.SourcePortIdentity.PortNumber = c.portNumber
	h.LogMessageInterval = 0x7f // required by Table 42 of the standard
}

func ManagementSetBinaryMsg[D ManagementData](c *ManagementClient, data D) ([]byte, error) {
	msg := NewManagementSetMsg(c, data)
	return msg.MarshalBinary()
}

func NewManagementSetMsg[D ManagementData](c *ManagementClient, data D) *ManagementMsg[ManagementV[D]] {
	msg := new(ManagementMsg[ManagementV[D]])
	// Header
	c.SetHeader(&msg.Header)
	// ManagementBody
	msg.ActionField = ActionSet
	msg.TargetPortIdentity = AnyPortIdentity
	msg.TLVType = TLVTypeManagement
	// length will be fixed by MarshalBinary
	// ManagementTLV
	SetManagementV(&msg.V, data)
	return msg
}

func SetManagementV[D ManagementData](mv *ManagementV[D], data D) {
	// tlv.LengthField is filled in by MarshalBinary
	mv.ManagementID = data.ManagementID()
	mv.Data = data
}

func NewGrandmasterSettingsNPMsg(c *ManagementClient, gsn GrandmasterSettingsNP) *GrandmasterSettingsNPMsg {
	return NewManagementSetMsg(c, gsn)
}

func NewManagementErrorStatusMsg(c *ManagementClient, eid ManagementErrorID, mid ManagementID, display string) *ManagementErrorStatusMsg {
	msg := new(ManagementErrorStatusMsg)
	// Header
	c.SetHeader(&msg.Header)
	// ManagementBody
	msg.ActionField = ActionResponse // or AcknowledgeAction?
	msg.TargetPortIdentity = AnyPortIdentity

	msg.TLVType = TLVTypeManagementErrorStatus
	// LengthField is filled in by MarshalBinary
	msg.V.ManagementErrorID = eid
	msg.V.ManagementID = mid
	msg.V.DisplayData = display
	return msg
}
