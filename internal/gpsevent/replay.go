package gpsevent

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/combine"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/phc"
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

func ReplayFile(fn string, phcFlags phc.DriverFlags, sampler combine.Sampler, ls ptime.LeapSecond, lg *slog.Logger) error {
	pt := combine.PulseType{EdgesPerPulse: phcFlags.Edges(), PulseWidth: time.Second / 10}
	ccfg := combine.Config{}
	ccfg.SetDefault(pt)
	if phcFlags&phc.DriverPoll4Hz != 0 {
		ccfg.PulsePollInterval = time.Second / 4
	}
	cb, err := combine.NewCombiner(pt, sampler, lg, ccfg)
	if err != nil {
		return err
	}

	replayer := LogReplayer{
		ls:     ls,
		tStart: time.Now(),
		cb:     cb,
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
	}
	return nil
}
