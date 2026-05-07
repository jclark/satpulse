//go:build ignore
// +build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/time/internal/gpsevent"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/timemsg"
)

// mockClock implements phcsync.Clock interface for replay (no actual PHC adjustment)
type mockClock struct {
	freqOffset float64
}

func (m *mockClock) SetFreqOffset(offset float64) error {
	m.freqOffset = offset
	return nil
}

func (m *mockClock) FreqOffset() (float64, error) {
	return m.freqOffset, nil
}

func (m *mockClock) MaxFreqOffset() float64 {
	return 62500000.0 // 62.5 million PPB = 62500 PPM
}

func (m *mockClock) AdjTime(d time.Duration) (phctime.Era, error) {
	return phctime.Era(1), nil
}

// replaySampler implements phcsync.Observer to collect samples during replay
type replaySampler struct {
	ref    ptime.Time
	ls     ptime.LeapSecond
	lg     *slog.Logger
	count  int
	output *os.File // nil if no output file is specified
}

func (rs *replaySampler) Sample(data phcsync.Sample) {
	// Only process valid samples
	if data.Kind == phcsync.SampleMissing {
		return
	}
	if rs.output != nil {
		fmt.Fprintf(rs.output, "%s %3d\n", rs.logDateTime(data.Ref), data.Offset.Nanoseconds())
	}
	// Check for duplicate samples
	if data.Ref == rs.ref {
		rs.lg.Warn("duplicate sample detected", "ref", data.Ref)
	}
	rs.ref = data.Ref
	rs.count++
}

func (rs *replaySampler) ModeChanged(_, _ phcsync.Mode) {}

func (rs *replaySampler) logDateTime(ref ptime.Time) string {
	// Round because the reference time may have had a pulse offset applied
	s := rs.ls.FormatTime(ref.Round(time.Second))
	s = strings.Replace(s, "T", " ", 1)
	return strings.Replace(s, "Z", "", 1)
}

func main() {
	// Command-line flags
	var verbose bool
	var outputFile string
	var twoEdges bool
	flag.BoolVar(&verbose, "v", false, "Enable verbose (debug) logging")
	flag.StringVar(&outputFile, "o", "", "File to output samples to")
	flag.BoolVar(&twoEdges, "2", false, "Use 2 edges per pulse (rising and falling)")
	flag.Parse()

	// Ensure a log file is provided
	if flag.NArg() < 1 {
		log.Fatalf("Usage: %s [options] <logfile>", os.Args[0])
	}
	fn := flag.Arg(0)

	var logLevel slog.Level
	if verbose {
		logLevel = slog.LevelDebug
	} else {
		logLevel = slog.LevelInfo
	}
	var curTime time.Time
	opts := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == "time" && !curTime.IsZero() {
				return slog.Attr{
					Key:   a.Key,
					Value: slog.TimeValue(curTime),
				}
			}
			return a
		},
	}
	warnCountHandler := gpsevent.NewWarnCountHandler(slog.NewTextHandler(os.Stderr, opts))
	lg := slog.New(warnCountHandler)

	ls := ptime.LeapSecond2016()

	var output *os.File
	if outputFile != "" {
		var err error
		output, err = os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to open output file: %v", err)
		}
		defer output.Close()
	}

	rs := &replaySampler{
		ls:     ls,
		lg:     lg,
		output: output,
	}

	// Create mock clock
	clock := &mockClock{}

	// Create controller config
	cfg := phcsync.DefaultConfig()

	// Create pulse type
	edgesPerPulse := 1
	if twoEdges {
		edgesPerPulse = 2
	}
	pt := phcsync.PulseType{
		EdgesPerPulse: edgesPerPulse,
		PulseWidth:    100 * time.Millisecond,
	}

	// Create controller
	ctrl, err := phcsync.NewController(clock, rs, nil, cfg, ls, pt, lg)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer ctrl.Close()

	// Create time message buffer
	timeMsgBuffer := timemsg.NewBuffer(lg, ctrl.RequiredMsgWindow(), ls, gpsprot.GPS)

	// Inject buffer into controller
	ctrl.SetTimeMsgBuffer(timeMsgBuffer)

	// Replay the file
	err = gpsevent.ReplayFile(fn, ctrl, timeMsgBuffer, &curTime)
	if err != nil {
		log.Fatalf("Error replaying file: %v", err)
	}

	lg.Info("Replay complete",
		"samples", rs.count,
		"warnings", warnCountHandler.WarnCount(),
		"mode", ctrl.Mode())
}
