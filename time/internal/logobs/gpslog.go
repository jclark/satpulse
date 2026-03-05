package logobs

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/obs"
)

// gpsStatus captures the fields of NavEpochMsg that we track for changes.
type gpsStatus struct {
	fixLevel   gpsprot.FixLevel
	fixDim     gpsprot.SolutionDim
	correction gpsprot.CorrKind
	auxSrc     gpsprot.AuxSrc
}

func statusFromMsg(msg *gpsprot.NavEpochMsg) gpsStatus {
	return gpsStatus{
		fixLevel:   msg.FixLevel,
		fixDim:     msg.SolutionDim,
		correction: msg.Correction,
		auxSrc:     msg.AuxSrc,
	}
}

// GPSLogObserver logs GPS fix status changes via slog.
type GPSLogObserver struct {
	obs.DefaultObserver
	lg  *slog.Logger
	cur gpsStatus
}

// NewGPSLogObserver creates a new GPSLogObserver.
func NewGPSLogObserver(lg *slog.Logger) *GPSLogObserver {
	return &GPSLogObserver{lg: lg}
}

func (o *GPSLogObserver) logStatus(msg string, s gpsStatus) {
	args := []any{"fixLevel", s.fixLevel}
	if s.fixDim != 0 {
		args = append(args, "solutionDim", s.fixDim)
	}
	if s.correction != 0 {
		args = append(args, "correction", s.correction)
	}
	if s.auxSrc != 0 {
		args = append(args, "auxSrc", s.auxSrc)
	}
	if s.fixLevel == gpsprot.FixLevelNone {
		o.lg.Warn(msg, args...)
	} else {
		o.lg.Info(msg, args...)
	}
}

// NavEpoch implements gpsprot.MsgHandler.
func (o *GPSLogObserver) NavEpoch(msg *gpsprot.NavEpochMsg, _ time.Time) {
	cur := statusFromMsg(msg)
	// note that if fixLevel is 0, then we do not have the messages enabled that give fix quality info, so do not log at all
	if cur == o.cur {
		return
	}
	if cur.fixLevel == 0 {
		o.lg.Info("unknown GPS fix status")
	} else if o.cur.fixLevel == 0 {
		o.logStatus("initial GPS fix status", cur)
	} else {
		o.logStatus("new GPS fix status", cur)
	}
	o.cur = cur
}
