// Package logobs provides observability through logging.
package logobs

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/gps/ptime"
)

const ClockLogExtension = ".log"

// ClockLogObserver writes per-sample clock data to a file
type ClockLogObserver struct {
	obs.DefaultObserver
	lg *slog.Logger
	lf logfile.LogFile
	ls ptime.LeapSecond
}

// NewClockLogObserver creates a new ClockLogObserver that logs clock samples to a file
func NewClockLogObserver(lg *slog.Logger, path string, ls ptime.LeapSecond) (*ClockLogObserver, error) {
	o := &ClockLogObserver{
		lg: lg,
		ls: ls,
	}
	err := o.lf.Open(path)
	if err != nil {
		return nil, err
	}
	o.writeLogHeader()
	return o, nil
}

// Sample implements phcsync.Sampler - logs clock data samples
func (o *ClockLogObserver) Sample(data phcsync.Sample) {
	// Don't log missing samples
	if data.Kind == phcsync.SampleMissing {
		return
	}
	o.writeLogEntry(data)
}

// LeapSecond implements gpsprot.MsgHandler - updates cached leap second when GPS announces a change
func (o *ClockLogObserver) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	if msg.UpdateLeapSecond(&o.ls) {
		o.lg.Debug("clock log observer updated leap second",
			"offset_before", msg.UTCOffBefore,
			"offset_after", msg.UTCOffAfter,
			"change_time", msg.Date())
	}
}

// ReopenLog implements obs.Observer - handles log rotation
func (o *ClockLogObserver) ReopenLog() {
	o.lf.Reopen(o.lg)
	o.writeLogHeader()
}

// Release implements obs.Observer - closes the log file
func (o *ClockLogObserver) Release() {
	o.lf.Close(o.lg)
}

const logHeader = "# date time offset freq outlier era sync\n"

func (o *ClockLogObserver) writeLogHeader() {
	if o.lf.File == nil {
		return
	}
	_, err := o.lf.File.WriteString(logHeader)
	o.lf.HandleWriteError(err, o.lg)
}

func (o *ClockLogObserver) writeLogEntry(data phcsync.Sample) {
	if o.lf.File == nil {
		return
	}
	outlierFlag := 0
	if data.Kind == phcsync.SampleOutlier {
		outlierFlag = 1
	}
	syncFlag := 0
	if data.Mode.InSync() {
		syncFlag = 1
	}
	// Format offset to width 3 for alignment
	// Almost all the time, the absolute value of offset will be < 100,
	// so we format to a width of 3 so the values including the sign will align.
	_, err := fmt.Fprintf(o.lf.File, "%s %3d %.0f %d %d %d\n",
		o.formatDateTime(data.Ref),
		data.Offset,
		data.Freq,
		outlierFlag,
		uint64(data.Era),
		syncFlag)
	o.lf.HandleWriteError(err, o.lg)
}

func (o *ClockLogObserver) formatDateTime(ref ptime.Time) string {
	// We need to round because the reference time (which will be a whole number of seconds)
	// may have had a pulse offset (sawtooth correction) of a few nanoseconds applied.
	s := o.ls.FormatTime(ref.Round(time.Second))
	s = strings.Replace(s, "T", " ", 1)
	return strings.Replace(s, "Z", "", 1)
}