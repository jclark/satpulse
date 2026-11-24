package as

import (
	"sort"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/asbin"
)

// The Allystar CFG-MSG Period is a count of the receiver's update cycles,
// not a time: Period 1 gives one message per second on a 1Hz receiver and
// five on a 5Hz receiver. The update rate is 1, 5 or 10Hz, cannot be read
// or set, and is not reliably implied by the chip. So the Period that
// gives one message per second is found by watching: a message that is
// seen arriving once per second has the right Period, whatever the
// update rate is. No rate is ever calculated from a gap, because the gap
// between two messages is not always a whole number of update periods:
// receivers step and slew their epochs around start-up, with nothing in
// the messages to mark it.
//
// The gap between consecutive messages of one type is measured twice, by
// the timestamps in the messages (NAV time of week, NMEA time of day) and
// by their arrival times, and the two must agree: the timestamps jump
// when the receiver sets its time, and arrival times bunch on the line.
//
// With Period 1, 5 or 10 on a 1, 5 or 10Hz receiver the steady gaps
// nearest to one second are 0.5s and 2s. gapMin and gapMax are the
// halfway points.
const (
	gapMin = 750 * time.Millisecond
	gapMax = 1500 * time.Millisecond
)

// gapTightTol is how far from one second a single gap may be and still
// show, alone, that a message is arriving once per second. A steady gap
// is a multiple of 100ms, so the nearest alternatives are 900 and 1100ms;
// this is a tenth of that separation.
const gapTightTol = 10 * time.Millisecond

// fastGapSplit separates the gaps of a message arriving at 10Hz from one
// arriving at 5Hz, for choosing which Period to try first.
const fastGapSplit = 150 * time.Millisecond

const (
	weekMs = 7 * 24 * 3600 * 1000
	dayMs  = 24 * 3600 * 1000
)

// msgWatcher records, for each timestamped message type, the gaps between
// its recent messages. It is fed every message from the first packet of
// the session, by the ConfigProtocol that creates it, and is shared with
// the Configurator.
type msgWatcher struct {
	tracks   map[asbin.MsgID]*msgTrack
	first    time.Time // arrival of the first message of any kind
	last     time.Time // arrival of the latest message of any kind
	overflow bool      // the receiver has reported that it is dropping output
}

// msgTrack is the recent history of one message type.
type msgTrack struct {
	ts    uint32     // timestamp of the latest message, ms
	arr   time.Time  // arrival of the latest message
	ngaps int        // gaps seen, saturating at 2
	gaps  [2]gapKind // the latest gap and the one before
	gap   time.Duration
}

// gapKind classifies one gap between consecutive messages of a type.
type gapKind uint8

const (
	gapOther  gapKind = iota // the measurements disagree, or the timestamp went backwards
	gapSecond                // about a second by both measurements
	gapTight                 // gapSecond, and within gapTightTol of a second by the timestamps
	gapShort                 // under gapMin by both measurements
	gapLong                  // over gapMax by both measurements
)

func newMsgWatcher() *msgWatcher {
	return &msgWatcher{tracks: make(map[asbin.MsgID]*msgTrack)}
}

// sentence is the part of an NMEA sentence the watcher needs, declared
// structurally so this package need not import the nmea package (which
// would cycle through the nmea tests). *nmea.Sentence satisfies it.
type sentence interface {
	AddressField() string
	TimeOfDay() (string, bool)
	Body() string
}

// observe feeds one message from the receiver to the watcher. NAV
// messages are timed by their time of week and RMC, GGA, GLL and ZDA by
// their time of day; other messages only mark that the receiver is
// sending. RTCM epochs are not used: a receiver sending nothing else is
// handled by setting a NAV message and watching that.
func (w *msgWatcher) observe(msg any, tRead time.Time) {
	if w.first.IsZero() {
		w.first = tRead
	}
	if tRead.After(w.last) {
		w.last = tRead
	}
	switch m := msg.(type) {
	case asbin.NavMsg:
		w.arrival(m.ID(), m.NavEpoch(), weekMs, tRead)
	case sentence:
		if strings.HasSuffix(m.AddressField(), "TXT") && strings.Contains(m.Body(), "BANDWIDTH OVERFLOW") {
			w.overflow = true
		}
		if mid, ts, ok := nmeaArrival(m); ok {
			w.arrival(mid, ts, dayMs, tRead)
		}
	}
}

// arrival records one message of type mid with timestamp ts, which wraps
// at mod. A message with the same timestamp as the last is a repeat
// within one epoch and is ignored.
func (w *msgWatcher) arrival(mid asbin.MsgID, ts, mod uint32, tRead time.Time) {
	tr := w.tracks[mid]
	if tr == nil {
		w.tracks[mid] = &msgTrack{ts: ts, arr: tRead}
		return
	}
	dts := (ts%mod + mod - tr.ts%mod) % mod
	if dts == 0 {
		return
	}
	tr.gaps[1] = tr.gaps[0]
	tr.gap = time.Duration(dts) * time.Millisecond
	tr.gaps[0] = classifyGap(tr.gap, dts > mod/2, tRead.Sub(tr.arr))
	if tr.ngaps < 2 {
		tr.ngaps++
	}
	tr.ts = ts
	tr.arr = tRead
}

// classifyGap classifies a gap measured as byTS by the timestamps and
// byArr by arrival time.
func classifyGap(byTS time.Duration, backwards bool, byArr time.Duration) gapKind {
	if backwards {
		return gapOther
	}
	if byTS < gapMin && byArr < gapMin {
		return gapShort
	}
	if byTS > gapMax && byArr > gapMax {
		return gapLong
	}
	if byTS < gapMin || byTS > gapMax || byArr < gapMin || byArr > gapMax {
		return gapOther
	}
	if d := byTS - time.Second; d >= -gapTightTol && d <= gapTightTol {
		return gapTight
	}
	return gapSecond
}

// oncePerSecond reports whether the message type is arriving once per
// second: one gap that is tight, or two in a row that are about a
// second. One disturbance can make a single rough gap anything; two in a
// row needs two disturbances.
func (tr *msgTrack) oncePerSecond() bool {
	if tr == nil || tr.ngaps == 0 {
		return false
	}
	if tr.gaps[0] == gapTight {
		return true
	}
	return tr.ngaps == 2 && tr.gaps[0] == gapSecond && (tr.gaps[1] == gapSecond || tr.gaps[1] == gapTight)
}

// tooFast reports whether the last two gaps were both short.
func (tr *msgTrack) tooFast() bool {
	return tr != nil && tr.ngaps == 2 && tr.gaps[0] == gapShort && tr.gaps[1] == gapShort
}

// notOncePerSecond reports whether the message type is clearly not
// arriving once per second as of now: its latest gap was long, it has
// been silent for gapMax, or it is too fast. This only rules a message
// out, so a wrong call costs time and never a wrong Period.
func (tr *msgTrack) notOncePerSecond(now time.Time) bool {
	if tr.ngaps > 0 && tr.gaps[0] == gapLong {
		return true
	}
	return now.Sub(tr.arr) >= gapMax || tr.tooFast()
}

// sortedMids returns the track ids in ascending order, so that choices
// among them do not depend on map order (replay traces must not).
func (w *msgWatcher) sortedMids() []asbin.MsgID {
	mids := make([]asbin.MsgID, 0, len(w.tracks))
	for mid := range w.tracks {
		mids = append(mids, mid)
	}
	sort.Slice(mids, func(i, j int) bool { return mids[i] < mids[j] })
	return mids
}

// fastHint returns the Period suggested by the messages that are too
// fast: a message cannot arrive more often than the update rate, so gaps
// of about 100ms mean 10Hz and gaps of about 200ms mean at least 5Hz. It
// only orders the trials; whatever it suggests must still be seen
// arriving once per second. It returns 0 when nothing is too fast.
func (w *msgWatcher) fastHint() uint8 {
	var hint uint8
	for _, tr := range w.tracks {
		if !tr.tooFast() {
			continue
		}
		if tr.gap < fastGapSplit {
			return 10
		}
		hint = 5
	}
	return hint
}

// nmeaArrival maps a time-bearing NMEA sentence to its CFG-MSG target id
// and timestamp (time of day in ms). Only RMC, GGA, GLL and ZDA carry a
// time; a sentence whose time field is empty gives nothing.
func nmeaArrival(sen sentence) (asbin.MsgID, uint32, bool) {
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

// todMillis parses an NMEA hhmmss.sss time-of-day field into milliseconds
// since midnight.
func todMillis(s string) (uint32, bool) {
	if len(s) < 6 {
		return 0, false
	}
	var d [6]uint32
	for i := range d {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		d[i] = uint32(s[i] - '0')
	}
	ms := ((d[0]*10+d[1])*3600 + (d[2]*10+d[3])*60 + d[4]*10 + d[5]) * 1000
	if len(s) == 6 {
		return ms, true
	}
	if s[6] != '.' {
		return 0, false
	}
	scale := uint32(100)
	for _, r := range s[7:] {
		if r < '0' || r > '9' {
			return 0, false
		}
		ms += uint32(r-'0') * scale
		scale /= 10
	}
	return ms, true
}
