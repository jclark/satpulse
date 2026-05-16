package daemon

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/logobs"
	"github.com/jclark/satpulse/time/internal/obs"
)

// positionObserver stores the latest GPS position for the HTTP endpoint.
type positionObserver struct {
	obs.DefaultObserver
	mu    sync.Mutex
	utc   *ptime.UTCTime
	entry logobs.TrackLogEntry
}

// Tick caches the latest UTC time.
func (o *positionObserver) Tick(msg *gpsprot.TimeMsg, _ time.Time) {
	o.utc = msg.UTCTime.Ptr()
}

// NavEpochPV stores the latest position.
func (o *positionObserver) NavEpochPV(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, _ time.Time) {
	e := logobs.NewTrackLogEntry(msg, pv, o.utc)
	if e.T == "" {
		return
	}
	o.mu.Lock()
	o.entry = e
	o.mu.Unlock()
}

func (o *positionObserver) position() logobs.TrackLogEntry {
	o.mu.Lock()
	e := o.entry
	o.mu.Unlock()
	return e
}

func positionHandler(po *positionObserver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := po.position()
		if e.T == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, err := json.Marshal(&e)
		if err != nil {
			panic(err)
		}
		w.Write(b)
	}
}
