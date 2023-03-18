package pmc

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
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

type MessageType uint8

const (
	MessageTypeMgmt MessageType = 0xD
)

type Control uint8

const (
	ControlMgmt Control = 0x4
)

type ClockIdentity [8]byte

type PortIdentity struct {
	ClockIdentity ClockIdentity
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

type MgmtID uint16

const (
	MIDDefaultDataSet        MgmtID = 0x2000
	MIDCurrentDataSet        MgmtID = 0x2001
	MIDParentDataSet         MgmtID = 0x2002
	MIDTimePropertiesDataSet MgmtID = 0x2003
	MIDGrandmasterSettings   MgmtID = 0xC001
)

func (mid MgmtID) String() string {
	// return the same strings as in the standard
	switch mid {
	case MIDDefaultDataSet:
		return "DEFAULT_DATA_SET"
	case MIDCurrentDataSet:
		return "CURRENT_DATA_SET"
	case MIDParentDataSet:
		return "PARENT_DATA_SET"
	case MIDTimePropertiesDataSet:
		return "TIME_PROPERTIES_DATA_SET"
	case MIDGrandmasterSettings:
		return "GRANDMASTER_SETTINGS_NP"
	}
	return fmt.Sprintf("0x%0x", uint16(mid))
}

type MgmtValue[D MgmtData] struct {
	MgmtID MgmtID
	Data   D
}

type MgmtData interface {
	MgmtID() MgmtID
}

type MgmtErrorID uint16

const (
	_ MgmtErrorID = iota
	MEIDResponseTooBig
	MEIDNoSuchID
	MEIDWrongLength
	MEIDWrongValue
	MEIDNotSetable
	MEIDNotSupported
	MEIDUnpopulated
	MEIDGeneralError MgmtErrorID = 0xFFFE
)

func (meid MgmtErrorID) String() string {
	// return the same strings as in the standard
	switch meid {
	case MEIDResponseTooBig:
		return "RESPONSE_TOO_BIG"
	case MEIDNoSuchID:
		return "NO_SUCH_ID"
	case MEIDWrongLength:
		return "WRONG_LENGTH"
	case MEIDWrongValue:
		return "WRONG_VALUE"
	case MEIDNotSetable:
		return "NOT_SETABLE"
	case MEIDNotSupported:
		return "NOT_SUPPORTED"
	case MEIDUnpopulated:
		return "UNPOPULATED"
	case MEIDGeneralError:
		return "GENERAL_ERROR"
	}
	return fmt.Sprintf("0x%0x", uint16(meid))
}

type MgmtErrorStatus struct {
	MgmtErrorID MgmtErrorID
	MgmtID      MgmtID
	_           [4]byte
	DisplayData *string
}

type MgmtErrorStatusMsg = MgmtMsgWithValue[MgmtErrorStatus]

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

func unmarshalMID(r io.Reader, mid MgmtID) (MgmtMsg, error) {
	switch mid {
	case MIDGrandmasterSettings:
		return unmarshalMgmtValue[GrandmasterSettings](r)
	case MIDDefaultDataSet:
		return unmarshalMgmtValue[DefaultDS](r)
	case MIDCurrentDataSet:
		return unmarshalMgmtValue[CurrentDS](r)
	case MIDParentDataSet:
		return unmarshalMgmtValue[ParentDS](r)
	case MIDTimePropertiesDataSet:
		return unmarshalMgmtValue[TimePropertiesDS](r)
	default:
		return nil, fmt.Errorf("unsupported management ID: 0x%04x", mid)
	}
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

func (m *MgmtErrorStatus) WriteBinaryTo(w io.Writer) error {
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
	if m.DisplayData == nil {
		return nil
	}
	text := *m.DisplayData
	if err := writePTPText(w, text); err != nil {
		return err
	}
	if len(text)%2 == 0 {
		var padding uint8
		if err := binary.Write(w, binary.BigEndian, &padding); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalMgmtErrorStatus(r *bytes.Reader, m *MgmtErrorStatus) error {
	if err := binary.Read(r, binary.BigEndian, &m.MgmtErrorID); err != nil {
		return fmt.Errorf("unmarshaling MgmtErrorID failed: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &m.MgmtID); err != nil {
		return fmt.Errorf("unmarshaling MgmtID failed: %w", err)
	}
	var reserved [4]byte
	if err := binary.Read(r, binary.BigEndian, &reserved); err != nil {
		return fmt.Errorf("unmarshaling MgmtErrorStatus reserved bytes failed: %w", err)
	}
	// The DisplayData can be omitted completely.
	if r.Len() == 0 {
		return nil
	}
	text, err := readPTPText(r)
	if err != nil {
		return err
	}
	m.DisplayData = &text
	// The padding needs to make the length of the TLV even.
	// Since there is a single byte of length, even lengths require a padding byte.
	if len(text)%2 == 0 {
		var padding uint8
		if err := binary.Read(r, binary.BigEndian, &padding); err != nil {
			return fmt.Errorf("unmarshaling MgmtErrorStatus padding failed: %w", err)
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

func readPTPText(r io.Reader) (string, error) {
	lengthField := uint8(0)
	if err := binary.Read(r, binary.BigEndian, &lengthField); err != nil {
		return "", fmt.Errorf("unmarshaling PTPText length failed: %w", err)
	}
	textField := make([]byte, lengthField)
	if err := binary.Read(r, binary.BigEndian, &textField); err != nil {
		return "", fmt.Errorf("unmarshaling PTPText string failed: %w", err)
	}
	return string(textField), nil
}

func AnyClockIdentity() ClockIdentity {
	return ClockIdentity([8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
}

func AnyPortIdentity() PortIdentity {
	return PortIdentity{AnyClockIdentity(), 0xffff}
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

type GrandmasterSettingsMsg = MgmtMsgWithValue[MgmtValue[GrandmasterSettings]]

type GrandmasterSettings struct {
	ClockQuality ClockQuality
	UTCOffset    int16
	TimeFlags    TimeFlags
	TimeSource   uint8
}

func (GrandmasterSettings) MgmtID() MgmtID {
	return MIDGrandmasterSettings
}

type DefaultDS struct {
	Flags         DefaultDSFlags
	_             uint8
	NumberPorts   uint16
	Priority1     uint8
	ClockQuality  ClockQuality
	Priority2     uint8
	ClockIdentity ClockIdentity
	DomainNumber  uint8
	_             uint8
}

type DefaultDSFlags uint8

const (
	DefaultDSFlagTSC DefaultDSFlags = 1 << iota
	DefaultDSFlagSO
)

func (DefaultDS) MgmtID() MgmtID {
	return MIDDefaultDataSet
}

func (ds DefaultDS) TwoStepFlag() bool {
	return ds.Flags&DefaultDSFlagTSC != 0
}

func (ds DefaultDS) SlaveOnly() bool {
	return ds.Flags&DefaultDSFlagSO != 0
}

type TimeInterval int64

func (ti TimeInterval) Duration() time.Duration {
	scaledNanos := int64(ti)
	nanos := scaledNanos >> 16
	frac := scaledNanos & 0xffff
	if frac > 0x8000 {
		nanos++
	}
	return time.Duration(nanos)
}

func (ti TimeInterval) Nanos() float64 {
	return float64(int64(ti)) / 65536.0
}
func (ti TimeInterval) String() string {
	return fmt.Sprintf("%0.1f", ti.Nanos())
}

type CurrentDS struct {
	StepsRemoved     uint16
	OffsetFromMaster TimeInterval
	MeanPathDelay    TimeInterval
}

func (CurrentDS) MgmtID() MgmtID {
	return MIDCurrentDataSet
}

type ParentDS struct {
	ParentPortIdentity                    PortIdentity
	ParentStats                           bool
	_                                     uint8
	ObservedParentOffsetScaledLogVariance uint16
	ObservedParentClockPhaseChangeRate    int32
	GrandmasterPriority1                  uint8
	GrandmasterClockQuality               ClockQuality
	GrandmasterPriority2                  uint8
	GrandmasterIdentity                   ClockIdentity
}

func (ParentDS) MgmtID() MgmtID {
	return MIDParentDataSet
}

type TimePropertiesDS struct {
	CurrentUTCOffset int16
	Flags            TimePropertiesDSFlags
	TimeSource       uint8
}

type TimePropertiesDSFlags uint8

const (
	TimePropertiesDSFlagLI61 TimePropertiesDSFlags = iota
	TimePropertiesDSFlagLI59
	TimePropertiesDSFlagUTCV
	TimePropertiesDSFlagPTP
	TimePropertiesDSFlagTTRA
	TimePropertiesDSFlagFTRA
)

func (TimePropertiesDS) MgmtID() MgmtID {
	return MIDTimePropertiesDataSet
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

func (c *MgmtClient) PrepareMsg(m MgmtMsg) {
	h := &m.Prefix().Header
	h.DomainNumber = c.domain
	h.SourcePortIdentity.PortNumber = c.portNumber
	h.SequenceID = c.getSequenceID()
}

func (c *MgmtClient) getSequenceID() uint16 {
	id := c.sequenceID
	c.sequenceID++
	return id
}

func GrandmasterSettingsBinaryMsg(c *MgmtClient, data GrandmasterSettings) ([]byte, error) {
	return MgmtSetBinaryMsg(c, data)
}

func MgmtSetBinaryMsg[D MgmtData](c *MgmtClient, data D) ([]byte, error) {
	msg := NewMgmtSetMsg(data)
	c.PrepareMsg(msg)
	return msg.MarshalBinary()
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
	h.MessageType = MessageTypeMgmt // We can add transport specific stuff in MgmtClient.PrepareMsg
	// MessageLength is filled in by MarshalBinary
	h.Version = 2
	h.ControlField = ControlMgmt
	h.LogMessageInterval = 0x7f // required by Table 42 of the standard
}

func NewMgmtErrorStatusMsg(meid MgmtErrorID, mid MgmtID, display string) *MgmtErrorStatusMsg {
	msg := new(MgmtErrorStatusMsg)
	// Header
	initHeader(&msg.Header)
	// MgmtBody
	msg.ActionField = ActionResponse // or AcknowledgeAction?
	msg.TargetPortIdentity = AnyPortIdentity()
	msg.TLVType = TLVTypeMgmtErrorStatus
	// LengthField is filled in by MarshalBinary
	msg.V.MgmtErrorID = meid
	msg.V.MgmtID = mid
	if len(display) > 0 {
		msg.V.DisplayData = &display
	}
	return msg
}
