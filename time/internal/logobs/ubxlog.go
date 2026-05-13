package logobs

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/time/internal/obs"
)

// UBXLogObserver logs notable UBX protocol messages via slog.
type UBXLogObserver struct {
	obs.DefaultObserver
	lg               *slog.Logger
	trustedTimeState trustedTimeState
	osnmaState       osnmaState
}

// NewUBXLogObserver creates a new UBXLogObserver.
func NewUBXLogObserver(lg *slog.Logger) *UBXLogObserver {
	return &UBXLogObserver{lg: lg}
}

const trustedTimeAccuracyLogStep = 500 * time.Millisecond

type trustedTimeState struct {
	seen         bool
	refSys       ubxbin.NavTimeTrustedRefSys
	valid        ubxbin.NavTimeTrustedValid
	propTAcc     uint32
	propTAccStep uint32
}

type osnmaOperationalState string

const (
	osnmaStateDisabled             osnmaOperationalState = "disabled"
	osnmaStateTimeSyncFailed       osnmaOperationalState = "timeSyncFailed"
	osnmaStateServiceFailed        osnmaOperationalState = "serviceFailed"
	osnmaStateDSMFailed            osnmaOperationalState = "dsmFailed"
	osnmaStateMissingTrustMaterial osnmaOperationalState = "missingTrustMaterial"
	osnmaStateTeslaFailed          osnmaOperationalState = "teslaFailed"
	osnmaStateBadData              osnmaOperationalState = "badData"
	osnmaStateNoData               osnmaOperationalState = "noData"
	osnmaStatePending              osnmaOperationalState = "pending"
	osnmaStateAuthenticating       osnmaOperationalState = "authenticating"
)

type osnmaState struct {
	seen                bool
	last                osnmaSnapshot
	osnmaSVs            noisyCountState
	authenticatedSVs    noisyCountState
	authenticatedTiming noisyCountState
}

type noisyCountState struct {
	seen      bool
	highWater uint8
}

type osnmaSnapshot struct {
	state               osnmaOperationalState
	osnmaEnabled        bool
	nmaStatus           string
	nmaStatusRaw        ubxbin.SecOsnmaNmaHeader
	cpks                string
	osnmaSVs            uint8
	noData              bool
	wrongData           bool
	wrongFlxMac         bool
	wrongMaclt          bool
	timeSyncEnabled     bool
	timeSyncStatus      string
	timeSyncStatusRaw   ubxbin.SecOsnmaTimSyncReq
	timeSyncDiff        float64
	dsmAuth             string
	dsmAuthRaw          ubxbin.SecOsnmaDsmAuth
	teslaKeyAuth        string
	teslaKeyAuthRaw     ubxbin.SecOsnmaTeslaKey
	pubKeyValid         bool
	merkleRootValid     bool
	authenticatedSVs    uint8
	authenticatedTiming uint8
	timingAuth          string
	macAdkdType         string
}

// NativeMsg implements obs.Observer by dispatching recognized UBX messages
// to type-specific handlers.
func (o *UBXLogObserver) NativeMsg(tag gpsprot.Tag, _ string, msg any, _ time.Time) bool {
	if tag != gpsreg.TagUBX {
		return false
	}
	switch m := msg.(type) {
	case *ubxbin.NavTimeTrusted:
		o.logTrustedTime(m)
	case *ubxbin.SecOsnma:
		o.logOSNMA(m)
	case *ubxbin.InfDebug:
		o.logInf(m.ID(), m.InfText)
	case *ubxbin.InfNotice:
		o.logInf(m.ID(), m.InfText)
	case *ubxbin.InfWarning:
		o.logInf(m.ID(), m.InfText)
	case *ubxbin.InfError:
		o.logInf(m.ID(), m.InfText)
	case *ubxbin.InfTest:
		o.logInf(m.ID(), m.InfText)
	default:
		return false
	}
	return true
}

func (o *UBXLogObserver) logTrustedTime(m *ubxbin.NavTimeTrusted) {
	next := o.trustedTimeState.fromMsg(m)
	log := o.trustedTimeState.shouldLog(next)
	o.trustedTimeState = next
	if !log {
		return
	}
	o.lg.Info("change in receiver's trusted time state",
		"refSys", m.RefSys,
		"trustedTimeValid", m.Valid&ubxbin.NavTimeTrustedValidTrustedTime != 0,
		"deltaTimeValid", m.Valid&ubxbin.NavTimeTrustedValidDeltaTime != 0,
		"initialAccuracy", float64(m.IniTAcc)/1000,
		"propagatedAccuracy", float64(m.PropTAcc)/1000,
		"deltaTime", float64(m.DeltaS)+float64(m.DeltaMs)/1000,
	)
}

func (trustedTimeState) fromMsg(m *ubxbin.NavTimeTrusted) trustedTimeState {
	return trustedTimeState{
		seen:         true,
		refSys:       m.RefSys,
		valid:        m.Valid,
		propTAcc:     m.PropTAcc,
		propTAccStep: uint32((time.Duration(m.PropTAcc) * time.Millisecond) / trustedTimeAccuracyLogStep),
	}
}

func (s trustedTimeState) shouldLog(next trustedTimeState) bool {
	if !s.seen {
		return true
	}
	if next.refSys != s.refSys || next.valid != s.valid {
		return true
	}
	if next.valid&ubxbin.NavTimeTrustedValidTrustedTime == 0 {
		return false
	}
	return next.propTAcc < s.propTAcc || next.propTAccStep > s.propTAccStep
}

func (o *UBXLogObserver) logOSNMA(m *ubxbin.SecOsnma) {
	next := osnmaSnapshotFromMsg(m)
	log := o.osnmaState.shouldLog(next)
	o.osnmaState.update(next)
	if !log {
		return
	}
	o.lg.Info("change in receiver's OSNMA state", next.logArgs()...)
}

func (s osnmaState) shouldLog(next osnmaSnapshot) bool {
	return !s.seen ||
		s.last.significantNonCountFields() != next.significantNonCountFields() ||
		s.shouldLogCounts(next)
}

func (s osnmaState) shouldLogCounts(next osnmaSnapshot) bool {
	if s.osnmaSVs.shouldLog(next.osnmaSVs) && next.showsOSNMASVs() {
		return true
	}
	if s.authenticatedSVs.shouldLog(next.authenticatedSVs) && next.state == osnmaStateAuthenticating {
		return true
	}
	return s.authenticatedTiming.shouldLog(next.authenticatedTiming) && next.state == osnmaStateAuthenticating
}

func (s *osnmaState) update(next osnmaSnapshot) {
	s.seen = true
	s.last = next
	s.osnmaSVs.update(next.osnmaSVs)
	s.authenticatedSVs.update(next.authenticatedSVs)
	s.authenticatedTiming.update(next.authenticatedTiming)
}

func (s noisyCountState) shouldLog(value uint8) bool {
	if !s.seen {
		return false
	}
	return value > s.highWater || value*2 < s.highWater
}

func (s *noisyCountState) update(value uint8) {
	if !s.seen || s.shouldLog(value) {
		s.seen = true
		s.highWater = value
	}
}

func osnmaSnapshotFromMsg(m *ubxbin.SecOsnma) osnmaSnapshot {
	mon := m.OsnmaMonitoring
	hdr := m.NmaHeader
	tim := m.TimSyncReq
	dsm := m.DsmAuthentication
	tesla := m.TeslaKey
	gen := m.GeneralAndTiming
	nmaStatus := hdr & ubxbin.SecOsnmaHeaderNmaStatusMask
	s := osnmaSnapshot{
		osnmaEnabled:        mon&ubxbin.SecOsnmaMonitoringOsnmaEnabled != 0,
		nmaStatus:           osnmaNmaStatus(nmaStatus),
		nmaStatusRaw:        nmaStatus,
		cpks:                osnmaCPKS(hdr & ubxbin.SecOsnmaHeaderCPKSMask),
		osnmaSVs:            uint8((mon & ubxbin.SecOsnmaMonitoringNumberSVsMask) >> 1),
		noData:              mon&ubxbin.SecOsnmaMonitoringNoData != 0,
		wrongData:           mon&ubxbin.SecOsnmaMonitoringWrongData != 0,
		wrongFlxMac:         mon&ubxbin.SecOsnmaMonitoringWrongFlxMac != 0,
		wrongMaclt:          mon&ubxbin.SecOsnmaMonitoringWrongMaclt != 0,
		timeSyncEnabled:     tim&ubxbin.SecOsnmaTimSyncEnabled != 0,
		timeSyncStatusRaw:   tim & ubxbin.SecOsnmaTimSyncStatusMask,
		timeSyncDiff:        float64(m.TimSyncReqDiff) / 1000,
		dsmAuthRaw:          dsm & ubxbin.SecOsnmaDsmAuthStatusMask,
		teslaKeyAuthRaw:     tesla & ubxbin.SecOsnmaTeslaKeyAuthStatusMask,
		pubKeyValid:         gen&ubxbin.SecOsnmaGeneralPubKeyVal != 0,
		merkleRootValid:     gen&ubxbin.SecOsnmaGeneralMerkleRootVal != 0,
		authenticatedSVs:    uint8(gen & ubxbin.SecOsnmaGeneralAuthSVsMask),
		authenticatedTiming: uint8((gen & ubxbin.SecOsnmaGeneralAuthNumTimMask) >> 6),
		timingAuth:          osnmaTimingAuth(gen & ubxbin.SecOsnmaGeneralTimingAuthMask),
		macAdkdType:         osnmaMacAdkdType(gen & ubxbin.SecOsnmaGeneralMacAdkdTypeMask),
	}
	s.timeSyncStatus = osnmaTimeSyncStatus(s.timeSyncStatusRaw)
	s.dsmAuth = osnmaDsmAuth(s.dsmAuthRaw)
	s.teslaKeyAuth = osnmaTeslaKeyAuth(s.teslaKeyAuthRaw)
	s.state = s.operationalState()
	return s
}

func (s osnmaSnapshot) operationalState() osnmaOperationalState {
	switch {
	case !s.osnmaEnabled:
		return osnmaStateDisabled
	case s.timeSyncEnabled && (s.timeSyncStatusRaw == ubxbin.SecOsnmaTimSyncStatusNoTrustedTime ||
		s.timeSyncStatusRaw == ubxbin.SecOsnmaTimSyncStatusNotAccurate ||
		s.timeSyncStatusRaw == ubxbin.SecOsnmaTimSyncStatusFailed):
		return osnmaStateTimeSyncFailed
	case s.nmaStatusRaw == ubxbin.SecOsnmaNmaStatusInvalid:
		return osnmaStateServiceFailed
	case osnmaDsmAuthFailed(s.dsmAuthRaw):
		return osnmaStateDSMFailed
	case !s.pubKeyValid || !s.merkleRootValid:
		return osnmaStateMissingTrustMaterial
	case s.teslaKeyAuthRaw == ubxbin.SecOsnmaTeslaKeyAuthFail ||
		s.teslaKeyAuthRaw == ubxbin.SecOsnmaTeslaKeyAuthPast ||
		s.teslaKeyAuthRaw == ubxbin.SecOsnmaTeslaKeyAuthOldRoot:
		return osnmaStateTeslaFailed
	case s.wrongData || s.wrongFlxMac || s.wrongMaclt:
		return osnmaStateBadData
	case s.osnmaSVs == 0 || s.noData:
		return osnmaStateNoData
	case s.authenticatedSVs == 0:
		return osnmaStatePending
	default:
		return osnmaStateAuthenticating
	}
}

func osnmaDsmAuthFailed(auth ubxbin.SecOsnmaDsmAuth) bool {
	switch auth {
	case ubxbin.SecOsnmaDsmAuthStatusAlert,
		ubxbin.SecOsnmaDsmAuthStatusKROOTFail,
		ubxbin.SecOsnmaDsmAuthStatusPKRFail,
		ubxbin.SecOsnmaDsmAuthStatusUnknownPK,
		ubxbin.SecOsnmaDsmAuthStatusPKDecompressFail,
		ubxbin.SecOsnmaDsmAuthStatusConfigNotSupported,
		ubxbin.SecOsnmaDsmAuthStatusNMTMissingMerkle:
		return true
	default:
		return false
	}
}

func (s osnmaSnapshot) significantNonCountFields() osnmaSnapshot {
	sig := osnmaSnapshot{state: s.state}
	switch s.state {
	case osnmaStateDisabled:
		sig.osnmaEnabled = s.osnmaEnabled
	case osnmaStateTimeSyncFailed:
		sig.timeSyncStatus = s.timeSyncStatus
	case osnmaStateServiceFailed:
		sig.nmaStatus = s.nmaStatus
		sig.cpks = s.cpks
	case osnmaStateDSMFailed:
		sig.dsmAuth = s.dsmAuth
		sig.cpks = s.cpks
	case osnmaStateMissingTrustMaterial:
		sig.pubKeyValid = s.pubKeyValid
		sig.merkleRootValid = s.merkleRootValid
		sig.dsmAuth = s.dsmAuth
		sig.cpks = s.cpks
	case osnmaStateTeslaFailed:
		sig.teslaKeyAuth = s.teslaKeyAuth
	case osnmaStateBadData:
		sig.wrongData = s.wrongData
		sig.wrongFlxMac = s.wrongFlxMac
		sig.wrongMaclt = s.wrongMaclt
	case osnmaStateNoData:
		sig.noData = s.noData
	case osnmaStatePending:
		sig.nmaStatus = s.nmaStatus
		sig.dsmAuth = s.dsmAuth
		sig.teslaKeyAuth = s.teslaKeyAuth
		sig.timeSyncStatus = s.timeSyncStatus
	case osnmaStateAuthenticating:
		sig.nmaStatus = s.nmaStatus
		sig.timeSyncEnabled = s.timeSyncEnabled
		sig.macAdkdType = s.macAdkdType
		sig.timingAuth = s.timingAuth
	}
	return sig
}

func (s osnmaSnapshot) logArgs() []any {
	args := []any{"state", string(s.state)}
	switch s.state {
	case osnmaStateDisabled:
		args = append(args, "osnmaEnabled", s.osnmaEnabled)
	case osnmaStateTimeSyncFailed:
		args = append(args, "timeSyncStatus", s.timeSyncStatus)
		if s.timeSyncStatusRaw == ubxbin.SecOsnmaTimSyncStatusFailed {
			args = append(args, "timeSyncDiff", s.timeSyncDiff)
		}
	case osnmaStateServiceFailed:
		args = append(args, "nmaStatus", s.nmaStatus, "cpks", s.cpks)
	case osnmaStateDSMFailed:
		args = append(args, "dsmAuth", s.dsmAuth, "cpks", s.cpks)
	case osnmaStateMissingTrustMaterial:
		args = append(args,
			"pubKeyValid", s.pubKeyValid,
			"merkleRootValid", s.merkleRootValid,
			"dsmAuth", s.dsmAuth,
			"cpks", s.cpks)
	case osnmaStateTeslaFailed:
		args = append(args, "teslaKeyAuth", s.teslaKeyAuth)
	case osnmaStateBadData:
		args = append(args,
			"osnmaSVs", s.osnmaSVs,
			"wrongData", s.wrongData,
			"wrongFlxMac", s.wrongFlxMac,
			"wrongMaclt", s.wrongMaclt)
	case osnmaStateNoData:
		args = append(args, "osnmaSVs", s.osnmaSVs, "noData", s.noData)
	case osnmaStatePending:
		args = append(args,
			"osnmaSVs", s.osnmaSVs,
			"nmaStatus", s.nmaStatus,
			"dsmAuth", s.dsmAuth,
			"teslaKeyAuth", s.teslaKeyAuth,
			"timeSyncStatus", s.timeSyncStatus)
	case osnmaStateAuthenticating:
		args = append(args,
			"nmaStatus", s.nmaStatus,
			"osnmaSVs", s.osnmaSVs,
			"authenticatedSVs", s.authenticatedSVs,
			"timeSyncEnabled", s.timeSyncEnabled,
			"macAdkdType", s.macAdkdType,
			"timingAuth", s.timingAuth,
			"authenticatedTiming", s.authenticatedTiming)
	}
	return args
}

func (s osnmaSnapshot) showsOSNMASVs() bool {
	switch s.state {
	case osnmaStateBadData, osnmaStateNoData, osnmaStatePending, osnmaStateAuthenticating:
		return true
	default:
		return false
	}
}

func osnmaNmaStatus(status ubxbin.SecOsnmaNmaHeader) string {
	switch status {
	case ubxbin.SecOsnmaNmaStatusNotAuth:
		return "notAuthenticated"
	case ubxbin.SecOsnmaNmaStatusTest:
		return "test"
	case ubxbin.SecOsnmaNmaStatusOperational:
		return "operational"
	case ubxbin.SecOsnmaNmaStatusInvalid:
		return "invalid"
	default:
		return fmt.Sprintf("unknown(%d)", status>>1)
	}
}

func osnmaCPKS(cpks ubxbin.SecOsnmaNmaHeader) string {
	switch cpks {
	case ubxbin.SecOsnmaCPKSNotApplicable:
		return "notApplicable"
	case ubxbin.SecOsnmaCPKSNominal:
		return "nominal"
	case ubxbin.SecOsnmaCPKSEOC:
		return "endOfChain"
	case ubxbin.SecOsnmaCPKSCREV:
		return "chainRevoked"
	case ubxbin.SecOsnmaCPKSNPK:
		return "newPublicKey"
	case ubxbin.SecOsnmaCPKSPKREV:
		return "publicKeyRevoked"
	case ubxbin.SecOsnmaCPKSNMT:
		return "newMerkleTree"
	case ubxbin.SecOsnmaCPKSAM:
		return "alert"
	default:
		return fmt.Sprintf("unknown(%d)", cpks>>5)
	}
}

func osnmaTimeSyncStatus(status ubxbin.SecOsnmaTimSyncReq) string {
	switch status {
	case ubxbin.SecOsnmaTimSyncStatusNotPerformed:
		return "notPerformed"
	case ubxbin.SecOsnmaTimSyncStatusNoTrustedTime:
		return "noTrustedTime"
	case ubxbin.SecOsnmaTimSyncStatusNotAccurate:
		return "notAccurate"
	case ubxbin.SecOsnmaTimSyncStatusPassed:
		return "passed"
	case ubxbin.SecOsnmaTimSyncStatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", status>>1)
	}
}

func osnmaDsmAuth(auth ubxbin.SecOsnmaDsmAuth) string {
	switch auth {
	case ubxbin.SecOsnmaDsmAuthStatusNone:
		return "none"
	case ubxbin.SecOsnmaDsmAuthStatusKROOTOK:
		return "krootOK"
	case ubxbin.SecOsnmaDsmAuthStatusPKROK:
		return "pkrOK"
	case ubxbin.SecOsnmaDsmAuthStatusAlert:
		return "alert"
	case ubxbin.SecOsnmaDsmAuthStatusKROOTFail:
		return "krootFail"
	case ubxbin.SecOsnmaDsmAuthStatusPKRFail:
		return "pkrFail"
	case ubxbin.SecOsnmaDsmAuthStatusUnknownPK:
		return "unknownPK"
	case ubxbin.SecOsnmaDsmAuthStatusPKDecompressFail:
		return "pkDecompressFail"
	case ubxbin.SecOsnmaDsmAuthStatusConfigNotSupported:
		return "configNotSupported"
	case ubxbin.SecOsnmaDsmAuthStatusNMTMissingMerkle:
		return "nmtMissingMerkle"
	default:
		return fmt.Sprintf("unknown(%d)", auth)
	}
}

func osnmaTeslaKeyAuth(auth ubxbin.SecOsnmaTeslaKey) string {
	switch auth {
	case ubxbin.SecOsnmaTeslaKeyAuthNone:
		return "none"
	case ubxbin.SecOsnmaTeslaKeyAuthOK:
		return "ok"
	case ubxbin.SecOsnmaTeslaKeyAuthFail:
		return "fail"
	case ubxbin.SecOsnmaTeslaKeyAuthOngoing:
		return "ongoing"
	case ubxbin.SecOsnmaTeslaKeyAuthPast:
		return "past"
	case ubxbin.SecOsnmaTeslaKeyAuthOldRoot:
		return "oldRoot"
	default:
		return fmt.Sprintf("unknown(%d)", auth)
	}
}

func osnmaTimingAuth(auth ubxbin.SecOsnmaGeneral) string {
	switch auth {
	case ubxbin.SecOsnmaTimingAuthNotPerformed:
		return "notPerformed"
	case ubxbin.SecOsnmaTimingAuthOK:
		return "ok"
	case ubxbin.SecOsnmaTimingAuthFail:
		return "fail"
	default:
		return fmt.Sprintf("unknown(%d)", auth>>12)
	}
}

func osnmaMacAdkdType(macAdkd ubxbin.SecOsnmaGeneral) string {
	switch macAdkd {
	case ubxbin.SecOsnmaMacAdkdTypeFast:
		return "ADKD0"
	case ubxbin.SecOsnmaMacAdkdTypeSlow:
		return "ADKD12"
	default:
		return fmt.Sprintf("unknown(%d)", macAdkd>>14)
	}
}

func (o *UBXLogObserver) logInf(id ubxbin.MsgID, text ubxbin.InfText) {
	msg := fmt.Sprintf("UBX-%s message received", id)
	args := []any{"text", string(text)}
	switch id {
	case ubxbin.InfDebugID, ubxbin.InfTestID:
		o.lg.Debug(msg, args...)
	case ubxbin.InfNoticeID:
		o.lg.Info(msg, args...)
	case ubxbin.InfWarningID, ubxbin.InfErrorID:
		o.lg.Warn(msg, args...)
	}
}
