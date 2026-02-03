package gpsevent

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/timemsg"
)

type Replayer struct {
	tStart        time.Time
	timeMsgBuffer *timemsg.Buffer
	ctrl          *phcsync.Controller
	tPtr          *time.Time // optional: for logging timestamps
}

func (r *Replayer) tStartInit(event *LogEvent) {
	// Keep existing subtle logic - it still works!
	now := time.Now()
	replayDelay := now.Sub(event.T)
	r.tStart = now.Add(-replayDelay).Add(-event.Nanos)
}

func (r *Replayer) replayEvent(bytes []byte) error {
	event := LogEvent{}
	err := json.Unmarshal(bytes, &event)
	if err != nil {
		return err
	}
	if r.tStart.IsZero() {
		r.tStartInit(&event)
	}
	tRead := r.tStart.Add(event.Nanos)
	if r.tPtr != nil {
		*r.tPtr = tRead
	}
	// Replay pulse edges
	if event.PulseEdge != nil {
		r.ctrl.PulseEdge(phcsync.PulseEdge{
			Timestamp: phctime.Time{
				T:   event.PulseEdge.T,
				Era: event.PulseEdge.Era,
			},
			TRead: phctime.Sample{
				PHC: phctime.Time{
					T:   event.PulseEdge.TRead,
					Era: event.PulseEdge.Era,
				},
				Sys: tRead, // reconstructed monotonic time
			},
		})
	}
	// Replay time messages
	if event.Time != nil {
		r.timeMsgBuffer.Time(event.Time, tRead)
		r.ctrl.TimeMessage()
	}
	// Replay leap second messages
	if event.LeapSecond != nil {
		var ls ptime.LeapSecond
		if event.LeapSecond.UpdateLeapSecond(&ls) {
			r.timeMsgBuffer.LeapSecond(event.LeapSecond, tRead)
			r.ctrl.LeapSecond(ls)
		}
	}
	return nil
}

// ReplayFile replays events from a log file through a phcsync controller.
// It stops after reset mode completes (after first sample is produced).
func ReplayFile(fn string, ctrl *phcsync.Controller, timeMsgBuffer *timemsg.Buffer, tPtr *time.Time) error {
	replayer := Replayer{
		timeMsgBuffer: timeMsgBuffer,
		ctrl:          ctrl,
		tPtr:          tPtr,
	}
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		err := replayer.replayEvent([]byte(line))
		if err != nil {
			return err
		}
		// Stop after reset mode completes - continuing is meaningless
		// because phcsync will adjust PHC in converging/tracking modes
		if ctrl.Mode() != phcsync.ModeReset {
			break
		}
	}
	return scanner.Err()
}
