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

	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

type replaySampler struct {
	ref    ptime.Time
	ls     ptime.LeapSecond
	lg     *slog.Logger
	count  int
	output *os.File // nil if no output file is specified
}

func (rs *replaySampler) Sample(ref ptime.Time, local ptime.ClockTime, delayed bool) {
	// Format the reference time and calculate the offset
	offset := local.T.Sub(ref)

	if rs.output != nil {
		fmt.Fprintf(rs.output, "%s %3d\n", rs.logDateTime(ref), offset.Nanoseconds())
	}

	// Check for duplicate samples and log using slog.Logger
	if ref == rs.ref {
		rs.lg.Warn("duplicate sample detected", "ref", ref)
	}
	rs.ref = ref
	rs.count++
}

// Copied from monitor.go
func (rs *replaySampler) logDateTime(ref ptime.Time) string {
	// We need to round because the reference time (which will be a whole number of seconds)
	// may have had a pulse offset (sawtooth correction) of a few nanoseconds applied.
	s := rs.ls.FormatTime(ref.Round(time.Second))
	s = strings.Replace(s, "T", " ", 1)
	return strings.Replace(s, "Z", "", 1)
}

func main() {
	// Command-line flags
	driverBothEdges := flag.Bool("2", false, "Use DriverBothEdges")
	driverOneEdgePoll4Hz := flag.Bool("cm", false, "Use DriverOneEdge|DriverPoll4Hz")
	verbose := flag.Bool("v", false, "Enable verbose (debug) logging")
	outputFile := flag.String("o", "", "File to output samples to")
	flag.Parse()

	// Ensure a log file is provided
	if flag.NArg() < 1 {
		log.Fatalf("Usage: %s [options] <logfile>", os.Args[0])
	}
	fn := flag.Arg(0)

	var logLevel slog.Level
	if *verbose {
		logLevel = slog.LevelDebug
	} else {
		logLevel = slog.LevelInfo
	}
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	var phcFlags phc.DriverFlags
	if *driverBothEdges {
		phcFlags = phc.DriverBothEdges
	} else if *driverOneEdgePoll4Hz {
		phcFlags = phc.DriverOneEdge | phc.DriverPoll4Hz
	}

	ls := ptime.LeapSecond2016()

	var output *os.File
	if *outputFile != "" {
		var err error
		output, err = os.Create(*outputFile)
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

	err := gpsevent.ReplayFile(fn, phcFlags, rs, ls, lg)
	if err != nil {
		log.Fatalf("Error replaying file: %v", err)
	}
	lg.Info("Replay complete", "samples", rs.count)
}
