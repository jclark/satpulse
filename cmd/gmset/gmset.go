package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"golang.org/x/exp/slices"
)

var header string = "\x0d\x02\x00\x3e\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x31\x4d\x00\x00\x04\x7f"
var body string = "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x01\x00"
var tlv string = "\x00\x01\x00\x0a\xc0\x01"
var grandmaster_settings_np string = "\x06\x23\xff\xff\x00\x25\x1c\xa0"

var msg = header + body + tlv + grandmaster_settings_np

const SizeofManagementHeaderBody = 48
const ManagementMessageType = 0xD

type ManagementMsg[V any] struct {
	Header Header
	ManagementBody
	TLV TLV[V]
}

type Header struct {
	MessageType         uint8
	Version             uint8
	MessageLength       uint16
	DomainNumber        uint8
	MinorSdoID          uint8
	FlagField           uint16
	CorrectionField     uint64
	MessageTypeSpecific uint32
	SourcePortIdentity  PortIdentity
	SequenceID          uint16
	ControlField        uint8
	LogMessageInterval  uint8
}

type ManagementBody struct {
	TargetPortIdentity   PortIdentity
	StartingBoundaryHops uint8
	BoundaryHops         uint8
	ActionField          Action
	_                    uint8
}

type Action uint8

const (
	GetAction Action = iota
	SetAction
	ResponseAction
	CommandAction
	AcknowledgeAction
)

// Offset of length field of TLV within ManagementMsg.
const OffsetofTLVLength = 50

type TLV[V any] struct {
	TLVType     uint16
	LengthField uint16
	ValueField  V
}

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

type BinaryReaderFrom interface {
	ReadBinaryFrom(io.Reader) error
}

type BinaryWriterTo interface {
	WriteBinaryTo(io.Writer) error
}

func (m *ManagementMsg[V]) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.BigEndian, &m.Header); err != nil {
		return err
	}
	err := binary.Read(r, binary.BigEndian, &m.ManagementBody)
	if err != nil {
		return err
	}
	br, ok := any(&m.TLV).(BinaryReaderFrom)
	if ok {
		err = br.ReadBinaryFrom(r)
	} else {
		err = binary.Read(r, binary.BigEndian, &m.TLV)
	}
	if err != nil {
		return err
	}
	_, err = r.ReadByte()
	if err == nil {
		return fmt.Errorf("trailing bytes")
	}
	if err != io.EOF {
		return err
	}
	return nil
}

func (m *ManagementMsg[V]) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, &m.Header); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, &m.ManagementBody); err != nil {
		return nil, err
	}
	if err := m.TLV.WriteBinaryTo(buf); err != nil {
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
	m.TLV.LengthField = l - (OffsetofTLVLength + 2)
}

func (t *TLV[V]) WriteBinaryTo(w io.Writer) error {
	err := binary.Write(w, binary.BigEndian, &t.TLVType)
	if err != nil {
		return err
	}
	err = binary.Write(w, binary.BigEndian, &t.LengthField)
	if err != nil {
		return err
	}
	bw, ok := any(&t.ValueField).(BinaryWriterTo)
	if ok {
		err = bw.WriteBinaryTo(w)
	} else {
		err = binary.Write(w, binary.BigEndian, &t.ValueField)
	}
	if err != nil {
		return err
	}
	return nil
}

func (t *TLV[V]) ReadBinaryFrom(r io.Reader) error {
	err := binary.Read(r, binary.BigEndian, &t.TLVType)
	if err != nil {
		return err
	}
	err = binary.Read(r, binary.BigEndian, &t.LengthField)
	if err != nil {
		return err
	}
	br, ok := any(&t.ValueField).(BinaryReaderFrom)
	if ok {
		err = br.ReadBinaryFrom(r)
	} else {
		err = binary.Read(r, binary.BigEndian, &t.ValueField)
	}
	if err != nil {
		return err
	}
	return nil
}

var AnyPortIdentity = PortIdentity{[8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 0xffff}

type PortIdentity struct {
	ClockIdentity [8]byte
	PortNumber    uint16
}

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

type GrandmasterSettingsNP struct {
	ClockQuality ClockQuality
	UTCOffset    int16
	TimeFlags    TimeFlags
	TimeSource   uint8
}

func (gsn GrandmasterSettingsNP) ManagementID() ManagementID {
	return IDGrandmasterSettingsNP
}

const TLVTypeManagement = 0x0001
const ControlManagement = 0x4

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

func ManagementBinaryMsg[D ManagementData](c *ManagementClient, data D) ([]byte, error) {
	msg := NewManagementSetMsg(c, data)
	return msg.MarshalBinary()
}

func NewManagementSetMsg[D ManagementData](c *ManagementClient, data D) *ManagementMsg[ManagementV[D]] {
	msg := new(ManagementMsg[ManagementV[D]])
	// Header
	c.SetHeader(&msg.Header)
	// ManagementBody
	msg.ActionField = SetAction
	msg.TargetPortIdentity = AnyPortIdentity
	// ManagementTLV
	SetManagementDataTLV(&msg.TLV, data)
	return msg
}

func SetManagementDataTLV[D ManagementData](tlv *TLV[ManagementV[D]], data D) {
	tlv.TLVType = TLVTypeManagement
	// tlv.LengthField is filled in by MarshalBinary
	tlv.ValueField.ManagementID = data.ManagementID()
	tlv.ValueField.Data = data
}

type GrandmasterSettingsNPMsg = ManagementMsg[ManagementV[GrandmasterSettingsNP]]

func NewGrandmasterSettingsNPMsg(c *ManagementClient) *GrandmasterSettingsNPMsg {
	return NewManagementSetMsg(c, GrandmasterSettingsNP{
		ClockQuality: ClockQuality{
			ClockClass:              0x6,
			ClockAccuracy:           0x23,
			OffsetScaledLogVariance: 0xFFFF,
		},
		UTCOffset:  37,
		TimeFlags:  CurrentUTCOffsetValid | PTPTimescale | TimeTraceable,
		TimeSource: 0xA0, // what is this?
	})
}

const testPID = 12621

func run() error {
	client := NewManagementClient()
	client.portNumber = testPID
	gsn := NewGrandmasterSettingsNPMsg(client)
	bytes, err := gsn.MarshalBinary()
	if err != nil {
		return fmt.Errorf("first MarshalBinaryFailed: %v", err)
	}
	if len(msg) != len(bytes) {
		return fmt.Errorf("wrong length: got %d, want %d", len(bytes), len(msg))
	}
	for i := 0; i < len(msg); i++ {
		if msg[i] != bytes[i] {
			fmt.Printf("wrong byte at %d: got %02x, want %02x\n", i, bytes[i], msg[i])
		}
	}
	gsn.SetLength(uint16(len(bytes)))
	gsn2 := new(GrandmasterSettingsNPMsg)
	err = gsn2.UnmarshalBinary(bytes)
	if err != nil {
		return fmt.Errorf("UnmarshalBinary failed: %v", err)
	}
	if gsn.TLV.ValueField.Data != gsn2.TLV.ValueField.Data {
		return fmt.Errorf("data not round tripped: got %v, want %v", gsn2.TLV.ValueField.Data, gsn.TLV.ValueField.Data)
	}
	bytes2, err := gsn2.MarshalBinary()
	if err != nil {
		return err
	}
	if !slices.Equal(bytes, bytes2) {
		return fmt.Errorf("got different bytes: got %v, want %v", bytes2, bytes)
	}
	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
