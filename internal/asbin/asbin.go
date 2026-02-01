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

func (mid MsgID) unpack() (byte, byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

type Msg interface {
	ID() MsgID
}

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)

func (mid MsgID) String() string {
	idName := idNameMap[mid]
	cls, id := mid.unpack()
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

const (
	NavTimeID      MsgID = clsNav | (0x05 << 8)
	NavClockID     MsgID = clsNav | (0x22 << 8)
	NavClock2ID    MsgID = clsNav | (0x23 << 8)
	NavSvinID      MsgID = clsNav | (0x31 << 8)
	AckNakID       MsgID = clsAck | (0x00 << 8)
	AckAckID       MsgID = clsAck | (0x01 << 8)
	CfgCfgID       MsgID = clsCfg | (0x09 << 8)
	CfgNavSatID    MsgID = clsCfg | (0x0C << 8)
	CfgSurveyID    MsgID = clsCfg | (0x12 << 8)
	CfgFixedECEFID MsgID = clsCfg | (0x14 << 8)
	CfgPrtID       MsgID = clsCfg | (0x00 << 8)
	CfgMsgID       MsgID = clsCfg | (0x01 << 8)
	CfgPpsID       MsgID = clsCfg | (0x07 << 8)
	CfgSimpleRstID MsgID = clsCfg | (0x40 << 8)
	MonVerID       MsgID = clsMon | (0x04 << 8)
)

// These are used in CfgMsg
const (
	NmeaGgaID MsgID = clsNmea | (iota << 8)
	NmeaGllID
	NmeaGsaID
	NmeaGrsID
	NmeaGsvID
	NmeaRmcID
	NmeaVtgID
	NmeaZdaID
	NmeaTxtID MsgID = clsNmea | (0x20 << 8)
)

func init() {
	regMsg[NavTime]("TIME")
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
	regMsg[CfgCfg]("CFG")
	regMsg[CfgNavSat]("NAVSAT")
	regMsg[CfgSurvey]("SURVEY")
	regMsg[CfgFixedECEF]("FIXEDECEF")
	regMsg[CfgPrt]("PRT")
	regMsg[CfgMsg]("MSG")
	regMsg[CfgPps]("PPS")
	regMsg[CfgSimpleRst]("SIMPLERST")
	regMsg[MonVer]("VER")
	regMsg[NavClock]("CLOCK")
	regMsg[NavSvin]("SVIN")
}

// NavTimeSys defines the GNSS system values for NavTime.NavSys
type NavTimeSys byte

const (
	NavTimeSysGPS NavTimeSys = iota
	NavTimeSysBeiDou
	NavTimeSysGLONASS
	NavTimeSysGalileo
)

// NavTimeFlags defines the validity flag bits for NavTime.Flags
type NavTimeFlags byte

const (
	NavTimeFlagWeekValid    NavTimeFlags = 1 << iota // Bit 0: Week number valid
	NavTimeFlagSecondValid                           // Bit 1: Second of week valid
	NavTimeFlagLeapSecValid                          // Bit 2: Leap seconds valid
)

// NAV-TIME (0x01 0x05)
type NavTime struct {
	NavSys  NavTimeSys   // GNSS system: 0=GPS, 1=BD, 2=Glonass, 3=Galileo
	Flags   NavTimeFlags // Validity flags: bit0=week, bit1=second, bit2=leapsecond
	Fractow int16        // ns, Fractional part of time of week
	RefTow  uint32       // ms, GNSS time of week
	Week    uint16       // GNSS week number
	LeapSec int16        // s, Leap seconds to UTC
	TimeErr uint32       // ns, Estimated time error
}

func (m *NavTime) ID() MsgID { return NavTimeID }

// ACK-NAK (0x05 0x00) and ACK-ACK (0x05 0x01)
type Ack struct {
	MsgClass uint8 // Message class
	MsgID    uint8 // Message ID
}

func (m *Ack) ID() MsgID { return AckNakID } // This would be overridden for AckAck

type AckAck Ack

func (m *AckAck) ID() MsgID { return AckAckID }

type AckNak Ack

func (m *AckNak) ID() MsgID { return AckNakID }

// CfgCfgAction defines the action values for CfgCfg.Action
type CfgCfgAction uint32

const (
	CfgCfgActionSave  CfgCfgAction = iota // Save configuration
	CfgCfgActionLoad                      // Load configuration
	CfgCfgActionClear                     // Clear configuration
)

// CfgCfgMask defines the bitmask values for CfgCfg.Mask
type CfgCfgMask uint32

const (
	CfgCfgMaskBaudrate    CfgCfgMask = 1 << iota // Bit 0: Baudrate
	CfgCfgMaskNmeaMsgRate                        // Bit 1: NMEA message rate
	CfgCfgMaskNavSettings                        // Bit 2: Navigation settings
	_                                            // Bit 3: Reserved
	_                                            // Bit 4: Reserved (implicit)
	_                                            // Bit 5: Reserved
)

// Special value for factory reset
const CfgCfgMaskFactoryReset CfgCfgMask = 0xFFFFFFFF // Factory reset

// CFG-CFG (0x06 0x09)
type CfgCfg struct {
	Action CfgCfgAction
	Mask   CfgCfgMask // Bitmask of settings to affect
}

func (m *CfgCfg) ID() MsgID { return CfgCfgID }

// CfgNavSatMask defines the satellite system bitmask values for CfgNavSat.EnableMask
type CfgNavSatMask uint32

const (
	CfgNavSatMaskGPSL1      CfgNavSatMask = 0x00000001 // GPS L1
	CfgNavSatMaskGLONASSG1  CfgNavSatMask = 0x00000002 // GLONASS G1
	CfgNavSatMaskBEIDOUB1   CfgNavSatMask = 0x00000004 // BEIDOU B1
	CfgNavSatMaskGALILEOE1  CfgNavSatMask = 0x00000010 // GALILEO E1
	CfgNavSatMaskQZSSL1     CfgNavSatMask = 0x00000020 // QZSS L1
	CfgNavSatMaskSBASL1     CfgNavSatMask = 0x00000040 // SBAS L1
	CfgNavSatMaskIRNSSL5    CfgNavSatMask = 0x00000080 // IRNSS L5
	CfgNavSatMaskGPSL1C     CfgNavSatMask = 0x00000100 // GPS L1C
	CfgNavSatMaskGPSL5      CfgNavSatMask = 0x00000200 // GPS L5
	CfgNavSatMaskGPSL2C     CfgNavSatMask = 0x00000400 // GPS L2C
	CfgNavSatMaskGLONASSG2  CfgNavSatMask = 0x00002000 // GLONASS G2
	CfgNavSatMaskBEIDOUB1C  CfgNavSatMask = 0x00004000 // BEIDOU B1C
	CfgNavSatMaskBEIDOUB2   CfgNavSatMask = 0x00040000 // BEIDOU B2
	CfgNavSatMaskBEIDOUB2A  CfgNavSatMask = 0x00008000 // BEIDOU B2A
	CfgNavSatMaskBEIDOUB3I  CfgNavSatMask = 0x00010000 // BEIDOU B3I
	CfgNavSatMaskBEIDOUB5   CfgNavSatMask = 0x00020000 // BEIDOU B5
	CfgNavSatMaskGALILEOE5A CfgNavSatMask = 0x00100000 // GALILEO E5A
	CfgNavSatMaskGALILEOE5B CfgNavSatMask = 0x00200000 // GALILEO E5B
	CfgNavSatMaskGALILEOE6  CfgNavSatMask = 0x00400000 // GALILEO E6
	CfgNavSatMaskQZSSL1C    CfgNavSatMask = 0x02000000 // QZSS L1C
	CfgNavSatMaskQZSSL5     CfgNavSatMask = 0x04000000 // QZSS L5
	CfgNavSatMaskQZSSL2C    CfgNavSatMask = 0x08000000 // QZSS L2C
	CfgNavSatMaskQZSSL6     CfgNavSatMask = 0x01000000 // QZSS L6
)

// CFG-NAVSAT (0x06 0x0C)
type CfgNavSat struct {
	EnableMask CfgNavSatMask // Satellite system enable bitmask
}

func (m *CfgNavSat) ID() MsgID { return CfgNavSatID }

// CFG-SURVEY (0x06 0x12)
type CfgSurvey struct {
	MinDur   uint32 // s, Minimum duration of survey
	AccLimit uint32 // mm, Required accuracy
}

func (m *CfgSurvey) ID() MsgID { return CfgSurveyID }

// CFG-FIXEDECEF (0x06 0x14)
type CfgFixedECEF struct {
	X int32 // cm, ECEF X coordinate
	Y int32 // cm, ECEF Y coordinate
	Z int32 // cm, ECEF Z coordinate
}

func (m *CfgFixedECEF) ID() MsgID { return CfgFixedECEFID }

// CFG-PRT (0x06 0x00)
type CfgPrt struct {
	PortID   uint8    // Port number: 0=UART0, 1=UART1
	Res      [3]uint8 // Reserved
	Baudrate uint32   // Bits/s, Baud rate
}

func (m *CfgPrt) ID() MsgID { return CfgPrtID }

// CFG-MSG (0x06 0x01)
type CfgMsg struct {
	MsgClass uint8 // Message class
	MsgID    uint8 // Message ID
	Rate     uint8 // Rate or interval
}

func (m *CfgMsg) ID() MsgID { return CfgMsgID }

// CfgPpsPolarity defines values for CfgPps.Polarity
type CfgPpsPolarity uint8

const (
	CfgPpsPolarityFallingEdge CfgPpsPolarity = 0 // Falling edge (negative polarity)
	CfgPpsPolarityRisingEdge  CfgPpsPolarity = 1 // Rising edge (positive polarity)
)

// CfgPpsSync defines values for CfgPps.Sync
type CfgPpsSync uint8

const (
	CfgPpsSyncOnlyWithFix CfgPpsSync = 0 // Output PPS only when position fix is available
	CfgPpsSyncAlways      CfgPpsSync = 1 // Always output PPS regardless of fix status
)

// CFG-PPS (0x06 0x07) — Cynosure II/III
type CfgPps struct {
	Period    uint32         // us, PPS period
	Offset    int32          // ns, PPS offset
	DutyCycle uint32         // 10^-6, Duty cycle
	Polarity  CfgPpsPolarity // 0=falling edge, 1=rising edge
	GPIO      uint8          // GPIO pin
	Sync      CfgPpsSync     // 0=only when fix, 1=always output
}

func (m *CfgPps) ID() MsgID { return CfgPpsID }

// CfgSimpleRstMode defines the mode values for CfgSimpleRst.Mode
type CfgSimpleRstMode uint8

const (
	CfgSimpleRstModeReset       CfgSimpleRstMode = 0x00 // Hardware reset
	CfgSimpleRstModeColdStart   CfgSimpleRstMode = 0x01 // Cold start
	CfgSimpleRstModeWarmStart   CfgSimpleRstMode = 0x02 // Warm start
	CfgSimpleRstModeHotStart    CfgSimpleRstMode = 0x03 // Hot start
	CfgSimpleRstModeStop        CfgSimpleRstMode = 0x10 // Stop GNSS
	CfgSimpleRstModeStart       CfgSimpleRstMode = 0x11 // Start GNSS
	CfgSimpleRstModeClearAllTRK CfgSimpleRstMode = 0x80 // Clear all TRK channels
)

// CFG-SIMPLERST (0x06 0x40)
type CfgSimpleRst struct {
	Mode CfgSimpleRstMode // Reset mode
}

func (m *CfgSimpleRst) ID() MsgID { return CfgSimpleRstID }

// MON-VER (0x0A 0x04)
type MonVer struct {
	SwVersion [16]byte // Software version string
	HwVersion [16]byte // Hardware version string
}

func (m *MonVer) ID() MsgID { return MonVerID }

// NAV-CLOCK (0x01 0x22)
type NavClock struct {
	ITow uint32 // ms, GNSS time of week
	ClkB int32  // ns, Clock bias
	ClkD int32  // ns/s, Clock drift
	TAcc uint32 // ns, Time accuracy estimate
	FAcc uint32 // ps/s, Frequency accuracy estimate
}

func (m *NavClock) ID() MsgID { return NavClockID }

// NAV-SVIN (0x01 0x31) - Survey-in status
// This is not in 2.3.6 protocol spec, but is in satrack 1.31 app
type NavSvin struct {
	ITow       uint32 // ms, GNSS time of week
	PosUsed    uint32 // Number of position samples used
	MeanStdDev uint32 // 0.1mm, Mean standard deviation
	Valid      uint8  // 0=not valid, 1=valid
	Status     uint8  // 0=not finish, 1=finish (survey complete)
}

func (m *NavSvin) ID() MsgID { return NavSvinID }

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

// 2 bytes sync, 2 bytes clsid, 2 bytes length, 2 bytes checksum
const packetMinLength = 8

func ParseMsg(packet string) (Msg, error) {
	n := len(packet)
	if n < packetMinLength {
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
	// For UBX-INF-* messages, the payload does not have a fixed part.
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

func Poll(mid MsgID) []byte {
	packet, _ := packMsg(mid, []byte{})
	return packet
}

func PollNavTime(navSys NavTimeSys) []byte {
	packet, _ := packMsg(NavTimeID, []byte{byte(navSys)})
	return packet
}

func PollPrt(uartIdx int) []byte {
	packet, _ := packMsg(CfgPrtID, []byte{byte(uartIdx)})
	return packet
}

func SetCfgMsg(mid MsgID, rate byte) []byte {
	cls, id := mid.unpack()
	packet, _ := packMsg(CfgMsgID, []byte{cls, id, rate})
	return packet
}

func PollCfgMsg(mid MsgID) []byte {
	cls, id := mid.unpack()
	packet, _ := packMsg(CfgMsgID, []byte{cls, id})
	return packet
}

func packMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("as-%s payload too long (%d bytes)", mid.String(), len(payload))
	}
	cls, id := mid.unpack()
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
