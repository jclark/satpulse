package bin

const (
	NavDOPID         MsgID = clsNav | (0x04 << 8)
	NavPosECEFID     MsgID = clsNav | (0x01 << 8)
	NavPosLLHID      MsgID = clsNav | (0x02 << 8)
	NavSolID         MsgID = clsNav | (0x06 << 8)
	NavPVTID         MsgID = clsNav | (0x07 << 8)
	NavSatID         MsgID = clsNav | (0x35 << 8)
	NavSigID         MsgID = clsNav | (0x43 << 8)
	NavSVInfoID      MsgID = clsNav | (0x30 << 8)
	NavTimeGPSID     MsgID = clsNav | (0x20 << 8)
	NavTimeUTCID     MsgID = clsNav | (0x21 << 8)
	NavTimeBDSID     MsgID = clsNav | (0x24 << 8)
	NavTimeGLOID     MsgID = clsNav | (0x23 << 8)
	NavTimeGalID     MsgID = clsNav | (0x25 << 8)
	NavTimeLSID      MsgID = clsNav | (0x26 << 8)
	NavTimeTrustedID MsgID = clsNav | (0x64 << 8)
	NavVelECEFID     MsgID = clsNav | (0x11 << 8)
	NavVelNEDID      MsgID = clsNav | (0x12 << 8)
	NavSvinID        MsgID = clsNav | (0x3B << 8)
)

type NavMsg interface {
	Msg
	NavEpoch() uint32
}

type NavITOW struct {
	ITOW uint32
}

func (m *NavITOW) NavEpoch() uint32 {
	return m.ITOW
}

type NavDOP struct {
	NavITOW
	GDOP uint16
	PDOP uint16
	TDOP uint16
	VDOP uint16
	HDOP uint16
	NDOP uint16
	EDOP uint16
}

func (m *NavDOP) ID() MsgID { return NavDOPID }

type NavPosECEF struct {
	NavITOW
	ECEF [3]int32
	PAcc uint32
}

func (m *NavPosECEF) ID() MsgID { return NavPosECEFID }

type NavVelECEF struct {
	NavITOW
	ECEFV [3]int32
	SAcc  uint32
}

func (m *NavVelECEF) ID() MsgID { return NavVelECEFID }

type NavPosLLH struct {
	NavITOW
	Lon    int32
	Lat    int32
	Height int32
	HMSL   int32
	HAcc   uint32
	VAcc   uint32
}

func (m *NavPosLLH) ID() MsgID { return NavPosLLHID }

type NavVelNED struct {
	NavITOW
	VelNED  [3]int32
	Speed   uint32
	GSpeed  uint32
	Heading int32
	SAcc    uint32
	CAcc    uint32
}

func (m *NavVelNED) ID() MsgID { return NavVelNEDID }

type NavPVT struct {
	NavITOW
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
	Flags3  NavPVTFlags3
	_       [4]byte
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

type NavPVTFlags3 uint16

const (
	NavPVTInvalidLlh   NavPVTFlags3 = 1 << 0
	NavPVTAuthTime     NavPVTFlags3 = 1 << 13
	NavPVTNmaFixStatus NavPVTFlags3 = 1 << 14
)

const (
	NavPVTLastCorrectionAgeNotAvailable NavPVTFlags3 = iota << 1
	NavPVTLastCorrectionAge0to1
	NavPVTLastCorrectionAge1to2
	NavPVTLastCorrectionAge2to5
	NavPVTLastCorrectionAge5to10
	NavPVTLastCorrectionAge10to15
	NavPVTLastCorrectionAge15to20
	NavPVTLastCorrectionAge20to30
	NavPVTLastCorrectionAge30to45
	NavPVTLastCorrectionAge45to60
	NavPVTLastCorrectionAge60to90
	NavPVTLastCorrectionAge90to120
	NavPVTLastCorrectionAge120Plus
	NavPVTLastCorrectionAgeMask NavPVTFlags3 = 0b1111 << 1
)

func (m *NavPVT) ID() MsgID { return NavPVTID }

type NavSat struct {
	NavSatFixed
	SVs []NavSatSV
}

type NavSatFixed struct {
	NavITOW
	Version byte
	NumSVs  byte
	_       [2]byte
}

type NavSatSV struct {
	GNSSID GNSSID
	SVID   byte
	CNO    byte
	Elev   int8
	Azim   int16
	PRRes  int16
	Flags  NavSatFlags
}

var _ VaryingMsg = (*NavSat)(nil)
var _ PartiallyHandledMsg = (*NavSat)(nil)

func (m *NavSat) ID() MsgID { return NavSatID }

func (m *NavSat) IsHandled() bool {
	return m.Version == 1
}

func (m *NavSat) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSatSV, len)
	}
	return
}

func (m *NavSat) FixedPart() any {
	return &m.NavSatFixed
}

func (m *NavSat) VaryingPart() any {
	return &m.SVs
}

type NavSatFlags uint32

// Quality indicator values (bits 2..0)
const (
	NavSatQualityNoSignal NavSatFlags = iota
	NavSatQualitySearchingSignal
	NavSatQualitySignalAcquired
	NavSatQualitySignalDetectedUnusable
	NavSatQualityCodeLocked
	NavSatQualityCodeAndCarrierLocked1
	NavSatQualityCodeAndCarrierLocked2
	NavSatQualityCodeAndCarrierLocked3
)

// Health values (bits 5..4)
const (
	NavSatHealthUnknown NavSatFlags = iota << 4
	NavSatHealthHealthy
	NavSatHealthUnhealthy
)

// Orbit source values (bits 10..8)
const (
	NavSatOrbitSourceNone NavSatFlags = iota << 8
	NavSatOrbitSourceEphemeris
	NavSatOrbitSourceAlmanac
	NavSatOrbitSourceAssistNowOffline
	NavSatOrbitSourceAssistNowAutonomous
	NavSatOrbitSourceOther1
	NavSatOrbitSourceOther2
	NavSatOrbitSourceOther3
)

// Masks to extract the quality, health, and orbit source values
const (
	NavSatQuality NavSatFlags = 0b111
	NavSatHealth  NavSatFlags = 0b11 << 4
	NavSatOrbit   NavSatFlags = 0b111 << 8
)

// Boolean flags
const (
	NavSatSVUsed   NavSatFlags = 1 << 3
	NavSatDiffCorr NavSatFlags = 1 << 6
	NavSatSmoothed NavSatFlags = 1 << 7
	NavSatEphAvail NavSatFlags = 1 << (iota + 11)
	NavSatAlmAvail
	NavSatAnoAvail
	NavSatAopAvail
	_
	NavSatSbasCorrUsed
	NavSatRtcmCorrUsed
	NavSatSlasCorrUsed
	NavSatSpartnCorrUsed
	NavSatPrCorrUsed
	NavSatCrCorrUsed
	NavSatDoCorrUsed
	NavSatClasCorrUsed
)

type NavSig struct {
	NavSigFixed
	Signals []NavSigSignal
}

type NavSigFixed struct {
	NavITOW
	Version byte
	NumSigs byte
	_       [2]byte
}

type NavSigSignal struct {
	GNSSID     GNSSID
	SVID       byte
	SigID      byte
	FreqID     byte
	PRRes      int16
	CNO        byte
	QualityInd NavSigQuality
	CorrSource NavSigCorrSource
	IonoModel  NavSigIonoModel
	SigFlags   NavSigFlags
	_          [4]byte
}

var _ VaryingMsg = (*NavSig)(nil)
var _ PartiallyHandledMsg = (*NavSig)(nil)

func (m *NavSig) ID() MsgID { return NavSigID }

func (m *NavSig) IsHandled() bool {
	return m.Version == 0
}

func (m *NavSig) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 8, 16)
	if err == nil {
		m.Signals = make([]NavSigSignal, len)
	}
	return
}

func (m *NavSig) FixedPart() any {
	return &m.NavSigFixed
}

func (m *NavSig) VaryingPart() any {
	return &m.Signals
}

type NavSigQuality byte

const (
	NavSigQualityNoSignal NavSigQuality = iota
	NavSigQualitySearching
	NavSigQualityAcquired
	NavSigQualityDetectedUnusable
	NavSigQualityCodeLocked
	NavSigQualityCodeCarrierLocked5
	NavSigQualityCodeCarrierLocked6
	NavSigQualityCodeCarrierLocked7
)

type NavSigCorrSource byte

const (
	NavSigCorrSourceNone NavSigCorrSource = iota
	NavSigCorrSourceSBAS
	NavSigCorrSourceBeiDou
	NavSigCorrSourceRTCM2
	NavSigCorrSourceRTCM3OSR
	NavSigCorrSourceRTCM3SSR
	NavSigCorrSourceQZSSSLAS
	NavSigCorrSourceSPARTN
	NavSigCorrSourceCLAS
)

type NavSigIonoModel byte

const (
	NavSigIonoModelNone NavSigIonoModel = iota
	NavSigIonoModelKlobucharGPS
	NavSigIonoModelSBAS
	NavSigIonoModelKlobucharBeiDou
	NavSigIonoModelDualFreq NavSigIonoModel = 8
)

type NavSigFlags uint16

// Boolean flags
const (
	NavSigHealthUnknown NavSigFlags = 0
	NavSigHealthHealthy NavSigFlags = 1 << iota
	NavSigHealthUnhealthy
	NavSigPrSmoothed
	NavSigPrUsed
	NavSigCrUsed
	NavSigDoUsed
	NavSigPrCorrUsed
	NavSigCrCorrUsed
	NavSigDoCorrUsed
)

// Mask to extract health value
const (
	NavSigHealth NavSigFlags = 0b11
)

type NavSVInfo struct {
	NavSVInfoFixed
	SVs []NavSVInfoSV
}

type NavSVInfoFixed struct {
	NavITOW
	NumCh       byte
	GlobalFlags NavSVInfoGlobalFlags
	_           [2]byte
}

type NavSVInfoSV struct {
	ChN     byte
	SVID    byte
	Flags   NavSVInfoFlags
	Quality NavSVInfoQuality
	CNO     byte
	Elev    int8
	Azim    int16
	PRRes   int32
}

func (m *NavSVInfo) ID() MsgID { return NavSVInfoID }

func (m *NavSVInfo) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfoSV, len)
	}
	return
}

func (m *NavSVInfo) FixedPart() any {
	return &m.NavSVInfoFixed
}

func (m *NavSVInfo) VaryingPart() any {
	return &m.SVs
}

type NavSVInfoGlobalFlags byte

const (
	NavSVInfoAntaris NavSVInfoGlobalFlags = iota
	NavSVInfoUblox5
	NavSVInfoUblox6
	NavSVInfoChipGen = 0b111
)

type NavSVInfoFlags byte

const (
	NavSVInfoSVUsed NavSVInfoFlags = 1 << iota
	NavSVInfoDiffCorr
	NavSVInfoOrbitAvail
	NavSVInfoOrbitEph
	NavSVInfoUnhealthy
	NavSVInfoOrbitAlm
	NavSVInfoOrbotAop
	NavSVInfoSmoothed
)

type NavSVInfoQuality byte

const (
	NavSVInfoQualityIdle NavSVInfoQuality = iota
	NavSVInfoQualitySearching
	NavSVInfoQualitySignalAcquired
	NavSVInfoQualitySignalDetectedUnusable
	NavSVInfoQualityCodeLockOnSignal
	NavSVInfoCodeAndCarrierLocked1
	NavSVInfoCodeAndCarrierLocked2
	NavSVInfoCodeAndCarrierLocked3
	NavSVInfoQualityInd = 0b1111
)

type NavSol struct {
	NavITOW
	FTOW       int32
	Week       int16
	GPSFixType NavSolGPSFixType
	Flags      NavSolFlags
	ECEF       [3]int32
	PAcc       uint32
	ECEFV      [3]int32
	SAcc       uint32
	PDOP       uint16
	_          byte
	NumSV      byte
	_          [4]byte
}

type NavSolGPSFixType byte

const (
	NavSolNoFix NavSolGPSFixType = iota
	NavSolDeadReckoningOnly
	NavSol2DFix
	NavSol3DFix
	NavSolGPSDeadReckoning
	NavSolTimeOnlyFix
)

type NavSolFlags byte

const (
	NavSolGPSFixOK NavSolFlags = 1 << iota
	NavSolDiffSoln
	NavSolWKNSET
	NavSolTOWSET
)

func (m *NavSol) ID() MsgID { return NavSolID }

type NavTimeGPS struct {
	NavITOW
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
	NavITOW
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
	NavITOW
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
	NavITOW
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
	NavITOW
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
	NavITOW
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

var _ PartiallyHandledMsg = (*NavTimeLS)(nil)

func (m *NavTimeLS) ID() MsgID { return NavTimeLSID }

func (m *NavTimeLS) IsHandled() bool {
	return m.Version == 0
}

type NavTimeTrusted struct {
	Version  byte
	RefSys   NavTimeTrustedRefSys
	Valid    NavTimeTrustedValid
	_        byte
	NavITOW
	IniWno   uint16
	PropWno  uint16
	IniTow   uint32
	PropTow  uint32
	IniTAcc  uint32
	PropTAcc uint32
	DeltaS   int32
	DeltaMs  int32
	_        [4]byte
}

type NavTimeTrustedRefSys byte

const (
	NavTimeTrustedRefSysNone  NavTimeTrustedRefSys = 0
	NavTimeTrustedRefSysGPS   NavTimeTrustedRefSys = 1
	NavTimeTrustedRefSysGal   NavTimeTrustedRefSys = 2
	NavTimeTrustedRefSysBDS   NavTimeTrustedRefSys = 3
	NavTimeTrustedRefSysNavIC NavTimeTrustedRefSys = 15
)

type NavTimeTrustedValid byte

const (
	NavTimeTrustedValidTrustedTime NavTimeTrustedValid = 1 << iota
	NavTimeTrustedValidDeltaTime
)

var _ PartiallyHandledMsg = (*NavTimeTrusted)(nil)

func (m *NavTimeTrusted) ID() MsgID { return NavTimeTrustedID }

func (m *NavTimeTrusted) IsHandled() bool {
	return m.Version == 1
}

type NavSvin struct {
	Version byte
	_       [3]byte
	NavITOW
	Dur     uint32
	MeanX   int32
	MeanY   int32
	MeanZ   int32
	MeanXHP int8
	MeanYHP int8
	MeanZHP int8
	_       byte
	MeanAcc uint32
	Obs     uint32
	Valid   byte
	Active  byte
	_       [2]byte
}

var _ PartiallyHandledMsg = (*NavSvin)(nil)

func (m *NavSvin) ID() MsgID { return NavSvinID }

func (m *NavSvin) IsHandled() bool {
	return m.Version == 0
}

func init() {
	regMsg[NavDOP]("DOP")
	regMsg[NavPosECEF]("POSECEF")
	regMsg[NavPosLLH]("POSLLH")
	regMsg[NavPVT]("PVT")
	regMsg[NavSat]("SAT")
	regMsg[NavSig]("SIG")
	regMsg[NavSVInfo]("SVINFO")
	regMsg[NavSol]("SOL")
	regMsg[NavSvin]("SVIN")
	regMsg[NavTimeGPS]("TIMEGPS")
	regMsg[NavTimeBDS]("TIMEBDS")
	regMsg[NavTimeGal]("TIMEGAL")
	regMsg[NavTimeGLO]("TIMEGLO")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavTimeLS]("TIMELS")
	regMsg[NavTimeTrusted]("TIMETRUSTED")
	regMsg[NavVelECEF]("VELECEF")
	regMsg[NavVelNED]("VELNED")
}
