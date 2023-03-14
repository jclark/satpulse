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

const IDGrandmasterSettingsNP = 0xC001
const SizeofManagementHeaderBody = 48
const SizeofGrandmasterSettingsNP = 8
const ManagementMessageType = 0xD

type GrandmasterSettingsNPMsg struct {
	Header
	ManagementBody
	ManagementTLV
	GrandmasterSettingsNP
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
	SequenceId          uint16
	ControlField        uint8
	LogMessageInterval  uint8
}

const SetAction = 0x01

type ManagementBody struct {
	TargetPortIdentity   PortIdentity
	StartingBoundaryHops uint8
	BoundaryHops         uint8
	ActionField          uint8
	_                    uint8
}

var AnyPortIdentity = PortIdentity{[8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 0xffff}

type PortIdentity struct {
	ClockIdentity [8]byte
	PortNumber    uint16
}

type ManagementTLV struct {
	TLVType      uint16
	LengthField  uint16
	ManagementID uint16
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
	SyncchronizationUncertain
)

type GrandmasterSettingsNP struct {
	ClockQuality ClockQuality
	UTCOffset    int16
	TimeFlags    TimeFlags
	TimeSource   uint8
}

const TLVTypeManagement = 0x0001
const ControlManagement = 0x4
const testPID = 12621

func GetPID() int {
	return testPID
}

func NewGrandmasterSettingsNPMsg() *GrandmasterSettingsNPMsg {
	msg := new(GrandmasterSettingsNPMsg)
	// Header
	msg.MessageType = ManagementMessageType // XXX this also has transport specific
	msg.Version = 2
	msg.MessageLength = SizeofManagementHeaderBody + 6 + SizeofGrandmasterSettingsNP
	msg.ControlField = ControlManagement
	msg.SourcePortIdentity.PortNumber = uint16(GetPID()) // They will reply on /var/run/pmc.$pid
	msg.LogMessageInterval = 0x7f                        // required by Table 42 of the standard
	// ManagementBody
	msg.ActionField = SetAction
	msg.TargetPortIdentity = AnyPortIdentity
	// ManagementTLV
	msg.TLVType = TLVTypeManagement
	msg.ManagementID = IDGrandmasterSettingsNP
	msg.LengthField = SizeofGrandmasterSettingsNP + 2

	// GrandmasterSettingsNP
	msg.ClockQuality = ClockQuality{
		ClockClass:              0x6,
		ClockAccuracy:           0x23,
		OffsetScaledLogVariance: 0xFFFF,
	}
	msg.UTCOffset = 37
	msg.TimeFlags = CurrentUTCOffsetValid | PTPTimescale | TimeTraceable
	msg.TimeSource = 0xA0 // what is this?
	return msg
}

func run() error {
	gsn := NewGrandmasterSettingsNPMsg()
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
