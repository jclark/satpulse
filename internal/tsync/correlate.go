package tsync

import (
	"time"

	"github.com/jclark/gps4ptp/internal/ptime"
	"golang.org/x/exp/slog"
)

type clockTimeReading struct {
	ptime.ClockTime
	tRead time.Time
}

type gpsTimeReading struct {
	tGPS     ptime.Time
	tRead    time.Time
	pulseOff time.Duration
}

func (gps *gpsTimeReading) pulseTime() ptime.Time {
	return gps.tGPS.Add(-gps.pulseOff)
}

func (gtr *gpsTimeReading) isZero() bool {
	return gtr.tRead.IsZero()
}

type Sampler interface {
	Sample(ref ptime.Time, local ptime.ClockTime, tRead time.Time, delayed bool)
}

type Correlator struct {
	sampler Sampler
	lg      *slog.Logger
	edges   []clockTimeReading
	// These are readings before the navigation epoch, which give the pulse offset
	pulseOffs []gpsTimeReading
	// if non-zero, a GPS reading that we haven't yet found an edge for
	gpsPending gpsTimeReading
	// if non-Zero corresponds to edges[0]
	// and edges per pulse must be 1 or 2
	gpsSampled gpsTimeReading
	// 0, 1 or 2 (0 means unknown)
	edgesPerPulse int
}

func NewCorrelator(s Sampler, lg *slog.Logger) *Correlator {
	c := new(Correlator)
	c.sampler = s
	c.lg = lg
	return c
}

func (c *Correlator) GPSTime(tGPS ptime.Time, tRead time.Time) {
	c.gpsPending = gpsTimeReading{tGPS: tGPS, tRead: tRead}
	po := c.findPulseOffset(tGPS, tRead)
	if po != nil {
		c.gpsPending.pulseOff = po.pulseOff
		c.lg.Debug("gpsTime", "t", tGPS, "pulseOffset", po.pulseOff)
	} else {
		c.lg.Debug("gpsTime", "t", tGPS)
	}

	c.correlate()
}

const maxEdges = 8

func (c *Correlator) PulseEdge(tClock ptime.ClockTime, tRead time.Time) {
	c.lg.Debug("pulseEdge", "t", tClock.T, "epoch", tClock.Epoch)
	edge := clockTimeReading{ClockTime: tClock, tRead: tRead}
	c.edges = append(c.edges, edge)
	c.correlate()
	if len(c.edges) > maxEdges {
		c.edges = c.edges[1:]
		c.gpsSampled = gpsTimeReading{}
	}
}

func (c *Correlator) correlate() {
	if c.gpsPending.isZero() {
		return
	}
	i := c.followEdge()
	if i >= 0 {
		c.gpsEmit(i)
		return
	}
	i = c.initialEdge()
	if i >= 0 {
		c.emitUpTo(i)
	}
}

func (c *Correlator) emitUpTo(edgeIndex int) {
	i := 0
	if !c.gpsSampled.isZero() {
		i++
	}
	if (edgeIndex-i)%c.edgesPerPulse != 0 {
		i++
	}
	tGPS := c.gpsPending.tGPS
	for ; i < edgeIndex; i += c.edgesPerPulse {
		secs := (edgeIndex - i) / c.edgesPerPulse
		c.sampler.Sample(tGPS.Add(time.Second*time.Duration(-secs)), c.edges[i].ClockTime, c.edges[i].tRead, true)
	}
	c.gpsEmit(edgeIndex)
}

func (c *Correlator) gpsEmit(edgeIndex int) {
	c.sampler.Sample(c.gpsPending.pulseTime(), c.edges[edgeIndex].ClockTime, c.edges[edgeIndex].tRead, false)
	c.edges = c.edges[edgeIndex:]
	c.gpsSampled = c.gpsPending
	c.gpsPending = gpsTimeReading{}
}

// PulseOffset corresponds to U-blox quantization error.
// This is also sometimes called sawtooth error.
func (c *Correlator) PulseOffset(tGPS ptime.Time, tRead time.Time, off time.Duration) {
	c.lg.Debug("pulseOffset", "tGPS", tGPS, "offset", off)
	if len(c.pulseOffs) > 1 {
		c.pulseOffs = c.pulseOffs[len(c.pulseOffs)-1:]
	}
	c.pulseOffs = append(c.pulseOffs, gpsTimeReading{tGPS: tGPS, tRead: tRead, pulseOff: off})
}

func (c *Correlator) findPulseOffset(tGPS ptime.Time, tRead time.Time) *gpsTimeReading {
	for _, po := range c.pulseOffs {
		if po.tGPS == tGPS {
			return &po
		}
	}
	return nil
}

func (c *Correlator) followEdge() int {
	// No previous sample
	if c.gpsSampled.isZero() {
		return -1
	}
	// This is the expected case
	if c.edgesPerPulse < len(c.edges) && consistent(c.gpsSampled, c.gpsPending, c.edges[0], c.edges[c.edgesPerPulse]) {
		return c.edgesPerPulse
	}
	for i := 1; i < len(c.edges); i++ {
		// Should do some more sanity testing here
		// XXX how to deal with both edges here
		if consistent(c.gpsSampled, c.gpsPending, c.edges[0], c.edges[i]) {
			return i
		}
	}
	return -1
}

const maxDelay = time.Millisecond * 600
const readWindow = time.Millisecond * 900
const minDelay = maxDelay - readWindow

func (c *Correlator) initialEdge() int {
	// no GPS message yet
	if c.gpsPending.isZero() {
		return -1
	}
	// still have a previous sample,
	// let's try to be consistent with that
	if !c.gpsSampled.isZero() {
		return -1
	}
	epp := 1
	last := c.initialEdge1()
	if last < 0 {
		last = c.initialEdge2()
		if last < 0 {
			return last
		}
		epp = 2
	}
	delay := c.gpsPending.tRead.Sub(c.edges[last].tRead)
	if delay > maxDelay {
		c.lg.Debug("excessDelay", "delay", delay)
		return -1
	}
	if delay < minDelay {
		c.lg.Debug("pulseBeforeGps", "delay", delay)
		return -1
	}
	c.edgesPerPulse = epp
	c.lg.Info("regularPulse", "edgesPerPulse", epp)
	// we should really generate multiple samples from this
	return last
}

// Find a candidate initial edge assuming 2 edges per pulse
func (c *Correlator) initialEdge1() int {
	if len(c.edges) < 4 {
		return -1
	}
	v := variation(c.edges)
	if v > time.Second/100 {
		c.lg.Debug("singleEdgeInconsistent", "variation", v)
		return -1
	}
	return len(c.edges) - 1
}

// Find a candidate initial edge, assuming 1 edge per pulse
func (c *Correlator) initialEdge2() int {
	if len(c.edges) < 7 {
		return -1
	}
	edges1, edges2 := alternate(c.edges)
	v1 := variation(edges1)
	v2 := variation(edges2)
	if v1 > time.Second/100 || v2 > time.Second/100 {
		return -1
	}
	d := edges2[0].T.Sub(edges1[0].T)
	var good int
	if d < time.Second/4 {
		good = 0
	} else if d > time.Second-time.Second/4 {
		good = 1
	} else {
		return -1
	}
	last := len(c.edges) - 1
	if last%2 != good {
		last--
	}
	return last
}

func variation(edges []clockTimeReading) time.Duration {
	min := edges[1].T.Sub(edges[0].T)
	max := min
	for i := 2; i < len(edges); i++ {
		d := edges[i].T.Sub(edges[i-1].T)
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return max - min
}

func alternate(edges []clockTimeReading) (edges1, edges2 []clockTimeReading) {
	for i := 0; i < len(edges); i++ {
		if i%2 == 0 {
			edges1 = append(edges1, edges[i])
		} else {
			edges2 = append(edges2, edges[i])
		}
	}
	return
}

func consistent(master1, master2 gpsTimeReading, slave1, slave2 clockTimeReading) bool {
	masterDiff := master2.tGPS.Sub(master1.tGPS)
	if slave1.Epoch == slave2.Epoch && !slave1.Epoch.Ambig() {
		return slave1.T.Add(masterDiff).Sub(slave2.T).Abs() <= time.Second/100
	} else {
		return slave1.tRead.Add(masterDiff).Sub(slave2.tRead).Abs() <= time.Second/20
	}
}

type multiSampler struct {
	samplers []Sampler
}

func (ms *multiSampler) Sample(ref ptime.Time, local ptime.ClockTime, tRead time.Time, delayed bool) {
	for _, sampler := range ms.samplers {
		sampler.Sample(ref, local, tRead, delayed)
	}
}

func MultiSampler(samplers ...Sampler) Sampler {
	return &multiSampler{samplers: samplers}
}
