// Package stream moves correction and packet data between the local
// receiver and the network.  Pull connects to a correction source and
// writes scanned packets to a serial port; Push forwards receiver
// packets to a remote destination.  Both handle reconnection
// internally and expose a bcast of scanned packets so callers can
// subscribe for UI updates, logging, or other purposes.
package stream

import (
	"math/rand/v2"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/scan"
)

// State represents the connection state of a Pull or Push.
type State int

const (
	Connecting State = iota
	Connected
	Reconnecting
	// Failed is a terminal state: the stream hit an unrecoverable
	// error (e.g. a rejected NTRIP password) and gave up rather than
	// reconnecting.  Push reports it; Pull always retries, so Pull
	// never reaches Failed today.
	Failed
)

func (s State) String() string {
	switch s {
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Reconnecting:
		return "reconnecting"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// pruningQueue is a FIFO that deduplicates by message type.  When a
// packet with the same message type is enqueued, the older entry is
// removed -- unless both entries belong to the same MSM epoch, in
// which case a single epoch can legitimately contain multiple packets
// with the same message type.
type pruningQueue struct {
	entries  []pqEntry
	msmEpoch uint64
}

type pqEntry struct {
	pkt      scan.Packet
	msgID    string
	msmEpoch *uint64
}

func (q *pruningQueue) len() int {
	return len(q.entries)
}

func (q *pruningQueue) reconnect() {
	q.msmEpoch++
}

func (q *pruningQueue) enqueue(pkt scan.Packet) {
	msgID := pkt.Format.MsgID([]byte(pkt.Data))
	var epoch *uint64
	if pkt.Format.Tag() == gpsreg.TagRTCM {
		if mmb, ok := rtcmbin.MultipleMessageBit(pkt.Data); ok {
			ep := q.msmEpoch
			epoch = &ep
			if !mmb {
				q.msmEpoch++
			}
		}
	}
	q.entries = slices.DeleteFunc(q.entries, func(e pqEntry) bool {
		return e.msgID == msgID && (epoch == nil || e.msmEpoch == nil || *epoch != *e.msmEpoch)
	})
	q.entries = append(q.entries, pqEntry{pkt: pkt, msgID: msgID, msmEpoch: epoch})
}

func (q *pruningQueue) dequeue() scan.Packet {
	e := q.entries[0]
	q.entries = q.entries[1:]
	return e.pkt
}

const (
	backoffInitial    = 1.0 // seconds
	backoffMultiplier = 1.5
	backoffCap        = 3600.0 // 1 hour in seconds
	backoffFloor      = 1.0    // seconds
	backoffDecay      = time.Minute
)

type backoff struct {
	cur float64 // current backoff in seconds
	rng *rand.Rand
}

func newBackoff() *backoff {
	return &backoff{
		cur: backoffInitial,
		rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
	}
}

// increase increases the backoff after a failed connection attempt.
func (b *backoff) increase() {
	b.cur *= backoffMultiplier
	if b.cur > backoffCap {
		b.cur = backoffCap
	}
}

// decrease reduces the backoff.  Called on successful connect and
// periodically while connected.
func (b *backoff) decrease() {
	b.cur /= backoffMultiplier
	if b.cur < backoffFloor {
		b.cur = backoffFloor
	}
}

// delay returns a random duration in [0, b.cur) to wait before
// the next connection attempt.
func (b *backoff) delay() time.Duration {
	return time.Duration(b.rng.Float64() * b.cur * float64(time.Second))
}
