package as

import (
	"log"
	"math"
	"sort"
	"time"

	"github.com/jclark/satpulse/gps/lib/asbin"
)

// The Allystar CFG-MSG Rate field is a divisor of the receiver's native
// measurement cycle, not a frequency, so enabling a message at rate=1 on
// a 5Hz-native unit yields 5Hz output. To honour the 1Hz output contract
// the configurator must divide by the native rate, which is not
// predictable from the chip number and has no CFG-RATE register to read.
// The rate estimator computes it from the spacing of same-id messages:
// exactly whenever any traffic flows, falling back to a bounded wait and
// an optimistic rate=1 only when the receiver is silent. It never
// produces a too-fast divisor - the only way a wrong rate could be
// applied - because every divisor comes from exact interval arithmetic;
// absence of traffic can only ever conclude 1Hz.
//
// The invariant that makes a wrong divisor impossible by construction:
// divisors are computed only from CONTENT-time deltas (asbin NavMsg iTOW,
// NMEA time-of-day), never from wall-clock read-time deltas. A content-time
// delta cannot be shorter than the true native period, whereas a wall-clock
// delta can (delivery coalescing/jitter shortens it), which through rule 1
// (poll+interval) or rule 2 (self-set) could yield a too-small - too-fast -
// divisor. The one NAV message without iTOW is NAV-AUTO; its arrivals still
// count toward the anchored-negative/cap, they just never produce a divisor.

// oneHzTolMs is how far an observed content-time interval may sit from
// 1000ms and still count as 1Hz output. Only content time feeds this
// decision (wall-clock intervals are barred), so intervals land on exact
// multiples of the native period; the band is tight enough that no
// realizable non-1Hz grid interval falls inside (a 5Hz unit's nearest
// values are 800ms and 1200ms).
const oneHzTolMs = 100

// plausibleHz is the set of native rates seen in the field. It is a
// sanity check only: a computed rate outside it is logged as a surprise
// and then used anyway (10Hz is documented Allystar capability, and a
// future faster firmware must degrade to a log line plus a correct
// measurement, never a wrong divisor).
var plausibleHz = map[int]bool{1: true, 2: true, 5: true, 10: true}

// rateEstimator observes message arrivals and computes the receiver's
// native measurement rate (Hz) from the spacing of same-id messages. It
// is fed from both native channels (the messages the data path consumed
// and those it did not) and is shared between the ConfigProtocol that
// creates it and the Configurator, so observation runs continuously from
// the first packet of the session.
type rateEstimator struct {
	tracks         map[asbin.MsgID]*rateTrack
	nativeHz       int         // 0 until resolved
	capped         bool        // resolved by the silent cap: rate could not be verified
	anyRead        bool        // any periodic arrival has been seen
	lastRead       time.Time   // wall clock of the latest message of any kind, for the watch deadline
	lastWasArrival bool        // the most recent observe was a periodic arrival
	lastArrivalMid asbin.MsgID // the id of that arrival, for the watch deadline extension
}

// rateTrack holds what is known about one CFG-MSG target: the spacing of
// its arrivals and, when polled, its stored rate divisor.
type rateTrack struct {
	count       int     // periodic arrivals counted
	lastContent float64 // content time (ms) of the last arrival
	lastRead    time.Time
	hasContent  bool    // this id carries content time (iTOW / NMEA time-of-day)
	minInterval float64 // smallest observed same-id interval (ms); 0 = none yet
	lastDelta   float64 // most recent same-id interval (ms)
	haveDelta   bool    // at least one interval observed
	consistent  bool    // the last two deltas agree (step immunity)
	polledRate  uint8   // R from a CFG-MSG poll of this id
	hasPoll     bool    // polledRate is known
	selfSet     bool    // we set this id to rate=1
}

func newRateEstimator() *rateEstimator {
	return &rateEstimator{tracks: make(map[asbin.MsgID]*rateTrack)}
}

func (e *rateEstimator) track(mid asbin.MsgID) *rateTrack {
	tr := e.tracks[mid]
	if tr == nil {
		tr = &rateTrack{}
		e.tracks[mid] = tr
	}
	return tr
}

// observe feeds one message from the receiver to the estimator. A
// CFG-MSG readback records the polled rate of its target; a NAV message
// is a periodic arrival timed by its iTOW; an NMEA time-bearing sentence
// is a periodic arrival timed by its time-of-day. Everything else
// (ACKs, MON-VER, non-time NMEA sentences) is ignored: non-time-bearing
// NMEA repeats within one epoch and would fabricate fast intervals.
func (e *rateEstimator) observe(msg any, tRead time.Time) {
	e.lastWasArrival = false
	if tRead.After(e.lastRead) {
		e.lastRead = tRead
	}
	switch m := msg.(type) {
	case *asbin.CfgMsg:
		e.cfgMsgReadback(m)
	case asbin.NavMsg:
		e.arrival(m.ID(), float64(m.NavEpoch()), true, tRead)
	case asbin.Msg:
		if cls, _ := m.ID().Unpack(); cls == navClass {
			// A NAV message without iTOW (NAV-AUTO): count it by wall
			// clock for the anchored-negative/cap, but hasContent is false
			// so it never produces an interval - a wall-clock delta can be
			// shortened below the true native period, and only content time
			// may set a divisor.
			e.arrival(m.ID(), 0, false, tRead)
		}
	case timedSentence:
		if mid, content, ok := nmeaArrival(m); ok {
			e.arrival(mid, content, true, tRead)
		}
	}
}

const navClass = 0x01

// timedSentence is the part of an NMEA sentence the estimator needs,
// declared structurally so this package need not import the nmea
// package (which would cycle through the nmea tests). *nmea.Sentence
// satisfies it.
type timedSentence interface {
	AddressField() string
	TimeOfDay() (string, bool)
}

// nmeaArrival maps a time-bearing NMEA sentence to its CFG-MSG target id
// and content time (time-of-day in ms). Only RMC, GGA, GLL, and ZDA
// carry a timestamp; other sentences report false from TimeOfDay and
// contribute nothing.
func nmeaArrival(sen timedSentence) (asbin.MsgID, float64, bool) {
	tod, ok := sen.TimeOfDay()
	if !ok {
		return 0, 0, false
	}
	mid, ok := nmeaFormatID(sen.AddressField())
	if !ok {
		return 0, 0, false
	}
	ms, ok := todMillis(tod)
	if !ok {
		return 0, 0, false
	}
	return mid, ms, true
}

// nmeaFormatID maps an NMEA address field (talker + format, e.g. GNRMC)
// to the CFG-MSG target id of its sentence format.
func nmeaFormatID(addr string) (asbin.MsgID, bool) {
	if len(addr) < 3 {
		return 0, false
	}
	switch addr[len(addr)-3:] {
	case "RMC":
		return asbin.NmeaRmcID, true
	case "GGA":
		return asbin.NmeaGgaID, true
	case "GLL":
		return asbin.NmeaGllID, true
	case "ZDA":
		return asbin.NmeaZdaID, true
	}
	return 0, false
}

// todMillis parses an NMEA hhmmss.ss time-of-day field into milliseconds
// since midnight. A constant per-id offset would cancel in same-id
// deltas, so the absolute value need not be exact, only consistent.
func todMillis(s string) (float64, bool) {
	if len(s) < 6 {
		return 0, false
	}
	var hh, mm, ss int
	var frac float64
	for i, r := range s[:6] {
		if r < '0' || r > '9' {
			return 0, false
		}
		d := int(r - '0')
		switch i {
		case 0:
			hh = d * 10
		case 1:
			hh += d
		case 2:
			mm = d * 10
		case 3:
			mm += d
		case 4:
			ss = d * 10
		case 5:
			ss += d
		}
	}
	if len(s) > 6 {
		if s[6] != '.' {
			return 0, false
		}
		var scale = 0.1
		for _, r := range s[7:] {
			if r < '0' || r > '9' {
				return 0, false
			}
			frac += float64(r-'0') * scale
			scale /= 10
		}
	}
	return (float64(hh*3600+mm*60+ss) + frac) * 1000, true
}

// cfgMsgReadback records the polled rate of a CFG-MSG target from a poll
// response. It is not a periodic arrival.
func (e *rateEstimator) cfgMsgReadback(m *asbin.CfgMsg) {
	tr := e.track(asbin.MakeMsgID(m.MsgClass, m.MsgID))
	tr.polledRate = m.Rate
	tr.hasPoll = true
	e.tryResolve()
}

// arrival records one periodic arrival of mid. When hasContent is true,
// content is the message's content time (ms) and feeds the divisor rules;
// a wall-clock-only arrival (hasContent false) still counts toward the
// anchored-negative/cap but never sets an interval, because a wall-clock
// delta can be shorter than the true native period and only content time
// may yield a divisor. A zero or negative content delta (a repeat within
// one epoch, or an iTOW step backwards at fix acquisition) is discarded.
func (e *rateEstimator) arrival(mid asbin.MsgID, content float64, hasContent bool, tRead time.Time) {
	e.anyRead = true
	e.lastWasArrival = true
	e.lastArrivalMid = mid
	tr := e.track(mid)
	if hasContent && tr.count > 0 {
		delta := content - tr.lastContent
		if delta > 0 {
			if tr.minInterval == 0 || delta < tr.minInterval {
				tr.minInterval = delta
			}
			tr.consistent = tr.haveDelta && closeEnough(delta, tr.lastDelta)
			tr.lastDelta = delta
			tr.haveDelta = true
		}
	}
	tr.count++
	tr.lastRead = tRead
	if hasContent {
		tr.lastContent = content
		tr.hasContent = true
	}
	e.tryResolve()
}

// closeEnough reports whether two intervals agree closely enough to be
// two samples of the same period (25% or 30ms, whichever is larger).
func closeEnough(a, b float64) bool {
	diff := math.Abs(a - b)
	tol := 0.25 * math.Max(a, b)
	if tol < 30 {
		tol = 30
	}
	return diff <= tol
}

// sortedMids returns the track ids in ascending order. Iteration over the
// tracks map is otherwise random, which would make the query-phase poll id
// and the resolution choice differ run to run (replay traces must not).
func (e *rateEstimator) sortedMids() []asbin.MsgID {
	mids := make([]asbin.MsgID, 0, len(e.tracks))
	for mid := range e.tracks {
		mids = append(mids, mid)
	}
	sort.Slice(mids, func(i, j int) bool { return mids[i] < mids[j] })
	return mids
}

// tryResolve computes nativeHz from present evidence, by the first rule
// that fires: poll+interval (exact, any value), then a self-set interval
// confirmed by two consistent deltas (step immunity). Both rules use only
// content-time evidence (hasContent): a wall-clock interval could be short
// enough to set a too-fast divisor. Ids are visited in a defined order so
// the choice is deterministic. The 1Hz-from-absence rules live in the
// resolve phase's timeout, not here.
func (e *rateEstimator) tryResolve() {
	if e.nativeHz != 0 {
		return
	}
	mids := e.sortedMids()
	for _, mid := range mids {
		tr := e.tracks[mid]
		if tr.hasContent && tr.hasPoll && tr.polledRate >= 1 && tr.minInterval > 0 {
			if hz := hzFromInterval(tr.minInterval, int(tr.polledRate)); hz >= 1 {
				e.set(hz, mid, "poll+interval")
				return
			}
		}
	}
	for _, mid := range mids {
		tr := e.tracks[mid]
		if tr.hasContent && tr.selfSet && tr.consistent {
			if hz := hzFromInterval(tr.lastDelta, 1); hz >= 1 {
				e.set(hz, mid, "self-set interval")
				return
			}
		}
	}
}

// hzFromInterval returns the native rate from an observed interval of a
// message whose stored divisor is r: native = round(1000 * r / interval).
func hzFromInterval(intervalMs float64, r int) int {
	return int(math.Round(float64(r) * 1000 / intervalMs))
}

// set records the resolved native rate, logging a rate outside the
// plausible set as a surprise (but using it anyway).
func (e *rateEstimator) set(hz int, mid asbin.MsgID, how string) {
	e.nativeHz = hz
	if !plausibleHz[hz] {
		log.Printf("as: computed native rate %dHz from %v (%s) is outside the expected set {1,2,5,10}; using it anyway", hz, mid, how)
	}
}

// concludeSilent resolves the rate when no computation was possible: the
// receiver produced no fast traffic, so it is 1Hz. It is called at the
// resolve-phase deadline. capped records that the rate could not be
// verified (the receiver was silent - likely no fix).
func (e *rateEstimator) concludeSilent() {
	if e.nativeHz != 0 {
		return
	}
	e.nativeHz = 1
	if !e.anyRead {
		e.capped = true
		log.Printf("as: receiver silent during configuration; assuming 1Hz native rate (could not verify - likely no fix)")
	}
}

// observedInterval returns the smallest observed same-id interval (ms)
// for mid, if any, for the msg-phase already-1Hz skip decision.
func (e *rateEstimator) observedInterval(mid asbin.MsgID) (float64, bool) {
	tr := e.tracks[mid]
	if tr == nil || tr.minInterval == 0 {
		return 0, false
	}
	return tr.minInterval, true
}

// isOneHz reports whether an observed interval means 1Hz output.
func isOneHz(intervalMs float64) bool {
	return math.Abs(intervalMs-1000) <= oneHzTolMs
}

// sawPeriodic reports whether any id has produced at least two arrivals,
// i.e. traffic is flowing and an interval is (or will soon be) available.
func (e *rateEstimator) sawPeriodic() bool {
	for _, tr := range e.tracks {
		if tr.count >= 2 {
			return true
		}
	}
	return false
}

// flowingPollID returns a flowing id to poll for its stored rate,
// preferring one in keep (ids the request will not disable, so they
// keep producing the second arrival that turns the polled rate into a
// native rate). Returns false if nothing is flowing.
func (e *rateEstimator) flowingPollID(keep map[asbin.MsgID]bool) (asbin.MsgID, bool) {
	var best asbin.MsgID
	bestCount := 0
	bestKept := false
	bestContent := false
	for _, mid := range e.sortedMids() {
		tr := e.tracks[mid]
		if tr.count < 1 {
			continue
		}
		kept := keep[mid]
		// Prefer a content-bearing id (only its interval turns the polled
		// rate into a native rate), then a kept id, then the most-arrived.
		// Sorted iteration makes the tie-break deterministic.
		better := best == 0 ||
			(tr.hasContent && !bestContent) ||
			(tr.hasContent == bestContent && kept && !bestKept) ||
			(tr.hasContent == bestContent && kept == bestKept && tr.count > bestCount)
		if better {
			best, bestCount, bestKept, bestContent = mid, tr.count, kept, tr.hasContent
		}
	}
	return best, best != 0
}

// lazyPollID returns a content-bearing flowing id whose stored rate is
// not yet known and that we did not self-set, for the resolve-phase
// fallback poll. Only a content-bearing id qualifies: rule 1 pairs the
// polled rate with the id's content-time interval, and only content time
// may set a divisor. A self-set id is excluded: rule 2 covers it, and
// polling an off-grid self-set stream (a fixless unit) could yield a
// spurious rule-1 divisor instead of letting the cap conclude 1Hz. Ids
// are visited in a defined order so the choice is deterministic.
func (e *rateEstimator) lazyPollID() (asbin.MsgID, bool) {
	for _, mid := range e.sortedMids() {
		tr := e.tracks[mid]
		if tr.hasContent && tr.count >= 1 && !tr.hasPoll && !tr.selfSet {
			return mid, true
		}
	}
	return 0, false
}

// markSelfSet records that we have set mid to rate=1, so a clean interval
// on it is the native period directly.
func (e *rateEstimator) markSelfSet(mid asbin.MsgID) {
	e.track(mid).selfSet = true
}
