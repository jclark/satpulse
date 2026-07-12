package as

import (
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
)

// navMsg builds a NAV-POSLLH carrying the given iTOW (ms), a NavMsg the
// estimator times by its content.
func navMsg(itow uint32) asbin.Msg {
	return &asbin.NavPosLlh{NavITOW: asbin.NavITOW{ITow: itow}}
}

// feedNav feeds a run of NAV-POSLLH arrivals with the given iTOWs, each
// one second of wall clock apart (wall clock is unused for a NavMsg).
func feedNav(e *rateEstimator, itows ...uint32) {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, w := range itows {
		e.observe(navMsg(w), t)
		t = t.Add(time.Second)
	}
}

// rmcSentence builds a valid $GNRMC sentence with the given time-of-day.
func rmcSentence(t *testing.T, tod string) *nmea.Sentence {
	t.Helper()
	payload := "GNRMC," + tod + ",A,5550.60,N,03732.23,E,0.0,0.0,310518,,,A,V"
	data := fmt.Sprintf("$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload)))
	sen := nmea.NewSentence(data)
	if sen == nil {
		t.Fatalf("invalid test sentence: %q", data)
	}
	return sen
}

func TestEstimatorSelfSet(t *testing.T) {
	// A message we set to rate=1 has interval = native period. Two
	// consistent short deltas (three arrivals) conclude the rate; a
	// single delta never does (step immunity).
	tests := []struct {
		name   string
		itows  []uint32
		wantHz int // 0 means not resolved
	}{
		{"5Hz", []uint32{1000, 1200, 1400}, 5},
		{"1Hz", []uint32{1000, 2000, 3000}, 1},
		{"10Hz", []uint32{1000, 1100, 1200}, 10},
		{"2Hz", []uint32{1000, 1500, 2000}, 2},
		{"one_delta_only", []uint32{1000, 1200}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newRateEstimator()
			e.markSelfSet(asbin.NavPosLlhID)
			feedNav(e, tc.itows...)
			if e.nativeHz != tc.wantHz {
				t.Errorf("nativeHz = %d, want %d", e.nativeHz, tc.wantHz)
			}
		})
	}
}

func TestEstimatorPollInterval(t *testing.T) {
	// native = R / interval, exact from a single clean interval of an
	// as-found (not self-set) message whose stored divisor R is polled.
	tests := []struct {
		name     string
		polled   uint8
		itows    []uint32
		wantHz   int
	}{
		{"R1_200ms", 1, []uint32{1000, 1200}, 5},
		{"R5_1000ms", 5, []uint32{1000, 2000}, 5},
		{"R1_100ms", 1, []uint32{1000, 1100}, 10},
		{"R2_400ms", 2, []uint32{1000, 1400}, 5},
		{"R1_1000ms", 1, []uint32{1000, 2000}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newRateEstimator()
			e.cfgMsgReadback(&asbin.CfgMsg{CfgMsgFixed: asbin.CfgMsgFixed{
				MsgClass: 0x01, MsgID: 0x02, Rate: tc.polled}}) // NAV-POSLLH target
			feedNav(e, tc.itows...)
			if e.nativeHz != tc.wantHz {
				t.Errorf("nativeHz = %d, want %d", e.nativeHz, tc.wantHz)
			}
		})
	}
}

func TestEstimatorTimeUTCOffset(t *testing.T) {
	// NAV-TIMEUTC reports iTOW consistently 1ms less than other NAV
	// messages; the constant offset cancels in same-id deltas, so a
	// self-set 5Hz TIMEUTC still resolves at 5.
	e := newRateEstimator()
	e.markSelfSet(asbin.NavTimeUtcID)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, w := range []uint32{999, 1199, 1399} {
		e.observe(&asbin.NavTimeUTC{NavITOW: asbin.NavITOW{ITow: w}}, t0)
		t0 = t0.Add(time.Second)
	}
	if e.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5", e.nativeHz)
	}
}

func TestEstimatorOffGridNoRate(t *testing.T) {
	// Without a fix the TAU1201 emits at irregular sub-second intervals
	// (168/62/172ms observed). Inconsistent deltas must never conclude a
	// fast rate: the estimator stays unresolved (the resolve phase will
	// conclude 1Hz safely).
	e := newRateEstimator()
	e.markSelfSet(asbin.NavPosLlhID)
	feedNav(e, 1000, 1168, 1230, 1402)
	if e.nativeHz != 0 {
		t.Errorf("nativeHz = %d, want 0 (no consistent short deltas)", e.nativeHz)
	}
}

func TestEstimatorITowStep(t *testing.T) {
	// The TAU1201 slews its grid at fix acquisition, producing one large
	// iTOW step. A single stepped delta must not conclude a rate, and
	// must not prevent a later conclusion from two clean deltas.
	e := newRateEstimator()
	e.markSelfSet(asbin.NavPosLlhID)
	feedNav(e, 1000, 1200, 5000) // step at the third arrival
	if e.nativeHz != 0 {
		t.Fatalf("nativeHz = %d after step, want 0 (step must not conclude)", e.nativeHz)
	}
	feedNav2(e, 5200, 5400) // two clean 200ms deltas after the step
	if e.nativeHz != 5 {
		t.Errorf("nativeHz = %d after recovery, want 5", e.nativeHz)
	}
	// The step interval must not pollute the smallest-interval estimate.
	if iv, ok := e.observedInterval(asbin.NavPosLlhID); !ok || iv != 200 {
		t.Errorf("observedInterval = %v/%v, want 200ms", iv, ok)
	}
}

// feedNav2 continues feeding NAV arrivals without resetting the clock.
func feedNav2(e *rateEstimator, itows ...uint32) {
	t := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	for _, w := range itows {
		e.observe(navMsg(w), t)
		t = t.Add(time.Second)
	}
}

func TestEstimatorSilentCap(t *testing.T) {
	// No arrivals at all: the cap concludes 1Hz and flags that the rate
	// could not be verified.
	e := newRateEstimator()
	e.concludeSilent()
	if e.nativeHz != 1 || !e.capped {
		t.Errorf("nativeHz = %d capped = %v, want 1 and true", e.nativeHz, e.capped)
	}
}

func TestEstimatorAnchoredNegative(t *testing.T) {
	// Traffic flowed but only at 1Hz (one 1s delta, not two): the
	// resolve deadline concludes 1Hz, and it is not the silent cap
	// because traffic was seen.
	e := newRateEstimator()
	e.markSelfSet(asbin.NavPosLlhID)
	feedNav(e, 1000, 2000)
	if e.nativeHz != 0 {
		t.Fatalf("nativeHz = %d, want 0 before the deadline", e.nativeHz)
	}
	e.concludeSilent()
	if e.nativeHz != 1 || e.capped {
		t.Errorf("nativeHz = %d capped = %v, want 1 and false", e.nativeHz, e.capped)
	}
}

func TestEstimatorNMEAContentTime(t *testing.T) {
	// A time-bearing NMEA sentence is a periodic arrival timed by its
	// time-of-day; with a polled rate it resolves like a NAV message.
	e := newRateEstimator()
	e.cfgMsgReadback(&asbin.CfgMsg{CfgMsgFixed: asbin.CfgMsgFixed{
		MsgClass: 0xF0, MsgID: 0x05, Rate: 1}}) // RMC target
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.observe(rmcSentence(t, "120000.00"), t0)
	e.observe(rmcSentence(t, "120000.20"), t0.Add(time.Second)) // 200ms content delta
	if e.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5 (RMC 200ms grid)", e.nativeHz)
	}
}

func TestEstimatorNonTimeNMEAIgnored(t *testing.T) {
	// GSV carries no timestamp and repeats within one epoch; it must
	// contribute nothing (no fake-fast interval).
	e := newRateEstimator()
	payload := "GPGSV,3,1,11,01,05,000,20"
	data := fmt.Sprintf("$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload)))
	sen := nmea.NewSentence(data)
	if sen == nil {
		t.Fatal("invalid GSV sentence")
	}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e.observe(sen, t0)
	e.observe(sen, t0.Add(5*time.Millisecond))
	if e.sawPeriodic() {
		t.Error("GSV must not register as periodic traffic")
	}
}

func TestEstimator4ByteReadback(t *testing.T) {
	// A 5Hz-native unit answers a CFG-MSG poll with the 4-byte form; the
	// estimator reads the polled rate from it and, with an interval,
	// resolves the native rate.
	e := newRateEstimator()
	pkt, err := asbin.Serialize(&asbin.CfgMsg{
		CfgMsgFixed: asbin.CfgMsgFixed{MsgClass: 0x01, MsgID: 0x02, Rate: 1},
		PortMask:    []byte{0xFF}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := asbin.ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	e.observe(m, time.Time{})
	feedNav(e, 1000, 1200)
	if e.nativeHz != 5 {
		t.Errorf("nativeHz = %d, want 5", e.nativeHz)
	}
}

func TestEstimatorObservedInterval(t *testing.T) {
	// observedInterval reports the smallest same-id interval, for the
	// msg-phase already-1Hz skip.
	e := newRateEstimator()
	feedNav(e, 1000, 2000, 3000) // 1000ms deltas
	iv, ok := e.observedInterval(asbin.NavPosLlhID)
	if !ok || !isOneHz(iv) {
		t.Errorf("observedInterval = %v/%v, want ~1000ms (1Hz)", iv, ok)
	}
	if _, ok := e.observedInterval(asbin.NavTimeID); ok {
		t.Error("observedInterval for an unseen id should be absent")
	}
}
