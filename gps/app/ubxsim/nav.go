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
	reboot chan struct{}
}

func (n *navEngine) run(ctx context.Context) {
	if len(n.epochs) == 0 {
		// No message bank: the receiver is silent, as with no antenna. It
		// still drains reboot signals so restart never blocks.
		for n.idle(ctx) {
		}
		return
	}
	for n.replay(ctx) {
		// replay returned on a reboot: restart the bank from the top.
	}
}

// restart signals the replay loop to reboot, restarting the bank from
// the first epoch. The send is non-blocking, so a reboot arriving while
// one is already pending collapses into it: a real receiver reboots
// once, not once per queued request.
func (n *navEngine) restart() {
	select {
	case n.reboot <- struct{}{}:
	default:
	}
}

// replay emits the enabled bank once, epoch by epoch, pacing to the nav
// rate. It returns true when a reboot interrupts it (the caller restarts
// the bank) and false when the context is cancelled, the writer fails,
// or the bank runs out with no reboot pending.
func (n *navEngine) replay(ctx context.Context) bool {
	next := time.Now()
	for i := 0; i < len(n.epochs); i++ {
		for _, pkt := range n.epochs[i] {
			if !n.enabled(pkt) {
				continue
			}
			if err := n.w.send(ctx, pkt.Data); err != nil {
				return false
			}
		}
		next = next.Add(n.epochPeriod())
		reboot, ok := n.sleep(ctx, time.Until(next))
		if reboot {
			return true
		}
		if !ok {
			return false
		}
	}
	n.lg.Warn("nav replay bank exhausted", "epochs", len(n.epochs))
	// Stay alive rather than exit, so a reboot after exhaustion restarts
	// the replay and the simulator keeps serving.
	return n.idle(ctx)
}

// idle blocks with no output until a reboot signal (returns true) or
// context cancellation (returns false), modelling a receiver that is
// alive but emitting nothing.
func (n *navEngine) idle(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-n.reboot:
		return true
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

// sleep waits until the next epoch tick, a reboot, or context
// cancellation. It reports whether a reboot arrived and, absent one,
// whether the sleep completed (ok false means the context was
// cancelled). A pending reboot is taken first so a ready timer never
// starves it.
func (n *navEngine) sleep(ctx context.Context, d time.Duration) (reboot, ok bool) {
	select {
	case <-n.reboot:
		return true, false
	default:
	}
	if d <= 0 {
		return false, ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false, false
	case <-n.reboot:
		return true, false
	case <-t.C:
		return false, true
	}
}
