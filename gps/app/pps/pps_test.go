package pps

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

var testLog = slog.New(slog.DiscardHandler)

func TestGenerator(t *testing.T) {
	msgUTC := time.Unix(1_000, 0).UTC()
	msgRead := time.Unix(900, 125_000_000)
	edge := time.Unix(900, 1_000_000)
	tests := []struct {
		name string
		msg  bool
		age  time.Duration
		leap ptime.LeapSecondKind
		ok   bool
	}{
		{name: "identifies second", msg: true, leap: ptime.LeapSecondNone, ok: true},
		{name: "positive leap passthrough", msg: true, leap: ptime.LeapSecondPositive, ok: true},
		{name: "negative leap passthrough", msg: true, leap: ptime.LeapSecondNegative, ok: true},
		{name: "message exactly three seconds old", msg: true, age: 3 * time.Second, ok: true},
		{name: "stale message", msg: true, age: 3*time.Second + time.Nanosecond},
		{name: "no message"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(DefaultGeneratorConfig())
			tEdge := edge
			utc := msgUTC
			read := msgRead
			if tc.age != 0 {
				read = tEdge.Add(-tc.age)
				utc = read.Add(msgUTC.Sub(msgRead))
			}
			if tc.msg {
				g.MsgUTCTime(utc, read, tc.leap)
			}
			sample, ok := g.Sample(Edge{Timestamp: tEdge, TRead: tEdge})
			if ok != tc.ok {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			wantRef := time.Unix(1_000, 0).UTC()
			if !sample.Ref.Equal(wantRef) {
				t.Errorf("reference = %v, want %v", sample.Ref, wantRef)
			}
			if !sample.Sys.Equal(tEdge) {
				t.Errorf("system = %v, want %v", sample.Sys, tEdge)
			}
			if sample.Leap != tc.leap {
				t.Errorf("leap = %v, want %v", sample.Leap, tc.leap)
			}
		})
	}
}

func TestGeneratorTransfersTimestampThroughReadTime(t *testing.T) {
	tests := []struct {
		name          string
		readSinceMsg  time.Duration
		readAfterEdge time.Duration
		wantRef       time.Time
	}{
		{
			name:          "delivery correction determines second label",
			readSinceMsg:  9 * time.Millisecond,
			readAfterEdge: 10 * time.Millisecond,
			wantRef:       time.Unix(1_000, 0).UTC(),
		},
		{
			name:          "delivery correction determines message age",
			readSinceMsg:  3100 * time.Millisecond,
			readAfterEdge: 100 * time.Millisecond,
			wantRef:       time.Unix(1_003, 0).UTC(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(DefaultGeneratorConfig())
			msgRead := time.Now()
			g.MsgUTCTime(time.Unix(1_000, 0).UTC(), msgRead, ptime.LeapSecondNone)
			tRead := msgRead.Add(tc.readSinceMsg)
			// Reconstruct Timestamp from wall time to model a kernel or
			// Windows timestamp without a monotonic reading.
			timestamp := time.Unix(0, tRead.UnixNano()).Add(-tc.readAfterEdge)
			sample, ok := g.Sample(Edge{Timestamp: timestamp, TRead: tRead})
			if !ok {
				t.Fatal("Edge returned no sample")
			}
			if !sample.Ref.Equal(tc.wantRef) {
				t.Errorf("reference = %v, want %v", sample.Ref, tc.wantRef)
			}
			if !sample.Sys.Equal(timestamp) {
				t.Errorf("system = %v, want timestamp %v", sample.Sys, timestamp)
			}
		})
	}
}

func TestGeneratorKeepsNewestMessage(t *testing.T) {
	g := NewGenerator(DefaultGeneratorConfig())
	newRead := time.Unix(100, 100_000_000)
	g.MsgUTCTime(time.Unix(200, 0), newRead, ptime.LeapSecondPositive)
	g.MsgUTCTime(time.Unix(300, 0), newRead.Add(-time.Second), ptime.LeapSecondNegative)
	sample, ok := g.Sample(Edge{Timestamp: time.Unix(100, 0), TRead: time.Unix(100, 0)})
	if !ok {
		t.Fatal("Edge returned no sample")
	}
	if !sample.Ref.Equal(time.Unix(200, 0)) || sample.Leap != ptime.LeapSecondPositive {
		t.Fatalf("sample = %+v, want newest message reference and leap", sample)
	}
}

func TestGeneratorDelayBounds(t *testing.T) {
	cfg := DefaultGeneratorConfig()
	tests := []struct {
		name  string
		delay time.Duration
		ok    bool
	}{
		{name: "at negative uncertainty bound", delay: -ptime.Seconds(cfg.DelayUncertainty), ok: true},
		{name: "below negative uncertainty bound", delay: -ptime.Seconds(cfg.DelayUncertainty) - time.Nanosecond},
		{name: "zero delay", delay: 0, ok: true},
		{name: "below maximum delay", delay: ptime.Seconds(cfg.MaxDelay) - time.Nanosecond, ok: true},
		{name: "at maximum delay", delay: ptime.Seconds(cfg.MaxDelay)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(cfg)
			utc := time.Unix(1_000, 0).UTC()
			tRead := time.Unix(900, 0)
			g.MsgUTCTime(utc, tRead, ptime.LeapSecondNone)
			at := tRead.Add(-tc.delay)
			sample, ok := g.Sample(Edge{Timestamp: at, TRead: at})
			if ok != tc.ok {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.ok)
			}
			if ok && !sample.Ref.Equal(utc) {
				t.Errorf("reference = %v, want %v", sample.Ref, utc)
			}
		})
	}
}

func TestGeneratorLeapCrossing(t *testing.T) {
	tests := []struct {
		name       string
		utc        time.Time
		leap       ptime.LeapSecondKind
		elapsed    time.Duration // edge.Timestamp - tRead
		expectRef  time.Time
		expectLeap ptime.LeapSecondKind
		expectOK   bool
	}{
		{name: "pulse before positive leap", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: -125 * time.Millisecond, expectRef: time.Unix(86_399, 0), expectLeap: ptime.LeapSecondPositive, expectOK: true},
		{name: "inserted second pulse yields no sample", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: 875 * time.Millisecond},
		{name: "first pulse after positive leap", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: 1875 * time.Millisecond, expectRef: time.Unix(86_400, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
		{name: "second pulse after positive leap", utc: time.Unix(86_399, 0), leap: ptime.LeapSecondPositive,
			elapsed: 2875 * time.Millisecond, expectRef: time.Unix(86_401, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
		{name: "pulse before negative leap", utc: time.Unix(86_398, 0), leap: ptime.LeapSecondNegative,
			elapsed: -125 * time.Millisecond, expectRef: time.Unix(86_398, 0), expectLeap: ptime.LeapSecondNegative, expectOK: true},
		{name: "first pulse after negative leap", utc: time.Unix(86_398, 0), leap: ptime.LeapSecondNegative,
			elapsed: 875 * time.Millisecond, expectRef: time.Unix(86_400, 0), expectLeap: ptime.LeapSecondNone, expectOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGenerator(DefaultGeneratorConfig())
			read := time.Unix(1_000_000, 125_000_000)
			g.MsgUTCTime(tc.utc, read, tc.leap)
			at := read.Add(tc.elapsed)
			sample, ok := g.Sample(Edge{Timestamp: at, TRead: at})
			if ok != tc.expectOK {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.expectOK)
			}
			if !ok {
				return
			}
			if !sample.Ref.Equal(tc.expectRef) || sample.Leap != tc.expectLeap {
				t.Errorf("sample = %+v, want reference %v and leap %v", sample, tc.expectRef, tc.expectLeap)
			}
		})
	}
}

func TestPollStatsSummary(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	newReading := func(offset time.Duration) clockReading {
		return clockReading{stamp: base.Add(offset), mono: base.Add(offset)}
	}
	polls := []poll{
		{start: newReading(0), end: newReading(time.Millisecond)},
		{start: newReading(11 * time.Millisecond), end: newReading(13 * time.Millisecond)},
		{start: newReading(33 * time.Millisecond), end: newReading(36 * time.Millisecond)},
		{start: newReading(66 * time.Millisecond), end: newReading(70 * time.Millisecond)},
		{start: newReading(110 * time.Millisecond), end: newReading(115 * time.Millisecond)},
	}
	stats := new(PollStats)
	stats.addPoll(polls[0], nil)
	for i := 1; i < len(polls); i++ {
		stats.addPoll(polls[i], &polls[i-1])
	}
	stats.addWindow(false, false)
	stats.addWindow(true, false)
	stats.addWindow(true, true)

	got := stats.summary()
	want := pollStatsSummary{
		PollDuration: durationStats{
			Count: 5, Sampled: 5, Min: time.Millisecond, Median: 3 * time.Millisecond,
			Mean: 3 * time.Millisecond, P90: 5 * time.Millisecond, Max: 5 * time.Millisecond,
		},
		PollGap: durationStats{
			Count: 4, Sampled: 4, Min: 10 * time.Millisecond, Median: 30 * time.Millisecond,
			Mean: 25 * time.Millisecond, P90: 40 * time.Millisecond, Max: 40 * time.Millisecond,
		},
	}
	want.Acquire.Windows, want.Acquire.Edges = 2, 1
	want.Track.Windows, want.Track.Edges = 1, 1
	if got != want {
		t.Errorf("Summary() = %+v, want %+v", got, want)
	}
}

func TestPollStatsTimingSamplesAreBounded(t *testing.T) {
	stats := new(PollStats)
	base := time.Unix(1_700_000_000, 0)
	for i := range pollStatsSampleLimit + 1 {
		start := clockReading{stamp: base, mono: base}
		end := clockReading{stamp: base.Add(time.Duration(i) + 1), mono: base.Add(time.Duration(i) + 1)}
		stats.addPoll(poll{start: start, end: end}, nil)
	}
	summary := stats.summary().PollDuration
	if summary.Count != pollStatsSampleLimit+1 || summary.Sampled != pollStatsSampleLimit {
		t.Errorf("duration counts = %d total, %d sampled; want %d total, %d sampled",
			summary.Count, summary.Sampled, pollStatsSampleLimit+1, pollStatsSampleLimit)
	}
}

func TestPollStatsLog(t *testing.T) {
	stats := new(PollStats)
	stats.begin()
	start := clockReading{stamp: time.Unix(1_700_000_000, 0), mono: time.Unix(1_700_000_000, 0)}
	end := clockReading{stamp: start.stamp.Add(2 * time.Millisecond), mono: start.mono.Add(2 * time.Millisecond)}
	stats.addPoll(poll{start: start, end: end}, nil)
	stats.addWindow(true, false)

	var output bytes.Buffer
	stats.Log(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	for _, want := range []string{
		`msg="PPS polling statistics" acquire.windows=1 acquire.edges=1 track.windows=0 track.edges=0`,
		`msg="PPS poll query times" count=1 min=2ms median=2ms mean=2ms p90=2ms max=2ms`,
		`msg="PPS poll between-query times" count=0`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("log %q does not contain %q", output.String(), want)
		}
	}
}

func TestPollStatsLogSkipsUnusedStats(t *testing.T) {
	var output bytes.Buffer
	new(PollStats).Log(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if output.Len() != 0 {
		t.Errorf("unused polling statistics logged %q", output.String())
	}
}
