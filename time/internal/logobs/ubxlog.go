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
	lg *slog.Logger
}

// NewUBXLogObserver creates a new UBXLogObserver.
func NewUBXLogObserver(lg *slog.Logger) *UBXLogObserver {
	return &UBXLogObserver{lg: lg}
}

// NativeMsg implements obs.Observer by dispatching recognized UBX messages
// to type-specific handlers.
func (o *UBXLogObserver) NativeMsg(tag gpsprot.Tag, _ string, msg any, _ time.Time) bool {
	if tag != gpsreg.TagUBX {
		return false
	}
	switch m := msg.(type) {
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
