package timemsg

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/ptime"
)

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

// NewBuffer creates a new Buffer with the specified read window.
// The readWindow determines how far back in time to keep messages.
func NewBuffer(lg *slog.Logger, readWindow time.Duration, ls ptime.LeapSecond, timeGNSS gpsprot.GNSS) *Buffer {
	return &Buffer{
		readWindow: readWindow,
		ls:         ls,
		lg:         lg,
		timeGNSS:   timeGNSS,
	}
}

// Time implements gpsprot.MsgHandler.
func (buf *Buffer) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	cutoff := tRead.Add(-buf.readWindow)
	if msg.PulseOffset != nil {
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
func (buf *Buffer) GetPostTimeMessages(n int) (lastSec ptime.Time, tRead []time.Time) {
	level := levelEmpty
	defer func() {
		buf.noteBufMsgLevel(level)
	}()
	entries := buf.bestEntries(&level)
	start := epochStartMsg(entries)
	if start == nil {
		return 0, nil
	}
	level = levelMultipleTimes
	entries = entriesSameType(entries, start)
	// At this point all entries are of the same type.
	tRead = make([]time.Time, 0, len(entries))
	secs := make([]ptime.Time, 0, len(entries))
	for _, e := range entries {
		sec := e.msg.TAITime
		if sec.IsZero() {
			sec = buf.ls.UTCtoTime(*e.msg.UTCTime)
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
		return 0, nil
	}
	level = levelSufficient
	secs = secs[len(secs)-n:]
	tRead = tRead[len(tRead)-n:]
	for i := 1; i < len(secs); i++ {
		if secs[i].Sub(secs[i-1]) != time.Second {
			return 0, nil
		}
	}
	level = levelConsecutive
	return secs[len(secs)-1], tRead
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
			if *entries[i].msg.UTCTime == *utc {
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
	if e.msg.TAITime.IsZero() && e.msg.UTCTime == nil {
		return false
	}
	*level = max(*level, levelTime)
	if e.msg.Ref == gpsprot.PrePulse {
		return false
	}
	*level = levelPost
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
		if m.PulseOffset != nil && t == refTime {
			return buf.validatePulseOffset(m)
		}
		// Since pulse offsets of each Ref type are in order, stop if we've gone past the time we're looking for
		if t < refTime && m.Ref == lastCorr.Ref {
			break
		}
	}
	return 0, false
}

// maxPulseOffset is the maximum acceptable pulse offset in nanoseconds.
const maxPulseOffset = 100

// validatePulseOffset checks that the PulseOffset in the message is within acceptable limits.
// It returns the PulseOffset as a time.Duration and true if valid, or (0, false) if invalid.
func (buf *Buffer) validatePulseOffset(msg *gpsprot.TimeMsg) (time.Duration, bool) {
	if msg.PulseOffset == nil {
		return 0, false
	}
	off := *msg.PulseOffset
	if math.Abs(off) > maxPulseOffset {
		buf.lg.Warn("pulse offset exceeds acceptable limit", "pulseOffset", off, "limit", maxPulseOffset, "tag", msg.Tag, "msgID", msg.NativeMsgID, "tai", msg.TAITime)
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
