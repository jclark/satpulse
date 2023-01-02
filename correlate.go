package main

import (
	"fmt"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/tai"
)

type TimeReading struct {
	T     tai.Time
	TRead time.Time
}

type EpochTimeReading struct {
	TimeReading
	Epoch phc.Epoch
}

type GpsTimeReading struct {
	TimeReading
	corr time.Duration
}

type PPSCorr struct {
	T    tai.Time
	Corr time.Duration
}

func (tr *TimeReading) IsZero() bool {
	return tr.TRead.IsZero()
}

type Correlator struct {
	servo    *Servo
	edges    []EpochTimeReading
	ppsCorrs []PPSCorr
	// if non-zero, a GPS reading that we haven't yet found an edge for
	gpsPending GpsTimeReading
	// if non-Zero corresponds to edges[0]
	// and edges per pulse must be 1 or 2
	gpsSampled GpsTimeReading
	// 0, 1 or 2 (0 means unknown)
	edgesPerPulse int
}

func (c *Correlator) emitEdge(i int, gps GpsTimeReading) {
	c.edges = c.edges[i:]
	c.servo.Sample(gps.T, c.edges[0].T.Add(gps.corr), c.edges[0].Epoch)
	c.gpsSampled = gps
}

func (c *Correlator) gpsTime(t tai.Time, tRead time.Time) {

	tr := GpsTimeReading{TimeReading{T: t, TRead: tRead}, c.getCorr(t)}
	i := c.nextEdge(tr)
	if i >= 0 {
		c.emitEdge(i, tr)
		return
	}
	if c.gpsSampled.IsZero() {
		i = c.goodEdge(tr)
		if i >= 0 {
			c.emitEdge(i, tr)
		}
		return
	}
	fmt.Println("no edge found")
	c.gpsPending = tr
}

const maxEdges = 8

func (c *Correlator) ppsEdge(t tai.Time, tRead time.Time, epoch phc.Epoch) {
	tr := EpochTimeReading{TimeReading{T: t, TRead: tRead}, epoch}
	c.edges = append(c.edges, tr)
	// The PPS edge arrives just after GPS (weird but maybe just possible)
	if !c.gpsPending.IsZero() {
		if c.nextEdge(c.gpsPending) == len(c.edges)-1 {
			c.emitEdge(len(c.edges)-1, c.gpsPending)
		}
		c.gpsPending = GpsTimeReading{}
	}
	if len(c.edges) > maxEdges {
		c.edges = c.edges[1:]
		c.gpsSampled = GpsTimeReading{}
	}
}

func (c *Correlator) ppsCorr(t tai.Time, corr time.Duration) {
	//fmt.Printf("PPS correction for %v: %v\n", t, corr)
	if len(c.ppsCorrs) > 1 {
		c.ppsCorrs = c.ppsCorrs[len(c.ppsCorrs)-1:]
	}
	c.ppsCorrs = append(c.ppsCorrs, PPSCorr{T: t, Corr: corr})
}

func (c *Correlator) getCorr(t tai.Time) time.Duration {
	for _, pc := range c.ppsCorrs {
		if pc.T == t {
			return pc.Corr
		}
	}
	return 0
}

func (c *Correlator) nextEdge(gps GpsTimeReading) int {
	// No previous sample
	if c.gpsSampled.IsZero() {
		return -1
	}
	// This is the expected case
	if IsSane(c.gpsSampled, gps, c.edges[0], c.edges[c.edgesPerPulse]) {
		return c.edgesPerPulse
	}
	for i := 1; i < len(c.edges); i++ {
		// Should do some more sanity testing here
		// XXX how to deal with both edges here
		if IsSane(c.gpsSampled, gps, c.edges[0], c.edges[i]) {
			return i
		}
	}
	return -1
}

func (c *Correlator) goodEdge(gps GpsTimeReading) int {
	// XXX single edge case
	if len(c.edges) < 7 {
		return -1
	}
	edges1, edges2 := alternate(c.edges)
	var1 := variation(edges1)
	var2 := variation(edges2)
	if var1 > time.Second/100 || var2 > time.Second/100 {
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
	delay := gps.TRead.Sub(c.edges[last].TRead)
	if delay > time.Second/2 {
		return -1
	}
	// we should really generate multiple samples from this
	return last
}

func variation(edges []EpochTimeReading) time.Duration {
	min := edges[1].T.Sub(edges[0].T)
	max := min
	for i := 2; i < len(edges); i++ {
		d := edges[i].T.Sub(edges[i-1].T)
		if d < min {
			d = min
		}
		if d > max {
			d = max
		}
	}
	return max - min
}

func alternate(edges []EpochTimeReading) (edges1, edges2 []EpochTimeReading) {
	for i := 0; i < len(edges); i++ {
		if i%2 == 0 {
			edges1 = append(edges1, edges[i])
		} else {
			edges2 = append(edges2, edges[i])
		}
	}
	return
}

func IsSane(master1, master2 GpsTimeReading, slave1, slave2 EpochTimeReading) bool {
	masterDiff := master2.T.Sub(master1.T)
	if slave1.Epoch == slave2.Epoch && !slave1.Epoch.Ambig() {
		return slave1.T.Add(masterDiff).Sub(slave2.T).Abs() <= time.Second/100
	} else {
		return slave1.TRead.Add(masterDiff).Sub(slave2.TRead).Abs() <= time.Second/20
	}
}
