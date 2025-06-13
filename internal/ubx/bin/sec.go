package bin

const (
	SecOsnmaID MsgID = clsSec | (0x0a << 8)
)

type SecOsnma struct {
	SecOsnmaFixed
	AuthSVs []SecOsnmaAuthSV
}

type SecOsnmaFixed struct {
	Version           byte
	NmaHeader         SecOsnmaNmaHeader
	OsnmaMonitoring   SecOsnmaMonitoring
	TimSyncReq        SecOsnmaTimSyncReq
	_                 [3]byte
	TimSyncReqDiff    int32
	_                 [4]byte
	DsmAuthentication SecOsnmaDsmAuth
	TeslaKey          SecOsnmaTeslaKey
	GeneralAndTiming  SecOsnmaGeneral
}

type SecOsnmaAuthSV struct {
	Bitfield1 SecOsnmaAuthSVBitfield
	SvId      byte
	Reserved2 byte
}

type SecOsnmaNmaHeader byte

const (
	SecOsnmaHeaderAuthStatus       SecOsnmaNmaHeader = 1          // bit 0
	SecOsnmaHeaderNmaStatusMask    SecOsnmaNmaHeader = 0b11 << 1  // bits 2...1
	SecOsnmaHeaderChainInForceMask SecOsnmaNmaHeader = 0b11 << 3  // bits 4...3
	SecOsnmaHeaderCPKSMask         SecOsnmaNmaHeader = 0b111 << 5 // bits 7...5
)

const (
	SecOsnmaNmaStatusNotAuth SecOsnmaNmaHeader = iota << 1
	SecOsnmaNmaStatusTest
	SecOsnmaNmaStatusOperational
	SecOsnmaNmaStatusInvalid
)

const (
	SecOsnmaCPKSNotApplicable SecOsnmaNmaHeader = iota << 5
	SecOsnmaCPKSNominal
	SecOsnmaCPKSEOC
	SecOsnmaCPKSCREV
	SecOsnmaCPKSNPK
	SecOsnmaCPKSPKREV
	SecOsnmaCPKSNMT
	SecOsnmaCPKSAM
)

type SecOsnmaMonitoring uint16

const (
	SecOsnmaMonitoringOsnmaEnabled        SecOsnmaMonitoring = 1 << 0        // bit 0
	SecOsnmaMonitoringNumberSVsMask       SecOsnmaMonitoring = 0b1_1111 << 1 // bits 5...1
	SecOsnmaMonitoringNmaHeaderUpdateMask SecOsnmaMonitoring = 0b11 << 6     // bits 7...6
	SecOsnmaMonitoringNoData              SecOsnmaMonitoring = 1 << 8
	SecOsnmaMonitoringWrongData           SecOsnmaMonitoring = 1 << 9
	SecOsnmaMonitoringWrongFlxMac         SecOsnmaMonitoring = 1 << 10
	SecOsnmaMonitoringWrongMaclt          SecOsnmaMonitoring = 1 << 11
)

const (
	SecOsnmaNmaHeaderUpdateSame SecOsnmaMonitoring = iota << 6
	SecOsnmaNmaHeaderUpdateHealthy
	SecOsnmaNmaHeaderUpdateProblem
)

type SecOsnmaTimSyncReq byte

const (
	SecOsnmaTimSyncEnabled    SecOsnmaTimSyncReq = 1
	SecOsnmaTimSyncStatusMask SecOsnmaTimSyncReq = 0b111 << 1
)

const (
	SecOsnmaTimSyncStatusNotPerformed SecOsnmaTimSyncReq = iota << 1
	SecOsnmaTimSyncStatusNoTrustedTime
	SecOsnmaTimSyncStatusNotAccurate
	SecOsnmaTimSyncStatusPassed
	SecOsnmaTimSyncStatusFailed
)

type SecOsnmaDsmAuth uint32

const (
	SecOsnmaDsmAuthStatusMask         SecOsnmaDsmAuth = 0b11_1111    // bits 5...0
	SecOsnmaDsmAuthHashFunctionMask   SecOsnmaDsmAuth = 0b11 << 6    // bits 7...6
	SecOsnmaDsmAuthMacFunctionMask    SecOsnmaDsmAuth = 0b11 << 8    // bits 9...8
	SecOsnmaDsmAuthPubKeyIdMask       SecOsnmaDsmAuth = 0b1111 << 10 // bits 13...10
	SecOsnmaDsmAuthMacLookupTableMask SecOsnmaDsmAuth = 0xFF << 14   // bits 21...14
	SecOsnmaDsmAuthKeySizeMask        SecOsnmaDsmAuth = 0b1111 << 22 // bits 25...22
	SecOsnmaDsmAuthMacSizeMask        SecOsnmaDsmAuth = 0b1111 << 26 // bits 29...26
	SecOsnmaDsmAuthFromNVS            SecOsnmaDsmAuth = 1 << 30      // bit 30
)

const (
	SecOsnmaDsmAuthStatusNone SecOsnmaDsmAuth = iota
	SecOsnmaDsmAuthStatusKROOTOK
	SecOsnmaDsmAuthStatusPKROK
	SecOsnmaDsmAuthStatusAlert
	SecOsnmaDsmAuthStatusKROOTFail
	SecOsnmaDsmAuthStatusPKRFail
	SecOsnmaDsmAuthStatusUnknownPK
	SecOsnmaDsmAuthStatusPKDecompressFail
	SecOsnmaDsmAuthStatusConfigNotSupported
	SecOsnmaDsmAuthStatusNMTMissingMerkle
)

type SecOsnmaTeslaKey uint32

const (
	SecOsnmaTeslaKeyAuthStatusMask SecOsnmaTeslaKey = 0b111        // bits 2...0
	SecOsnmaTeslaKeyWnSfMask       SecOsnmaTeslaKey = 0xFFF << 3   // bits 14...3
	SecOsnmaTeslaKeyTowSfMask      SecOsnmaTeslaKey = 0x7FFF << 15 // bits 29...15
	SecOsnmaTeslaKeyChainIdMask    SecOsnmaTeslaKey = 0b11 << 30   // bits 31...30
)

const (
	SecOsnmaTeslaKeyAuthNone SecOsnmaTeslaKey = iota
	SecOsnmaTeslaKeyAuthOK
	SecOsnmaTeslaKeyAuthFail
	SecOsnmaTeslaKeyAuthOngoing
	SecOsnmaTeslaKeyAuthPast
	SecOsnmaTeslaKeyAuthOldRoot
)

type SecOsnmaGeneral uint32

const (
	SecOsnmaGeneralAuthSVsMask             SecOsnmaGeneral = 0b11_1111      // bits 5...0
	SecOsnmaGeneralAuthNumTimMask          SecOsnmaGeneral = 0b11_1111 << 6 // bits 11...6
	SecOsnmaGeneralTimingAuthMask          SecOsnmaGeneral = 0b11 << 12     // bits 13...12
	SecOsnmaGeneralMacAdkdTypeMask         SecOsnmaGeneral = 0b1 << 14      // bits 14
	SecOsnmaGeneralPubKeySrcMask           SecOsnmaGeneral = 0b11 << 15     // bits 16...15
	SecOsnmaGeneralMerkleRootSrcMask       SecOsnmaGeneral = 0b11 << 17     // bits 18...17
	SecOsnmaGeneralMerkleRootVal           SecOsnmaGeneral = 1 << 19        // bit 19
	SecOsnmaGeneralFutureMerkleRootSrcMask SecOsnmaGeneral = 0b11 << 20     // bits 21...20
	SecOsnmaGeneralFutureMerkleRootVal     SecOsnmaGeneral = 1 << 22        // bit 22
	SecOsnmaGeneralPubKeyVal               SecOsnmaGeneral = 1 << 23        // bit 23
	SecOsnmaGeneralFuturePubKeyVal         SecOsnmaGeneral = 1 << 24        // bit 24
	SecOsnmaGeneralFuturePubKeySrcMask     SecOsnmaGeneral = 0b11 << 25     // bits 26...25
	SecOsnmaGeneralFuturePubKeyIdMask      SecOsnmaGeneral = 0b1111 << 27   // bits 30...27
)

const (
	SecOsnmaTimingAuthNotPerformed SecOsnmaGeneral = iota << 12
	SecOsnmaTimingAuthOK
	SecOsnmaTimingAuthFail
)

const (
	SecOsnmaMacAdkdTypeFast SecOsnmaGeneral = 0 << 14
	SecOsnmaMacAdkdTypeSlow SecOsnmaGeneral = 1 << 14
)

const (
	SecOsnmaPubKeySrcFactory SecOsnmaGeneral = iota << 15
	SecOsnmaPubKeySrcSatellites
	SecOsnmaPubKeySrcAided
	SecOsnmaPubKeySrcNVS
)

const (
	SecOsnmaMerkleRootSrcFactory SecOsnmaGeneral = 0 << 17
	SecOsnmaMerkleRootSrcAided   SecOsnmaGeneral = 2 << 17
	SecOsnmaMerkleRootSrcNVS     SecOsnmaGeneral = 3 << 17
)

type SecOsnmaAuthSVBitfield uint16

const (
	SecOsnmaAuthSVIODEMask    SecOsnmaAuthSVBitfield = 0x3FF
	SecOsnmaAuthSVAuthNumMask SecOsnmaAuthSVBitfield = 0x1F << 10
	SecOsnmaAuthSVAuthStatus  SecOsnmaAuthSVBitfield = 1 << 15
)

var _ VaryingMsg = (*SecOsnma)(nil)
var _ PartiallyHandledMsg = (*SecOsnma)(nil)

func (m *SecOsnma) ID() MsgID { return SecOsnmaID }

func (m *SecOsnma) IsHandled() bool {
	return m.Version == 3
}

func (m *SecOsnma) InitVaryingPart(payloadLen int) error {
	len, err := sliceLen(m, payloadLen, 28, 4)
	if err != nil {
		return err
	}
	m.AuthSVs = make([]SecOsnmaAuthSV, len)
	return nil
}

func (m *SecOsnma) FixedPart() any {
	return &m.SecOsnmaFixed
}

func (m *SecOsnma) VaryingPart() any {
	return &m.AuthSVs
}

func init() {
	regMsg[SecOsnma]("OSNMA")
}
