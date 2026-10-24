// Package stream moves correction and packet data between the local
// receiver and the network.  Pull connects to a correction source and
// writes scanned packets to a serial port; Push forwards receiver
// packets to a remote destination.  Both handle reconnection
// internally and expose a bcast of scanned packets so callers can
// subscribe for UI updates, logging, or other purposes.
package stream

import (
	"math/rand/v2"
	"time"
)

// State represents the connection state of a Pull or Push.
type State int

const (
	Connecting State = iota
	Connected
	Reconnecting
	// Failed is a terminal state: the stream hit an unrecoverable
	// error (e.g. a rejected NTRIP password) and gave up rather than
	// reconnecting.
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
