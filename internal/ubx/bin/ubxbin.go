package bin

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const (
	Sync1 = 0xB5
	Sync2 = 0x62
)

type MsgID uint16

type GNSSID byte

const (
	GPS GNSSID = iota
	SBAS
	Galileo
	BeiDou
	IMES
	QZSS
	GLONASS
	NavIC
)

const (
	PortI2C = iota
	PortUART1
	PortUART2
	PortUSB
	PortSPI
)

const (
	clsNav = 0x01
	clsAck = 0x05
	clsInf = 0x04
	clsCfg = 0x06
	clsMon = 0x0A
	clsTim = 0x0D
)

var clsMap = map[byte]string{
	clsNav: "NAV",
	clsAck: "ACK",
	clsInf: "INF",
	clsCfg: "CFG",
	clsMon: "MON",
	clsTim: "TIM",
}

func makeMsgID(cls byte, id byte) MsgID {
	return MsgID(uint16(cls) | (uint16(id) << 8))
}

func (mid MsgID) unpack() (byte, byte) {
	return byte(mid & 0xFF), byte((mid >> 8) & 0xFF)
}

func (mid MsgID) Ackable() bool {
	cls, _ := mid.unpack()
	return cls == clsCfg
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
	AckNakID     MsgID = clsAck | (0x00 << 8)
	AckAckID     MsgID = clsAck | (0x01 << 8)
	CfgGNSSID    MsgID = clsCfg | (0x3E << 8)
	CfgMsgID     MsgID = clsCfg | (0x01 << 8)
	CfgRateID    MsgID = clsCfg | (0x08 << 8)
	CfgTmode2ID  MsgID = clsCfg | (0x3D << 8)
	CfgTmode3ID  MsgID = clsCfg | (0x71 << 8)
	CfgTp5ID     MsgID = clsCfg | (0x31 << 8)
	CfgValdelID  MsgID = clsCfg | (0x8C << 8)
	CfgValgetID  MsgID = clsCfg | (0x8B << 8)
	CfgValsetID  MsgID = clsCfg | (0x8A << 8)
	InfDebugID   MsgID = clsInf | (0x04 << 8)
	InfErrorID   MsgID = clsInf | (0x00 << 8)
	InfNoticeID  MsgID = clsInf | (0x02 << 8)
	InfTestID    MsgID = clsInf | (0x03 << 8)
	InfWarningID MsgID = clsInf | (0x01 << 8)
	MonHwID      MsgID = clsMon | (0x09 << 8)
	MonVerID     MsgID = clsMon | (0x04 << 8)
	NavPVTID     MsgID = clsNav | (0x07 << 8)
	NavTimeGPSID MsgID = clsNav | (0x20 << 8)
	NavTimeUTCID MsgID = clsNav | (0x21 << 8)
	NavTimeBDSID MsgID = clsNav | (0x24 << 8)
	NavTimeGLOID MsgID = clsNav | (0x23 << 8)
	NavTimeGalID MsgID = clsNav | (0x25 << 8)
	NavTimeLSID  MsgID = clsNav | (0x26 << 8)
	NavSvinID    MsgID = clsNav | (0x3B << 8)
	TimSvinID    MsgID = clsTim | (0x04 << 8)
	TimTosID     MsgID = clsTim | (0x12 << 8)
	TimTPID      MsgID = clsTim | (0x01 << 8)
)

func init() {
	regMsg[AckNak]("NAK")
	regMsg[AckAck]("ACK")
	regMsg[CfgGNSS]("GNSS")
	regMsg[CfgMsg]("MSG")
	regMsg[CfgRate]("RATE")
	regMsg[CfgTmode2]("TMODE2")
	regMsg[CfgTmode3]("TMODE3")
	regMsg[CfgTp5]("TP5")
	regMsg[CfgValget]("VALGET")
	regMsg[CfgValset]("VALSET")
	regMsg[CfgValdel]("VALDEL")
	regMsg[InfDebug]("DEBUG")
	regMsg[InfError]("ERROR")
	regMsg[InfNotice]("NOTICE")
	regMsg[InfTest]("TEST")
	regMsg[InfWarning]("WARNING")
	regMsg[MonHw]("HW")
	regMsg[MonVer]("VER")
	regMsg[NavPVT]("PVT")
	regMsg[NavSvin]("SVIN")
	regMsg[NavTimeGPS]("TIMEGPS")
	regMsg[NavTimeBDS]("TIMEBDS")
	regMsg[NavTimeGal]("TIMEGAL")
	regMsg[NavTimeGLO]("TIMEGLO")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavTimeLS]("TIMELS")
	regMsg[TimSvin]("SVIN")
	regMsg[TimTos]("TOS")
	regMsg[TimTP]("TP")
}

type AckNak struct {
	MsgID MsgID
}

func (m *AckNak) ID() MsgID { return AckNakID }

type AckAck struct {
	MsgID MsgID
}

func (m *AckAck) ID() MsgID { return AckAckID }

type CfgMsg struct {
	MsgID MsgID
	Rate  [6]byte
}

func (m *CfgMsg) ID() MsgID { return CfgMsgID }

type CfgRate struct {
	MeasRate uint16
	NavRate  uint16
	TimeRef  CfgRateTimeRef
}

func (m *CfgRate) ID() MsgID { return CfgRateID }

type CfgRateTimeRef uint16

const (
	CfgRateUTC CfgRateTimeRef = iota
	CfgRateGPS
	CfgRateGLONASS
	CfgRateBeiDou
	CfgRateGalileo
	CfgRateNavIC
)

type CfgTp5 struct {
	TpIdx             byte
	Version           byte
	_                 [2]byte
	AntCableDelay     int16
	RfGroupDelay      int16
	FreqPeriod        uint32
	FreqPeriodLock    uint32
	PulseLenRatio     uint32
	PulseLenRatioLock uint32
	UserConfigDelay   int32
	Flags             CfgTp5Flags
}

func (m *CfgTp5) ID() MsgID { return CfgTp5ID }

type CfgTp5Flags uint32

const (
	CfgTp5Active CfgTp5Flags = 1 << iota
	CfgTp5LockGpsFreq
	CfgTp5LockedOtherSet
	CfgTp5IsFreq
	CfgTp5IsLength
	CfgTp5AlignToTow
	CfgTp5Polarity
	CfgTp5GridUtcGnss
)

type CfgTmode2 struct {
	TimeMode     CfgTmode2TimeMode
	_            byte
	Flags        CfgTmode2Flags
	EcefXOrLat   int32
	EcefYOrLon   int32
	EcefZOrAlt   int32
	FixedPosAcc  uint32
	SvinMinDur   uint32
	SvinAccLimit uint32
}

func (m *CfgTmode2) ID() MsgID { return CfgTmode2ID }

type CfgTmode2TimeMode byte

const (
	CfgTmode2Disabled CfgTmode2TimeMode = iota
	CfgTmode2SurveyIn
	CfgTmode2FixedMode
)

type CfgTmode2Flags uint16

const (
	CfgTmode2LLA CfgTmode2Flags = 1 << iota
	CfgTMode2AltInv
)

type CfgTmode3 struct {
	Version      byte
	_            byte
	Flags        CfgTmode3Flags
	EcefXOrLat   int32
	EcefYOrLon   int32
	EcefZOrAlt   int32
	EcefXOrLatHP int8
	EcefYOrLonHP int8
	EcefZOrAltHP int8
	_            byte
	FixedPosAcc  uint32
	SvinMinDur   uint32
	SvinAccLimit uint32
	_            [8]byte
}

func (m *CfgTmode3) ID() MsgID { return CfgTmode3ID }

type CfgTmode3Flags uint16

const (
	CfgTmode3Disabled CfgTmode3Flags = iota
	CfgTmode3SurveyIn
	CfgTmode3FixedMode
	CfgTmode3Mode CfgTmode3Flags = 0xFF
	CfgTmode3LLA  CfgTmode3Flags = 0x100
)

type UTCStandard byte

const (
	UTCStandardNotAvailable UTCStandard = iota
	UTCStandardCRL
	UTCStandardNIST
	UTCStandardUSNO
	UTCStandardBIPM
	UTCStandardEU
	UTCStandardSU
	UTCStandardNTSC
	UTCStandardNPLI
	UTCStandardUnknown UTCStandard = 15
)

type NavPVT struct {
	ITOW    uint32
	Year    uint16
	Month   byte
	Day     byte
	Hour    byte
	Min     byte
	Sec     byte
	Valid   NavPVTValid
	TAcc    uint32
	Nano    int32
	FixType NavPVTFixType
	Flags   NavPVTFlags
	Flags2  NavPVTFlags2
	NumSV   byte
	Lon     int32
	Lat     int32
	Height  int32
	HMSL    int32
	HAcc    uint32
	VAcc    uint32
	VelN    int32
	VelE    int32
	VelD    int32
	GSpeed  int32
	HeadMot int32
	SAcc    uint32
	HeadAcc uint32
	PDOP    uint16
	_       [6]byte
	HeadVeh int32
	MagDec  int16
	MagAcc  uint16
}

type NavPVTValid byte

const (
	NavPVTValidDate NavPVTValid = 1 << iota
	NavPVTValidTime
	NavPVTValidFullyResolved
	NavPVTValidMag
)

type NavPVTFixType byte

const (
	NavPVTNoFix NavPVTFixType = iota
	NavPVTDeadReckoningOnly
	NavPVT2DFix
	NavPVT3DFix
	NavPVTGNSSDeadReckoning
	NavPVTTimeOnlyFix
)

type NavPVTFlags byte

const (
	NavPVTGNSSFixOK NavPVTFlags = 1 << iota
	NavPVTDiffSoln
	navPVTPSMState0
	navPVTPSMState1
	navPVTPSMState2
	NavPVTHeadVehValid
	navPVTCarrSoln0
	navPVTCarrSoln1
	NavPctPSMState NavPVTFlags = navPVTPSMState0 | navPVTPSMState1 | navPVTPSMState2
	NavPctCarrSoln NavPVTFlags = navPVTCarrSoln0 | navPVTCarrSoln1
)

const (
	NavPVTPSMStateNotActive NavPVTFlags = iota << 2
	NavPVTPSMStateAcquisition
	NavPVTPSMStateTracking
	NavPVTPSMStatePowerOptimizedTracking
	NavPVTPSMStateInactive
)

const (
	NavPVTCarrSolnNone NavPVTFlags = iota << 6
	NavPVTCarrSolnFloat
	NavPVTCarrSolnFixed
)

type NavPVTFlags2 byte

const (
	NavPVTConfirmedAvai NavPVTFlags2 = 1 << (iota + 5)
	NavPVTConfirmedDate
	NavPVTConfirmedTime
)

func (m *NavPVT) ID() MsgID { return NavPVTID }

type NavTimeGPS struct {
	ITOW  uint32
	FTOW  int32
	Week  int16
	LeapS byte
	Valid NavTimeGPSValid
	TAcc  uint32
}

type NavTimeGPSValid byte

const (
	NavTimeGPSTOWValid NavTimeGPSValid = 1 << iota
	NavTimeGPSWeekValid
	NavTimeGPSLeapSValid
)

func (m *NavTimeGPS) ID() MsgID { return NavTimeGPSID }

type NavTimeUTC struct {
	ITOW  uint32
	TAcc  uint32
	Nano  int32
	Year  uint16
	Month byte
	Day   byte
	Hour  byte
	Min   byte
	Sec   byte
	Valid NavTimeUTCValid
}

func (m *NavTimeUTC) ID() MsgID { return NavTimeUTCID }

type NavTimeUTCValid byte

const (
	NavTimeUTCValidTOW NavTimeUTCValid = 1 << iota
	NavTimeUTCValidWkn
	NavTimeUTCValidUTC
)

func (v NavTimeUTCValid) UTCStandard() UTCStandard {
	return UTCStandard(v >> 4)
}

type NavTimeBDS struct {
	ITOW  uint32
	SOW   uint32
	FSOW  int32
	Week  int16
	LeapS byte
	Valid NavTimeBDSValid
	TAcc  uint32
}

func (m *NavTimeBDS) ID() MsgID { return NavTimeBDSID }

type NavTimeBDSValid byte

const (
	NavTimeBDSSOWValid NavTimeBDSValid = 1 << iota
	NavTimeBDSWeekValid
	NavTimeBDSLeapSValid
)

type NavTimeGal struct {
	ITOW    uint32
	GalTOW  uint32
	FGalTOW int32
	GalWno  int16
	LeapS   byte
	Valid   NavTimeGalValid
	TAcc    uint32
}

func (m *NavTimeGal) ID() MsgID { return NavTimeGalID }

type NavTimeGalValid byte

const (
	NavTimeGalTOWValid NavTimeGalValid = 1 << iota
	NavTimeGalWnoValid
	NavTimeGalLeapSValid
)

type NavTimeGLO struct {
	ITOW  uint32
	TOD   uint32
	FTOD  int32
	Nt    uint16
	N4    byte
	Valid NavTimeGLOValid
	TAcc  uint32
}

func (m *NavTimeGLO) ID() MsgID { return NavTimeGLOID }

type NavTimeGLOValid byte

const (
	NavTimeGLOTODValid NavTimeGLOValid = 1 << iota
	NavTimeGLODateValid
)

type NavTimeLS struct {
	ITOW          uint32
	Version       byte
	_             [3]byte
	SrcOfCurrLS   NavTimeSrcOfCurrLS
	CurrLS        int8
	SrcOfLSChange NavTimeLSSrcOfLSChange
	LSChange      NavTimeLSChange
	TimeToLSEvent int32
	DateOfLSGPSWN uint16
	DateOfLSGPSDN uint16
	_             [3]byte
	Valid         NavTimeLSValid
}

type NavTimeSrcOfCurrLS byte

const (
	NavTimeSrcOfCurrLSFirmware NavTimeSrcOfCurrLS = iota
	NavTimeSrcOfCurrLSGPSDiffGLONASS
	NavTimeSrcOfCurrLSGPS
	NavTimeSrcOfCurrLSSBAS
	NavTimeSrcOfCurrLSBeiDou
	NavTimeSrcOfCurrLSGalileo
	NavTimeSrcOfCurrLSAided
	NavTimeSrcOfCurrLSConfigured
	NavTimeSrcOfCurrLSNavIC
	NavTimeSrcOfCurrLSUnknown NavTimeSrcOfCurrLS = 255
)

type NavTimeLSSrcOfLSChange byte

const (
	NavTimeLSSrcOfLSChangeNone NavTimeLSSrcOfLSChange = iota
	_
	NavTimeLSSrcOfLSChangeGPS
	NavTimeLSSrcOfLSChangeSBAS
	NavTimeLSSrcOfLSChangeBeiDou
	NavTimeLSSrcOfLSChangeGalileo
	NavTimeLSSrcOfLSChangeGLONASS
	NavTimeLSSrcOfLSChangeNavIC
)

type NavTimeLSChange int8

const NavTimeLSChangePositive NavTimeLSChange = +1
const NavTimeLSChangeNone NavTimeLSChange = 0
const NavTimeLSChangeNegative NavTimeLSChange = -1

type NavTimeLSValid byte

const (
	NavTimeLSValidCurrLS NavTimeLSValid = 1 << iota
	NavTimeLSValidTimeToLSEvent
)

func (m *NavTimeLS) ID() MsgID { return NavTimeLSID }

type NavSvin struct {
	Version byte
	_       [3]byte
	ITOW    uint32
	Dur     uint32
	MeanX   int32
	MeanY   int32
	MeanZ   int32
	MeanXHP byte
	MeanYHP byte
	MeanZHP byte
	_       byte
	MeanAcc uint32
	Obs     uint32
	Valid   byte
	Active  byte
	_       [2]byte
}

func (m *NavSvin) ID() MsgID { return NavSvinID }

type TimTos struct {
	Version           byte
	GNSSID            GNSSID
	_                 [2]byte
	Flags             TimTosFlags
	Year              uint16
	Month             byte
	Day               byte
	Hour              byte
	Minute            byte
	Second            byte
	UTCStandard       UTCStandard
	UTCOffset         int32
	UTCUncertainty    uint32
	Week              uint32
	TOW               uint32
	GNSSOffset        int32
	GNSSUncertainty   uint32
	IntOscOffset      int32
	IntOscUncertainty uint32
	ExtOscOffset      int32
	ExtOscUncertainty uint32
}

func (m *TimTos) ID() MsgID { return TimTosID }

type TimTosFlags uint32

const (
	TimTosLeapNow TimTosFlags = 1 << iota
	TimTosLeapSoon
	TimTosLeapPositive
	TimTosTimeInLimit
	TimTosIntOscInLimit
	TimTosExtOscInLimit
	TimTosGNSSTimeValid
	TimTosUTCTimeValid
	TimTosDiscSrc0
	TimTosDiscSrc1
	TimTosDiscSrc2
	TimTosRAIM
	TimTosCohPulse
	TimTosLockedPulse
	TimTosDiscSrc TimTosFlags = TimTosDiscSrc0 | TimTosDiscSrc1 | TimTosDiscSrc2
)

// values for (Flags & TimTosDiscSrc)
const (
	TimTosDiscSrcIntOsc TimTosFlags = iota << 8
	TimTosDiscSrcGNSS
	TimTosDiscSrcEXTINT0
	TimTosDiscSrcEXTINT1
	TimTosDiscSrcIntOscHost
	TimTosDiscSrcExtOscHost
)

type TimTP struct {
	TOWMS    uint32
	TOWSubMS uint32
	QErr     int32
	Week     uint16
	Flags    TimTPFlags
	RefInfo  TimTPRefInfo
}

func (m *TimTP) ID() MsgID { return TimTPID }

type TimTPFlags byte

const (
	TimTPTimeBase TimTPFlags = 1 << iota
	TimTPUTCAvailable
	timTPRAIM1
	timTPRAIM2
	TimTPQErrInvalid
	TimTPRAIM TimTPFlags = timTPRAIM1 | timTPRAIM2
)

const (
	TimTPTimeBaseGNSS TimTPFlags = 0
	TimTPTimeBaseUTC  TimTPFlags = TimTPTimeBase
)

// Values for TimTPFlags & TimTPRAIM
const (
	TimTPRAIMNotActive TimTPFlags = timTPRAIM1
	TimTPRAIMActive    TimTPFlags = timTPRAIM2
)

type TimTPRefInfo byte

const (
	TimTPTimeRefGNSS TimTPRefInfo = 0x0F
)

func (ri TimTPRefInfo) UTCStandard() UTCStandard {
	return UTCStandard(ri >> 4)
}

// Values for TimTPRefInfo & TimTPTimeRefGNSS
const (
	TimTPTimeRefGPS TimTPRefInfo = iota
	TimTPTimeRefGLONASS
	TimTPTimeRefBeiDou
	TimTPTimeRefGalileo
	TimTPTimeRefNavIC
	TimTPTimeRefGNSSUnknown TimTPRefInfo = 15
)

type TimSvin struct {
	Dur    uint32
	MeanX  int32
	MeanY  int32
	MeanZ  int32
	MeanV  uint32
	Obs    uint32
	Valid  byte
	Active byte
	_      [2]byte
}

func (m *TimSvin) ID() MsgID { return TimSvinID }

type MonHw struct {
	PinSel        uint32
	PinBank       uint32
	PinDir        uint32
	PinVal        uint32
	NoisePerMS    uint16
	AgcCnt        uint16
	AStatus       byte
	APower        byte
	Flags         byte
	_             byte
	UsedMask      uint32
	VP            [17]byte
	CwSuppression byte
	_             [2]byte
	PinIrq        uint32
	PullH         uint32
	PullL         uint32
}

func (m *MonHw) ID() MsgID { return MonHwID }

type MonVerFixed struct {
	SwVersion [30]byte
	HwVersion [10]byte
}

type MonVer struct {
	MonVerFixed
	Extension [][30]byte
}

func (m *MonVer) ID() MsgID { return MonVerID }

func (m *MonVer) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 30+10, 30)
	if err == nil {
		m.Extension = make([][30]byte, len)
	}
	return
}

func (m *MonVer) Parts() (fixed any, slice any) {
	fixed = &m.MonVerFixed
	slice = &m.Extension
	return
}

type CfgGNSS struct {
	CfgGNSSFixed
	Blocks []CfgGNSSBlock
}

type CfgGNSSFixed struct {
	MsgVer          byte
	NumTrkChHw      byte
	NumTrkChUse     byte
	NumConfigBlocks byte
}

type CfgGNSSBlock struct {
	GNSSID     GNSSID
	ResTrkCh   byte
	MaxTrkCh   byte
	_          byte
	Enable     byte
	_          byte
	SigCfgMask byte
	_          byte
}

func (m *CfgGNSS) ID() MsgID { return CfgGNSSID }

func (m *CfgGNSS) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 8)
	if err == nil {
		m.Blocks = make([]CfgGNSSBlock, len)
	}
	return
}

func (m *CfgGNSS) Parts() (fixed any, slice any) {
	fixed = &m.CfgGNSSFixed
	slice = &m.Blocks
	return
}

type CfgValget struct {
	CfgValgetFixed
	CfgData []byte
}

func (m *CfgValget) ID() MsgID { return CfgValgetID }

type CfgValgetFixed struct {
	Version  CfgValgetVersion
	Layer    CfgValgetLayer
	Position uint16
}

type CfgValgetVersion byte

const (
	CfgValgetVersionRequest  CfgValgetVersion = 0 // CfgData is just keys
	CfgValgetVersionResponse CfgValgetVersion = 1 // CfgData is keys and values
)

type CfgValgetLayer byte

const (
	CfgValgetLayerRAM CfgValgetLayer = iota
	CfgValgetLayerBBR
	CfgValgetLayerFlash
	CfgValgetLayerDefault CfgValgetLayer = 7
)

func (m *CfgValget) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 1)
	if err == nil {
		m.CfgData = make([]byte, len)
	}
	return
}

func (m *CfgValget) Parts() (fixed any, slice any) {
	fixed = &m.CfgValgetFixed
	slice = &m.CfgData
	return
}

// This is shared between valdel and valset
type CfgValTransaction byte

const (
	CfgValTransactionNone CfgValTransaction = iota
	CfgValTransactionStart
	CfgValTransactionOngoing
	CfgValTransactionEnd
)

type CfgValset struct {
	CfgValsetFixed
	CfgData []byte
}

func (m *CfgValset) ID() MsgID { return CfgValsetID }

type CfgValsetFixed struct {
	Version     CfgValsetVersion
	Layers      CfgValsetLayer
	Transaction CfgValTransaction
	_           byte
}

type CfgValsetVersion byte

const (
	CfgValsetVersionNoTransaction CfgValsetVersion = iota
	CfgValsetVersionTransaction
)

type CfgValsetLayer byte

const (
	CfgValsetLayerRAM CfgValsetLayer = 1 << iota
	CfgValsetLayerBBR
	CfgValsetLayerFlash
)

func (m *CfgValset) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 1)
	if err == nil {
		m.CfgData = make([]byte, len)
	}
	return
}

func (m *CfgValset) Parts() (fixed any, slice any) {
	fixed = &m.CfgValsetFixed
	slice = &m.CfgData
	return
}

type CfgValdel struct {
	CfgValdelFixed
	CfgData []byte
}

func (m *CfgValdel) ID() MsgID { return CfgValdelID }

type CfgValdelFixed struct {
	Version     CfgValdelVersion
	Layers      CfgValdelLayer
	Transaction CfgValTransaction
	_           byte
}

type CfgValdelVersion byte

const (
	CfgValdelVersionNoTransaction CfgValdelVersion = iota
	CfgValdelVersionTransaction
)

type CfgValdelLayer byte

const (
	CfgValdelLayerBBR   CfgValdelLayer = 1
	CfgValdelLayerFlash CfgValdelLayer = 2
)

func (m *CfgValdel) InitForLen(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 1)
	if err == nil {
		m.CfgData = make([]byte, len)
	}
	return
}

func (m *CfgValdel) Parts() (fixed any, slice any) {
	fixed = &m.CfgValdelFixed
	slice = &m.CfgData
	return
}

type InfDebug []byte

func (m *InfDebug) ID() MsgID { return InfDebugID }
func (m *InfDebug) InitForLen(payloadLen int) error {
	*m = make([]byte, payloadLen)
	return nil
}
func (m *InfDebug) Parts() (fixed any, slice any) {
	return nil, (*[]byte)(m)
}

type InfError []byte

func (m *InfError) ID() MsgID { return InfErrorID }
func (m *InfError) InitForLen(payloadLen int) error {
	*m = make([]byte, payloadLen)
	return nil
}
func (m *InfError) Parts() (fixed any, slice any) {
	return nil, (*[]byte)(m)
}

type InfNotice []byte

func (m *InfNotice) ID() MsgID { return InfNoticeID }
func (m *InfNotice) InitForLen(payloadLen int) error {
	*m = make([]byte, payloadLen)
	return nil
}
func (m *InfNotice) Parts() (fixed any, slice any) {
	return nil, (*[]byte)(m)
}

type InfTest []byte

func (m *InfTest) ID() MsgID { return InfTestID }
func (m *InfTest) InitForLen(payloadLen int) error {
	*m = make([]byte, payloadLen)
	return nil
}
func (m *InfTest) Parts() (fixed any, slice any) {
	return nil, (*[]byte)(m)
}

type InfWarning []byte

func (m *InfWarning) ID() MsgID { return InfWarningID }
func (m *InfWarning) InitForLen(payloadLen int) error {
	*m = make([]byte, payloadLen)
	return nil
}
func (m *InfWarning) Parts() (fixed any, slice any) {
	return nil, (*[]byte)(m)
}

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

// PacketMsgId returns the MsgId of a packet.
// This assumes a minimally-valid packet
func PacketMsgId(packet []byte) MsgID {
	return makeMsgID(packet[2], packet[3])
}

// 2 bytes sync, 2 bytes clsid, 2 bytes length, 2 bytes checksum
const packetMinLength = 8

func ParseMsg(packet string) (Msg, error) {
	n := len(packet)
	if n < packetMinLength {
		return nil, fmt.Errorf("UBX message too short (length %d bytes)", n)
	}
	checksumIndex := n - 2
	trimmed := packet[2:checksumIndex]
	ckA, ckB := checksum(trimmed)
	if ckA != packet[checksumIndex] || ckB != packet[checksumIndex+1] {
		return nil, fmt.Errorf("ubx message: checksum failed: in message 0x%02x%02x; got 0x%02x%02x; data %x", packet[checksumIndex], packet[checksumIndex+1], ckA, ckB, []byte(trimmed))
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
		return nil, fmt.Errorf("parsing ubx-%s: %v", mid.String(), err)
	}
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing ubx-%s: trailing bytes", mid.String())
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

func SetRate(mid MsgID, rate byte) []byte {
	cls, id := mid.unpack()
	packet, _ := packMsg(CfgMsgID, []byte{cls, id, rate})
	return packet
}

func PollRate(mid MsgID) []byte {
	cls, id := mid.unpack()
	packet, _ := packMsg(CfgMsgID, []byte{cls, id})
	return packet
}

func packMsg(mid MsgID, payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("ubx-%s payload too long (%d bytes", mid.String(), len(payload))
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
	ckA, ckB := checksum(packet[2:])
	packet = append(packet, ckA, ckB)
	return packet, nil
}

type Bytes interface {
	string | []byte
}

func checksum[B Bytes](bytes B) (ckA, ckB byte) {
	for i := 0; i < len(bytes); i++ {
		ckA += bytes[i]
		ckB += ckA
	}
	return
}

// Latin1ZString create a string from a ISO Latin-1, nul-terminated byte slice.
// This can be used for the fields of MonVer
func Latin1ZToString(chars []byte) string {
	r := make([]rune, 0)
	for _, ch := range chars {
		if ch == 0 {
			break
		}
		r = append(r, rune(ch))
	}
	return string(r)
}
