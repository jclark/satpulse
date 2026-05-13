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

// NativeMsg implements obs.Observer by dispatching recognized UBX messages
// to type-specific handlers.
func (o *UBXLogObserver) NativeMsg(tag gpsprot.Tag, _ string, msg any, _ time.Time) bool {
	if tag != gpsreg.TagUBX {
		return false
	}
	switch m := msg.(type) {
	case *ubxbin.NavTimeTrusted:
		o.logTrustedTime(m)
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
