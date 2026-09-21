// Package pollsim simulates the serial PPS polling loop of gps/app/pps
// against a modelled pulse and host, in virtual time.
//
// The real Poll runs unchanged: the simulator supplies its clock, its timer
// and its pulse reader. Virtual time advances only when the loop does
// something that takes time: a query, a clock read, or a sleep. Faults are a
// pulse outage, a stall of the polling thread, which lands wherever the
// loop happens to be, inside a query, between queries or at a wakeup, and a
// period of slowed queries. Both can produce anomalously wide intervals. The
// consumer applies the daemon's forwarding rule, and the statistics judge
// what the time daemon would have received: how many edges, how wrong, and
// with what gaps between them.
package pollsim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"slices"
	"sort"
	"time"

	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/ptime"
)

// Stats summarises a run. Errors are of forwarded edges against the true
// edge on the pin; a forwarded edge is wrong when its reported uncertainty
// interval does not contain the true edge. Gaps are between consecutive
// forwarded edges.
type Stats struct {
	Duration     Seconds
	Pulses       int // pulses present during the run
	Edges        int // candidates the loop sent
	Forwarded    int // candidates with no rejection reason
	Acquiring    int
	Anomalous    int
	Missed       int // pulses present after the first forwarded edge with no candidate
	Wrong        int // forwarded edges whose uncertainty interval excludes the true edge
	ErrMedian    Seconds
	ErrP90       Seconds
	ErrMax       Seconds
	LongestGap   Seconds
	GapsOver4s   int
	Acquisitions int
	Lost         int
	TrackMisses  int
	Queries      int
	Blocked      float64 // fraction of the run not spent in a query or clock read: asleep in the timer, or stalled
}

// String formats the statistics as TOML key/value lines.
func (s Stats) String() string {
	return fmt.Sprintf("duration = %g\npulses = %d\nedges = %d\nforwarded = %d\nacquiring = %d\nanomalous = %d\n"+
		"missed = %d\nwrong = %d\nerrMedian = %.6f\nerrP90 = %.6f\nerrMax = %.6f\n"+
		"longestGap = %.3f\ngapsOver4s = %d\nacquisitions = %d\nlost = %d\ntrackMisses = %d\n"+
		"queries = %d\nqueriesPerSecond = %.1f\nblocked = %.4f\n",
		s.Duration, s.Pulses, s.Edges, s.Forwarded, s.Acquiring, s.Anomalous, s.Missed, s.Wrong,
		s.ErrMedian, s.ErrP90, s.ErrMax, s.LongestGap, s.GapsOver4s, s.Acquisitions, s.Lost,
		s.TrackMisses, s.Queries, float64(s.Queries)/s.Duration, s.Blocked)
}

// EdgeRecord is one candidate as the consumer saw it, with its error against
// the true edge.
type EdgeRecord struct {
	T           Seconds          `json:"t"`
	Err         Seconds          `json:"err"`
	Uncertainty [2]Seconds       `json:"uncertainty"`
	PollWidths  [2]Seconds       `json:"pollWidths"`
	Reject      pps.RejectReason `json:"reject,omitempty"`
	Forwarded   bool             `json:"forwarded"`
}

// Simulate runs the poll loop under cfg, which must be valid. lg receives
// the loop's own log lines with simulated timestamps; edges, if non-nil, is
// called for every candidate.
func Simulate(cfg Config, lg *slog.Logger, edges func(EdgeRecord)) (Stats, error) {
	s := newSim(cfg)
	ceCh := make(chan pps.CandidateEdge)
	errCh := make(chan error, 1)
	params := pps.PollParams{
		MinSpacing: ptime.Seconds(cfg.Poll.MinSpacing),
		PreWarm:    ptime.Seconds(cfg.Poll.PreWarm),
		Wait:       s.wait,
		Now:        s.time,
	}
	go func() {
		errCh <- pps.Poll(context.Background(), slog.New(&logHandler{Handler: lg.Handler(), s: s}), s, params, ceCh, nil)
	}()
	c := consumer{s: s, caught: make(map[int64]bool), record: edges}
	for {
		select {
		case ce := <-ceCh:
			c.candidate(ce)
		case err := <-errCh:
			if !errors.Is(err, errDone) {
				return Stats{}, err
			}
			return c.stats(), nil
		}
	}
}

// errDone is the reader's answer once the simulated duration has elapsed.
var errDone = errors.New("simulation complete")

const period = time.Second

// simBase is the wall-clock time of simulated zero.
var simBase = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

type interval struct {
	at, dur time.Duration
}

type slow struct {
	from, to time.Duration
	factor   float64
}

// sim is the virtual host: its clock, its timer, the pin, and the faults.
// All of it is driven from the polling goroutine; the consumer only reads
// the candidates that goroutine sends.
type sim struct {
	cfg       Config
	end       time.Duration
	now       time.Duration
	work      time.Duration
	stalls    []interval
	nextStall int
	slows     []slow
	nextSlow  int
	rng       *rand.Rand
	jitter    []time.Duration
	clockRead time.Duration
	idleAfter time.Duration
	recover   time.Duration
	// Idle slowdown state: when the thread was last active, and how much
	// continuous activity has accumulated since it went cold.
	lastActive time.Duration
	cold       bool
	warmed     time.Duration
	queries    int
	acquired   int
	lost       int
	misses     int
}

func newSim(cfg Config) *sim {
	s := &sim{
		cfg: cfg, end: ptime.Seconds(cfg.Sim.Duration), rng: rand.New(rand.NewSource(cfg.Sim.Seed)),
		clockRead: ptime.Seconds(cfg.Host.ClockRead),
		idleAfter: ptime.Seconds(cfg.Host.Query.Idle.After),
		recover:   ptime.Seconds(cfg.Host.Query.Idle.Recover),
	}
	s.jitter = make([]time.Duration, int(cfg.Sim.Duration)+2)
	for i := range s.jitter {
		s.jitter[i] = time.Duration(s.rng.NormFloat64() * cfg.Pulse.Jitter * 1e9)
	}
	for _, st := range cfg.Fault.Stall {
		if st.Duration > 0 {
			s.stalls = append(s.stalls, interval{at: ptime.Seconds(st.At), dur: ptime.Seconds(st.Duration)})
		}
	}
	for _, b := range cfg.Fault.Stalls {
		s.stalls = append(s.stalls, s.burst(b.Start, b.Duration, b.Rate, b.Min, b.Max)...)
	}
	sort.Slice(s.stalls, func(i, j int) bool { return s.stalls[i].at < s.stalls[j].at })
	for _, sl := range cfg.Fault.Slow {
		if sl.Duration > 0 && sl.Factor > 1 {
			s.slows = append(s.slows, slow{from: ptime.Seconds(sl.Start), to: ptime.Seconds(sl.Start + sl.Duration), factor: sl.Factor})
		}
	}
	for _, b := range cfg.Fault.Slows {
		if b.Factor <= 1 {
			continue
		}
		for _, v := range s.burst(b.Start, b.Duration, b.Rate, b.Min, b.Max) {
			s.slows = append(s.slows, slow{from: v.at, to: v.at + v.dur, factor: b.Factor})
		}
	}
	sort.Slice(s.slows, func(i, j int) bool { return s.slows[i].from < s.slows[j].from })
	return s
}

func (s *sim) burst(start, duration Seconds, rate float64, lo, hi Seconds) []interval {
	if rate == 0 {
		return nil
	}
	end := s.end
	if duration > 0 {
		end = min(end, ptime.Seconds(start+duration))
	}
	logMin, logMax := math.Log(lo), math.Log(hi)
	var v []interval
	for t := start + s.rng.ExpFloat64()/rate; ptime.Seconds(t) < end; t += s.rng.ExpFloat64() / rate {
		dur := math.Exp(logMin + s.rng.Float64()*(logMax-logMin))
		v = append(v, interval{at: ptime.Seconds(t), dur: ptime.Seconds(dur)})
	}
	return v
}

// time is the loop's clock: reading it is work.
func (s *sim) time() time.Time {
	s.run(s.clockRead)
	return simBase.Add(s.now)
}

// wait is the loop's timer. A precise sleep ends at its deadline, as the
// loop sleeps out whatever the timer cannot; any other is truncated to a
// multiple of Resolution, as the runtime does on Linux at one millisecond,
// and returns at once when that leaves nothing. A sleep that happens
// overshoots, and a stall in progress at the wakeup delays it further.
func (s *sim) wait(ctx context.Context, t time.Time, precise bool) (bool, error) {
	if s.now >= s.end {
		return false, errDone
	}
	d := t.Sub(simBase) - s.now
	if res := ptime.Seconds(s.cfg.Host.Timer.Resolution); res > 0 && !precise {
		d = d.Truncate(res)
	}
	if d <= 0 {
		return false, nil
	}
	s.now += d
	tm := s.cfg.Host.Timer
	if o := tm.Overshoot + s.rng.NormFloat64()*tm.OvershootJitter; o > 0 {
		s.now += ptime.Seconds(o)
	}
	for s.nextStall < len(s.stalls) && s.stalls[s.nextStall].at <= s.now {
		st := s.stalls[s.nextStall]
		s.nextStall++
		if end := st.at + st.dur; end > s.now {
			s.now = end
		}
	}
	return true, nil
}

// InPulse is the loop's state query. The pin is sampled at a uniformly
// random point of the query's own running time, so a stall inside the query
// falls before or after the sampling instant.
func (s *sim) InPulse() (bool, error) {
	if s.now >= s.end {
		return false, errDone
	}
	s.queries++
	q := s.cfg.Host.Query
	d := q.Duration + s.rng.NormFloat64()*q.Jitter
	d = max(d, q.Duration/4)
	if s.cold {
		d *= q.Idle.Factor
	}
	d *= s.slowFactor()
	dur := ptime.Seconds(d)
	before := time.Duration(s.rng.Float64() * float64(dur))
	s.run(before)
	on := s.pulseOn(s.now)
	s.run(dur - before)
	return on, nil
}

// slowFactor is the query time multiplier of the slow periods in progress,
// the largest of them when they overlap.
func (s *sim) slowFactor() float64 {
	for s.nextSlow < len(s.slows) && s.slows[s.nextSlow].to <= s.now {
		s.nextSlow++
	}
	factor := 1.0
	for i := s.nextSlow; i < len(s.slows) && s.slows[i].from <= s.now; i++ {
		if s.slows[i].to > s.now {
			factor = max(factor, s.slows[i].factor)
		}
	}
	return factor
}

// run advances the clock by d of running time, inserting any stall that
// begins meanwhile, and keeps the idle-slowdown state.
func (s *sim) run(d time.Duration) {
	if s.idleAfter > 0 {
		if s.now-s.lastActive > s.idleAfter {
			s.cold, s.warmed = true, 0
		}
		if s.cold {
			if s.warmed += d; s.warmed >= s.recover {
				s.cold = false
			}
		}
	}
	s.work += d
	for s.nextStall < len(s.stalls) && s.stalls[s.nextStall].at < s.now+d {
		st := s.stalls[s.nextStall]
		s.nextStall++
		if st.at > s.now {
			d -= st.at - s.now
			s.now = st.at
		}
		s.now = max(s.now, st.at+st.dur)
	}
	s.now += d
	s.lastActive = s.now
}

// pulseOn reports the pin state at t: inside the width after a leading edge
// that is not suppressed by an outage.
func (s *sim) pulseOn(t time.Duration) bool {
	width := ptime.Seconds(s.cfg.Pulse.Width)
	for n := t/period + 1; n >= 0 && n >= t/period-1; n-- {
		if e := s.edge(n); s.present(n) && t >= e && t < e+width {
			return true
		}
	}
	return false
}

func (s *sim) edge(n time.Duration) time.Duration {
	if int(n) >= len(s.jitter) {
		return n * period
	}
	return n*period + s.jitter[n]
}

func (s *sim) present(n time.Duration) bool {
	e := float64(s.edge(n)) / 1e9
	for _, o := range s.cfg.Fault.Outage {
		if o.Duration > 0 && e >= o.Start && e < o.Start+o.Duration {
			return false
		}
	}
	return true
}

// logHandler stamps the loop's log records with simulated time and counts
// the events the statistics report.
type logHandler struct {
	slog.Handler
	s *sim
}

func (h *logHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	switch r.Message {
	case "serial PPS acquired":
		h.s.acquired++
	case "serial PPS track status":
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "reason" {
				switch a.Value.String() {
				case "lost":
					h.s.lost++
				case "miss":
					h.s.misses++
				}
			}
			return true
		})
	}
	if !h.Handler.Enabled(ctx, r.Level) {
		return nil
	}
	r.Time = simBase.Add(h.s.now)
	return h.Handler.Handle(ctx, r)
}

// consumer applies the daemon's forwarding rule and accumulates the
// statistics.
type consumer struct {
	s          *sim
	caught     map[int64]bool
	record     func(EdgeRecord)
	edges      int
	forwarded  int
	acquiring  int
	anomalous  int
	wrong      int
	errs       []time.Duration
	firstFwd   time.Duration
	lastFwd    time.Duration
	longestGap time.Duration
	gapsOver4s int
}

func (c *consumer) candidate(ce pps.CandidateEdge) {
	t := ce.Timestamp.Sub(simBase)
	n := (t + period/2) / period
	err := t - c.s.edge(n)
	c.edges++
	c.caught[int64(n)] = true
	switch ce.Reject {
	case pps.RejectAcquiring:
		c.acquiring++
	case pps.RejectAnomalous:
		c.anomalous++
	}
	fwd := ce.Reject == ""
	if fwd {
		c.forwarded++
		c.errs = append(c.errs, err.Abs())
		if err > ce.Uncertainty[0] || err < -ce.Uncertainty[1] {
			c.wrong++
		}
		if c.forwarded == 1 {
			c.firstFwd = t
		} else {
			gap := t - c.lastFwd
			c.longestGap = max(c.longestGap, gap)
			if gap > 4*period {
				c.gapsOver4s++
			}
		}
		c.lastFwd = t
	}
	if c.record != nil {
		c.record(EdgeRecord{
			T: t.Seconds(), Err: err.Seconds(),
			Uncertainty: [2]Seconds{ce.Uncertainty[0].Seconds(), ce.Uncertainty[1].Seconds()},
			PollWidths:  [2]Seconds{ce.PollWidths[0].Seconds(), ce.PollWidths[1].Seconds()},
			Reject:      ce.Reject, Forwarded: fwd,
		})
	}
}

func (c *consumer) stats() Stats {
	s := c.s
	st := Stats{Duration: s.cfg.Sim.Duration, Edges: c.edges, Forwarded: c.forwarded,
		Acquiring: c.acquiring, Anomalous: c.anomalous,
		Wrong: c.wrong, LongestGap: c.longestGap.Seconds(), GapsOver4s: c.gapsOver4s,
		Acquisitions: s.acquired, Lost: s.lost, TrackMisses: s.misses,
		Queries: s.queries, Blocked: 1 - float64(s.work)/float64(s.end)}
	for n := time.Duration(0); n*period < s.end; n++ {
		if !s.present(n) {
			continue
		}
		st.Pulses++
		if c.forwarded > 0 && n*period > c.firstFwd && !c.caught[int64(n)] {
			st.Missed++
		}
	}
	if len(c.errs) > 0 {
		slices.Sort(c.errs)
		st.ErrMedian = c.errs[len(c.errs)/2].Seconds()
		st.ErrP90 = c.errs[len(c.errs)*9/10].Seconds()
		st.ErrMax = c.errs[len(c.errs)-1].Seconds()
	}
	return st
}
