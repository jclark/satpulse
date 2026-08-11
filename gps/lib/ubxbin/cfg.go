package ubxbin

import (
	"fmt"
)

const (
	CfgCfgID    MsgID = clsCfg | (0x09 << 8)
	CfgGNSSID   MsgID = clsCfg | (0x3E << 8)
	CfgInfID    MsgID = clsCfg | (0x02 << 8)
	CfgMsgID    MsgID = clsCfg | (0x01 << 8)
	CfgNav5ID   MsgID = clsCfg | (0x24 << 8)
	CfgPrtID    MsgID = clsCfg | (0x00 << 8)
	CfgRateID   MsgID = clsCfg | (0x08 << 8)
	CfgRstID    MsgID = clsCfg | (0x04 << 8)
	CfgTmodeID  MsgID = clsCfg | (0x1D << 8)
	CfgTmode2ID MsgID = clsCfg | (0x3D << 8)
	CfgTmode3ID MsgID = clsCfg | (0x71 << 8)
	CfgTp5ID    MsgID = clsCfg | (0x31 << 8)
	CfgValdelID MsgID = clsCfg | (0x8C << 8)
	CfgValgetID MsgID = clsCfg | (0x8B << 8)
	CfgValsetID MsgID = clsCfg | (0x8A << 8)
)

type CfgCfg struct {
	CfgCfgFixed
	DeviceMask []CfgCfgDeviceMask `json:"deviceMask"` // this is optional so we use a slice; it will always be length 0 or 1
}

var _ VaryingMsg = (*CfgCfg)(nil)

func (m *CfgCfg) ID() MsgID { return CfgCfgID }

func (m *CfgCfg) InitVaryingPart(payloadLen int) error {
	// The implementation of InitVaryingPart is different for CfgCfg from others
	// because with CfgCfg the variable part is at most 1 byte long.
	if payloadLen == 13 {
		m.DeviceMask = make([]CfgCfgDeviceMask, 1)
	} else if payloadLen != 12 {
		// use similar message to sliceLen
		return fmt.Errorf("bad UBX-CFG-CFG payload length (%d bytes); expected 12 or 13", payloadLen)
	}
	return nil
}

func (m *CfgCfg) FixedPart() any {
	return &m.CfgCfgFixed
}

func (m *CfgCfg) VaryingPart() any {
	return &m.DeviceMask
}

type CfgCfgFixed struct {
	ClearMask CfgCfgSectionMask `json:"clearMask"`
	SaveMask  CfgCfgSectionMask `json:"saveMask"`
	LoadMask  CfgCfgSectionMask `json:"loadMask"`
}

type CfgCfgSectionMask uint32

const (
	CfgCfgIOPort CfgCfgSectionMask = 1 << iota
	CfgCfgMsgConf
	CfgCfgInfMsg
	CfgCfgNavConf
	CfgCfgRXMConf
	_
	_
	_
	CfgCfgSenConf
	CfgCfgRinvConf
	CfgCfgAntConf
	CfgCfgLogConf
	CfgCfgFtsConf
)

const CfgCfgSectionMaskAll CfgCfgSectionMask = 0x1F1F

type CfgCfgDeviceMask uint8

const (
	CfgCfgDevBBR CfgCfgDeviceMask = 1 << iota
	CfgCfgDevFlash
	CfgCfgDevEEPROM
	_
	CfgCfgDevSpiFlash
)

type CfgMsg struct {
	MsgID MsgID       `json:"msgId"`
	Rate  [NPort]byte `json:"rate"`
}

func (m *CfgMsg) ID() MsgID { return CfgMsgID }

// UBX-CFG-INF Information message configuration.
// The payload is entirely repeated blocks, one per protocol, so there is
// no fixed part. Messages output by the receiver contain a single block.

type CfgInf struct {
	Blocks []CfgInfBlock `json:"blocks"`
}

var _ VaryingMsg = (*CfgInf)(nil)

func (m *CfgInf) ID() MsgID { return CfgInfID }

func (m *CfgInf) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 0, 10)
	if err == nil {
		m.Blocks = make([]CfgInfBlock, len)
	}
	return
}

func (m *CfgInf) FixedPart() any {
	return nil
}

func (m *CfgInf) VaryingPart() any {
	return &m.Blocks
}

type CfgInfBlock struct {
	ProtocolID CfgInfProtocolID  `json:"protocolId"`
	_          [3]byte
	InfMsgMask [NPort]CfgInfMask `json:"infMsgMask"`
}

type CfgInfProtocolID byte

const (
	CfgInfProtoUBX CfgInfProtocolID = iota
	CfgInfProtoNMEA
)

type CfgInfMask byte

const (
	CfgInfError CfgInfMask = 1 << iota
	CfgInfWarning
	CfgInfNotice
	CfgInfTest
	CfgInfDebug
)

// UBX-CFG-NAV5 Navigation engine settings

type CfgNav5 struct {
	Mask              CfgNav5Mask        `json:"mask"`
	DynModel          CfgNav5DynModel    `json:"dynModel"`
	FixMode           byte               `json:"fixMode"`
	FixedAlt          int32              `json:"fixedAlt"`
	FixedAltVar       uint32             `json:"fixedAltVar"`
	MinElev           int8               `json:"minElev"`
	DrLimit           byte               `json:"drLimit"`
	PDop              uint16             `json:"pDop"`
	TDop              uint16             `json:"tDop"`
	PAcc              uint16             `json:"pAcc"`
	TAcc              uint16             `json:"tAcc"`
	StaticHoldThresh  byte               `json:"staticHoldThresh"`
	DgnssTimeout      byte               `json:"dgnssTimeout"`
	CnoThreshNumSvs   byte               `json:"cnoThreshNumSVs"`
	CnoThresh         byte               `json:"cnoThresh"`
	_                 [2]byte
	StaticHoldMaxDist uint16             `json:"staticHoldMaxDist"`
	UtcStandard       CfgNav5UtcStandard `json:"utcStandard"`
	_                 [5]byte
}

func (m *CfgNav5) ID() MsgID { return CfgNav5ID }

type CfgNav5Mask uint16

const (
	CfgNav5MaskDyn CfgNav5Mask = 1 << iota
	CfgNav5MaskMinElev
	CfgNav5MaskPosFixMode
	CfgNav5MaskDrLim
	CfgNav5MaskPosMask
	CfgNav5MaskTimeMask
	CfgNav5MaskStaticHoldMask
	CfgNav5MaskDgpsMask
	CfgNav5MaskCnoThresh
	_
	CfgNav5MaskUtc
)

type CfgNav5DynModel byte

const (
	CfgNav5DynPortable CfgNav5DynModel = iota
	_
	CfgNav5DynStationary
	CfgNav5DynPedestrian
	CfgNav5DynAutomotive
	CfgNav5DynSea
	CfgNav5DynAirborne1g
	CfgNav5DynAirborne2g
	CfgNav5DynAirborne4g
	CfgNav5DynWristwatch
	CfgNav5DynMotorbike
	CfgNav5DynLawnmower
)

type CfgNav5UtcStandard byte

const (
	CfgNav5UtcAuto CfgNav5UtcStandard = 0
	CfgNav5UtcUSNO CfgNav5UtcStandard = 3
	CfgNav5UtcEU   CfgNav5UtcStandard = 5
	CfgNav5UtcSU   CfgNav5UtcStandard = 6
	CfgNav5UtcNTSC CfgNav5UtcStandard = 7
	CfgNav5UtcNPLI CfgNav5UtcStandard = 8
)

type CfgPrt struct {
	PortID       PortID          `json:"portID"`
	_            byte
	TxReady      CfgPrtTxReady   `json:"txReady"`
	Mode         CfgPrtMode      `json:"mode"`
	BaudRate     uint32          `json:"baudRate"`
	InProtoMask  CfgPrtProtoMask `json:"inProtoMask"`
	OutProtoMask CfgPrtProtoMask `json:"outProtoMask"`
	Flags        CfgPrtFlags     `json:"flags"`
	_            [2]byte
}

func (m *CfgPrt) ID() MsgID { return CfgPrtID }

type CfgPrtTxReady uint16

const (
	CfgPrtTxReadyEn     CfgPrtTxReady = 1
	CfgPrtTxReadyPol    CfgPrtTxReady = 2
	CfgPrtTxReadyPin    CfgPrtTxReady = 0b11111 << 2
	CfgPrtTxReadyThresh CfgPrtTxReady = 0x1F << 7
)

type CfgPrtMode uint32

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
	CfgPrtModeParityStop    CfgPrtMode = 0b11 << 12
	CfgPrtModeParityStop1   CfgPrtMode = 0
	CfgPrtModeParityStop15  CfgPrtMode = 0b01 << 12
	CfgPrtModeParityStop2   CfgPrtMode = 0b10 << 12
	CfgPrtModeParityStop05  CfgPrtMode = 0b11 << 12
)

type CfgPrtProtoMask uint16

const (
	CfgPrtProtoUBX CfgPrtProtoMask = 1 << iota
	CfgPrtProtoNMEA
	CfgPrtProtoRTCM2
	_
	_
	CfgPrtProtoRTCM3
)

type CfgPrtFlags uint16

const (
	CfgPrtFlagsExtendedTxTimeout CfgPrtFlags = 2
)

type CfgRate struct {
	MeasRate uint16         `json:"measRate"`
	NavRate  uint16         `json:"navRate"`
	TimeRef  CfgRateTimeRef `json:"timeRef"`
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

type CfgRst struct {
	NavBbrMask CfgRstNavBbrMask `json:"navBbrMask"`
	ResetMode  CfgRstResetMode  `json:"resetMode"`
	_          byte
}

func (m *CfgRst) ID() MsgID { return CfgRstID }

type CfgRstNavBbrMask uint16

const (
	CfgRstNavBbrEph CfgRstNavBbrMask = 1 << iota
	CfgRstNavBbrAlm
	CfgRstNavBbrHealth
	CfgRstNavBbrKlob
	CfgRstNavBbrPos
	CfgRstNavBbrClkd
	CfgRstNavBbrOsc
	CfgRstNavBbrUtc
	CfgRstNavBbrRtc
	_
	_
	CfgRstNavBbrSfdr
	CfgRstNavBbrVmon
	CfgRstNavBbrTct
	_
	CfgRstNavBbrAop
	CfgRstNavBbrHotStart  CfgRstNavBbrMask = 0x0000
	CfgRstNavBbrWarmStart CfgRstNavBbrMask = 0x0001
	CfgRstNavBbrColdStart CfgRstNavBbrMask = 0xFFFF
)

type CfgRstResetMode byte

const (
	CfgRstResetModeHardwareResetImmediately CfgRstResetMode = iota
	CfgRstResetModeControlledSoftwareReset
	CfgRstResetModeControlledSoftwareResetGnssOnly
	_
	CfgRstResetModeHardwareResetAfterShutdown
	_
	_
	_
	CfgRstResetModeControlledGnssStop
	CfgRstResetModeControlledGnssStart
)

type CfgTp5 struct {
	TpIdx             byte        `json:"tpIdx"`
	Version           byte        `json:"version"`
	_                 [2]byte
	AntCableDelay     int16       `json:"antCableDelay"`
	RfGroupDelay      int16       `json:"rfGroupDelay"`
	FreqPeriod        uint32      `json:"freqPeriod"`
	FreqPeriodLock    uint32      `json:"freqPeriodLock"`
	PulseLenRatio     uint32      `json:"pulseLenRatio"`
	PulseLenRatioLock uint32      `json:"pulseLenRatioLock"`
	UserConfigDelay   int32       `json:"userConfigDelay"`
	Flags             CfgTp5Flags `json:"flags"`
}

var _ PartiallyHandledMsg = (*CfgTp5)(nil)

func (m *CfgTp5) ID() MsgID { return CfgTp5ID }

func (m *CfgTp5) IsHandled() bool {
	// Support both version 0 (protocol 15) and version 1 (protocols 16-23)
	// Version 1 is a compatible upgrade to version 0
	return m.Version == 0 || m.Version == 1
}

type CfgTp5Flags uint32

const (
	CfgTp5Active CfgTp5Flags = 1 << iota
	CfgTp5LockGpsFreq
	CfgTp5LockedOtherSet
	CfgTp5IsFreq
	CfgTp5IsLength
	CfgTp5AlignToTow
	CfgTp5Polarity
)

const (
	CfgTp5GridUTC CfgTp5Flags = iota << 7
	CfgTp5GridGPS
	CfgTp5GridGLONASS
	CfgTp5GridBeiDou
	CfgTp5GridGalileo
	CfgTp5GridUTCGNSS CfgTp5Flags = 0b1111 << 7
)

type CfgTmodeTimeMode uint32

const (
	CfgTmodeDisabled CfgTmodeTimeMode = iota
	CfgTmodeSurveyIn
	CfgTmodeFixedMode
)

type CfgTmode struct {
	TimeMode     CfgTmodeTimeMode `json:"timeMode"`
	FixedPosX    int32            `json:"fixedPosX"`
	FixedPosY    int32            `json:"fixedPosY"`
	FixedPosZ    int32            `json:"fixedPosZ"`
	FixedPosVar  uint32           `json:"fixedPosVar"` // mm^2
	SvinMinDur   uint32           `json:"svinMinDur"`
	SvinVarLimit uint32           `json:"svinVarLimit"` // mm^2
}

func (m *CfgTmode) ID() MsgID { return CfgTmodeID }

type CfgTmode2 struct {
	TimeMode     CfgTmode2TimeMode `json:"timeMode"`
	_            byte
	Flags        CfgTmode2Flags    `json:"flags"`
	EcefXOrLat   int32             `json:"ecefXOrLat"`
	EcefYOrLon   int32             `json:"ecefYOrLon"`
	EcefZOrAlt   int32             `json:"ecefZOrAlt"`
	FixedPosAcc  uint32            `json:"fixedPosAcc"`
	SvinMinDur   uint32            `json:"svinMinDur"`
	SvinAccLimit uint32            `json:"svinAccLimit"`
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
	Version      byte           `json:"version"`
	_            byte
	Flags        CfgTmode3Flags `json:"flags"`
	EcefXOrLat   int32          `json:"ecefXOrLat"`
	EcefYOrLon   int32          `json:"ecefYOrLon"`
	EcefZOrAlt   int32          `json:"ecefZOrAlt"`
	EcefXOrLatHP int8           `json:"ecefXOrLatHP"`
	EcefYOrLonHP int8           `json:"ecefYOrLonHP"`
	EcefZOrAltHP int8           `json:"ecefZOrAltHP"`
	_            byte
	FixedPosAcc  uint32         `json:"fixedPosAcc"`
	SvinMinDur   uint32         `json:"svinMinDur"`
	SvinAccLimit uint32         `json:"svinAccLimit"`
	_            [8]byte
}

var _ PartiallyHandledMsg = (*CfgTmode3)(nil)

func (m *CfgTmode3) ID() MsgID { return CfgTmode3ID }

func (m *CfgTmode3) IsHandled() bool {
	return m.Version == 0
}

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

type CfgGNSS struct {
	CfgGNSSFixed
	Blocks []CfgGNSSBlock `json:"blocks"`
}

type CfgGNSSFixed struct {
	MsgVer          byte `json:"msgVer"`
	NumTrkChHw      byte `json:"numTrkChHw"`
	NumTrkChUse     byte `json:"numTrkChUse"`
	NumConfigBlocks byte `json:"numConfigBlocks"`
}

type CfgGNSSSigMask byte

type CfgGNSSBlock struct {
	GNSSID     GNSSID         `json:"gnssId"`
	ResTrkCh   byte           `json:"resTrkCh"`
	MaxTrkCh   byte           `json:"maxTrkCh"`
	_          byte
	Enable     byte           `json:"enable"`
	_          byte
	SigCfgMask CfgGNSSSigMask `json:"sigCfgMask"`
	_          byte
}

const (
	CfgGNSSGPSL1CA CfgGNSSSigMask = 0x01
	CfgGNSSGPSL2C  CfgGNSSSigMask = 0x10
	CfgGNSSGPSL5   CfgGNSSSigMask = 0x20
)

const CfgGNSSSBASL1CA CfgGNSSSigMask = 0x01

const (
	CfgGNSSGALE1  CfgGNSSSigMask = 0x01
	CfgGNSSGALE5a CfgGNSSSigMask = 0x10
	CfgGNSSGALE5b CfgGNSSSigMask = 0x20
)

const (
	CfgGNSSBDSB1I CfgGNSSSigMask = 0x01
	CfgGNSSBDSB2I CfgGNSSSigMask = 0x10
	CfgGNSSBDSB2A CfgGNSSSigMask = 0x80
)

const (
	CfgGNSSQZSSL1CA CfgGNSSSigMask = 0x01
	CfgGNSSQZSSL1S  CfgGNSSSigMask = 0x04
	CfgGNSSQZSSL2C  CfgGNSSSigMask = 0x10
	CfgGNSSQZSSL5   CfgGNSSSigMask = 0x20
)

const (
	CfgGNSSGLOL1 CfgGNSSSigMask = 0x01
	CfgGNSSGLOL2 CfgGNSSSigMask = 0x10
)

func (m *CfgGNSS) ID() MsgID { return CfgGNSSID }

func (m *CfgGNSS) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 8)
	if err == nil {
		m.Blocks = make([]CfgGNSSBlock, len)
	}
	return
}

func (m *CfgGNSS) FixedPart() any {
	return &m.CfgGNSSFixed
}

func (m *CfgGNSS) VaryingPart() any {
	return &m.Blocks
}

type CfgValget struct {
	CfgValgetFixed
	CfgData []byte `json:"cfgData"`
}

func (m *CfgValget) ID() MsgID { return CfgValgetID }

type CfgValgetFixed struct {
	Version  CfgValgetVersion `json:"version"`
	Layer    CfgValgetLayer   `json:"layer"`
	Position uint16           `json:"position"`
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

func (m *CfgValget) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 1)
	if err == nil {
		m.CfgData = make([]byte, len)
	}
	return
}

func (m *CfgValget) FixedPart() any {
	return &m.CfgValgetFixed
}

func (m *CfgValget) VaryingPart() any {
	return &m.CfgData
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
	CfgData []byte `json:"cfgData"`
}

func (m *CfgValset) ID() MsgID { return CfgValsetID }

type CfgValsetFixed struct {
	Version     CfgValsetVersion  `json:"version"`
	Layers      CfgValsetLayer    `json:"layers"`
	Transaction CfgValTransaction `json:"transaction"`
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

func (m *CfgValset) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 1)
	if err == nil {
		m.CfgData = make([]byte, len)
	}
	return
}

func (m *CfgValset) FixedPart() any {
	return &m.CfgValsetFixed
}

func (m *CfgValset) VaryingPart() any {
	return &m.CfgData
}

type CfgValdel struct {
	CfgValdelFixed
	CfgData []byte `json:"keys"`
}

func (m *CfgValdel) ID() MsgID { return CfgValdelID }

type CfgValdelFixed struct {
	Version     CfgValdelVersion  `json:"version"`
	Layers      CfgValdelLayer    `json:"layers"`
	Transaction CfgValTransaction `json:"transaction"`
	_           byte
}

type CfgValdelVersion byte

const (
	CfgValdelVersionNoTransaction CfgValdelVersion = iota
	CfgValdelVersionTransaction
)

type CfgValdelLayer byte

const (
	CfgValdelLayerBBR   CfgValdelLayer = 0x02
	CfgValdelLayerFlash CfgValdelLayer = 0x04
)

func (m *CfgValdel) InitVaryingPart(payloadLen int) (err error) {
	len, err := sliceLen(m, payloadLen, 4, 1)
	if err == nil {
		m.CfgData = make([]byte, len)
	}
	return
}

func (m *CfgValdel) FixedPart() any {
	return &m.CfgValdelFixed
}

func (m *CfgValdel) VaryingPart() any {
	return &m.CfgData
}

// PollPayloadLen returns the maximum payload length that indicates
// a poll for mid.
func PollPayloadLen(mid MsgID) int {
	switch mid {
	case CfgPrtID:
		return 1 // port ID
	case CfgInfID:
		return 1 // protocol ID
	case CfgMsgID:
		return 2 // class + ID
	case CfgTp5ID:
		return 1 // timepulse index
	default:
		return 0
	}
}

func PollCfgTp5(tpIdx int) []byte {
	packet, _ := PackMsg(CfgTp5ID, []byte{byte(tpIdx)})
	return packet
}

func SetCfgMsg(mid MsgID, rate byte) []byte {
	cls, id := mid.Unpack()
	packet, _ := PackMsg(CfgMsgID, []byte{cls, id, rate})
	return packet
}

func PollCfgMsg(mid MsgID) []byte {
	cls, id := mid.Unpack()
	packet, _ := PackMsg(CfgMsgID, []byte{cls, id})
	return packet
}

func init() {
	regMsg[CfgCfg]("CFG")
	regMsg[CfgGNSS]("GNSS")
	regMsg[CfgInf]("INF")
	regMsg[CfgMsg]("MSG")
	regMsg[CfgNav5]("NAV5")
	regMsg[CfgPrt]("PRT")
	regMsg[CfgRate]("RATE")
	regMsg[CfgRst]("RST")
	regMsg[CfgTmode]("TMODE")
	regMsg[CfgTmode2]("TMODE2")
	regMsg[CfgTmode3]("TMODE3")
	regMsg[CfgTp5]("TP5")
	regMsg[CfgValget]("VALGET")
	regMsg[CfgValset]("VALSET")
	regMsg[CfgValdel]("VALDEL")
}
