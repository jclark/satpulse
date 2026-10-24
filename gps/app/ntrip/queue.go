package ntrip

import (
	"slices"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/scan"
)

// PruningQueueMaxLen is the most packets a PruningQueue holds.
const PruningQueueMaxLen = 64

// PruningQueue is a bounded FIFO that eagerly prunes RTCM packets by
// message type.  Non-RTCM and unformatted packets are kept FIFO until
// the queue hits its size bound, where oldest packets are dropped.
// For RTCM MSM packets, entries in the same epoch are not pruned from
// each other because a single epoch can legitimately contain multiple
// packets with the same message type. The zero value is an empty
// queue. A PruningQueue is not safe for concurrent use.
type PruningQueue struct {
	entries  []pqEntry
	msmEpoch uint64
}

type pqEntry struct {
	pkt      scan.Packet
	msgID    string
	msmEpoch *uint64
}

// Len returns the number of packets in the queue.
func (q *PruningQueue) Len() int {
	return len(q.entries)
}

// Reconnect starts a new MSM epoch, so that MSM packets enqueued after
// a reconnection are never treated as part of the same epoch as those
// enqueued before it.
func (q *PruningQueue) Reconnect() {
	q.msmEpoch++
}

// Enqueue adds pkt to the queue. If pkt is an RTCM packet, it first
// removes queued RTCM packets with the same message ID, except MSM
// packets in the same epoch as pkt. If the queue is then longer than
// PruningQueueMaxLen, it drops the oldest packet and returns it with
// true.
func (q *PruningQueue) Enqueue(pkt scan.Packet) (scan.Packet, bool) {
	if pkt.Format == nil || pkt.Format.Tag() != gpsreg.TagRTCM {
		q.entries = append(q.entries, pqEntry{pkt: pkt})
		return q.dropOldest()
	}
	msgID := pkt.Format.MsgID([]byte(pkt.Data))
	var epoch *uint64
	if mmb, ok := rtcmbin.MultipleMessageBit(pkt.Data); ok {
		ep := q.msmEpoch
		epoch = &ep
		if !mmb {
			q.msmEpoch++
		}
	}
	q.entries = slices.DeleteFunc(q.entries, func(e pqEntry) bool {
		return e.pkt.Format != nil && e.pkt.Format.Tag() == gpsreg.TagRTCM &&
			e.msgID == msgID && (epoch == nil || e.msmEpoch == nil || *epoch != *e.msmEpoch)
	})
	q.entries = append(q.entries, pqEntry{pkt: pkt, msgID: msgID, msmEpoch: epoch})
	return q.dropOldest()
}

// Dequeue removes and returns the oldest packet. The queue must not
// be empty.
func (q *PruningQueue) Dequeue() scan.Packet {
	e := q.entries[0]
	q.entries = q.entries[1:]
	return e.pkt
}

func (q *PruningQueue) dropOldest() (scan.Packet, bool) {
	if len(q.entries) <= PruningQueueMaxLen {
		return scan.Packet{}, false
	}
	e := q.entries[0]
	q.entries = q.entries[1:]
	return e.pkt, true
}
