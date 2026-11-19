package as

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestClassifyGap(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name      string
		byTS      time.Duration
		backwards bool
		byArr     time.Duration
		expect    gapKind
	}{
		{name: "exact second", byTS: 1000 * ms, byArr: 1004 * ms, expect: gapTight},
		{name: "edge of tight", byTS: 990 * ms, byArr: 995 * ms, expect: gapTight},
		{name: "slewing second", byTS: 980 * ms, byArr: 991 * ms, expect: gapSecond},
		{name: "lower edge", byTS: 750 * ms, byArr: 750 * ms, expect: gapSecond},
		{name: "upper edge", byTS: 1500 * ms, byArr: 1500 * ms, expect: gapSecond},
		{name: "5Hz", byTS: 200 * ms, byArr: 204 * ms, expect: gapShort},
		{name: "half second", byTS: 500 * ms, byArr: 518 * ms, expect: gapShort},
		{name: "two seconds", byTS: 2000 * ms, byArr: 1998 * ms, expect: gapLong},
		{name: "timestamp jumped forward", byTS: 780 * time.Second, byArr: 1051 * ms, expect: gapOther},
		{name: "timestamp went backwards", byTS: 1000 * ms, backwards: true, byArr: 1000 * ms, expect: gapOther},
		{name: "second by timestamp, short by arrival", byTS: 800 * ms, byArr: 728 * ms, expect: gapOther},
		{name: "second by timestamp, bunched on arrival", byTS: 1000 * ms, byArr: 3 * ms, expect: gapOther},
		{name: "tight by timestamp, late on arrival", byTS: 1000 * ms, byArr: 1600 * ms, expect: gapOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyGap(tc.byTS, tc.backwards, tc.byArr); got != tc.expect {
				t.Errorf("got %v, want %v", got, tc.expect)
			}
		})
	}
}

// watched is what the watcher concludes about one message type.
type watched struct {
	Once, Not bool
}

func TestMsgWatcherGaps(t *testing.T) {
	// each case is a run of NAV-TIME messages given as the gaps between
	// them in ms, by timestamp; they arrive when stamped unless arrive
	// says otherwise
	tests := []struct {
		name   string
		gaps   []int
		arrive []int
		after  time.Duration // how long after the last message the question is asked
		expect watched
	}{
		{name: "one message", expect: watched{}},
		{name: "one exact second", gaps: []int{1000}, expect: watched{Once: true}},
		{name: "one rough second is not enough", gaps: []int{980}, expect: watched{}},
		{name: "two rough seconds", gaps: []int{980, 980}, expect: watched{Once: true}},
		{name: "TAU1201 at first fix", gaps: []int{800, 500}, arrive: []int{728, 518}, expect: watched{}},
		{name: "TAU1201 after first fix", gaps: []int{800, 500, 980, 980}, arrive: []int{728, 518, 998, 991}, expect: watched{Once: true}},
		{name: "TAU951M steering", gaps: []int{180, 180, 180}, expect: watched{Not: true}},
		{name: "one short gap rules nothing out", gaps: []int{1000, 200}, expect: watched{}},
		{name: "steady 5Hz", gaps: []int{200, 200}, expect: watched{Not: true}},
		{name: "second across a steering second then steady", gaps: []int{920, 1000}, expect: watched{Once: true}},
		{name: "long gap", gaps: []int{1600}, expect: watched{Not: true}},
		{name: "gone silent", gaps: []int{1000}, after: 1500 * time.Millisecond, expect: watched{Once: true, Not: true}},
		{name: "time set between messages", gaps: []int{780000000}, arrive: []int{1051}, expect: watched{}},
		{name: "repeat within an epoch is ignored", gaps: []int{1000, 0}, arrive: []int{1000, 5}, expect: watched{Once: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newMsgWatcher()
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			ts := uint32(35222790)
			w.observe(&asbin.NavTime{RefTow: ts}, now)
			for i, g := range tc.gaps {
				arr := g
				if tc.arrive != nil {
					arr = tc.arrive[i]
				}
				ts += uint32(g)
				now = now.Add(time.Duration(arr) * time.Millisecond)
				w.observe(&asbin.NavTime{RefTow: ts}, now)
			}
			tr := w.tracks[asbin.NavTimeID]
			got := watched{tr.oncePerSecond(), tr.notOncePerSecond(now.Add(tc.after))}
			if got != tc.expect {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func sentenceOf(t *testing.T, body string) *nmea.Sentence {
	t.Helper()
	sen := nmea.NewSentence(string(nmeaPacket(body)))
	if sen == nil {
		t.Fatalf("invalid test sentence: %q", body)
	}
	return sen
}

func TestMsgWatcherNMEA(t *testing.T) {
	type result struct {
		Tracks   []asbin.MsgID
		Once     bool
		Overflow bool
	}
	tests := []struct {
		name   string
		bodies []string // one per second
		expect result
	}{
		{
			name:   "RMC once per second",
			bodies: []string{"GNRMC,094652.000,A,,,,,,,050726,,,A,S", "GNRMC,094653.000,A,,,,,,,050726,,,A,S"},
			expect: result{Tracks: []asbin.MsgID{asbin.NmeaRmcID}, Once: true},
		},
		{
			name:   "GLL time is its fifth field",
			bodies: []string{"GNGLL,,,,,094652.000,A,A", "GNGLL,,,,,094653.000,A,A"},
			expect: result{Tracks: []asbin.MsgID{asbin.NmeaGllID}, Once: true},
		},
		{
			name:   "empty time gives nothing",
			bodies: []string{"GNGGA,,,,,,0,00,,,M,,M,,", "GNGGA,,,,,,0,00,,,M,,M,,"},
			expect: result{},
		},
		{
			name:   "untimed sentences give nothing",
			bodies: []string{"GNGSA,A,1,,,,,,,,,,,,,,,,1", "GNVTG,,T,,M,,N,,K,N"},
			expect: result{},
		},
		{
			name:   "overflow warning",
			bodies: []string{"GNTXT,01,04,BANDWIDTH OVERFLOW WARNING"},
			expect: result{Overflow: true},
		},
		{
			name:   "other TXT",
			bodies: []string{"GNTXT,01,01,01,ANTENNA OPEN"},
			expect: result{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newMsgWatcher()
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, b := range tc.bodies {
				w.observe(sentenceOf(t, b), now)
				now = now.Add(time.Second)
			}
			got := result{Overflow: w.overflow}
			for _, mid := range w.sortedMids() {
				got.Tracks = append(got.Tracks, mid)
				got.Once = got.Once || w.tracks[mid].oncePerSecond()
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestFastHint(t *testing.T) {
	tests := []struct {
		name   string
		gaps   map[asbin.MsgID][]int // ms between successive messages of each type
		expect uint8
	}{
		{name: "nothing arriving", expect: 0},
		{name: "once per second", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {1000, 1000}}, expect: 0},
		{name: "one short gap", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {200}}, expect: 0},
		{name: "5Hz", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {200, 200}}, expect: 5},
		{name: "5Hz being steered", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {180, 180}}, expect: 5},
		{name: "10Hz", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {100, 100}}, expect: 10},
		{name: "fastest wins", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {200, 200}, asbin.NavTimeUtcID: {100, 100}}, expect: 10},
		{name: "half second", gaps: map[asbin.MsgID][]int{asbin.NavTimeID: {500, 500}}, expect: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newMsgWatcher()
			for mid, gaps := range tc.gaps {
				now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				ts := uint32(100000)
				w.arrival(mid, ts, weekMs, now)
				for _, g := range gaps {
					ts += uint32(g)
					now = now.Add(time.Duration(g) * time.Millisecond)
					w.arrival(mid, ts, weekMs, now)
				}
			}
			if got := w.fastHint(); got != tc.expect {
				t.Errorf("got %d, want %d", got, tc.expect)
			}
		})
	}
}

func TestTodMillis(t *testing.T) {
	type result struct {
		Ms uint32
		OK bool
	}
	tests := []struct {
		in     string
		expect result
	}{
		{"094652.000", result{35212000, true}},
		{"094645.790", result{35205790, true}},
		{"120000.2", result{43200200, true}},
		{"120000", result{43200000, true}},
		{"", result{}},
		{"1200", result{}},
		{"12000x", result{}},
		{"120000,5", result{}},
		{"120000.x", result{}},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			ms, ok := todMillis(tc.in)
			if got := (result{ms, ok}); got != tc.expect {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
