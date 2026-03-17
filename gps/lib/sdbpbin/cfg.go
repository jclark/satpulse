package sdbpbin

// GNSSMask is a bitmask of enabled GNSS constellations.
type GNSSMask uint8

const (
	GNSSBitBDS GNSSMask = 1 << iota
	GNSSBitGPS
	GNSSBitGLO
	GNSSBitGAL
)

// CfgGNSS is CFG-GNSS (class 0x03, id 0x11).
type CfgGNSS struct {
	ConstellationMask GNSSMask
	Reserved          uint8
}

func (m *CfgGNSS) ID() MsgID { return makeMsgID(clsCFG, 0x11) }

// BaudRate identifies a serial baud rate.
type BaudRate uint8

const (
	Baud4800   BaudRate = 1
	Baud9600   BaudRate = 2
	Baud19200  BaudRate = 3
	Baud38400  BaudRate = 4
	Baud57600  BaudRate = 5
	Baud115200 BaudRate = 6
	Baud230400 BaudRate = 7
	Baud460800 BaudRate = 8
	Baud921600 BaudRate = 9
)

// Port identifies a communication port.
type Port uint8

const (
	PortUART1 Port = 1
	PortUART2 Port = 2
	PortSPI   Port = 3
	PortI2C   Port = 4
)

// CfgUART is CFG-UART (class 0x03, id 0x21).
type CfgUART struct {
	PortID   Port
	Baud     BaudRate
	DataBits uint8 // 0=8bit, 1=7bit
	StopBits uint8 // 0=0.5, 1=1, 2=1.5, 3=2
	Parity   uint8 // 0=none, 1=even, 2=odd
	Reserved uint8
}

func (m *CfgUART) ID() MsgID { return makeMsgID(clsCFG, 0x21) }

// CfgRate is CFG-RATE (class 0x03, id 0x36).
type CfgRate struct {
	MeasInterval uint16 // measurement interval in ms (integer multiple of 50)
	PosFrequency uint8  // n measurements per 1 positioning computation (n>=1)
	Reserved     uint8
}

func (m *CfgRate) ID() MsgID { return makeMsgID(clsCFG, 0x36) }

// PPSPolarity is the pulse polarity.
type PPSPolarity uint8

const (
	PPSNegative PPSPolarity = 0
	PPSPositive PPSPolarity = 1
)

// CfgPPS is CFG-PPS (class 0x03, id 0x41).
// Same format for set (I2) and response (O), 25 bytes.
// Pulse parameter 1 controls the pulse before positioning (state 1).
// Pulse parameter 2 controls the pulse after positioning (state 2).
type CfgPPS struct {
	Index      uint8    // 0=PPS0, 1=PPS1
	Switch     uint8       // 0=off, 1=on
	Polarity   PPSPolarity // 0=negative, 1=positive
	TimeRef    TimeRef // 0=UTC, 1=BDS, 2=GPS, 3=GLO, 4=GAL
	Period1    uint32      // state 1 (before fix) period in us, >= 100
	Width1     uint32      // state 1 pulse width in us
	Period2    uint32      // state 2 (after fix) period in us, >= 100
	Width2     uint32      // state 2 pulse width in us
	Offset     int32       // offset in ns
	Reserved   uint8
}

func (m *CfgPPS) ID() MsgID { return makeMsgID(clsCFG, 0x41) }

// PollCfgPPS builds a CFG-PPS query packet for the given PPS index.
func PollCfgPPS(index uint8) []byte {
	pkt, _ := PackMsg(makeMsgID(clsCFG, 0x41), []byte{byte(index)})
	return pkt
}

// NMEASentence identifies an NMEA sentence type.
type NMEASentence uint8

const (
	NMEASentZDA NMEASentence = iota + 1
	NMEASentDTM
	NMEASentRMC
	NMEASentGGA
	NMEASentGSA
	NMEASentGSV
	NMEASentVTG
	NMEASentGNS
	NMEASentGLL
	NMEASentGRS
	NMEASentGST
	NMEASentGBS
	NMEASentCLK
	NMEASentT01
)

var cfgNMEAID = makeMsgID(clsCFG, 0x51)

// CfgNMEA is CFG-NMEA response (class 0x03, id 0x51).
// Response format: SentenceID, UART1 freq, UART2 freq, SPI freq, I2C freq, Reserved.
type CfgNMEA struct {
	SentenceID NMEASentence
	UART1      uint8 // output frequency on UART1 (0=off, 1~N)
	UART2      uint8
	SPI        uint8
	I2C        uint8
	Reserved   uint8
}

func (m *CfgNMEA) ID() MsgID { return cfgNMEAID }

// SetCfgNMEA builds a CFG-NMEA set command packet.
func SetCfgNMEA(sentence NMEASentence, port Port, frequency uint8) []byte {
	pkt, _ := PackMsg(cfgNMEAID, []byte{byte(sentence), byte(port), frequency, 0})
	return pkt
}

// PollCfgNMEA builds a CFG-NMEA query packet.
func PollCfgNMEA(sentence NMEASentence) []byte {
	pkt, _ := PackMsg(cfgNMEAID, []byte{byte(sentence)})
	return pkt
}

var cfgSDBPID = makeMsgID(clsCFG, 0x52)

// CfgSDBP is CFG-SDBP response (class 0x03, id 0x52).
// Response format: MsgClass, MsgID, UART1 freq, UART2 freq, SPI freq, I2C freq, Reserved.
type CfgSDBP struct {
	MsgClass uint8
	MsgID    uint8
	UART1    uint8
	UART2    uint8
	SPI      uint8
	I2C      uint8
	Reserved uint8
}

func (m *CfgSDBP) ID() MsgID { return cfgSDBPID }

// SetCfgSDBP builds a CFG-SDBP set command packet.
func SetCfgSDBP(msgClass, msgID uint8, port Port, enable, rate uint8) []byte {
	pkt, _ := PackMsg(cfgSDBPID, []byte{msgClass, msgID, byte(port), enable, rate})
	return pkt
}

// PollCfgSDBP builds a CFG-SDBP query packet.
func PollCfgSDBP(msgClass, msgID uint8) []byte {
	pkt, _ := PackMsg(cfgSDBPID, []byte{msgClass, msgID})
	return pkt
}

// TMODEMode identifies the timing work mode.
type TMODEMode uint8

const (
	TMODENormal  TMODEMode = 1 // normal positioning (default)
	TMODESurvey  TMODEMode = 2 // autonomous survey
	TMODEFixed   TMODEMode = 3 // fixed position
)

// CfgTMODE is CFG-TMODE (class 0x03, id 0x43, 25 bytes).
// Fixed position in ECEF (X/Y/Z in 10^-2 m).
type CfgTMODE struct {
	Mode       TMODEMode
	MinObsTime uint32 // seconds, <=86400
	AccLimit   uint32 // 10^-2 m
	FixedX     int32  // 10^-2 m
	FixedY     int32
	FixedZ     int32
	FixedAcc   uint32 // 10^-2 m
}

func (m *CfgTMODE) ID() MsgID { return makeMsgID(clsCFG, 0x43) }

// CfgDELAY2 is CFG-DELAY2 (class 0x03, id 0x44, 9 bytes).
type CfgDELAY2 struct {
	CableDelay int32 // ns
	GroupDelay int32 // ns
	Reserved   uint8
}

func (m *CfgDELAY2) ID() MsgID { return makeMsgID(clsCFG, 0x44) }

// CfgTMODE2 is CFG-TMODE2 (class 0x03, id 0x45, 25 bytes).
// Fixed position in LLA (Lon/Lat in 10^-7 deg, Alt in 10^-2 m).
type CfgTMODE2 struct {
	Mode       TMODEMode
	MinObsTime uint32 // seconds, <=86400
	AccLimit   uint32 // 10^-2 m
	FixedLon   int32  // 10^-7 degrees
	FixedLat   int32
	FixedAlt   int32  // 10^-2 m
	FixedAcc   uint32 // 10^-2 m
}

func (m *CfgTMODE2) ID() MsgID { return makeMsgID(clsCFG, 0x45) }

func init() {
	regMsg[CfgGNSS]("GNSS")
	regMsg[CfgUART]("UART")
	regMsg[CfgRate]("RATE")
	regMsg[CfgPPS]("PPS")
	regMsg[CfgNMEA]("NMEA")
	regMsg[CfgSDBP]("SDBP")
	regMsg[CfgTMODE]("TMODE")
	regMsg[CfgDELAY2]("DELAY2")
	regMsg[CfgTMODE2]("TMODE2")
}
