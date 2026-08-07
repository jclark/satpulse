package serialpps

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestClassifyReading(t *testing.T) {
	base := time.Unix(1_000, 0)
	asserted := gpsio.ModemControlPinState(1 << gpsio.ModemCTS)
	tests := []struct {
		name       string
		curState   gpsio.ModemControlPinState
		curAt      time.Duration
		deadline   time.Duration
		wantEdgeAt time.Duration
		wantMissed bool
	}{
		{
			name:       "transition before deadline",
			curAt:      4 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantEdgeAt: 2 * time.Millisecond,
		},
		{
			name:       "transition crossing deadline",
			curAt:      12 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantEdgeAt: 6 * time.Millisecond,
		},
		{
			name:     "no transition before deadline",
			curState: asserted,
			curAt:    4 * time.Millisecond,
			deadline: 5 * time.Millisecond,
		},
		{
			name:       "no transition crossing deadline",
			curState:   asserted,
			curAt:      6 * time.Millisecond,
			deadline:   5 * time.Millisecond,
			wantMissed: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := reading{state: asserted, at: base}
			cur := reading{state: tc.curState, at: base.Add(tc.curAt)}
			edge, missed := classifyReading(prev, cur, Wiring{Pin: gpsio.ModemCTS}, base.Add(tc.deadline))
			if missed != tc.wantMissed {
				t.Errorf("missed = %v, want %v", missed, tc.wantMissed)
			}
			if tc.wantEdgeAt == 0 {
				if !edge.IsZero() {
					t.Errorf("edge = %v, want zero", edge)
				}
			} else if want := base.Add(tc.wantEdgeAt); !edge.Equal(want) {
				t.Errorf("edge = %v, want %v", edge, want)
			}
		})
	}
}

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
			g := NewGenerator()
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
			sample, ok := g.Edge(Edge{T: tEdge})
			if ok != tc.ok {
				t.Fatalf("Edge ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			wantRef := time.Unix(1_000, 0).UTC()
			if !sample.Reference.Equal(wantRef) {
				t.Errorf("reference = %v, want %v", sample.Reference, wantRef)
			}
			if !sample.System.Equal(tEdge) {
				t.Errorf("system = %v, want %v", sample.System, tEdge)
			}
			if sample.Leap != tc.leap {
				t.Errorf("leap = %v, want %v", sample.Leap, tc.leap)
			}
		})
	}
}

func TestGeneratorKeepsNewestMessage(t *testing.T) {
	g := NewGenerator()
	newRead := time.Unix(100, 100_000_000)
	g.MsgUTCTime(time.Unix(200, 0), newRead, ptime.LeapSecondPositive)
	g.MsgUTCTime(time.Unix(300, 0), newRead.Add(-time.Second), ptime.LeapSecondNegative)
	sample, ok := g.Edge(Edge{T: time.Unix(100, 0)})
	if !ok {
		t.Fatal("Edge returned no sample")
	}
	if !sample.Reference.Equal(time.Unix(200, 0)) || sample.Leap != ptime.LeapSecondPositive {
		t.Fatalf("sample = %+v, want newest message reference and leap", sample)
	}
}

func TestGeneratorSuppressesLeapAcrossDayBoundary(t *testing.T) {
	g := NewGenerator()
	utc := time.Unix(86_399, 500_000_000).UTC()
	read := time.Unix(86_399, 600_000_000)
	g.MsgUTCTime(utc, read, ptime.LeapSecondPositive)
	sample, ok := g.Edge(Edge{T: read.Add(900 * time.Millisecond)})
	if !ok {
		t.Fatal("Edge returned no sample")
	}
	if !sample.Reference.Equal(time.Unix(86_400, 0)) || sample.Leap != ptime.LeapSecondNone {
		t.Fatalf("sample = %+v, want midnight reference and no leap", sample)
	}
}
