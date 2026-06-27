package timemsg

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
)

// MsgUTCTimer receives UTC time samples derived from GPS time messages.
// Used in serial timing mode (no PHC) to feed chrony SOCK samples.
type MsgUTCTimer interface {
	MsgUTCTime(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind)
}

// MsgTAITimer receives TAI time samples derived from GPS time messages.
// Used in PHC free-running mode (phc.sync = false) to feed phcsample's
// edge labelling.
type MsgTAITimer interface {
	MsgTAITime(tai ptime.Time, tRead time.Time, leap ptime.LeapSecondKind)
}

// Buffer stores recent time messages from a GPS receiver.
// It implements gpsprot.MsgHandler to receive messages and
// provides methods to retrieve sequences of messages for time synchronization.
type Buffer struct {
	gpsprot.DefaultHandler
	entries         []entry
	startIndex      int
	readWindow      time.Duration
	ls              ptime.LeapSecond
	lg              *slog.Logger
	timeGNSS        gpsprot.GNSS     // GNSS system used for time messages; can be zero if unknown
	lastPreCorrMsg  *gpsprot.TimeMsg // last PrePulse msg with a correction
	lastPostCorrMsg *gpsprot.TimeMsg // PostPulse msg with PulseOffset with the greatest TAI time
	msgLevel        bufMsgLevel
	msgUTCTimer     MsgUTCTimer // UTC message sink; nil unless serial timing mode
	lastMsgUTC      time.Time   // UTC second already sent to msgUTCTimer
	msgTAITimer     MsgTAITimer // TAI message sink; nil unless PHC-base-clock mode
	lastMsgTAI      ptime.Time  // TAI second already sent to msgTAITimer
}

type entry struct {
	msg   *gpsprot.TimeMsg
	tRead time.Time
}

type bufMsgLevel int

const (
	levelUnknown       bufMsgLevel = iota
	levelEmpty                     // have no messages at all
	levelHaveMsg                   // have a time message
	levelTime                      // have a message with a valid time
	levelPost                      // have a post-pulse message
	levelMultipleTimes             // have messages with different times
	levelTopOfSecond               // have a message that is top of second aligned
	levelSufficient                // have the number of requested messages
	levelConsecutive               // have the requested messages and they are consecutive
)

// minReadWindow is the lower bound on the read window the Buffer will use,
// regardless of what the caller requests. It must be large enough to cover
// the package's own internal lookback needs (detectInvalidLeap looks back
// ~2s for a same-type entry; getPulseCorrectionLast looks back ~1s for a
// matching refTime).
const minReadWindow = 3 * time.Second

// NewBuffer creates a new Buffer with at least the given read window.
// readWindow is the minimum the caller requires; the Buffer may use a
// larger window to satisfy its own internal needs.
func NewBuffer(lg *slog.Logger, readWindow time.Duration, ls ptime.LeapSecond, timeGNSS gpsprot.GNSS) *Buffer {
	return &Buffer{
		readWindow: max(readWindow, minReadWindow),
		ls:         ls,
		lg:         lg,
		timeGNSS:   timeGNSS,
	}
}

// Time implements gpsprot.MsgHandler.
func (buf *Buffer) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	cutoff := tRead.Add(-buf.readWindow)
	if msg.PulseOffset.IsSet() {
		switch msg.Ref {
		case gpsprot.PrePulse:
			buf.lastPreCorrMsg = msg
		case gpsprot.PostPulse:
			buf.lastPostCorrMsg = msg
		}
	}
	// Find where we will need to start after adding this entry
	for buf.startIndex < len(buf.entries) && buf.entries[buf.startIndex].tRead.Before(cutoff) {
		buf.startIndex++
	}
	// Move the current entries if we can and it would allow us to store the new entry without growing the slice
	if buf.startIndex > 0 && len(buf.entries) < cap(buf.entries) {
		n := len(buf.entries) - buf.startIndex
		copy(buf.entries, buf.entries[buf.startIndex:])
		buf.entries = buf.entries[:n]
		buf.startIndex = 0
	}
	buf.entries = append(buf.entries, entry{msg: msg, tRead: tRead})
	buf.msgUTCTime(msg, tRead)
	buf.msgTAITime(msg, tRead)
}

// SetMsgUTCTimer sets the MsgUTCTimer sink.
// When set, Buffer.Time() calls the sink for each new eligible second.
func (buf *Buffer) SetMsgUTCTimer(t MsgUTCTimer) {
	buf.msgUTCTimer = t
}

// SetMsgTAITimer sets the MsgTAITimer sink.
// When set, Buffer.Time() calls the sink for each new eligible second.
func (buf *Buffer) SetMsgTAITimer(t MsgTAITimer) {
	buf.msgTAITimer = t
}

func (buf *Buffer) msgUTCTime(msg *gpsprot.TimeMsg, tRead time.Time) {
	if buf.msgUTCTimer == nil || !msg.UTCTime.IsSet() {
		return
	}
	ut := msg.UTCTime.Get()
	// Skip leap second (23:59:60)
	if ut.TimeOfDay >= 24*time.Hour {
		return
	}
	// Generally receivers compute a navigation solution at an epoch aligned to a exact fraction of second:
	// at 1Hz every second, at 5Hz every 200ms, etc. Let's call this the nominal time. In native messages,
	// the time reported in the message is often not exactly the nomimal time, but is the navigation solution
	// time computed at clock tick closest to the nominal time. This can be a few microseconds different from the
	// nominal time. You don't see this in NMEA messages, because times are rounded to the nearest millisecond.
	// If there are timing messages from different constellations, these exact times can be different for the
	// same nominal time. Accordingly, we round to the nearest millisecond to recover the nominal time.
	// In the PTP code, we discard samples where the nominal time is not exactly on the second, since the rest
	// of the code assumes one pulse per second. However, here we just pass them on to chrony. This
	// allows users to experiment with using higher-rate messages for timing. We could at some point add a
	// configuration option to discard non-second-aligned messages.
	utc := ut.SysTime().Round(time.Millisecond)
	if !utc.After(buf.lastMsgUTC) {
		return
	}
	buf.lastMsgUTC = utc
	buf.msgUTCTimer.MsgUTCTime(utc, tRead.Add(-time.Duration(msg.ReadDelay)), buf.ls.UTCStateAt(ut).LeapTonight)
}

func (buf *Buffer) msgTAITime(msg *gpsprot.TimeMsg, tRead time.Time) {
	if buf.msgTAITimer == nil {
		return
	}
	// PrePulse messages arrive before the pulse and carry the time of
	// the upcoming pulse; pairing that with the receive-time tRead would
	// feed misaligned points into the wallClock fit.
	if msg.Ref == gpsprot.PrePulse {
		return
	}
	t, ok := buf.msgTAI(msg)
	if !ok {
		return
	}
	// Round to the nominal time; see msgUTCTime for why.
	tai := t.Round(time.Millisecond)
	if tai <= buf.lastMsgTAI {
		return
	}
	buf.lastMsgTAI = tai
	buf.msgTAITimer.MsgTAITime(tai, tRead.Add(-time.Duration(msg.ReadDelay)), buf.ls.StateAt(tai).LeapTonight)
}

// LeapSecond implements gpsprot.MsgHandler.
func (buf *Buffer) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	buf.ls = msg.LeapSecond
}

// GetPostTimeMessages retrieves n time messages.
// It returns the reference time of the last message and
// the read times of all the messages in chronological order.
// The read times must have a valid monotonic part.
// The messages must be for consecutive seconds;
// the reference time of each time message is one greater than the previous one.
// The messages must be the same GNSS message type, which must be of a type that follows the time pulse.
// If there are insufficient messages in the buffer, the slice will be nil and lastSec will be zero.
//
// detectLeap is non-nil only when the selected time messages are
// UTC-source (i.e. start.TAITime.IsZero()). When non-nil, calling it
// with a minDelta runs the invalid-leap check on the buffer's current
// contents for the captured (Tag, NativeMsgID).
func (buf *Buffer) GetPostTimeMessages(n int) (lastSec ptime.Time, tRead []time.Time, detectLeap func(minDelta time.Duration) bool) {
	level := levelEmpty
	defer func() {
		buf.noteBufMsgLevel(level)
	}()
	entries := buf.bestEntries(&level)
	start := epochStartMsg(entries)
	if start == nil {
		return 0, nil, nil
	}
	level = levelMultipleTimes
	entries = entriesSameType(entries, start)
	// At this point all entries are of the same type.
	tRead = make([]time.Time, 0, len(entries))
	secs := make([]ptime.Time, 0, len(entries))
	for _, e := range entries {
		sec := e.msg.TAITime
		if sec.IsZero() {
			sec = buf.ls.UTCtoTime(e.msg.UTCTime.Get())
		}
		// With u-blox UBX, the time in navigation solution messages is not-exactly on the second:
		// it's the time of the navigation solution, which is slightly different.
		// However we don't want to round to a second, because the receiver might have been configured to generate
		// multiple solutions per second.
		// Measurement rate is configured in milliseconds, so the nominal time should be a multiple of a millisecond.
		sec = sec.Round(time.Millisecond)
		if sec.Round(time.Second) != sec {
			// It is not a second-aligned message so ignore it.
			continue
		}
		level = levelTopOfSecond
		secs = append(secs, sec)
		tRead = append(tRead, e.tRead.Add(-time.Duration(e.msg.ReadDelay)))
	}
	if len(secs) < n {
		return 0, nil, nil
	}
	level = levelSufficient
	secs = secs[len(secs)-n:]
	tRead = tRead[len(tRead)-n:]
	for i := 1; i < len(secs); i++ {
		if secs[i].Sub(secs[i-1]) != time.Second {
			return 0, nil, nil
		}
	}
	level = levelConsecutive
	if start.TAITime.IsZero() {
		tag, msgID := start.Tag, start.NativeMsgID
		detectLeap = func(minDelta time.Duration) bool {
			return buf.detectInvalidLeap(tag, msgID, minDelta)
		}
	}
	return secs[len(secs)-1], tRead, detectLeap
}

func (buf *Buffer) noteBufMsgLevel(level bufMsgLevel) {
	lastLevel := buf.msgLevel
	if level == lastLevel {
		return
	}
	buf.msgLevel = level
	failLevel := level + 1 // the level we failed to reach
	var msg string
	switch failLevel {
	case levelHaveMsg:
		msg = "no time messages received"
	case levelTime:
		msg = "GNSS receiver does not have valid time"
	case levelPost:
		msg = "incorrect receiver configuration: only pre-pulse time messages being received"
	case levelMultipleTimes:
		// all messages have the same time, so just wait
	case levelTopOfSecond:
		msg = "time messages not aligned to top of second"
	case levelSufficient:
		// insufficient messages, so just wait
	case levelConsecutive:
		msg = "gap in time messages"
	}
	if msg != "" {
		buf.lg.Warn(msg)
	}
}

// validEntries returns the slice of valid entries.
func (buf *Buffer) validEntries() []entry {
	return buf.entries[buf.startIndex:]
}

// bestEntries returns all the entries that are the best according to compareTimeMsg.
// They must also be eligible.
func (buf *Buffer) bestEntries(level *bufMsgLevel) []entry {
	entries := buf.validEntries()
	if len(entries) == 0 {
		return nil
	}
	*level = levelHaveMsg
	result := make([]entry, 0, len(entries))
	first := 0
	for first < len(entries) && !entries[first].eligible(level) {
		first++
	}
	if first == len(entries) {
		return result
	}
	result = append(result, entries[first])
	for _, e := range entries[first+1:] {
		if !e.eligible(level) {
			continue
		}
		switch buf.compareTimeMsg(e.msg, result[0].msg) {
		case -1:
			result[0] = e
			result = result[:1]
		case 0:
			result = append(result, e)
		}
	}
	return result
}

// Return -1 if a is better an b, 1 if b is better than a, 0 if they are equally good (same type).
func (buf *Buffer) compareTimeMsg(a, b *gpsprot.TimeMsg) int {
	// Prefer messages with TAI time
	if a.TAITime.IsZero() != b.TAITime.IsZero() {
		// a does not have a TAI time so prefer b
		if a.TAITime.IsZero() {
			return 1
		} else {
			return -1
		}
	}
	//  Prefer PostPulse to NavSolution
	if a.Ref != b.Ref {
		if a.Ref == gpsprot.PostPulse {
			return -1
		}
		if b.Ref == gpsprot.PostPulse {
			return 1
		}
	}
	// Prefer native messages over NMEA
	if (a.Tag == gpsreg.TagNMEA) != (b.Tag == gpsreg.TagNMEA) {
		if a.Tag == gpsreg.TagNMEA {
			return 1
		} else {
			return -1
		}
	}
	// Prefer the GNSS system we are using for time messages
	if a.GNSS != b.GNSS {
		if a.GNSS == buf.timeGNSS {
			return -1
		}
		if b.GNSS == buf.timeGNSS {
			return 1
		}
	}
	return 0
}

// epochStartMsg finds the message type that appears first at the start of a new epoch.
// Given a sequence of messages, it looks for the first message that has a different time
// than the initial message. This identifies which message type consistently arrives first
// after each PPS pulse, which minimizes the pulse-to-message delay.
//
// For example, if we receive ZDA at 50ms and RMC at 70ms after each pulse, this function
// will identify ZDA as the epoch start message type because it's the first to arrive
// with a new time value, even if the first message in the entries slice is an RMC message.
//
// Returns the first message with a different time, or nil if all messages have the same time.
// bestEntries ensures that every entry passed here either all have TAI populated or none do,
// so it is safe to compare either the TAI or UTC fields consistently.
func epochStartMsg(entries []entry) *gpsprot.TimeMsg {
	if len(entries) == 0 {
		return nil
	}
	tai := entries[0].msg.TAITime
	utc := entries[0].msg.UTCTime
	for i := 1; i < len(entries); i++ {
		if !tai.IsZero() {
			if entries[i].msg.TAITime == tai {
				continue
			}
		} else {
			if entries[i].msg.UTCTime.Get() == utc.Get() {
				continue
			}
		}
		return entries[i].msg
	}
	return nil
}

func entriesSameType(entries []entry, msg *gpsprot.TimeMsg) []entry {
	result := make([]entry, 0, len(entries))
	for _, e := range entries {
		if e.msg.Tag == msg.Tag && e.msg.NativeMsgID == msg.NativeMsgID {
			result = append(result, e)
		}
	}
	return result
}

func (e *entry) eligible(level *bufMsgLevel) bool {
	if e.msg.TAITime.IsZero() && !e.msg.UTCTime.IsSet() {
		return false
	}
	*level = max(*level, levelTime)
	if e.msg.Ref == gpsprot.PrePulse {
		return false
	}
	*level = levelPost
	return true
}

// detectInvalidLeap detects a backwards leap in the sequence of messages due to out of date firmware
// this needs to be called after each message is added to the buffer
func (buf *Buffer) detectInvalidLeap(tag gpsprot.Tag, nativeMsgID string, minDelta time.Duration) bool {
	entries := buf.validEntries()
	if len(entries) == 0 {
		return false
	}
	last := entries[len(entries)-1]
	if m := last.msg; m.Tag != tag || m.NativeMsgID != nativeMsgID || !m.UTCTime.IsSet() {
		return false
	}
	t2 := last.msg.UTCTime.Get()

	i := len(entries) - 2
	for {
		if i < 0 {
			return false
		}
		if msg := entries[i].msg; msg.Tag == tag && msg.NativeMsgID == nativeMsgID {
			if !msg.UTCTime.IsSet() {
				return false
			}
			break
		}
		i--
	}
	t1 := entries[i].msg.UTCTime.Get()
	delta := t2.Sub(t1)
	if delta > 0 {
		return false
	}
	if delta < 0 {
		// the GPS reported UTC time has actually gone backwards; definitely an invalid leap second
		buf.lg.Warn("detected backwards leap in GPS UTC time; likely due to out of date firmware", "tag", tag, "nativeMsgID", nativeMsgID, "t1", t1, "t2", t2, "leap", delta)
		return true
	}
	tRead1 := entries[i].tRead.Add(-time.Duration(entries[i].msg.ReadDelay))
	tRead2 := last.tRead.Add(-time.Duration(last.msg.ReadDelay))
	tReadDelta := tRead2.Sub(tRead1)
	if tReadDelta < minDelta {
		// not quite strong enough evidence that this is an invalid leap, but duplicate message of same msg ID and same UTC time is suspicious
		buf.lg.Info("suspicious duplicate GPS UTC time message", "tag", tag, "nativeMsgID", nativeMsgID, "t", t1)
		return false
	}
	buf.lg.Warn("detected duplicate GPS UTC time message with same UTC time; likely due to out of date firmware", "tag", tag, "nativeMsgID", nativeMsgID, "t", t1, "tReadDelta", tReadDelta)
	return true
}

// GetPulseCorrection retrieves the pulse offset correction for a given reference time.
// Returns the PulseOffset value if a PrePulse message is found for the given refTime,
// or (0, false) if no correction is available.
// The returned correction satisfies: true_time_of_second = pulse_time + correction
func (buf *Buffer) GetPulseCorrection(refTime ptime.Time) (time.Duration, bool) {
	corr, ok := buf.getPulseCorrectionLast(buf.lastPreCorrMsg, refTime)
	if ok {
		return corr, true
	}
	return buf.getPulseCorrectionLast(buf.lastPostCorrMsg, refTime)
}

func (buf *Buffer) getPulseCorrectionLast(lastCorr *gpsprot.TimeMsg, refTime ptime.Time) (time.Duration, bool) {
	if lastCorr == nil || refTime > lastCorr.TAITime {
		return 0, false
	}
	// we assume that a message with a PulseOffset has a TAI time that is equal to the pulse ref time
	if refTime == lastCorr.TAITime {
		return buf.validatePulseOffset(lastCorr)
	}
	entries := buf.validEntries()
	// Search backwards through entries for messages with the same TimeRef with matching TAI time
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].msg
		t := m.TAITime
		if m.PulseOffset.IsSet() && t == refTime {
			return buf.validatePulseOffset(m)
		}
		// Since pulse offsets of each Ref type are in order, stop if we've gone past the time we're looking for
		if t < refTime && m.Ref == lastCorr.Ref {
			break
		}
	}
	return 0, false
}

// GetTAIPulseCorrection retrieves the PrePulse pulse-offset correction
// for the pulse whose top-of-second has the given TAI time, as float64
// nanoseconds with no rounding. Used in PHC-base-clock mode, where edge
// labels are TAI. PostPulse correction messages are not consulted: in
// PostPulse mode the correction for pulse N arrives after pulse N's
// edge event, which is a different pipeline. The returned correction
// satisfies: true_time_of_second = pulse_time + correction
func (buf *Buffer) GetTAIPulseCorrection(refTime ptime.Time) (float64, bool) {
	lastCorr := buf.lastPreCorrMsg
	if lastCorr == nil {
		return 0, false
	}
	lastTAI, ok := buf.msgTAI(lastCorr)
	if !ok || refTime > lastTAI {
		return 0, false
	}
	if refTime == lastTAI {
		return buf.pulseOffsetNs(lastCorr)
	}
	entries := buf.validEntries()
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].msg
		if !m.PulseOffset.IsSet() || m.Ref != gpsprot.PrePulse {
			continue
		}
		t, ok := buf.msgTAI(m)
		if !ok {
			continue
		}
		if t == refTime {
			return buf.pulseOffsetNs(m)
		}
		if t < refTime {
			break
		}
	}
	return 0, false
}

// msgTAI returns msg's TAI time, preferring the explicit TAITime field
// but falling back to a UTC-derived value via buf.ls. Returns
// (zero, false) when neither is available.
func (buf *Buffer) msgTAI(msg *gpsprot.TimeMsg) (ptime.Time, bool) {
	if !msg.TAITime.IsZero() {
		return msg.TAITime, true
	}
	if msg.UTCTime.IsSet() {
		return buf.ls.UTCtoTime(msg.UTCTime.Get()), true
	}
	return 0, false
}

// maxPulseOffset is the maximum acceptable pulse offset in nanoseconds.
const maxPulseOffset = 100

// pulseOffsetNs returns msg.PulseOffset in nanoseconds if within limits,
// or (0, false) if missing or out of range. Shared validation core for
// both the rounded-Duration accessor and the float64 accessor.
func (buf *Buffer) pulseOffsetNs(msg *gpsprot.TimeMsg) (float64, bool) {
	if !msg.PulseOffset.IsSet() {
		return 0, false
	}
	off := msg.PulseOffset.Get()
	if math.Abs(off) > maxPulseOffset {
		buf.lg.Warn("pulse offset exceeds acceptable limit", "pulseOffset", off, "limit", maxPulseOffset, "tag", msg.Tag, "msgID", msg.NativeMsgID, "tai", msg.TAITime)
		return 0, false
	}
	return off, true
}

// validatePulseOffset checks that the PulseOffset in the message is within acceptable limits.
// It returns the PulseOffset as a time.Duration and true if valid, or (0, false) if invalid.
func (buf *Buffer) validatePulseOffset(msg *gpsprot.TimeMsg) (time.Duration, bool) {
	off, ok := buf.pulseOffsetNs(msg)
	if !ok {
		return 0, false
	}
	return time.Duration(math.Round(off)), true
}

// WaitForPulseCorrection indicates whether we should wait for a pulse correction.
// It assumes that no pulse correction is currently available for refTime (i.e. GetPulseCorrection returned false).
// It should be called after the pulse has been received.
func (buf *Buffer) WaitForPulseCorrection(refTime ptime.Time) bool {
	lpcm := buf.lastPostCorrMsg
	return lpcm != nil && refTime.Sub(lpcm.TAITime) == time.Second
}
