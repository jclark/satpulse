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
// port (zero is off, nonzero is on); the epoch period is CFG-RATE-MEAS x
// CFG-RATE-NAV; emission is paced by the configured UART baud rate, so
// the output has the burst-then-idle shape of real hardware.
type navEngine struct {
	db     *cfgDB
	port   ucv.Port
	epochs [][]Pkt
	w      *muxWriter
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
			if n.db.ramUint(pkt.KeyM.KeyU(n.port).Key()) == 0 {
				continue
			}
			if err := n.w.writePacket(pkt.Data); err != nil {
				return
			}
			if !sleepCtx(ctx, n.txTime(len(pkt.Data))) {
				return
			}
		}
		next = next.Add(n.epochPeriod())
		if !sleepCtx(ctx, time.Until(next)) {
			return
		}
	}
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

// txTime returns the time a packet of the given length occupies on the
// line at the configured baud rate (10 bits per byte). A non-UART port
// is not paced.
func (n *navEngine) txTime(length int) time.Duration {
	var k ucv.KeyU
	switch n.port {
	case ucv.UART1:
		k = ucv.KUart1Baudrate
	case ucv.UART2:
		k = ucv.KUart2Baudrate
	default:
		return 0
	}
	baud := n.db.ramUint(k.Key())
	if baud == 0 {
		return 0
	}
	return time.Duration(length*10) * time.Second / time.Duration(baud)
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
