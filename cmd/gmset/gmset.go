package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

var header string = "\x0d\x02\x00\x3e\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x31\x4d\x00\x00\x04\x7f"
var body string = "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x01\x00"
var tlv string = "\x00\x01\x00\x0a\xc0\x01"
var grandmaster_settings_np string = "\x06\x23\xff\xff\x00\x25\x1c\xa0"

var msg = header + body + tlv + grandmaster_settings_np

const SizeofManagementHeaderBody = 48
const ManagementMessageType = 0xD

type ManagementID uint16

const (
	IDGrandmasterSettingsNP ManagementID = 0xC001
)

type ManagementData interface {
	ManagementID() ManagementID
}

const SetAction = 0x01

type ManagementMsg[D ManagementData] struct {
	Header               Header
	TargetPortIdentity   PortIdentity
	StartingBoundaryHops uint8
	BoundaryHops         uint8
	ActionField          uint8
	_                    uint8
	ManagementTLV        ManagementTLV[D]
}

type ManagementTLV[D ManagementData] struct {
	TLVType      uint16
	LengthField  uint16
	ManagementID ManagementID
	Data         D
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

func (c *ManagementClient) getSequenceID() uint16 {
	id := c.sequenceID
	c.sequenceID++
	return id
}

func NewManagementMsg[D ManagementData](c *ManagementClient, data D) *ManagementMsg[D] {
	msg := new(ManagementMsg[D])
	// Header
	h := &msg.Header
	h.MessageType = ManagementMessageType // XXX this also has transport specific
	h.Version = 2
	h.DomainNumber = c.domain
	h.SequenceID = c.getSequenceID()
	sz := uint16(binary.Size(&data))
	h.MessageLength = SizeofManagementHeaderBody + 6 + sz
	h.ControlField = ControlManagement
	h.SourcePortIdentity.PortNumber = c.portNumber
	h.LogMessageInterval = 0x7f // required by Table 42 of the standard
	// ManagementBody
	msg.ActionField = SetAction
	msg.TargetPortIdentity = AnyPortIdentity
	// ManagementTLV
	tlv := &msg.ManagementTLV
	tlv.TLVType = TLVTypeManagement
	tlv.ManagementID = data.ManagementID()
	tlv.LengthField = sz + 2
	tlv.Data = data
	return msg
}

func NewGrandmasterSettingsNPMsg(c *ManagementClient) *ManagementMsg[GrandmasterSettingsNP] {
	return NewManagementMsg(c, GrandmasterSettingsNP{
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

func NewManagementClient() *ManagementClient {
	return &ManagementClient{
		portNumber: uint16(os.Getpid()),
	}
}

const testPID = 12621

func run() error {
	client := NewManagementClient()
	client.portNumber = testPID
	gsn := NewGrandmasterSettingsNPMsg(client)
	buf := new(bytes.Buffer)

	err := binary.Write(buf, binary.BigEndian, gsn)
	if err != nil {
		return err
	}
	bytes := buf.Bytes()
	if len(msg) != len(bytes) {
		return fmt.Errorf("wrong length: got %d, want %d", len(bytes), len(msg))
	}
	for i := 0; i < len(msg); i++ {
		if msg[i] != bytes[i] {
			fmt.Printf("wrong byte at %d: got %02x, want %02x\n", i, bytes[i], msg[i])
		}
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
