package ubxsim

import (
	"context"
	"log/slog"
	"time"

	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

// navEngine replays the personality's per-epoch message bank. At each
// epoch tick it emits the currently-enabled subset of that epoch's
// messages as a contiguous burst, the way a real receiver emits its
// enabled set. Enablement is the RAM-layer MSGOUT key for the simulated
// port and the port's OUTPROT key for the message's protocol (see
// enabled); the epoch period is CFG-RATE-MEAS x CFG-RATE-NAV. It enqueues
// each epoch's enabled burst to the writer, which paces the bytes onto the
// line, and then sleeps to the next epoch tick. Only the bank is gated: the
// config engine answers a poll whatever the OUTPROT keys say, so a client
// that disables UBX output can still configure the port back.
type navEngine struct {
	db     *cfgDB
	port   ucv.Port
	epochs [][]Pkt
	w      *writer
	lg     *slog.Logger
}

func (n *navEngine) run(ctx context.Context) {
	if len(n.epochs) == 0 {
		// No message bank: the receiver is silent, as with no antenna.
		return
	}
	next := time.Now()
	for i := 0; ; i++ {
		if i >= len(n.epochs) {
			n.lg.Warn("nav replay bank exhausted", "epochs", len(n.epochs))
			return
		}
		for _, pkt := range n.epochs[i] {
			if !n.enabled(pkt) {
				continue
			}
			if err := n.w.send(ctx, pkt.Data); err != nil {
				return
			}
		}
		next = next.Add(n.epochPeriod())
		if !sleepCtx(ctx, time.Until(next)) {
			return
		}
	}
}

// enabled reports whether the port would put pkt on the line: its MSGOUT
// key must be nonzero, and the port's OUTPROT key for its protocol must be
// on. Both gates are real: a receiver whose port has NMEA output disabled
// emits no NMEA however the per-message rates are set, which is how the
// configurator turns NMEA off (it clears OUTPROT rather than every rate).
func (n *navEngine) enabled(pkt Pkt) bool {
	if n.db.ramUint(pkt.KeyM.KeyU(n.port).Key()) == 0 {
		return false
	}
	k, ok := outProtKey(n.port, pkt.Tag)
	return ok && n.db.ramUint(k.Key()) != 0
}

// epochPeriod returns the current epoch period from the RAM-layer rate
// keys, defaulting to one second if they multiply to zero.
func (n *navEngine) epochPeriod() time.Duration {
	ms := n.db.ramUint(ucv.KRateMeas.Key()) * n.db.ramUint(ucv.KRateNav.Key())
	if ms == 0 {
		return time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

// sleepCtx sleeps for d and reports true, or reports false if ctx is
// done first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
