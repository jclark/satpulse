package gpsevent

import (
	"encoding/json"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

type LogReplayer struct {
	ls     ptime.LeapSecond
	tStart time.Time
	cb     *combine.Combiner
}

func (r *LogReplayer) replayEvent(bytes []byte) error {
	event := LogEvent{}
	err := json.Unmarshal(bytes, &event)
	if err != nil {
		return err
	}
	tRead := event.T
	if !r.tStart.IsZero() && event.Nanos != 0 {
		tRead = r.tStart.Add(event.Nanos)
	}
	if event.Time != nil {
		r.replayTime(event.Time, tRead)
	} else if event.Timestamp != nil {
		r.replayTimestamp(event.Timestamp, tRead)
	}
	return nil
}

func (r *LogReplayer) replayTime(mt *gpsprot.TimeMsg, tRead time.Time) {
	// XXX This code duplicates logic in Dispatcher.Time.
	// They should both move into combine.Combiner
	var sec ptime.Time
	if !mt.TAITime.IsZero() {
		sec = mt.TAITime
	} else {
		u := mt.UTCTime
		if u == nil {
			return
		}
		sec = r.ls.UTCtoTime(*u)
	}
	secRnd := sec.Round(time.Second)
	r.cb.TimeMsg(secRnd, tRead, mt.PulseOffset, mt.Ref)
}

func (r *LogReplayer) replayTimestamp(ts *Timestamp, tRead time.Time) {
	r.cb.PulseEdge(ptime.ClockTime{T: ts.T, Era: ts.Era}, tRead, ts.Delay)
}
