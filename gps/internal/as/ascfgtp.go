package as

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Time pulse configuration uses CFG-PPS (the 15-byte Cynosure II/III
// form on every tested unit). A set is a read-modify-write: the query
// phase polls CFG-PPS and the set phase merges the target properties
// into the readback, preserving the GPIO pin and the Offset field.
// The offset is factory-set per unit (530 ns on two of the three test
// units, surviving even a factory reset - evidently calibration, not
// user config), so it is deliberately not exposed as AntennaCableDelay
// and never overwritten. The pulse width maps to the duty cycle in
// 10^-6 of the period; sync 0 means pulse-only-when-fixing, so
// OnlyWhenLocked is sync==0. Alignment to GNSS vs UTC time has no
// CFG-PPS carrier: the AlignToGNSS property is absent on this backend.
//
// All of this is register semantics: the physical pulse on the PPS pin
// was never observed (no timing instrumentation on the test units), so
// how the registers shape the actual pulse is the doc's word, not ours.

// tpProps are the properties realized by CFG-PPS.
const tpProps = gpsprot.PropIDTimePulseWidth | gpsprot.PropIDTimePulsePeriod |
	gpsprot.PropIDTimePulseOnlyWhenLocked | gpsprot.PropIDTimePulsePolarityRising

// needsTP reports whether the target involves the time pulse.
func (c *Configurator) needsTP() bool {
	return c.target.UsesAny(tpProps)
}

// generateTPQuery polls CFG-PPS when the target involves the time
// pulse (read-modify-write, and the readback cache serves ConfigProps).
func (c *Configurator) generateTPQuery() {
	if c.needsTP() {
		c.pollPps()
	}
}

// pollPps polls CFG-PPS into the readback cache.
func (c *Configurator) pollPps() {
	c.addPollReq(asbin.CfgPpsID, func(m asbin.Msg) {
		if pps, ok := m.(*asbin.CfgPps); ok {
			c.pps = pps
		}
	})
}

// generateTPSet merges the target's time pulse properties into the
// CFG-PPS readback and sends the result. Without a readback (the poll
// was unanswered) there is nothing to merge into: the property does
// not exist on this receiver and nothing is sent. The acknowledged
// values are not the whole truth for CFG-PPS: the TAU1302 acknowledges
// a zero duty cycle while its readback sometimes shows the write had
// no effect (and sometimes a fully cleared block), so the achieved
// values come from one post-set readback.
func (c *Configurator) generateTPSet() {
	if !c.target.Props.SetsAny(tpProps) || c.pps == nil {
		return
	}
	pps := *c.pps
	props := &c.target.Props
	if p, ok := props.GetTimePulsePeriod(); ok {
		pps.Period = uint32(p / time.Microsecond)
	}
	if w, ok := props.GetTimePulseWidth(); ok {
		pps.DutyCycle = dutyCycle(w, pps.Period)
	}
	if r, ok := props.GetTimePulsePolarityRising(); ok {
		pps.Polarity = asbin.CfgPpsPolarityFallingEdge
		if r {
			pps.Polarity = asbin.CfgPpsPolarityRisingEdge
		}
	}
	if l, ok := props.GetTimePulseOnlyWhenLocked(); ok {
		pps.Sync = asbin.CfgPpsSyncAlways
		if l {
			pps.Sync = asbin.CfgPpsSyncOnlyWithFix
		}
	}
	c.addSetReq(&pps, nil)
	c.pollPps()
}

// dutyCycle converts a pulse width to the CFG-PPS duty cycle in 10^-6
// units of the period. A zero width requests duty 0, the message
// file's pps-off form.
func dutyCycle(width time.Duration, periodUs uint32) uint32 {
	if periodUs == 0 {
		return 0
	}
	return uint32(math.Round(float64(width/time.Microsecond) / float64(periodUs) * 1e6))
}

// tpConfigProps reports the time pulse properties from the latest
// CFG-PPS readback. AlignToGNSS is left absent (no carrier).
func (c *Configurator) tpConfigProps(props *gpsprot.ConfigProps) {
	if c.pps == nil {
		return
	}
	period := time.Duration(c.pps.Period) * time.Microsecond
	props.SetTimePulsePeriod(period)
	props.SetTimePulseWidth(period / 1e6 * time.Duration(c.pps.DutyCycle))
	props.SetTimePulsePolarityRising(c.pps.Polarity == asbin.CfgPpsPolarityRisingEdge)
	props.SetTimePulseOnlyWhenLocked(c.pps.Sync == asbin.CfgPpsSyncOnlyWithFix)
}
