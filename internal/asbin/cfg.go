package asbin

const (
	CfgPrtID       MsgID = clsCfg | (0x00 << 8)
	CfgMsgID       MsgID = clsCfg | (0x01 << 8)
	CfgPpsID       MsgID = clsCfg | (0x07 << 8)
	CfgCfgID       MsgID = clsCfg | (0x09 << 8)
	CfgElevID      MsgID = clsCfg | (0x0B << 8)
	CfgNavSatID    MsgID = clsCfg | (0x0C << 8)
	CfgSurveyID    MsgID = clsCfg | (0x12 << 8)
	CfgFixedECEFID MsgID = clsCfg | (0x14 << 8)
	CfgSimpleRstID MsgID = clsCfg | (0x40 << 8)
	CfgNmeaVerID   MsgID = clsCfg | (0x43 << 8)
)

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

// CFG-PPS (0x06 0x07) - Cynosure II/III
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

// CFG-ELEV (0x06 0x0B)
type CfgElev struct {
	TrkMask  float32 // radian, Track elevation angle mask
	NaviMask float32 // radian, Navigation elevation angle mask
}

func (m *CfgElev) ID() MsgID { return CfgElevID }

// CfgNmeaVerVersion defines values for CfgNmeaVer.Version
type CfgNmeaVerVersion uint8

const (
	CfgNmeaVerNotSupported CfgNmeaVerVersion = 0 // Not supported
	CfgNmeaVerV301         CfgNmeaVerVersion = 1 // NMEA V3.01
	CfgNmeaVerV400         CfgNmeaVerVersion = 2 // NMEA V4.00
	CfgNmeaVerV410         CfgNmeaVerVersion = 3 // NMEA V4.10
	CfgNmeaVerV411         CfgNmeaVerVersion = 4 // NMEA V4.11
)

// CFG-NMEAVER (0x06 0x43)
type CfgNmeaVer struct {
	Version CfgNmeaVerVersion // NMEA version
}

func (m *CfgNmeaVer) ID() MsgID { return CfgNmeaVerID }

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

func init() {
	regMsg[CfgCfg]("CFG")
	regMsg[CfgNavSat]("NAVSAT")
	regMsg[CfgSurvey]("SURVEY")
	regMsg[CfgFixedECEF]("FIXEDECEF")
	regMsg[CfgPrt]("PRT")
	regMsg[CfgMsg]("MSG")
	regMsg[CfgPps]("PPS")
	regMsg[CfgSimpleRst]("SIMPLERST")
	regMsg[CfgElev]("ELEV")
	regMsg[CfgNmeaVer]("NMEAVER")
}
