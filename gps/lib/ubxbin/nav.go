package ubxbin

const (
	NavClockID       MsgID = clsNav | (0x22 << 8)
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
	NavTimeQZSSID    MsgID = clsNav | (0x27 << 8)
	NavTimeLSID      MsgID = clsNav | (0x26 << 8)
	NavEOEID         MsgID = clsNav | (0x61 << 8)
	NavTimeTrustedID MsgID = clsNav | (0x64 << 8)
	NavVelECEFID     MsgID = clsNav | (0x11 << 8)
	NavVelNEDID      MsgID = clsNav | (0x12 << 8)
	NavHPPosECEFID   MsgID = clsNav | (0x13 << 8)
	NavHPPosLLHID    MsgID = clsNav | (0x14 << 8)
	NavSvinID        MsgID = clsNav | (0x3B << 8)
)

type NavMsg interface {
	Msg
	NavEpoch() uint32
}

type NavITOW struct {
	ITOW uint32 `json:"iTOW"`
}

func (m *NavITOW) NavEpoch() uint32 {
	return m.ITOW
}

// NavEOE is the end-of-epoch marker output after all NAV and NMEA messages.
type NavEOE struct {
	NavITOW
}

func (m *NavEOE) ID() MsgID { return NavEOEID }

type NavClock struct {
	NavITOW
	ClkB int32  `json:"clkB"` // Clock bias in ns
	ClkD int32  `json:"clkD"` // Clock drift in ns/s
	TAcc uint32 `json:"tAcc"` // Time accuracy estimate in ns
	FAcc uint32 `json:"fAcc"` // Frequency accuracy estimate in ps/s
}

func (m *NavClock) ID() MsgID { return NavClockID }

type NavDOP struct {
	NavITOW
	GDOP uint16 `json:"gDOP"`
	PDOP uint16 `json:"pDOP"`
	TDOP uint16 `json:"tDOP"`
	VDOP uint16 `json:"vDOP"`
	HDOP uint16 `json:"hDOP"`
	NDOP uint16 `json:"nDOP"`
	EDOP uint16 `json:"eDOP"`
}

func (m *NavDOP) ID() MsgID { return NavDOPID }

type NavPosECEF struct {
	NavITOW
	ECEF [3]int32 `json:"ecef"`
	PAcc uint32   `json:"pAcc"`
}

func (m *NavPosECEF) ID() MsgID { return NavPosECEFID }

type NavVelECEF struct {
	NavITOW
	ECEFV [3]int32 `json:"ecefV"`
	SAcc  uint32   `json:"sAcc"`
}

func (m *NavVelECEF) ID() MsgID { return NavVelECEFID }

type NavHPPosECEF struct {
	Version byte `json:"version"`
	_       [3]byte
	NavITOW
	ECEF   [3]int32          `json:"ecef"`
	ECEFHp [3]int8           `json:"ecefHp"`
	Flags  NavHPPosECEFFlags `json:"flags"`
	PAcc   uint32            `json:"pAcc"`
}

type NavHPPosECEFFlags byte

const NavHPPosECEFInvalidEcef NavHPPosECEFFlags = 1 << 0

var _ PartiallyHandledMsg = (*NavHPPosECEF)(nil)

func (m *NavHPPosECEF) ID() MsgID        { return NavHPPosECEFID }
func (m *NavHPPosECEF) IsHandled() bool   { return m.Version == 0 }

type NavPosLLH struct {
	NavITOW
	Lon    int32  `json:"lon"`
	Lat    int32  `json:"lat"`
	Height int32  `json:"height"`
	HMSL   int32  `json:"hMSL"`
	HAcc   uint32 `json:"hAcc"`
	VAcc   uint32 `json:"vAcc"`
}

func (m *NavPosLLH) ID() MsgID { return NavPosLLHID }

type NavHPPosLLH struct {
	Version byte             `json:"version"`
	_       [2]byte
	Flags   NavHPPosLLHFlags `json:"flags"`
	NavITOW
	Lon      int32  `json:"lon"`
	Lat      int32  `json:"lat"`
	Height   int32  `json:"height"`
	HMSL     int32  `json:"hMSL"`
	LonHp    int8   `json:"lonHp"`
	LatHp    int8   `json:"latHp"`
	HeightHp int8   `json:"heightHp"`
	HMSLHp   int8   `json:"hMSLHp"`
	HAcc     uint32 `json:"hAcc"`
	VAcc     uint32 `json:"vAcc"`
}

type NavHPPosLLHFlags byte

const NavHPPosLLHInvalidLlh NavHPPosLLHFlags = 1 << 0

var _ PartiallyHandledMsg = (*NavHPPosLLH)(nil)

func (m *NavHPPosLLH) ID() MsgID        { return NavHPPosLLHID }
func (m *NavHPPosLLH) IsHandled() bool   { return m.Version == 0 }

type NavVelNED struct {
	NavITOW
	VelNED  [3]int32 `json:"velNED"`
	Speed   uint32   `json:"speed"`
	GSpeed  uint32   `json:"gSpeed"`
	Heading int32    `json:"heading"`
	SAcc    uint32   `json:"sAcc"`
	CAcc    uint32   `json:"cAcc"`
}

func (m *NavVelNED) ID() MsgID { return NavVelNEDID }

type NavPVT struct {
	NavITOW
	Year    uint16        `json:"year"`
	Month   byte          `json:"month"`
	Day     byte          `json:"day"`
	Hour    byte          `json:"hour"`
	Min     byte          `json:"min"`
	Sec     byte          `json:"sec"`
	Valid   NavPVTValid   `json:"valid"`
	TAcc    uint32        `json:"tAcc"`
	Nano    int32         `json:"nano"`
	FixType NavPVTFixType `json:"fixType"`
	Flags   NavPVTFlags   `json:"flags"`
	Flags2  NavPVTFlags2  `json:"flags2"`
	NumSV   byte          `json:"numSV"`
	Lon     int32         `json:"lon"`
	Lat     int32         `json:"lat"`
	Height  int32         `json:"height"`
	HMSL    int32         `json:"hMSL"`
	HAcc    uint32        `json:"hAcc"`
	VAcc    uint32        `json:"vAcc"`
	VelN    int32         `json:"velN"`
	VelE    int32         `json:"velE"`
	VelD    int32         `json:"velD"`
	GSpeed  int32         `json:"gSpeed"`
	HeadMot int32         `json:"headMot"`
	SAcc    uint32        `json:"sAcc"`
	HeadAcc uint32        `json:"headAcc"`
	PDOP    uint16        `json:"pDOP"`
	Flags3  NavPVTFlags3  `json:"flags3"`
	_       [4]byte
	HeadVeh int32  `json:"headVeh"`
	MagDec  int16  `json:"magDec"`
	MagAcc  uint16 `json:"magAcc"`
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
	SVs []NavSatSV `json:"SVs"`
}

type NavSatFixed struct {
	NavITOW
	Version byte `json:"version"`
	NumSVs  byte `json:"numSvs"`
	_       [2]byte
}

type NavSatSV struct {
	GNSSID GNSSID      `json:"gnssId"`
	SVID   byte        `json:"svId"`
	CNO    byte        `json:"cno"`
	Elev   int8        `json:"elev"`
	Azim   int16       `json:"azim"`
	PRRes  int16       `json:"prRes"`
	Flags  NavSatFlags `json:"flags"`
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
)

const (
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
	NavSatLppCorrUsed
	NavSatHasCorrUsed
)

type NavSig struct {
	NavSigFixed
	Signals []NavSigSignal `json:"signals"`
}

type NavSigFixed struct {
	NavITOW
	Version byte `json:"version"`
	NumSigs byte `json:"numSigs"`
	_       [2]byte
}

type NavSigSignal struct {
	GNSSID     GNSSID           `json:"gnssId"`
	SVID       byte             `json:"svId"`
	SigID      byte             `json:"sigId"`
	FreqID     byte             `json:"freqId"`
	PRRes      int16            `json:"prRes"`
	CNO        byte             `json:"cno"`
	QualityInd NavSigQuality    `json:"qualityInd"`
	CorrSource NavSigCorrSource `json:"corrSource"`
	IonoModel  NavSigIonoModel  `json:"ionoModel"`
	SigFlags   NavSigFlags      `json:"sigFlags"`
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
	NavSigCorrSourceNone    NavSigCorrSource = 0
	NavSigCorrSourceSBAS    NavSigCorrSource = 1
	NavSigCorrSourceBeiDou  NavSigCorrSource = 2
	NavSigCorrSourceRTCM2   NavSigCorrSource = 3
	NavSigCorrSourceRTCM3OSR NavSigCorrSource = 4
	NavSigCorrSourceRTCM3SSR NavSigCorrSource = 5
	NavSigCorrSourceQZSSSLAS NavSigCorrSource = 6
	NavSigCorrSourceSPARTN  NavSigCorrSource = 7
	NavSigCorrSourceCLAS    NavSigCorrSource = 9
	NavSigCorrSourceLPPOSR  NavSigCorrSource = 10
	NavSigCorrSourceLPPSSR  NavSigCorrSource = 11
	NavSigCorrSourceGALHAS  NavSigCorrSource = 12
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

// Health values (2-bit enum in bits 0-1)
const (
	NavSigHealthUnknown NavSigFlags = iota
	NavSigHealthHealthy
	NavSigHealthUnhealthy
)

// Boolean flags
const (
	NavSigPrSmoothed NavSigFlags = 1 << (iota + 2)
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
	SVs []NavSVInfoSV `json:"SVs"`
}

type NavSVInfoFixed struct {
	NavITOW
	NumCh       byte                 `json:"numCh"`
	GlobalFlags NavSVInfoGlobalFlags `json:"globalFlags"`
	_           [2]byte
}

type NavSVInfoSV struct {
	ChN     byte             `json:"chn"`
	SVID    byte             `json:"svid"`
	Flags   NavSVInfoFlags   `json:"flags"`
	Quality NavSVInfoQuality `json:"quality"`
	CNO     byte             `json:"cno"`
	Elev    int8             `json:"elev"`
	Azim    int16            `json:"azim"`
	PRRes   int32            `json:"prRes"`
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
	FTOW       int32            `json:"fTOW"`
	Week       int16            `json:"week"`
	GPSFixType NavSolGPSFixType `json:"gpsFix"`
	Flags      NavSolFlags      `json:"flags"`
	ECEF       [3]int32         `json:"ecef"`
	PAcc       uint32           `json:"pAcc"`
	ECEFV      [3]int32         `json:"ecefV"`
	SAcc       uint32           `json:"sAcc"`
	PDOP       uint16           `json:"pDOP"`
	_          byte
	NumSV      byte `json:"numSV"`
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
	FTOW  int32           `json:"fTOW"`
	Week  int16           `json:"week"`
	LeapS byte            `json:"leapS"`
	Valid NavTimeGPSValid `json:"valid"`
	TAcc  uint32          `json:"tAcc"`
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
	TAcc  uint32          `json:"tAcc"`
	Nano  int32           `json:"nano"`
	Year  uint16          `json:"year"`
	Month byte            `json:"month"`
	Day   byte            `json:"day"`
	Hour  byte            `json:"hour"`
	Min   byte            `json:"min"`
	Sec   byte            `json:"sec"`
	Valid NavTimeUTCValid `json:"valid"`
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
	SOW   uint32          `json:"SOW"`
	FSOW  int32           `json:"fSOW"`
	Week  int16           `json:"week"`
	LeapS byte            `json:"leapS"`
	Valid NavTimeBDSValid `json:"valid"`
	TAcc  uint32          `json:"tAcc"`
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
	GalTOW  uint32          `json:"galTow"`
	FGalTOW int32           `json:"fGalTow"`
	GalWno  int16           `json:"galWno"`
	LeapS   byte            `json:"leapS"`
	Valid   NavTimeGalValid `json:"valid"`
	TAcc    uint32          `json:"tAcc"`
}

func (m *NavTimeGal) ID() MsgID { return NavTimeGalID }

type NavTimeGalValid byte

const (
	NavTimeGalTOWValid NavTimeGalValid = 1 << iota
	NavTimeGalWnoValid
	NavTimeGalLeapSValid
)

type NavTimeQZSS struct {
	NavITOW
	QzssTow  uint32           `json:"qzssTow"`
	FQzssTow int32            `json:"fQzssTow"`
	QzssWno  int16            `json:"qzssWno"`
	LeapS    byte             `json:"leapS"`
	Valid    NavTimeQZSSValid `json:"valid"`
	TAcc     uint32           `json:"tAcc"`
}

func (m *NavTimeQZSS) ID() MsgID { return NavTimeQZSSID }

type NavTimeQZSSValid byte

const (
	NavTimeQZSSTOWValid NavTimeQZSSValid = 1 << iota
	NavTimeQZSSWnoValid
	NavTimeQZSSLeapSValid
)

type NavTimeGLO struct {
	NavITOW
	TOD   uint32          `json:"TOD"`
	FTOD  int32           `json:"fTOD"`
	Nt    uint16          `json:"Nt"`
	N4    byte            `json:"N4"`
	Valid NavTimeGLOValid `json:"valid"`
	TAcc  uint32          `json:"tAcc"`
}

func (m *NavTimeGLO) ID() MsgID { return NavTimeGLOID }

type NavTimeGLOValid byte

const (
	NavTimeGLOTODValid NavTimeGLOValid = 1 << iota
	NavTimeGLODateValid
)

type NavTimeLS struct {
	NavITOW
	Version       byte                   `json:"version"`
	_             [3]byte
	SrcOfCurrLS   NavTimeSrcOfCurrLS     `json:"srcOfCurrLs"`
	CurrLS        int8                   `json:"currLs"`
	SrcOfLSChange NavTimeLSSrcOfLSChange `json:"srcOfLsChange"`
	LSChange      NavTimeLSChange        `json:"lsChange"`
	TimeToLSEvent int32                  `json:"timeToLsEvent"`
	DateOfLSGPSWN uint16                 `json:"dateOfLsGpsWn"`
	DateOfLSGPSDN uint16                 `json:"dateOfLsGpsDn"`
	_             [3]byte
	Valid         NavTimeLSValid `json:"valid"`
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
	Version  byte                 `json:"version"`
	RefSys   NavTimeTrustedRefSys `json:"refSys"`
	Valid    NavTimeTrustedValid  `json:"valid"`
	_        byte
	NavITOW
	IniWno   uint16 `json:"iniWno"`
	PropWno  uint16 `json:"propWno"`
	IniTow   uint32 `json:"iniTow"`
	PropTow  uint32 `json:"propTow"`
	IniTAcc  uint32 `json:"iniTAcc"`
	PropTAcc uint32 `json:"propTAcc"`
	DeltaS   int32  `json:"deltaS"`
	DeltaMs  int32  `json:"deltaMs"`
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
	Version byte `json:"version"`
	_       [3]byte
	NavITOW
	Dur     uint32 `json:"dur"`
	MeanX   int32  `json:"meanX"`
	MeanY   int32  `json:"meanY"`
	MeanZ   int32  `json:"meanZ"`
	MeanXHP int8   `json:"meanXHP"`
	MeanYHP int8   `json:"meanYHP"`
	MeanZHP int8   `json:"meanZHP"`
	_       byte
	MeanAcc uint32 `json:"meanAcc"`
	Obs     uint32 `json:"obs"`
	Valid   byte   `json:"valid"`
	Active  byte   `json:"active"`
	_       [2]byte
}

var _ PartiallyHandledMsg = (*NavSvin)(nil)

func (m *NavSvin) ID() MsgID { return NavSvinID }

func (m *NavSvin) IsHandled() bool {
	return m.Version == 0
}

func init() {
	regMsg[NavEOE]("EOE")
	regMsg[NavClock]("CLOCK")
	regMsg[NavDOP]("DOP")
	regMsg[NavPosECEF]("POSECEF")
	regMsg[NavHPPosECEF]("HPPOSECEF")
	regMsg[NavPosLLH]("POSLLH")
	regMsg[NavHPPosLLH]("HPPOSLLH")
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
	regMsg[NavTimeQZSS]("TIMEQZSS")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavTimeLS]("TIMELS")
	regMsg[NavTimeTrusted]("TIMETRUSTED")
	regMsg[NavVelECEF]("VELECEF")
	regMsg[NavVelNED]("VELNED")
}
