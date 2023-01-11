package tsync

import (
	"time"

	"github.com/jclark/gps2phc/ptime"
)

type clockTimeReading struct {
	ptime.ClockTime
	tRead time.Time
}

type gpsTimeReading struct {
	t     ptime.Time
	tRead time.Time
	corr  time.Duration
}

type pulseCorrection struct {
	tGPS  ptime.Time
	delta time.Duration
}

func (gtr *gpsTimeReading) isZero() bool {
	return gtr.tRead.IsZero()
}

type Correlator struct {
	servo *Servo
	edges []clockTimeReading
	pcs   []pulseCorrection
	// if non-zero, a GPS reading that we haven't yet found an edge for
	gpsPending gpsTimeReading
	// if non-Zero corresponds to edges[0]
	// and edges per pulse must be 1 or 2
	gpsSampled gpsTimeReading
	// 0, 1 or 2 (0 means unknown)
	edgesPerPulse int
}

func NewCorrelator(s *Servo) *Correlator {
	c := new(Correlator)
	c.servo = s
	return c
}

func (c *Correlator) emitEdge(i int, gps gpsTimeReading) {
	c.edges = c.edges[i:]
	c.servo.Sample(gps.t.Add(-gps.corr), c.edges[0].ClockTime)
	c.gpsSampled = gps
}

func (c *Correlator) GPSTime(t ptime.Time, tRead time.Time) {
	tr := gpsTimeReading{t, tRead, c.findCorrection(t)}
	c.servo.Logger().Debug("gpsTime", "t", t, "q", tr.corr)
	i := c.nextEdge(tr)
	if i >= 0 {
		c.emitEdge(i, tr)
		return
	}
	if c.gpsSampled.isZero() {
		i = c.goodEdge(tr)
		if i >= 0 {
			c.emitEdge(i, tr)
			return
		}
	}
	c.gpsPending = tr
}

const maxEdges = 8

func (c *Correlator) PulseEdge(tClock ptime.ClockTime, tRead time.Time) {
	c.servo.Logger().Debug("pulse", "t", tClock.T, "epoch", tClock.Epoch)
	tr := clockTimeReading{ClockTime: tClock, tRead: tRead}
	c.edges = append(c.edges, tr)
	// The PPS edge arrives just after GPS (can happen on rPI CM4)
	if !c.gpsPending.isZero() {
		last := len(c.edges) - 1
		if c.nextEdge(c.gpsPending) == last || c.goodEdge(c.gpsPending) == last {
			c.emitEdge(last, c.gpsPending)
		} else {
			c.servo.Logger().Warn("noPulseForGps", "t", c.gpsPending.t)
		}
		c.gpsPending = gpsTimeReading{}
	}
	if len(c.edges) > maxEdges {
		c.edges = c.edges[1:]
		c.gpsSampled = gpsTimeReading{}
	}
}

func (c *Correlator) PulseCorrection(tGPS ptime.Time, delta time.Duration) {
	//fmt.Printf("PPS correction for %v: %v\n", t, corr)
	if len(c.pcs) > 1 {
		c.pcs = c.pcs[len(c.pcs)-1:]
	}
	c.pcs = append(c.pcs, pulseCorrection{tGPS: tGPS, delta: delta})
}

func (c *Correlator) findCorrection(t ptime.Time) time.Duration {
	for _, pc := range c.pcs {
		if pc.tGPS == t {
			return pc.delta
		}
	}
	return 0
}

func (c *Correlator) nextEdge(gps gpsTimeReading) int {
	// No previous sample
	if c.gpsSampled.isZero() {
		return -1
	}
	// This is the expected case
	if c.edgesPerPulse < len(c.edges) && consistent(c.gpsSampled, gps, c.edges[0], c.edges[c.edgesPerPulse]) {
		return c.edgesPerPulse
	}
	for i := 1; i < len(c.edges); i++ {
		// Should do some more sanity testing here
		// XXX how to deal with both edges here
		if consistent(c.gpsSampled, gps, c.edges[0], c.edges[i]) {
			return i
		}
	}
	return -1
}

func (c *Correlator) goodEdge(gps gpsTimeReading) int {
	edgesPerPulse := 1
	last := c.goodEdge1(gps)
	if last < 0 {
		last = c.goodEdge2(gps)
		if last < 0 {
			return last
		}
		edgesPerPulse = 2
	}
	delay := gps.tRead.Sub(c.edges[last].tRead)
	if delay > time.Second/2 {
		c.servo.Logger().Debug("excessDelay", "delay", delay)
		return -1
	}
	c.edgesPerPulse = edgesPerPulse
	c.servo.Logger().Info("consistentEdges", "edgesPerPulse", edgesPerPulse)
	// we should really generate multiple samples from this
	return last
}

func (c *Correlator) goodEdge1(gps gpsTimeReading) int {
	if len(c.edges) < 4 {
		return -1
	}
	v := variation(c.edges)
	if v > time.Second/100 {
		c.servo.Logger().Debug("singleEdgeInconsistent", "variation", v)
		return -1
	}
	return len(c.edges) - 1
}

func (c *Correlator) goodEdge2(gps gpsTimeReading) int {
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
	masterDiff := master2.t.Sub(master1.t)
	if slave1.Epoch == slave2.Epoch && !slave1.Epoch.Ambig() {
		return slave1.T.Add(masterDiff).Sub(slave2.T).Abs() <= time.Second/100
	} else {
		return slave1.tRead.Add(masterDiff).Sub(slave2.tRead).Abs() <= time.Second/20
	}
}
