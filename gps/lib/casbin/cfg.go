package casbin

// CfgMsg is CFG-MSG (0x06 0x01) - message rate configuration (4 bytes).
// Rate 0xFFFF polls the target message instead of setting its rate.
// An empty-payload CFG-MSG query returns one CfgMsg response per known
// message (the whole rate table).
type CfgMsg struct {
	Target MsgID  // message whose rate is configured (cls byte, then id byte)
	Rate   uint16 // output rate in fixes per message (0=off)
}

func (m *CfgMsg) ID() MsgID { return CfgMsgID }

// PollRate in CfgMsg.Rate makes the request a poll of the target message.
const PollRate = 0xFFFF

// CfgPrt is CFG-PRT (0x06 0x00) - serial port configuration (8 bytes).
// An empty-payload query returns one CfgPrt response per UART.
type CfgPrt struct {
	PortID    uint8  // 0=UART0, 1=UART1, 0xFF=port in use
	ProtoMask uint8  // see PrtProto* bits
	Mode      uint16 // data bits/parity/stop encoding (8N1 = 0x08C0 region)
	BaudRate  uint32 // bits per second
}

func (m *CfgPrt) ID() MsgID { return CfgPrtID }

// CfgPrt.PortID value for the port the request arrives on.
const PortCurrent = 0xFF

// CfgPrt.ProtoMask bits.
const (
	PrtProtoBinaryIn  = 0x01
	PrtProtoTextIn    = 0x02
	PrtProtoBinaryOut = 0x10
	PrtProtoTextOut   = 0x20
	PrtProtoRTCMOut   = 0x80 // V6 only
)

// CfgRate is CFG-RATE (0x06 0x04) - navigation rate (4 bytes).
// V5 firmware fills only FixIntervalMs and leaves the rest zero.
type CfgRate struct {
	FixIntervalMs uint16 // fix interval, ms (1000=1Hz)
	FixRateHz     uint8  // fix rate, Hz (V6 only)
	Res           uint8
}

func (m *CfgRate) ID() MsgID { return CfgRateID }

// CfgCfg is CFG-CFG (0x06 0x05) - clear/save/load configuration (4 bytes).
// V5 firmware honours the section mask; V6 documents the field as reserved.
type CfgCfg struct {
	Mask   uint16 // sections affected, see CfgSection* bits
	OpMode uint8  // see CfgOp* values
	Res    uint8
}

func (m *CfgCfg) ID() MsgID { return CfgCfgID }

// CfgCfg.OpMode values.
const (
	CfgOpClear = 0 // reset sections to defaults
	CfgOpSave  = 1 // save running config to NVM
	CfgOpLoad  = 2 // load config from NVM
)

// CfgCfg.Mask section bits (V5 save granularity).
const (
	CfgSectionPort  = 0x0001 // CFG-PRT
	CfgSectionMsg   = 0x0002 // CFG-MSG rates
	CfgSectionInf   = 0x0004 // CFG-INF
	CfgSectionNav   = 0x0008 // CFG-RATE, CFG-TMODE; covers CFG-NAVX too (verified on the AT6558D: a save with only this bit persists a NAVX min-elev change across reload)
	CfgSectionTP    = 0x0010 // CFG-TP
	CfgSectionGroup = 0x0020 // CFG-GROUP
	CfgSectionAll   = 0xFFFF
)

// CfgRst is CFG-RST (0x06 0x02) - receiver restart (4 bytes). The
// fields are the V5 layout (casic2.md); zkw3.md 3.13.3 documents the
// V6 payload differently - bytes 0-2 reserved, byte 3 the start mode -
// but the encodings coincide: StartMode occupies byte 3 with the same
// hot/warm/cold/factory values, and a V6 request zeroes the rest, so
// serializing this struct is wire-correct for both families.
// The receiver does not acknowledge CFG-RST before restarting.
type CfgRst struct {
	NavBbrMask uint16 // BBR sections to clear, see Bbr* bits (V5)
	ResetMode  uint8  // see Reset* values (V5)
	StartMode  uint8  // see Start* values
}

func (m *CfgRst) ID() MsgID { return CfgRstID }

// CfgRst.NavBbrMask bits.
const (
	BbrEphemeris  = 0x0001
	BbrAlmanac    = 0x0002
	BbrHealth     = 0x0004
	BbrIonosphere = 0x0008
	BbrPosition   = 0x0010
	BbrClockDrift = 0x0020
	BbrOscParams  = 0x0040
	BbrUTCParams  = 0x0080
	BbrRTC        = 0x0100
	BbrConfig     = 0x0200
)

// CfgRst.ResetMode values.
const (
	ResetHWImmediate    = 0
	ResetSWControlled   = 1
	ResetSWGPSOnly      = 2
	ResetHWHardShutdown = 4
)

// CfgRst.StartMode values.
const (
	StartHot     = 0
	StartWarm    = 1
	StartCold    = 2
	StartFactory = 3
)

// CfgTP is CFG-TP (0x06 0x03) - time pulse configuration (16 bytes).
// Enum semantics differ between firmware families:
// PPSOutMode: V6 0=off, 1=time known, 2=sat sync, 3=pos+time valid,
// 5=reliable, 7=always on; V5 0=off, 1=on, 2=maintain after fix lost,
// 3=fix only. TBase: V6 0=GNSS, 1=UTC; V5 inverted: 0=UTC, 1=satellite.
// TSrcMode: V6 0-3=force GPS/BDS/GLN/GAL, 4-8=primary, 9=auto;
// V5 0=GPS, 1=BDS, 2=GLN, 4=BDS-main, 5=GPS-main, 6=GLN-main.
type CfgTP struct {
	Interval   uint32  // pulse period, us
	Width      uint32  // pulse width, us
	PPSOutMode uint8   // when the pulse is output (see above)
	Polarity   uint8   // 0=rising edge aligned, 1=falling edge
	TBase      uint8   // time base (see above)
	TSrcMode   uint8   // time source GNSS (see above)
	UserDelay  float32 // user time delay, s
}

func (m *CfgTP) ID() MsgID { return CfgTPID }

// CfgNavx is CFG-NAVX (0x06 0x07) - V5 navigation engine configuration
// (44 bytes). On set, only fields with their Mask bit are applied; the
// receiver ignores the rest, so no read-modify-write is needed. On query
// responses Mask is 0 and all fields hold current values.
type CfgNavx struct {
	Mask         uint32 // fields to apply, see Navx* bits
	DynModel     uint8  // see DynModel* values
	FixMode      uint8  // 1=2D, 2=3D, 3=auto
	MinSVs       uint8
	MaxSVs       uint8
	MinCNO       uint8
	Res1         uint8
	IniFix3D     uint8
	MinElev      int8 // degrees
	DrLimit      uint8
	NavSystem    uint8  // constellations, see NavSys* bits
	WnRollOver   uint16 // GPS week rollover reference
	FixedAlt     float32
	FixedAltVar  float32
	PDop         float32
	TDop         float32
	PAcc         float32
	TAcc         float32
	StaticHoldTh float32
}

func (m *CfgNavx) ID() MsgID { return CfgNavxID }

// CfgNavx.Mask bits.
const (
	NavxDynModel   = 0x0001
	NavxFixMode    = 0x0002
	NavxSVCount    = 0x0004
	NavxMinCNO     = 0x0008
	NavxIniFix3D   = 0x0020
	NavxMinElev    = 0x0040
	NavxDrLimit    = 0x0080
	NavxNavSystem  = 0x0100
	NavxWnRollOver = 0x0200
	NavxAltAssist  = 0x0400
	NavxPDop       = 0x0800
	NavxTDop       = 0x1000
	NavxStaticHold = 0x2000
)

// CfgNavx.NavSystem bits.
const (
	NavSysGPS = 0x01
	NavSysBDS = 0x02
	NavSysGLN = 0x04
)

// CfgNavx.DynModel values.
const (
	DynModelPortable   = 0
	DynModelStationary = 1
	DynModelPedestrian = 2
	DynModelAutomotive = 3
	DynModelMarine     = 4
	DynModelAirborne1G = 5
	DynModelAirborne2G = 6
	DynModelAirborne4G = 7
)

// CfgTMode is CFG-TMODE (0x06 0x06) - V5 timing mode configuration
// (40 bytes). The mode field is documented as U4 but the receiver
// returns garbage in the upper two bytes, so it is parsed as U2 + U2
// reserved (set Res to 0 when building).
type CfgTMode struct {
	Mode         uint16 // 0=auto, 1=survey-in, 2=fixed position
	Res          uint16
	EcefX        float64 // m
	EcefY        float64 // m
	EcefZ        float64 // m
	PosVar       float32 // m^2, fixed position variance
	SvinMinDur   uint32  // s, min survey-in duration
	SvinVarLimit float32 // m^2, survey-in variance limit
}

func (m *CfgTMode) ID() MsgID { return CfgTModeID }

// CfgTMode.Mode values (shared with CfgTMode2.TimFixMode).
const (
	TModeAuto   = 0
	TModeSurvey = 1
	TModeFixed  = 2
)

// CfgNavBand is CFG-NAVBAND (0x06 0x0F) - V6 signal selection (12 bytes).
// Signal mask bit positions are defined in protocol section 1.4
// (GPS L1CA=0, GPS L5=2, SBAS L1=3, SBAS L5=4, GLO L1=5, GAL E1=7,
// GAL E5A=8, BDS B1I GEO=10, BDS B1I MEO=11, BDS B1C=14, BDS B2A=15,
// QZSS L1CA=19, QZSS L5=21, IRNSS L5=23).
type CfgNavBand struct {
	SigBandAuto  uint8 // 1=auto signal band selection, 0=use masks
	Res1         uint8
	Res2         uint16
	SigIDMaskFix uint32 // signals used for positioning (when auto=0)
	SigIDMask    uint32 // signals received
}

func (m *CfgNavBand) ID() MsgID { return CfgNavBandID }

// CfgNmea is CFG-NMEA (0x06 0x12) - V6 NMEA output configuration (8 bytes).
type CfgNmea struct {
	NmeaVer       uint8 // 0=V2.2, 1=V4.0, 2=V4.10, 3=V4.11
	LatLonReso    uint8 // lat/lon decimal places
	HeightReso    uint8 // height decimal places
	GsaPlus       uint8
	NmeaValidOpen uint8
	Res           uint8
	Res2          uint16
}

func (m *CfgNmea) ID() MsgID { return CfgNmeaID }

// CfgNavLimit is CFG-NAVLIMIT (0x06 0x0A) - V6 satellite filtering
// (8 bytes).
type CfgNavLimit struct {
	MinSVs  uint8
	MaxSVs  uint8
	MinCNO  uint8
	MinElev int8 // degrees
	Res     uint32
}

func (m *CfgNavLimit) ID() MsgID { return CfgNavLimID }

// CfgRtcm is CFG-RTCM (0x06 0x14) - V6 RTCM output configuration
// (16 bytes). RTCM output additionally requires the RTCM bit in the
// port's protocol mask.
type CfgRtcm struct {
	MsgEnable uint32 // see RtcmEn* bits
	MsmVer    uint8  // MSM version: 4, 5, 6 or 7
	Res       uint8
	Res2      uint16
	Res3      uint32
	Res4      uint32
}

func (m *CfgRtcm) ID() MsgID { return CfgRtcmID }

// CfgRtcm.MsgEnable bits.
const (
	RtcmEn1005    = 1 << 0  // station ARP
	RtcmEnGPSEph  = 1 << 2  // 1019
	RtcmEnBDSEph  = 1 << 3  // 1042
	RtcmEnQZSSEph = 1 << 4  // 1044
	RtcmEnGALFNav = 1 << 5  // 1045
	RtcmEnGALINav = 1 << 6  // 1046
	RtcmEnGPSMSM  = 1 << 13 // 107x
	RtcmEnGALMSM  = 1 << 17 // 109x
	RtcmEnQZSSMSM = 1 << 19 // 111x
	RtcmEnBDSMSM  = 1 << 21 // 112x
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

// CfgTMode2 is CFG-TMODE2 (0x06 0x16) - timing mode configuration (28 bytes)
type CfgTMode2 struct {
	TimFixMode  CfgTMode2Mode // 0=realtime, 1=survey, 2=fixed
	BandMode    CfgTMode2Band // signal band selection
	AntDetMode  uint8         // 0=internal, 1=external pin
	TSrcMode    uint8         // 0-3=force single sys, 4-7=priority sys
	XFixed      int32         // 0.01 m, ECEF X
	YFixed      int32         // 0.01 m, ECEF Y
	ZFixed      int32         // 0.01 m, ECEF Z
	FixedPacc   uint32        // mm, position accuracy
	SvinMinDur  uint32        // s, min survey-in duration
	SvinPaccLim uint32        // mm, survey-in accuracy limit
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
