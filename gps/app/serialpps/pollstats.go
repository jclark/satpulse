package serialpps

import (
	"log/slog"
	"slices"
	"time"
)

// pollStatsSampleLimit bounds timing storage during an unlimited run.
const pollStatsSampleLimit = 10000

// PollStats accumulates optional timing and outcome statistics for one run
// of Poll. It must be read only after Poll returns.
type PollStats struct {
	started        bool
	acquire, track struct {
		windows, edges int
	}
	outliers  int
	durations durationSamples
	gaps      durationSamples
}

func (s *PollStats) begin() {
	if s != nil {
		s.started = true
	}
}

type durationSamples struct {
	count   int
	next    int
	samples []time.Duration
}

type durationStats struct {
	Count   int
	Sampled int
	Min     time.Duration
	Median  time.Duration
	Mean    time.Duration
	P90     time.Duration
	Max     time.Duration
}

type pollStatsSummary struct {
	Acquire, Track struct {
		Windows, Edges int
	}
	Outliers     int
	PollDuration durationStats
	PollGap      durationStats
}

func (s *durationSamples) add(d time.Duration) {
	s.count++
	if len(s.samples) < pollStatsSampleLimit {
		s.samples = append(s.samples, d)
		return
	}
	s.samples[s.next] = d
	s.next = (s.next + 1) % len(s.samples)
}

func (s *PollStats) addPoll(p poll, prev *poll) {
	if s == nil {
		return
	}
	s.durations.add(p.duration())
	if prev != nil {
		s.gaps.add(p.gapAfter(*prev))
	}
}

func (s *PollStats) addWindow(edge, acquired, outlier bool) {
	if s == nil {
		return
	}
	phase := &s.acquire
	if acquired {
		phase = &s.track
	}
	phase.windows++
	if edge {
		phase.edges++
	}
	if outlier {
		s.outliers++
	}
}

func (s *PollStats) summary() pollStatsSummary {
	if s == nil {
		return pollStatsSummary{}
	}
	sum := pollStatsSummary{PollDuration: s.durations.summary(), PollGap: s.gaps.summary()}
	sum.Acquire.Windows, sum.Acquire.Edges = s.acquire.windows, s.acquire.edges
	sum.Track.Windows, sum.Track.Edges = s.track.windows, s.track.edges
	sum.Outliers = s.outliers
	return sum
}

func (s *durationSamples) summary() durationStats {
	if len(s.samples) == 0 {
		return durationStats{Count: s.count}
	}
	sorted := slices.Clone(s.samples)
	slices.Sort(sorted)
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	return durationStats{
		Count:   s.count,
		Sampled: len(sorted),
		Min:     sorted[0],
		Median:  sorted[len(sorted)/2],
		Mean:    sum / time.Duration(len(sorted)),
		P90:     sorted[len(sorted)*9/10],
		Max:     sorted[len(sorted)-1],
	}
}

// Log writes the accumulated polling statistics at info level.
func (s *PollStats) Log(lg *slog.Logger) {
	if s == nil || !s.started {
		return
	}
	summary := s.summary()
	lg.Info("serial PPS polling statistics",
		slog.Group("acquire", "windows", summary.Acquire.Windows, "edges", summary.Acquire.Edges),
		slog.Group("track", "windows", summary.Track.Windows, "edges", summary.Track.Edges, "outliers", summary.Outliers))
	logDurationStats(lg, "serial PPS state read times", summary.PollDuration)
	logDurationStats(lg, "serial PPS between-read times", summary.PollGap)
}

func logDurationStats(lg *slog.Logger, msg string, stats durationStats) {
	args := []any{"count", stats.Count}
	if stats.Sampled < stats.Count {
		args = append(args, "sampled", stats.Sampled)
	}
	if stats.Sampled != 0 {
		args = append(args,
			"min", stats.Min,
			"median", stats.Median,
			"mean", stats.Mean,
			"p90", stats.P90,
			"max", stats.Max)
	}
	lg.Info(msg, args...)
}
