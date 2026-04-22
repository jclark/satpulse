// Package logobs provides observability through logging.
package logobs

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/obs"
)

// NTPSampleLogObserver logs NTP refclock sample events via slog.
// For now it only logs the first sample of the process lifetime;
// additional per-sample or summary logging belongs here as the
// observability story grows.
type NTPSampleLogObserver struct {
	obs.DefaultObserver
	lg          *slog.Logger
	loggedFirst bool
}

// NewNTPSampleLogObserver returns a new NTPSampleLogObserver that
// writes through lg.
func NewNTPSampleLogObserver(lg *slog.Logger) *NTPSampleLogObserver {
	return &NTPSampleLogObserver{lg: lg}
}

// NTPSample implements obs.Observer. Emits a single info log the
// first time a refclock sample is generated, signalling that the
// synchronization pipeline has produced usable output.
func (o *NTPSampleLogObserver) NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time) {
	if o.loggedFirst {
		return
	}
	o.loggedFirst = true
	o.lg.Info("generated first NTP refclock sample", "offset", offset)
}
