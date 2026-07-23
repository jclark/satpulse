package casbin

import (
	"math"
	"time"
)

// JSON tags in this file preserve the field-name spellings in the CASIC
// protocol documentation, including its unusual capitalization and
// underscores. Messages shared by V5 and V6 generally use the V6 names.
// The exceptions are called out on the affected types below.

const (
	CfgPrtID     MsgID = clsCfg | (0x00 << 8)
	CfgMsgID     MsgID = clsCfg | (0x01 << 8)
	CfgRstID     MsgID = clsCfg | (0x02 << 8)
	CfgTPID      MsgID = clsCfg | (0x03 << 8)
	CfgRateID    MsgID = clsCfg | (0x04 << 8)
	CfgCfgID     MsgID = clsCfg | (0x05 << 8)
	CfgTModeID   MsgID = clsCfg | (0x06 << 8)
	CfgNavxID    MsgID = clsCfg | (0x07 << 8)
	CfgNavLimID  MsgID = clsCfg | (0x0A << 8)
	CfgNavBandID MsgID = clsCfg | (0x0F << 8)
	CfgNmeaID    MsgID = clsCfg | (0x12 << 8)
	CfgRtcmID    MsgID = clsCfg | (0x14 << 8)
	CfgTMode2ID  MsgID = clsCfg | (0x16 << 8)
)

// CfgMsg is CFG-MSG (0x06 0x01) - message rate configuration (4 bytes).
// Rate 0xFFFF polls the target message instead of setting its rate.
// An empty-payload CFG-MSG query returns one CfgMsg response per known
// message (the whole rate table). The protocol documents separate clsID
// and msgID fields; Target combines both and uses msgID as its JSON name,
// consistently with the combined message identifier in the other *bin
// packages.
type CfgMsg struct {
	Target MsgID      `json:"msgID"` // message whose rate is configured (cls byte, then id byte)
	Rate   CfgMsgRate `json:"rate"`  // output rate in fixes per message
}

func (m *CfgMsg) ID() MsgID { return CfgMsgID }

// CfgMsgRate is a message's output divisor in navigation fixes.
type CfgMsgRate uint16

const (
	CfgMsgRateOff      CfgMsgRate = 0
	CfgMsgRateEveryFix CfgMsgRate = 1
	// CfgMsgRatePoll makes CFG-MSG immediately output the target once.
	CfgMsgRatePoll CfgMsgRate = 0xFFFF
)

// CfgPrt is CFG-PRT (0x06 0x00) - serial port configuration (8 bytes).
// An empty-payload query returns one CfgPrt response per UART.
type CfgPrt struct {
	PortID    CfgPrtPortID    `json:"portID"`
	ProtoMask CfgPrtProtoMask `json:"protoMask"`
	Mode      CfgPrtMode      `json:"mode"`
	BaudRate  uint32          `json:"baudRate"` // bits per second
}

func (m *CfgPrt) ID() MsgID { return CfgPrtID }

// CfgPrtPortID selects a receiver UART.
type CfgPrtPortID uint8

const (
	CfgPrtPortUART0 CfgPrtPortID = iota
	CfgPrtPortUART1
	// CfgPrtPortCurrent selects the port on which the request arrives.
	CfgPrtPortCurrent CfgPrtPortID = 0xFF
)

// CfgPrtProtoMask controls the protocols accepted and emitted by a UART.
type CfgPrtProtoMask uint8

// CfgPrt.ProtoMask bits.
const (
	CfgPrtProtoBinaryIn CfgPrtProtoMask = 1 << iota
	CfgPrtProtoTextIn
	_
	CfgPrtProtoRTCMIn // V6 only
	CfgPrtProtoBinaryOut
	CfgPrtProtoTextOut
	_
	CfgPrtProtoRTCMOut // V6 only
)

// CfgPrtMode encodes UART character length, parity, and stop bits.
type CfgPrtMode uint16

const (
	CfgPrtModeCharLen       CfgPrtMode = 0b11 << 6
	CfgPrtModeCharLen5      CfgPrtMode = 0
	CfgPrtModeCharLen6      CfgPrtMode = 0b01 << 6
	CfgPrtModeCharLen7      CfgPrtMode = 0b10 << 6
	CfgPrtModeCharLen8      CfgPrtMode = 0b11 << 6
	CfgPrtModeParity        CfgPrtMode = 0b111 << 9
	CfgPrtModeParityEven    CfgPrtMode = 0
	CfgPrtModeParityOdd     CfgPrtMode = 0b001 << 9
	CfgPrtModeParityNone    CfgPrtMode = 0b100 << 9
	CfgPrtModeParityNoneAlt CfgPrtMode = 0b101 << 9
	CfgPrtModeStopBits      CfgPrtMode = 0b11 << 12
	CfgPrtModeStopBits1     CfgPrtMode = 0
	CfgPrtModeStopBits1p5   CfgPrtMode = 0b01 << 12
	CfgPrtModeStopBits2     CfgPrtMode = 0b10 << 12
)

// CfgRate is CFG-RATE (0x06 0x04) - navigation rate (4 bytes).
// V5 firmware fills only FixIntervalMs and leaves the rest zero.
type CfgRate struct {
	FixIntervalMs CfgRateFixInterval `json:"fixIntervalMs"` // milliseconds
	FixRateHz     CfgRateFixRate     `json:"fixRateHz"`     // V6 only
	Res           uint8              `json:"res"`
}

func (m *CfgRate) ID() MsgID { return CfgRateID }

// CfgRateFixInterval is the positioning interval in milliseconds.
type CfgRateFixInterval uint16

// CfgRateFixRate is the V6 positioning frequency in hertz.
type CfgRateFixRate uint8

const (
	CfgRateFixInterval1Hz CfgRateFixInterval = 1000
	CfgRateFixRate1Hz     CfgRateFixRate     = 1
	// CfgRateFixRateV5Reserved is required in the V5 payload.
	CfgRateFixRateV5Reserved CfgRateFixRate = 0
)

// CfgCfg is CFG-CFG (0x06 0x05) - clear/save/load configuration (4 bytes).
// V5 firmware honours the section mask; V6 documents the field as res1.
// Hardware shows that V6 also requires a nonzero mask to save, and the
// configurator treats the field as a mask on both families, so its JSON
// name is the V5 mask rather than the V6 res1. The other JSON names use V6.
type CfgCfg struct {
	Mask   CfgCfgSectionMask `json:"mask"`
	OpMode CfgCfgOpMode      `json:"opMode"`
	Res    uint8             `json:"res2"`
}

func (m *CfgCfg) ID() MsgID { return CfgCfgID }

// CfgCfgOpMode selects a configuration-storage operation.
type CfgCfgOpMode uint8

const (
	CfgCfgOpClear CfgCfgOpMode = iota // reset sections to defaults
	CfgCfgOpSave                      // save running config to NVM
	CfgCfgOpLoad                      // load config from NVM
)

// CfgCfgSectionMask selects V5 configuration-storage sections. V6
// documents the field as reserved, but hardware requires a nonzero mask.
type CfgCfgSectionMask uint16

const (
	CfgCfgSectionPort  CfgCfgSectionMask = 1 << iota // CFG-PRT
	CfgCfgSectionMsg                                 // CFG-MSG rates
	CfgCfgSectionInf                                 // CFG-INF
	CfgCfgSectionNav                                 // CFG-RATE, CFG-TMODE; covers CFG-NAVX too (verified on the AT6558D: a save with only this bit persists a NAVX min-elev change across reload)
	CfgCfgSectionTP                                  // CFG-TP
	CfgCfgSectionGroup                               // CFG-GROUP
	CfgCfgSectionAll   CfgCfgSectionMask = 0xFFFF
)

// CfgRst is CFG-RST (0x06 0x02) - receiver restart (4 bytes). The
// fields are the V5 layout (casic2.md); zkw3.md 3.13.3 documents the
// V6 payload differently - bytes 0-2 reserved, byte 3 the start mode -
// but the encodings coincide: StartMode occupies byte 3 with the same
// hot/warm/cold/factory values, and a V6 request zeroes the rest, so
// serializing this struct is wire-correct for both families.
// Its JSON names follow the V5 semantic layout used by the configurator;
// the V6 document instead calls the fields res1, res2, and resetMode.
// The receiver does not acknowledge CFG-RST before restarting.
type CfgRst struct {
	NavBbrMask CfgRstNavBbrMask `json:"navBbrMask"` // V5; reserved on V6
	ResetMode  CfgRstResetMode  `json:"resetMode"`  // V5; reserved on V6
	StartMode  CfgRstStartMode  `json:"startMode"`
}

func (m *CfgRst) ID() MsgID { return CfgRstID }

// CfgRstNavBbrMask selects V5 battery-backed sections to clear.
type CfgRstNavBbrMask uint16

const (
	CfgRstNavBbrEphemeris CfgRstNavBbrMask = 1 << iota
	CfgRstNavBbrAlmanac
	CfgRstNavBbrHealth
	CfgRstNavBbrIonosphere
	CfgRstNavBbrPosition
	CfgRstNavBbrClockDrift
	CfgRstNavBbrOscParams
	CfgRstNavBbrUTCParams
	CfgRstNavBbrRTC
	CfgRstNavBbrConfig
	// CfgRstNavBbrV6Reserved is required in the V6 payload.
	CfgRstNavBbrV6Reserved CfgRstNavBbrMask = 0
)

// CfgRstResetMode selects the V5 reset mechanism.
type CfgRstResetMode uint8

const (
	CfgRstResetHardwareImmediate CfgRstResetMode = iota
	CfgRstResetSoftwareControlled
	CfgRstResetSoftwareGPSOnly
	_
	CfgRstResetHardwareAfterShutdown
	// CfgRstResetModeV6Reserved is required in the V6 payload.
	CfgRstResetModeV6Reserved CfgRstResetMode = 0
)

// CfgRstStartMode selects the restart scope. V6 assigns these values to
// its resetMode field at the same byte offset as the V5 startMode field.
type CfgRstStartMode uint8

const (
	CfgRstStartHot CfgRstStartMode = iota
	CfgRstStartWarm
	CfgRstStartCold
	CfgRstStartFactory
)

// CfgTP is CFG-TP (0x06 0x03) - time pulse configuration (16 bytes).
// Enum semantics differ between firmware families:
// PPSOutMode: V6 0=off, 1=time known, 2=sat sync, 3=pos+time valid,
// 5=reliable, 7=always on; V5 0=off, 1=on, 2=maintain after fix lost,
// 3=fix only. TBase: V6 0=GNSS, 1=UTC; V5 inverted: 0=UTC, 1=satellite.
// TSrcMode: V6 0-3=force GPS/BDS/GLN/GAL, 4-8=primary, 9=auto;
// V5 0=GPS, 1=BDS, 2=GLN, 4=BDS-main, 5=GPS-main, 6=GLN-main.
// The JSON names use the V6 spellings for this shared message.
type CfgTP struct {
	Interval   uint32          `json:"ppsInterval"` // microseconds
	Width      uint32          `json:"ppsWidth"`    // microseconds
	PPSOutMode CfgTPPPSOutMode `json:"ppsOutMode"`
	Polarity   CfgTPPolarity   `json:"polar"`
	TBase      CfgTPTBase      `json:"tBase"`
	TSrcMode   CfgTPTSrcMode   `json:"tSrcMode"`
	UserDelay  float32         `json:"userDelay"` // seconds
}

func (m *CfgTP) ID() MsgID { return CfgTPID }

// CfgTPMicroseconds encodes a pulse interval or width at the wire's
// microsecond resolution.
func CfgTPMicroseconds(d time.Duration) uint32 {
	return uint32(d.Round(time.Microsecond) / time.Microsecond)
}

// CfgTPDuration decodes a wire pulse interval or width.
func CfgTPDuration(us uint32) time.Duration {
	return time.Duration(us) * time.Microsecond
}

// CfgTPUserDelaySeconds encodes a user delay at the wire's float32
// precision.
func CfgTPUserDelaySeconds(d time.Duration) float32 {
	return float32(d.Seconds())
}

// CfgTPUserDelayDuration decodes a user delay, rounding to the nearest
// nanosecond.
func CfgTPUserDelayDuration(seconds float32) time.Duration {
	return time.Duration(math.Round(float64(seconds) * float64(time.Second)))
}

// CfgTPPPSOutMode controls when the receiver emits its pulse. V5 and V6
// assign different meanings to several values, so their constants are
// deliberately separate.
type CfgTPPPSOutMode uint8

const (
	CfgTPPPSOutV5Off      CfgTPPPSOutMode = 0
	CfgTPPPSOutV5On       CfgTPPPSOutMode = 1
	CfgTPPPSOutV5Maintain CfgTPPPSOutMode = 2
	CfgTPPPSOutV5FixOnly  CfgTPPPSOutMode = 3
)

const (
	CfgTPPPSOutV6Off                  CfgTPPPSOutMode = 0
	CfgTPPPSOutV6TimeKnown            CfgTPPPSOutMode = 1
	CfgTPPPSOutV6SatelliteSync        CfgTPPPSOutMode = 2
	CfgTPPPSOutV6PositionTimeValid    CfgTPPPSOutMode = 3
	CfgTPPPSOutV6PositionTimeReliable CfgTPPPSOutMode = 5
	CfgTPPPSOutV6AlwaysOn             CfgTPPPSOutMode = 7
)

// CfgTPPolarity selects which pulse edge is aligned to the reference time.
type CfgTPPolarity uint8

const (
	CfgTPPolarityRising CfgTPPolarity = iota
	CfgTPPolarityFalling
)

// CfgTPTBase selects the pulse's reference time. V5 and V6 invert the
// meanings of the two wire values.
type CfgTPTBase uint8

const (
	CfgTPTBaseV5UTC       CfgTPTBase = 0
	CfgTPTBaseV5Satellite CfgTPTBase = 1
)

const (
	CfgTPTBaseV6GNSS CfgTPTBase = 0
	CfgTPTBaseV6UTC  CfgTPTBase = 1
)

// CfgTPTSrcMode selects the timing GNSS. V5 and V6 meanings are kept
// separate even where their numeric values coincide.
type CfgTPTSrcMode uint8

const (
	CfgTPTSrcV5ForceGPS   CfgTPTSrcMode = 0
	CfgTPTSrcV5ForceBDS   CfgTPTSrcMode = 1
	CfgTPTSrcV5ForceGLN   CfgTPTSrcMode = 2
	CfgTPTSrcV5PrimaryBDS CfgTPTSrcMode = 4
	CfgTPTSrcV5PrimaryGPS CfgTPTSrcMode = 5
	CfgTPTSrcV5PrimaryGLN CfgTPTSrcMode = 6
)

const (
	CfgTPTSrcV6ForceGPS   CfgTPTSrcMode = 0
	CfgTPTSrcV6ForceBDS   CfgTPTSrcMode = 1
	CfgTPTSrcV6ForceGLN   CfgTPTSrcMode = 2
	CfgTPTSrcV6ForceGAL   CfgTPTSrcMode = 3
	CfgTPTSrcV6PrimaryBDS CfgTPTSrcMode = 4
	CfgTPTSrcV6PrimaryGPS CfgTPTSrcMode = 5
	CfgTPTSrcV6PrimaryGLN CfgTPTSrcMode = 6
	CfgTPTSrcV6PrimaryGAL CfgTPTSrcMode = 7
	CfgTPTSrcV6PrimaryIRN CfgTPTSrcMode = 8
	CfgTPTSrcV6Auto       CfgTPTSrcMode = 9
)

// CfgNavx is CFG-NAVX (0x06 0x07) - V5 navigation engine configuration
// (44 bytes). On set, only fields with their Mask bit are applied; the
// receiver ignores the rest, so no read-modify-write is needed. On query
// responses Mask is 0 and all fields hold current values.
type CfgNavx struct {
	Mask         CfgNavxMask      `json:"mask"`
	DynModel     CfgNavxDynModel  `json:"dyModel"`
	FixMode      CfgNavxFixMode   `json:"fixMode"`
	MinSVs       uint8            `json:"minSVs"`
	MaxSVs       uint8            `json:"maxSVs"`
	MinCNO       uint8            `json:"minCNO"`
	Res1         uint8            `json:"res1"`
	IniFix3D     CfgNavxIniFix3D  `json:"iniFix3D"`
	MinElev      int8             `json:"minElev"` // degrees
	DrLimit      uint8            `json:"drLimit"`
	NavSystem    CfgNavxNavSystem `json:"navSystem"`
	WnRollOver   uint16           `json:"wnRollOver"` // GPS week rollover reference
	FixedAlt     float32          `json:"fixedAlt"`
	FixedAltVar  float32          `json:"fixedAltVar"`
	PDop         float32          `json:"pDop"`
	TDop         float32          `json:"tDop"`
	PAcc         float32          `json:"pAcc"`
	TAcc         float32          `json:"tAcc"`
	StaticHoldTh float32          `json:"staticHoldTh"`
}

func (m *CfgNavx) ID() MsgID { return CfgNavxID }

// CfgNavxMask selects fields to apply in a set request.
type CfgNavxMask uint32

const (
	CfgNavxApplyDynModel CfgNavxMask = 1 << iota
	CfgNavxApplyFixMode
	CfgNavxApplySVCount
	CfgNavxApplyMinCNO
	_
	CfgNavxApplyInitialFix3D
	CfgNavxApplyMinElev
	CfgNavxApplyDrLimit
	CfgNavxApplyNavSystem
	CfgNavxApplyWnRollOver
	CfgNavxApplyAltAssist
	CfgNavxApplyPDop
	CfgNavxApplyTDop
	CfgNavxApplyStaticHold
)

// CfgNavxNavSystem selects V5 navigation constellations.
type CfgNavxNavSystem uint8

const (
	CfgNavxNavSystemGPS CfgNavxNavSystem = 1 << iota
	CfgNavxNavSystemBDS
	CfgNavxNavSystemGLN
)

// CfgNavxDynModel selects a V5 navigation dynamics model.
type CfgNavxDynModel uint8

const (
	CfgNavxDynPortable CfgNavxDynModel = iota
	CfgNavxDynStationary
	CfgNavxDynPedestrian
	CfgNavxDynAutomotive
	CfgNavxDynMarine
	CfgNavxDynAirborne1G
	CfgNavxDynAirborne2G
	CfgNavxDynAirborne4G
)

// CfgNavxFixMode selects dimensionality for a V5 navigation fix.
type CfgNavxFixMode uint8

const (
	CfgNavxFixReserved CfgNavxFixMode = iota
	CfgNavxFix2D
	CfgNavxFix3D
	CfgNavxFixAuto
)

// CfgNavxIniFix3D controls whether the initial fix must be 3D.
type CfgNavxIniFix3D uint8

const (
	CfgNavxIniFix2DAllowed CfgNavxIniFix3D = iota
	CfgNavxIniFix3DRequired
)

// CfgTMode is CFG-TMODE (0x06 0x06) - V5 timing mode configuration
// (40 bytes). The mode field is documented as U4 but the receiver
// returns garbage in the upper two bytes, so it is parsed as U2 + U2
// reserved (set Res to 0 when building). The protocol gives that upper
// half no separate field name; its JSON name is therefore res.
type CfgTMode struct {
	Mode         CfgTModeMode `json:"mode"`
	Res          uint16       `json:"res"`
	EcefX        float64      `json:"fixedPosX"`    // m
	EcefY        float64      `json:"fixedPosY"`    // m
	EcefZ        float64      `json:"fixedPosZ"`    // m
	PosVar       float32      `json:"fixedPosVar"`  // m^2, fixed position variance
	SvinMinDur   uint32       `json:"svinMinDur"`   // s, min survey-in duration
	SvinVarLimit float32      `json:"svinVarLimit"` // m^2, survey-in variance limit
}

func (m *CfgTMode) ID() MsgID { return CfgTModeID }

// CfgTModeMode selects the V5 timing mode.
type CfgTModeMode uint16

const (
	CfgTModeAuto CfgTModeMode = iota
	CfgTModeSurvey
	CfgTModeFixed
)

// CfgTModeVarianceFromAccuracy converts a position accuracy in metres
// to the V5 wire variance in square metres.
func CfgTModeVarianceFromAccuracy(accuracyMeters float64) float32 {
	return float32(accuracyMeters * accuracyMeters)
}

// CfgTModeAccuracyFromVariance converts a V5 wire variance in square
// metres to a position accuracy in metres.
func CfgTModeAccuracyFromVariance(variance float32) float64 {
	return math.Sqrt(float64(variance))
}

// CfgNavBand is CFG-NAVBAND (0x06 0x0F) - V6 signal selection (12 bytes).
// Signal mask bit positions are defined in protocol section 1.4
// (GPS L1CA=0, GPS L5=2, SBAS L1=3, SBAS L5=4, GLO L1=5, GAL E1=7,
// GAL E5A=8, BDS B1I GEO=10, BDS B1I MEO=11, BDS B1C=14, BDS B2A=15,
// QZSS L1CA=19, QZSS L5=21, IRNSS L5=23).
type CfgNavBand struct {
	SigBandAuto  CfgNavBandSigBandAuto `json:"sigBandAuto"`
	Res1         uint8                 `json:"res1"`
	Res2         uint16                `json:"res2"`
	SigIDMaskFix CfgNavBandSigIDMask   `json:"sigidMaskFix"`
	SigIDMask    CfgNavBandSigIDMask   `json:"sigidMask"`
}

func (m *CfgNavBand) ID() MsgID { return CfgNavBandID }

// CfgNavBandSigBandAuto selects automatic or explicit signal bands.
type CfgNavBandSigBandAuto uint8

const (
	CfgNavBandManual CfgNavBandSigBandAuto = iota
	CfgNavBandAutomatic
)

// CfgNavBandSigIDMask selects V6 signals using the SigID bit positions
// defined in protocol section 1.4.
type CfgNavBandSigIDMask uint32

const (
	CfgNavBandSigGPSL1CA   CfgNavBandSigIDMask = 1 << SigGPSL1CA
	CfgNavBandSigGPSL5     CfgNavBandSigIDMask = 1 << SigGPSL5
	CfgNavBandSigSBASL1    CfgNavBandSigIDMask = 1 << SigSBASL1
	CfgNavBandSigSBASL5    CfgNavBandSigIDMask = 1 << SigSBASL5
	CfgNavBandSigGLOL1     CfgNavBandSigIDMask = 1 << SigGLOL1
	CfgNavBandSigGALE1     CfgNavBandSigIDMask = 1 << SigGALE1
	CfgNavBandSigGALE5a    CfgNavBandSigIDMask = 1 << SigGALE5a
	CfgNavBandSigBDSB1IGEO CfgNavBandSigIDMask = 1 << SigBDSB1IGEO
	CfgNavBandSigBDSB1IMEO CfgNavBandSigIDMask = 1 << SigBDSB1IMEO
	CfgNavBandSigBDSB1C    CfgNavBandSigIDMask = 1 << SigBDSB1C
	CfgNavBandSigBDSB2a    CfgNavBandSigIDMask = 1 << SigBDSB2a
	CfgNavBandSigQZSSL1CA  CfgNavBandSigIDMask = 1 << SigQZSSL1CA
	CfgNavBandSigQZSSL5    CfgNavBandSigIDMask = 1 << SigQZSSL5
	CfgNavBandSigNAVICL5   CfgNavBandSigIDMask = 1 << SigNAVICL5
)

// Has reports whether a signal is selected.
func (mask CfgNavBandSigIDMask) Has(signal CfgNavBandSigIDMask) bool {
	return mask&signal != 0
}

// CfgNmea is CFG-NMEA (0x06 0x12) - V6 NMEA output configuration (8 bytes).
type CfgNmea struct {
	NmeaVer       CfgNmeaVersion   `json:"nmeaVer"`
	LatLonReso    uint8            `json:"latLonReso"` // lat/lon decimal places
	HeightReso    uint8            `json:"heightReso"` // height decimal places
	GsaPlus       uint8            `json:"gsaPlus"`    // max GSA sentences per system
	NmeaValidOpen CfgNmeaValidOpen `json:"nmeaValidOpen"`
	Res           uint8            `json:"res"`
	Res2          uint16           `json:"res2"`
}

func (m *CfgNmea) ID() MsgID { return CfgNmeaID }

// CfgNmeaVersion selects the emitted NMEA version.
type CfgNmeaVersion uint8

const (
	CfgNmeaVersion2p2 CfgNmeaVersion = iota
	CfgNmeaVersion4p0
	CfgNmeaVersion4p10
	CfgNmeaVersion4p11
)

// CfgNmeaValidOpen controls output when PVT or heading is invalid.
type CfgNmeaValidOpen uint8

const (
	CfgNmeaOnlyValidPVT CfgNmeaValidOpen = 1 << iota
	CfgNmeaHeadingHold
)

// CfgNavLimit is CFG-NAVLIMIT (0x06 0x0A) - V6 satellite filtering
// (8 bytes).
type CfgNavLimit struct {
	MinSVs  uint8  `json:"minSVs"`
	MaxSVs  uint8  `json:"maxSVs"`
	MinCNO  uint8  `json:"minCNO"`
	MinElev int8   `json:"minEle"` // degrees
	Res     uint32 `json:"res"`
}

func (m *CfgNavLimit) ID() MsgID { return CfgNavLimID }

// CfgRtcm is CFG-RTCM (0x06 0x14) - V6 RTCM output configuration
// (16 bytes). RTCM output additionally requires the RTCM bit in the
// port's protocol mask.
type CfgRtcm struct {
	MsgEnable CfgRtcmMsgEnable `json:"rtcm_msg_en"`
	MsmVer    CfgRtcmMsmVer    `json:"rtcm_msm_ver"`
	Res       uint8            `json:"res"`
	Res2      uint16           `json:"res2"`
	Res3      uint32           `json:"res3"`
	Res4      uint32           `json:"res4"`
}

func (m *CfgRtcm) ID() MsgID { return CfgRtcmID }

// CfgRtcmMsgEnable selects RTCM messages for output.
type CfgRtcmMsgEnable uint32

const (
	CfgRtcmEnable1005    CfgRtcmMsgEnable = 1 << 0  // station ARP
	CfgRtcmEnableGPSEph  CfgRtcmMsgEnable = 1 << 2  // 1019
	CfgRtcmEnableBDSEph  CfgRtcmMsgEnable = 1 << 3  // 1042
	CfgRtcmEnableQZSSEph CfgRtcmMsgEnable = 1 << 4  // 1044
	CfgRtcmEnableGALFNav CfgRtcmMsgEnable = 1 << 5  // 1045
	CfgRtcmEnableGALINav CfgRtcmMsgEnable = 1 << 6  // 1046
	CfgRtcmEnableGPSMSM  CfgRtcmMsgEnable = 1 << 13 // 107x
	CfgRtcmEnableGALMSM  CfgRtcmMsgEnable = 1 << 17 // 109x
	CfgRtcmEnableQZSSMSM CfgRtcmMsgEnable = 1 << 19 // 111x
	CfgRtcmEnableBDSMSM  CfgRtcmMsgEnable = 1 << 21 // 112x
)

// CfgRtcmMsmVer selects the MSM output version.
type CfgRtcmMsmVer uint8

const (
	CfgRtcmMsm4 CfgRtcmMsmVer = iota + 4
	CfgRtcmMsm5
	CfgRtcmMsm6
	CfgRtcmMsm7
)

// CfgTMode2Mode selects the timing mode.
type CfgTMode2Mode uint8

const (
	CfgTMode2Realtime CfgTMode2Mode = iota // normal positioning
	CfgTMode2Survey                        // auto-survey
	CfgTMode2Fixed                         // fixed position
)

// CfgTMode2Band selects the signal band.
type CfgTMode2Band uint8

const (
	CfgTMode2BandL1B1I CfgTMode2Band = iota
	CfgTMode2BandL1
	CfgTMode2BandL2
	CfgTMode2BandL5
	CfgTMode2BandMulti
)

// CfgTMode2AntDetMode selects how antenna detection is performed.
type CfgTMode2AntDetMode uint8

const (
	CfgTMode2AntDetInternal CfgTMode2AntDetMode = iota
	CfgTMode2AntDetExternalPin
)

// CfgTMode2TSrcMode selects the GNSS used for timing. The priority
// modes fall back to another timing system when their preferred system
// is unavailable.
type CfgTMode2TSrcMode uint8

const (
	CfgTMode2TSrcForceGPS CfgTMode2TSrcMode = iota
	CfgTMode2TSrcForceBDS
	CfgTMode2TSrcForceGLN
	CfgTMode2TSrcForceGAL
	CfgTMode2TSrcPriorityBDS
	CfgTMode2TSrcPriorityGPS
	CfgTMode2TSrcPriorityGLN
	CfgTMode2TSrcPriorityGAL
)

const (
	// CfgTMode2FixedPositionScale is the number of wire units per metre.
	CfgTMode2FixedPositionScale float64 = 100
	// CfgTMode2PositionAccuracyScale is the number of wire units per metre.
	CfgTMode2PositionAccuracyScale float64 = 1000
)

// CfgTMode2 is CFG-TMODE2 (0x06 0x16) - timing mode configuration (28 bytes)
type CfgTMode2 struct {
	TimFixMode  CfgTMode2Mode       `json:"timFixMode"`  // 0=realtime, 1=survey, 2=fixed
	BandMode    CfgTMode2Band       `json:"bandMode"`    // signal band selection
	AntDetMode  CfgTMode2AntDetMode `json:"antDetMode"`  // antenna-detection source
	TSrcMode    CfgTMode2TSrcMode   `json:"tsrc_mode"`   // timing GNSS selection
	XFixed      int32               `json:"xFixed"`      // 0.01 m, ECEF X
	YFixed      int32               `json:"yFixed"`      // 0.01 m, ECEF Y
	ZFixed      int32               `json:"zFixed"`      // 0.01 m, ECEF Z
	FixedPacc   uint32              `json:"fixedPacc"`   // mm, position accuracy
	SvinMinDur  uint32              `json:"svinMinDur"`  // s, min survey-in duration
	SvinPaccLim uint32              `json:"svinPaccLim"` // mm, survey-in accuracy limit
}

func (m *CfgTMode2) ID() MsgID { return CfgTMode2ID }

// PollPayloadLen returns the maximum payload length that indicates
// a poll for mid.
func PollPayloadLen(mid MsgID) int {
	switch mid {
	case CfgMsgID:
		return 4 // class + ID + rate (0xFFFF)
	default:
		return 0
	}
}

func init() {
	regMsg[CfgMsg]("MSG")
	regMsg[CfgPrt]("PRT")
	regMsg[CfgRate]("RATE")
	regMsg[CfgCfg]("CFG")
	regMsg[CfgRst]("RST")
	regMsg[CfgTP]("TP")
	regMsg[CfgNavx]("NAVX")
	regMsg[CfgTMode]("TMODE")
	regMsg[CfgNavBand]("NAVBAND")
	regMsg[CfgNmea]("NMEA")
	regMsg[CfgNavLimit]("NAVLIMIT")
	regMsg[CfgRtcm]("RTCM")
	regMsg[CfgTMode2]("TMODE2")
}
