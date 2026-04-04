package ts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/jclark/satpulse/time/lib/ifwait"
	"github.com/jclark/satpulse/time/phc"
	"github.com/jclark/satpulse/time/phctime"
)

type Clock struct {
	phc.Clock
	ifName      string
	eraCounter  atomicEra
	pinIndex    uint32
	chanIndex   uint32
	w           *ifwait.IfWaiter
	DriverFlags phc.DriverFlags
}

type Event struct {
	Kind       EventKind
	Ts         phctime.Time
	TReadMono  phctime.Sample // TReadMono.Sys includes a monotonic time
	TReadWall  phctime.Sample // TReadWall.Sys does not have a monotonic time, but PHC-system offset is more precise
	ResumeFunc func() phctime.Era
}

type EventKind int

const (
	_ EventKind = iota
	EdgeEvent
	PauseEvent
	ResumeEvent
)

const exttsTimeout = 100 * time.Millisecond // if we hit this timeout, then the next one isn't stale
const existTimeout = 30 * time.Second
const logWaitTimeout = time.Second / 2 // log if we have to wait more than this for an interface

type PinDesc struct {
	PinIndex  uint32
	ChanIndex uint32
}

func OpenClock(ctx context.Context, lg *slog.Logger, ifName string, pinDesc PinDesc, wait bool) (*Clock, error) {
	var (
		w   *ifwait.IfWaiter
		err error
	)
	defer func() {
		if w != nil {
			w.Close()
		}
	}()
	if wait {
		w, err = ifwait.NewIfWaiter(ifName)
		if err != nil {
			return nil, err
		}
		err = waitIface(ctx, lg, w, nil, "does not exist", existTimeout)
		if err != nil {
			return nil, err
		}
		err = waitIface(ctx, lg, w, func(flags net.Flags) bool { return flags&net.FlagUp != 0 }, "is down", 0)
		if err != nil {
			return nil, err
		}
	}
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return nil, err
	}
	if phcIndex < 0 {
		return nil, &ConfigError{msg: fmt.Sprintf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)}
	}
	flags, err := phc.IfDriverFlags(ifName)
	if err != nil {
		return nil, err
	}
	if !wait {
		iface, err := net.InterfaceByName(ifName)
		if err != nil {
			return nil, err
		}
		if iface.Flags&net.FlagUp == 0 {
			lg.Warn("interface is down: PTP hardware clock external timestamps may not work", "name", ifName)
		}
	}
	if flags&phc.DriverCarrier != 0 {
		if w == nil {
			w, err = ifwait.NewIfWaiter(ifName)
			if err != nil {
				return nil, err
			}
		}
		err = waitIface(ctx, lg, w, hasCarrier, "has no carrier", 0)
		if err != nil {
			return nil, err
		}
	}
	pc, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, err
	}
	clk := Clock{
		Clock:       *pc,
		ifName:      ifName,
		pinIndex:    pinDesc.PinIndex,
		chanIndex:   pinDesc.ChanIndex,
		DriverFlags: flags,
	}
	// We start off with an era that is certain.
	// Zero era represent stale PHC clock readings.
	clk.eraCounter.inc()
	err = clk.validatePinDesc()
	if err != nil {
		clk.Close()
		return nil, err
	}
	if flags&phc.DriverCarrier != 0 {
		clk.w = w
		w = nil
	}
	return &clk, nil
}

func Close(c *Clock) error {
	var err error
	if c.w != nil {
		err = c.w.Close()
	}
	return errors.Join(c.Close(), err)
}

func (clk *Clock) IfName() string {
	return clk.ifName
}

func waitIface(ctx context.Context, lg *slog.Logger, w *ifwait.IfWaiter, f func(net.Flags) bool, status string, timeout time.Duration) error {
	var timeoutTimer <-chan time.Time
	if timeout != 0 {
		timeoutTimer = time.After(timeout)
	}
	start := time.Now()
	logTimer := time.After(logWaitTimeout)
	ch := w.SetCond(f)
loop:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return w.Err()
			}
			break loop
		case <-logTimer:
			lg.Info("interface not ready; waiting", "name", w.Name(), "status", status)
			logTimer = nil
		case <-timeoutTimer:
			return fmt.Errorf("interface %s %s: waited for %s: giving up", w.Name(), status, existTimeout.String())
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if logTimer == nil {
		lg.Info("waited for interface", "name", w.Name(), "t", time.Since(start).String())
	}
	return nil
}

// ConfigError indicates a configuration that is incompatible with the PTP clock hardware.
type ConfigError struct {
	msg string
}

func (e *ConfigError) Error() string { return e.msg }

func (clk *Clock) validatePinDesc() error {
	var msg string
	if clk.ExttsChanCount() == 0 || clk.PinCount() == 0 {
		msg = fmt.Sprintf("PTP clock %s does not support external timestamping", clk.Path())
	} else if clk.pinIndex >= uint32(clk.PinCount()) {
		msg = fmt.Sprintf("pin index %d is out of range for PTP clock %s: maximum index is %d", clk.pinIndex, clk.Path(), clk.PinCount()-1)
	} else if clk.chanIndex >= uint32(clk.ExttsChanCount()) {
		msg = fmt.Sprintf("channel index %d is out of range for PTP clock %s: maximum index is %d", clk.chanIndex, clk.Path(), clk.ExttsChanCount()-1)
	} else {
		return nil
	}
	return &ConfigError{msg: msg}
}

func StartWorker(ctx context.Context, clk *Clock, lg *slog.Logger) (<-chan Event, error) {
	err := clk.PinSetFunc(clk.pinIndex, phc.PinFuncExtts, clk.chanIndex)
	if err != nil {
		return nil, err
	}
	edges, err := clk.ExttsEnable(clk.chanIndex, true)
	if err != nil {
		return nil, err
	}
	lg.Info("enabled external timestamping on the PTP hardware clock", "device", clk.Path(), "pin", clk.pinIndex, "channel", clk.chanIndex, "edgesPerPulse", edges)
	clk.DriverFlags = clk.DriverFlags.SetEdges(edges)
	ch := make(chan Event, 1)
	go clk.readWorker(ctx, lg, ch)
	return ch, nil
}

const StaleEra phctime.Era = phctime.Era(0)

func hasCarrier(flags net.Flags) bool {
	return flags&net.FlagRunning != 0
}

func noCarrier(flags net.Flags) bool {
	return flags&net.FlagRunning == 0
}

func (clk *Clock) handleNoCarrier(ctx context.Context, lg *slog.Logger, tsCh chan<- Event, _ net.Flags) <-chan net.Flags {
	tsCh <- Event{Kind: PauseEvent}
	lg.Info("carrier lost; waiting for it to return", "interface", clk.ifName)
	clk.logExttsEnable(lg, false)
	wCh := clk.w.SetCond(hasCarrier)
	// Note there's no default case in this select: we block until we get a carrier or cancellation
	select {
	case <-ctx.Done():
		return nil
	case _, ok := <-wCh:
		if !ok {
			lg.Warn("netlink error waiting for interface state change", "err", clk.w.Err(), "interface", clk.ifName)
		} else {
			lg.Info("carrier restored", "interface", clk.ifName)
		}
	}
	tsCh <- Event{
		Kind: ResumeEvent,
		ResumeFunc: func() phctime.Era {
			era := clk.eraCounter.load()
			clk.eraCounter.add(2)
			return era
		},
	}
	clk.logExttsEnable(lg, true)
	return clk.w.SetCond(noCarrier)
}

func (clk *Clock) logExttsEnable(lg *slog.Logger, enable bool) {
	_, err := clk.ExttsEnable(clk.chanIndex, enable)
	if err != nil {
		msg := "error disabling external timestamping"
		if enable {
			msg = "error enabling external timestamping"
		}
		lg.Warn(msg, "path", clk.Path(), "err", err)
	}
}

func (clk *Clock) readWorker(ctx context.Context, lg *slog.Logger, tsCh chan<- Event) {
	var wCh <-chan net.Flags
	if clk.w != nil {
		wCh = clk.w.SetCond(noCarrier)
	}
	defer clk.logExttsEnable(lg, false)
	defer close(tsCh)
	era := StaleEra
	for {
		select {
		case <-ctx.Done():
			return
		case flags, ok := <-wCh:
			if !ok {
				lg.Warn("netlink error waiting for interface state change", "err", clk.w.Err())
				wCh = nil
				break
			}
			wCh = clk.handleNoCarrier(ctx, lg, tsCh, flags)
			if wCh == nil {
				return
			}
		default:
		}
		// The idea is that if we poll and there are no pending events, then any step to the clock
		// that we have made with adjtimex will be in effect for the next read.
		if !clk.ExttsAvailable(exttsTimeout) {
			era = clk.eraCounter.load()
			continue
		}
		t, chanIndex, err := clk.ReadExtts()
		if err != nil {
			lg.Info("error reading external timestamp", "err", err)
			continue
		}
		if chanIndex != clk.chanIndex {
			// not for us
			continue
		}
		tClock := phctime.Time{
			T:   t,
			Era: clk.eraCounter.load(),
		}
		if tClock.Era != era && !tClock.Era.Uncertain() {
			if era.Uncertain() {
				tClock.Era = era
			} else {
				// Make the era uncertain.
				// We cannot be sure that the adjtimex is in effect now.
				// We have to wait for a poll that does not return any events.
				tClock.Era = era + 1
			}
		}
		event := Event{Ts: tClock}
		// Collect monotonic sample first
		event.TReadMono, err = clk.monoSample()
		if err != nil {
			lg.Warn("error reading PHC time", "err", err)
			event.TReadMono.Sys = time.Now()
		}
		// Collect wallclock sample
		event.TReadWall, err = clk.wallSample()
		if err != nil {
			lg.Warn("error from PTP_SYS_OFFSET", "err", err)
			event.TReadWall.Sys = time.Now()
		}
		event.Ts = tClock
		tsCh <- event
	}
}

// computeEra determines the era for a sample based on pre/post era values.
// If eraPre == eraPost or eraPre is uncertain, use eraPre.
// Else if eraPost is uncertain, use eraPost.
// Else the clock was stepped during sampling, so use eraPre + 1.
func computeEra(eraPre, eraPost phctime.Era) phctime.Era {
	if eraPre == eraPost || eraPre.Uncertain() {
		return eraPre
	}
	if eraPost.Uncertain() {
		return eraPost
	}
	return eraPre + 1
}

// monoSample reads a PHC/system time pair with monotonic system time.
// Uses a time.Now() sandwich around PHC read, which gives less precise
// correspondence but includes monotonic time in the result.
// This can be called only from readWorker.
func (clk *Clock) monoSample() (sample phctime.Sample, err error) {
	eraPre := clk.eraCounter.load()

	tSysPre := time.Now()
	tPHC, err := clk.GetTime()
	if err != nil {
		return
	}
	tSysPost := time.Now()

	eraPost := clk.eraCounter.load()
	sample.PHC.Era = computeEra(eraPre, eraPost)

	// Average the two time.Now() calls to estimate when PHC was read
	sample.Sys = tSysPre.Add(tSysPost.Sub(tSysPre) / 2)
	sample.PHC.T = tPHC
	return
}

// wallSample reads a PHC/system time pair with wallclock system time.
// Uses PTP_SYS_OFFSET which gives precise correspondence between
// PHC and system time, but result has only wallclock time (no monotonic).
// This can be called only from readWorker.
func (clk *Clock) wallSample() (sample phctime.Sample, err error) {
	eraPre := clk.eraCounter.load()

	ms, err := clk.SysOffset(6)
	if err != nil {
		return
	}

	eraPost := clk.eraCounter.load()
	sample.PHC.Era = computeEra(eraPre, eraPost)

	sample.PHC.T, sample.Sys = ms.Reduce()
	return
}

func (clk *Clock) AdjTime(d time.Duration) (phctime.Era, error) {
	clk.eraCounter.inc()
	err := clk.Clock.AdjTime(d)
	era := clk.eraCounter.inc()
	if err != nil {
		era = 0
	}
	return era, err
}
